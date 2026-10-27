---
title: Modelo de dados
status: ativo
updated: 2026-10-26
---

# Modelo de dados

SQLite único em `/data/db/app.db`, modo WAL. Datas em ISO 8601 com offset (`2026-10-26T07:00:00-03:00`) ou UTC quando não houver ambiguidade; `run_date` usa a data **local** do fuso configurado (`YYYY-MM-DD`).

## Schema

```sql
PRAGMA journal_mode = WAL;
PRAGMA synchronous  = NORMAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;

CREATE TABLE schema_migrations (
  version    INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);

CREATE TABLE documents (
  id             INTEGER PRIMARY KEY,
  title          TEXT    NOT NULL,
  author         TEXT,
  language       TEXT    NOT NULL DEFAULT 'pt-BR',
  format         TEXT    NOT NULL CHECK (format IN ('md','epub','pdf')),
  source_name    TEXT    NOT NULL,
  source_path    TEXT    NOT NULL,
  source_sha256  TEXT    NOT NULL UNIQUE,
  cover_path     TEXT,
  status         TEXT    NOT NULL DEFAULT 'processing'
                 CHECK (status IN ('processing','ready','paused','completed','failed','needs_ocr')),
  pacing_mode    TEXT    NOT NULL DEFAULT 'minutes_per_day'
                 CHECK (pacing_mode IN ('minutes_per_day','finish_by_days')),
  pacing_minutes INTEGER NOT NULL DEFAULT 10,
  pacing_days    INTEGER,
  wpm            INTEGER NOT NULL DEFAULT 200,
  difficulty     REAL    NOT NULL DEFAULT 1.0,
  priority       INTEGER NOT NULL DEFAULT 100,
  total_words    INTEGER NOT NULL DEFAULT 0,
  total_seconds  INTEGER NOT NULL DEFAULT 0,
  error          TEXT,
  created_at     TEXT    NOT NULL,
  updated_at     TEXT    NOT NULL
);
CREATE INDEX idx_documents_status ON documents(status, priority, id);

CREATE TABLE blocks (
  id          INTEGER PRIMARY KEY,
  document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  ord         INTEGER NOT NULL,
  kind        TEXT    NOT NULL CHECK (kind IN
              ('h1','h2','h3','h4','h5','h6','p','li','code','table','quote','figure','footnote','hr')),
  level       INTEGER NOT NULL DEFAULT 0,
  heading_id  INTEGER REFERENCES blocks(id) ON DELETE SET NULL,
  text        TEXT    NOT NULL,
  html        TEXT    NOT NULL DEFAULT '',
  words       INTEGER NOT NULL DEFAULT 0,
  seconds     INTEGER NOT NULL DEFAULT 0,
  hash        TEXT    NOT NULL,
  UNIQUE (document_id, ord)
);
CREATE INDEX idx_blocks_doc_ord   ON blocks(document_id, ord);
CREATE INDEX idx_blocks_heading   ON blocks(heading_id, ord);

-- heading_id aponta para o heading que possui o bloco; no próprio heading,
-- aponta para si; conteúdo antes do primeiro heading usa NULL.
-- Parágrafos > 300 palavras geram vários blocos p (ADR-0011).

CREATE TABLE plans (
  id             INTEGER PRIMARY KEY,
  document_id    INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  version        INTEGER NOT NULL,
  status         TEXT    NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active','superseded','stale')),
  pacing_mode    TEXT    NOT NULL,
  target_seconds INTEGER NOT NULL,
  horizon_days   INTEGER,
  params_json    TEXT    NOT NULL,
  created_at     TEXT    NOT NULL,
  UNIQUE (document_id, version)
);
CREATE UNIQUE INDEX ux_plans_active ON plans(document_id) WHERE status = 'active';

CREATE TABLE chunks (
  id                INTEGER PRIMARY KEY,
  plan_id           INTEGER NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
  document_id       INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  seq               INTEGER NOT NULL,
  first_block_id    INTEGER NOT NULL REFERENCES blocks(id),
  last_block_id     INTEGER NOT NULL REFERENCES blocks(id),
  first_hash        TEXT    NOT NULL,
  last_hash         TEXT    NOT NULL,
  section_path      TEXT    NOT NULL,          -- JSON: ["Cap. 32","32.4 — A carta"]
  starts_mid_section INTEGER NOT NULL DEFAULT 0,
  ends_mid_section   INTEGER NOT NULL DEFAULT 0,
  oversized         INTEGER NOT NULL DEFAULT 0,
  words             INTEGER NOT NULL,
  seconds           INTEGER NOT NULL,
  target_seconds    INTEGER NOT NULL,
  status            TEXT    NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','sent','skipped')),
  sent_at           TEXT,
  done_at           TEXT,
  UNIQUE (plan_id, seq)
);
CREATE INDEX idx_chunks_cursor ON chunks(plan_id, status, seq);

CREATE TABLE delivery_runs (
  id            INTEGER PRIMARY KEY,
  run_date      TEXT    NOT NULL UNIQUE,       -- YYYY-MM-DD no fuso configurado
  scheduled_for TEXT    NOT NULL,              -- RFC3339 com offset
  started_at    TEXT    NOT NULL,
  finished_at   TEXT,
  status        TEXT    NOT NULL DEFAULT 'running'
                CHECK (status IN ('running','sent','partial','failed')),
  late          INTEGER NOT NULL DEFAULT 0,    -- catch-up após horário
  error         TEXT
);

CREATE TABLE delivery_items (
  id          INTEGER PRIMARY KEY,
  run_id      INTEGER NOT NULL REFERENCES delivery_runs(id) ON DELETE CASCADE,
  document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  chunk_id    INTEGER NOT NULL REFERENCES chunks(id),
  status      TEXT    NOT NULL DEFAULT 'pending'
              CHECK (status IN ('pending','sent','failed','skipped')),
  attempts    INTEGER NOT NULL DEFAULT 0,
  bytes       INTEGER,
  error       TEXT,
  sent_at     TEXT,
  UNIQUE (run_id, document_id)
);
CREATE INDEX idx_items_doc ON delivery_items(document_id, id);

CREATE TABLE settings (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE events (
  id          INTEGER PRIMARY KEY,
  at          TEXT NOT NULL,
  level       TEXT NOT NULL CHECK (level IN ('debug','info','warn','error')),
  kind        TEXT NOT NULL,                   -- upload | extract | plan | delivery | auth | system
  document_id INTEGER,
  message     TEXT NOT NULL
);
CREATE INDEX idx_events_at ON events(at);
```

## Invariantes

| ID | Invariante | Verificação |
|---|---|---|
| DB-01 | `blocks.ord` contíguo `0..n-1` por documento | teste pós-extração + CHECK em query de saúde |
| DB-02 | Chunks do plano ativo cobrem todos os blocos, sem gap/overlap | `ValidatePlan` no store antes de ativar |
| DB-03 | Um único plano `active` por documento | índice parcial `ux_plans_active` |
| DB-04 | `run_date` única | `UNIQUE` |
| DB-05 | Um item por `(run, documento)` | `UNIQUE` |
| DB-06 | Cursor = menor `seq` com `status='pending'` do plano ativo | query canônica abaixo |
| DB-07 | `source_sha256` única | `UNIQUE` |
| DB-08 | Chunk enviado nunca é reescrito | trigger de aplicação + teste |
| DB-09 | `chunks.document_id` = `plans.document_id` | FK composta seria ideal; validado no store |
| DB-10 | Nenhum chunk termina em heading, exceto heading órfão no fim do documento | validação no `ValidatePlan` |

## Queries canônicas

Próximo chunk de um documento:

```sql
SELECT c.*
FROM chunks c
JOIN plans p ON p.id = c.plan_id AND p.status = 'active'
WHERE c.document_id = ? AND c.status = 'pending'
ORDER BY c.seq
LIMIT 1;
```

Documentos elegíveis para a run, na ordem de prioridade:

```sql
SELECT id, title, priority
FROM documents
WHERE status = 'ready'
ORDER BY priority, id;
```

Progresso:

```sql
SELECT
  COUNT(*)                                        AS total,
  SUM(status = 'sent')                            AS sent,
  ROUND(100.0 * SUM(status = 'sent') / COUNT(*), 1) AS pct
FROM chunks
WHERE plan_id = ?;
```

Validação de cobertura (deve retornar 0 linhas de gap):

```sql
WITH ordered AS (
  SELECT c.seq,
         c.first_block_id,
         c.last_block_id,
         LAG(c.last_block_id) OVER (ORDER BY c.seq) AS prev_last
  FROM chunks c
  WHERE c.plan_id = ?
)
SELECT * FROM ordered
WHERE seq > 0 AND first_block_id <> prev_last + 1;
```

## Migrações

- Arquivos embutidos: `internal/store/migrations/0001_init.sql`, `0002_....sql`.
- Aplicadas em transação, em ordem, no start; `schema_migrations` registra a versão.
- **Forward-only.** Correção de erro usa nova migração; nunca editar uma já publicada.
- Antes de migrar, o app copia o banco para `/data/backup/pre-migrate-{version}.db` (arquivo + `VACUUM INTO`).
- Falha na migração aborta o start com log claro; o processo não sobe com schema inconsistente.

## Retenção

| Dado | Política |
|---|---|
| `events` | 90 dias (job diário) |
| Runs/itens | mantidos; são o histórico de leitura |
| Planos `superseded` | mantidos enquanto houver `delivery_items` apontando para chunks |
| Documento excluído | cascata em blocos/planos/chunks/itens + remoção dos arquivos em disco |
| Originais | nunca removidos automaticamente |
| Links assinados | stateless (HMAC + expiração); nada a limpar |

## Backup

```sh
sqlite3 /data/db/app.db "VACUUM INTO '/data/backup/app-$(date +%F).db'"
tar -C /data -czf /data/backup/documents-$(date +%F).tar.gz documents
```

Restore: parar o container, substituir `app.db` e `documents/`, subir. Migrações pendentes rodam no start.

## Formato de `params_json`

```json
{
  "min_fill": 0.6,
  "max_overshoot": 1.4,
  "wpm": 200,
  "difficulty": 1.0,
  "mode": "finish_by_days",
  "horizon_days": 20,
  "target_seconds": 588,
  "section_alignment": true
}
```

`params_json` é gravado no plano para que o preview e o histórico expliquem exatamente como o plano foi gerado, mesmo que as configurações mudem depois.
