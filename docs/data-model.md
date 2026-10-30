---
title: Modelo de dados
status: ativo
updated: 2026-10-30
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
- Antes de migrar, o app grava `VACUUM INTO /data/backup/pre-migrate-{versão-alvo}.db` quando o banco já tem dados. Banco novo (arquivo ausente ou vazio) não gera backup.
- O backup só aparece no nome final depois de completo (temporário + rename). Se o arquivo já existe de uma tentativa anterior, é preservado e a migração continua.
- Falha no backup aborta o start antes de qualquer alteração de schema. Falha na migração também aborta com log claro; o processo não sobe com schema inconsistente.

### Implementação (2026-10-27, backup em 2026-10-29)

`store.Open(ctx, path)` cria o diretório se preciso, abre com os PRAGMAs do topo via DSN (`journal_mode`, `synchronous`, `foreign_keys` e `busy_timeout` são por conexão, exceto o journal mode, gravado no arquivo), usa no máximo 4 conexões e aplica as migrações pendentes antes de devolver o `*Store`. `Open` equivale a `OpenWithOptions(ctx, path, Options{})`, sem backup.

- `internal/store/migrate.go`: lê `migrations/*.sql` com `go:embed`, ordena pelo prefixo numérico, recusa versão duplicada e aplica cada arquivo em uma transação junto com o `INSERT` em `schema_migrations`. Reabrir o banco não reaplica nada.
- `schema_migrations.applied_at` usa `strftime('%Y-%m-%dT%H:%M:%SZ','now')`; datas de domínio são sempre passadas pelo chamador (sem `time.Now()` no store).
- `OpenWithOptions` recebe `Options{BackupDir, OnBackup}`. Com `BackupDir` preenchido e migrações pendentes num banco existente, chama `Store.Backup` antes de criar/aplicar qualquer migração; `OnBackup` recebe o caminho final para log. `cmd/app` usa `{APP_DATA_DIR}/backup` e loga “backup pré-migração gravado”.
- `Store.Backup(ctx, dest)` usa `VACUUM INTO ?` (seguro com WAL, ADR-0007), cria o diretório, grava em `dest + ".tmp"` e renomeia. Destino já existente é tratado como backup íntegro e não é sobrescrito.

### Repositórios (2026-10-27)

Todos em `internal/store`, com SQL restrito ao pacote:

| Repositório | Métodos | Observações |
|---|---|---|
| `Store.Documents()` | `Create`, `Get`, `BySHA256`, `List`, `UpdateStatus`, `TransitionStatus`, `UpdateTotais`, `SaveExtraction`, `UpdateSource`, `Delete` | `List` ordena por `priority, id`; `Get`/`BySHA256` devolvem `nil` sem erro quando não existem; defaults do schema são aplicados no `Create`; `TransitionStatus` só muda o status se o atual estiver entre os estados de origem (devolve `false` sem erro caso contrário); `Delete` remove derivados em transação e devolve `ErrDocumentNotFound` |
| `Store.Blocks()` | `Replace`, `List`, `Count`, `IDs`, `Between`, `Section` | `Replace` troca o documento inteiro em transação e grava `heading_id` na segunda passada; `List` converte o id do banco de volta para o índice do bloco (`-1` = sem heading); `IDs` serve ao repositório de planos; `Between` recorta o trecho entregue e `Section` devolve a seção do heading ancestral (`HeadingID < 0` = abertura), ambas usadas pelo leitor |
| `Store.Plans()` | `Create`, `Replan`, `Active`, `Get`, `Versions`, `Chunk`, `NextPending`, `PreviousChunk`, `NextChunk`, `MarkSent`, `MarkSkipped`, `MarkDone`, `Progress`, `HasDeliveryHistory`, `DiscardUndelivered` | `Create` substitui o plano ativo, calcula `version = MAX+1`, valida cobertura e grava os ids reais dos blocos; `Replan` faz o mesmo a partir de um bloco inicial e atualiza o ritmo do documento na mesma transação (RF-024); `MarkSent` só altera chunks `pending` (DB-08); `Chunk` devolve `nil` sem erro quando não existe; os vizinhos servem ao recap/rodapé da dose |
| `Store.Runs()` | `Create`, `GetByDate`, `Running`, `List`, `AddItem`, `ItemByDocument`, `ListItems`, `MarkItemSent`, `MarkItemFailed`, `MarkItemSkipped`, `FinishRun`, `Delete` | `Create` é idempotente por `run_date` (`ON CONFLICT DO NOTHING`) e devolve `criada=false` quando a run já existia; `Running` alimenta a retomada; `List` devolve as runs mais recentes (padrão 30, máximo 200), opcionalmente só as que tiveram item de um documento; `ItemByDocument` devolve `nil` sem erro quando o documento não tem item na run; `MarkItemSent` grava item e chunk na mesma transação e devolve `ErrChunkNotPending` sem alterar nada se o chunk já não está `pending`; `MarkItemFailed`/`MarkItemSkipped` só mudam o item; `Delete` remove só run `running` (cascata nos itens) e serve para desfazer uma run manual que falhou antes de enviar |
| `Store.Settings()` | `Get`, `Set`, `SetMany`, `All` | `Set` faz upsert e grava `updated_at`; `SetMany` grava o lote inteiro em uma transação (ou nada muda) e aceita valor vazio, que equivale a “não configurada” na leitura |
| `Store.Events()` | `Add`, `List` | `List` devolve do mais recente para o mais antigo; `document_id` é opcional |

`SaveExtraction` grava título/autor/idioma, totais, status e mensagem de erro em uma instrução. Título, autor e idioma vazios preservam o valor atual (a extração não apaga metadado editado); `Error` vazio limpa o erro anterior.

### Reprocessamento (2026-10-27)

Reextrair troca todos os blocos do documento, o que invalida os ids referenciados pelos chunks antigos. O worker consulta `Plans.HasDeliveryHistory` antes de extrair (evita trabalho caro) e chama `Plans.DiscardUndelivered` imediatamente antes de substituir os blocos:

- `HasDeliveryHistory` é verdadeiro se existe chunk fora de `pending` ou qualquer `delivery_items` do documento;
- havendo histórico, o worker devolve `store.ErrDeliveryHistory` sem alterar nada — o documento continua `ready`;
- sem histórico, `DiscardUndelivered` apaga `chunks` e `plans` em transação (revalidando a condição) e a nova extração recria o plano a partir da versão 1.

Planos descartados não tinham entrega, então não há histórico a preservar.

Mudar apenas ritmo ou metadados (RF-005/RF-024) não deve reextrair: isso entra com o `Replan` sobre os blocos existentes, preservando os chunks já enviados.

### Regeneração a partir do cursor (2026-10-30)

`Plans.Replan(ctx, documentID, startBlock, plano, params, agora)` atende RF-024:

- valida a cobertura do plano contra os blocos reais (começa em `startBlock`, contíguo, termina no último bloco) e os limites do documento **antes** de abrir a transação;
- atualiza `documents.pacing_mode` e `pacing_minutes`/`pacing_days` conforme `params`: em `finish_by_days` os minutos anteriores são preservados; em `minutes_per_day`, `pacing_days` volta a NULL;
- marca o plano ativo anterior como `superseded` (os chunks dele — inclusive `sent`/`skipped` — não são tocados) e insere a nova versão como `active` com `version = MAX+1`;
- rollback em qualquer falha: nem ritmo nem plano mudam.

`ingest.Service.Replan` localiza o cursor (`NextPending`), confere que o `first_block_id` existe, relê `plan.min_fill`/`plan.max_overshoot` e chama `plan.GenerateFrom` sobre o sufixo. Documentos `ready`/`paused` com plano ativo são aceitos; `ErrNoPlan`, `ErrNothingPending`, `ErrInvalidPacing` e `ErrNotProcessable` viram avisos na UI. O progresso (`Plans.Progress`) é sempre por plano, então passa a medir só o restante.

### Gestão de documentos (2026-10-28)

As ações da UI trazem a checagem de estado para o banco, para que um clique concorrente com o worker não sobrescreva um estado inesperado:

| Transição | Origem aceita | Efeito |
|---|---|---|
| pausar | `ready` | `paused`; a run deixa de selecionar o documento (RF-065) |
| retomar | `paused` | `ready`; o cursor continua no menor `seq` pendente |
| reprocessar | `ready`, `failed`, `needs_ocr` | `processing` + `Enqueue`; falha da fila restaura o status anterior |
| excluir | qualquer | cascata + remoção de `documents/{id}` |

`Documents.TransitionStatus` faz um `UPDATE … WHERE status IN (…)` e devolve `false` quando nenhuma linha mudou; a UI responde `?aviso=indisponivel` em vez de forçar o estado. O reprocessamento **não** aceita documento `paused`, `completed`, em `processing` ou com histórico (`Plans.HasDeliveryHistory`); os dois últimos evitam que a reextração invalide ids de bloco já entregues.

A exclusão remove primeiro o registro (`Documents.Delete`, em transação: itens → chunks → planos → blocos → documento) e depois `documents/{id}` via `library.Importer.RemoveOriginal`. Se o diretório falhar, o documento já saiu e o erro fica só no log. Excluir um documento em `processing` é permitido: a extração em andamento falha na gravação e as FKs impedem dados órfãos.

Cada ação grava um evento `kind='system'` com o id do documento (`documento pausado`, `retomado`, `reenfileirado para reprocessamento`, `excluído`). O histórico (`/history`) é só leitura sobre `delivery_runs`/`delivery_items`; a agregação (enviados, falhas, bytes, latência) é feita na camada web, sem coluna derivada no banco.

### Entregas (2026-10-27)

`delivery_runs` guarda uma linha por `run_date` local; `delivery_items` guarda a dose de cada documento na run, com no máximo uma por documento (`UNIQUE (run_id, document_id)`). O agendador cria a run com `ON CONFLICT DO NOTHING`; se a linha já existia, não reexecuta nada.

- `status='running'` é o estado inicial e também o de uma run interrompida; `Runs.Running` devolve a mais recente para retomada.
- `chunks.status` e `delivery_items.status` só avançam juntos em `MarkItemSent`, na mesma transação. `ErrChunkNotPending` aborta o item sem alterar o chunk.
- Falha de envio deixa o chunk `pending` e o item `failed`; a próxima run tenta o mesmo chunk.
- Item pulado (`skipped`) registra o motivo em `error` e não move o cursor.
- Uma mensagem agrupada grava o mesmo `bytes` em cada item que ela levou.
- O envio manual (`POST /documents/{id}/send-now`) reaproveita a run do dia; se ela não existe, cria com `scheduled_for` = instante do clique e `late=0`. A criação só acontece quando há chunk pendente, para um clique sem conteúdo não consumir a run programada. Se a publicação falhar antes de qualquer envio, a run criada é removida (`Runs.Delete`); runs de ticks anteriores não são tocadas nem fechadas.
- A run fecha em `sent` (nenhuma falha), `partial` (algumas) ou `failed` (nenhum envio bem-sucedido); `finished_at` e `error` resumem o resultado.

### Importação e arquivos (2026-10-27)

`internal/library.Importer` é o caminho único de entrada de arquivos (a rota `POST /documents` só faz o parse multipart):

1. valida a extensão com `docs.Detect` (`.md`, `.markdown`, `.epub`, `.pdf`);
2. grava o upload em `{APP_DATA_DIR}/documents/.upload-*` calculando `sha256` em streaming, lendo no máximo `limite + 1` bytes;
3. recusa vazio (`ErrEmptyFile`) e acima do limite (`TooLargeError`, padrão 95 MiB);
4. consulta `documents.source_sha256`; se existir, devolve `DuplicateError` com o documento atual (RF-007);
5. cria a linha em `processing` e move o temporário para `{APP_DATA_DIR}/documents/{id}/source{ext}` (mesmo sistema de arquivos, `rename` atômico);
6. atualiza `source_name`/`source_path` com `UpdateSource` e enfileira no worker.

Se qualquer passo depois da criação falhar — inclusive o enfileiramento — o documento e derivados são excluídos (`Delete`) e o diretório do original é removido: ou a importação conclui, ou nada fica para trás. Um crash entre a criação e o `rename` deixa o documento em `processing`; `Importer.CleanupTemp` remove os `.upload-*` órfãos e deve ser chamado no start do app.

- O nome original fica em `source_name` (apenas o nome base, sem caminho). No disco, o arquivo é sempre `source{ext}` com a extensão em minúsculas.
- A resposta do upload não espera a extração: o documento volta em `processing` e a UI faz polling até `ready`/`failed`/`needs_ocr` (RF-002).
- `Importer.Root` é o diretório de dados; `MaxBytes <= 0` usa `DefaultMaxBytes` (95 MiB).

## Retenção

| Dado | Política |
|---|---|
| `events` | 90 dias (job diário) |
| Runs/itens | mantidos; são o histórico de leitura |
| Planos `superseded` | mantidos enquanto houver `delivery_items` apontando para chunks |
| Documento excluído | cascata em blocos/planos/chunks/itens; o diretório `documents/{id}` é removido pela camada de importação |
| Originais | nunca removidos automaticamente |
| Links assinados | stateless (HMAC + expiração); nada a limpar |

## Backup

```sh
sqlite3 /data/db/app.db "VACUUM INTO '/data/backup/app-$(date +%F).db'"
tar -C /data -czf /data/backup/documents-$(date +%F).tar.gz documents
```

Restore: parar o container, substituir `app.db` e `documents/`, subir. Migrações pendentes rodam no start. O backup automático pré-migração fica em `backup/pre-migrate-{versão-alvo}.db` e cobre só o banco; os originais continuam em `documents/`.

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
