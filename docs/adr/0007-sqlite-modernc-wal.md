---
title: ADR-0007 — SQLite modernc em WAL
status: aceito
date: 2026-10-26
---

# ADR-0007 — SQLite modernc em WAL

## Contexto

O volume de dados é modesto (dezenas de documentos, milhares de blocos, histórico de entregas). O deploy é um container único com volume local. O projeto já exige CGO para PDF, mas queremos manter o núcleo testável sem toolchain C.

## Decisão

- SQLite via **`modernc.org/sqlite`** (implementação pura Go).
- `journal_mode=WAL`, `synchronous=NORMAL`, `foreign_keys=ON`, `busy_timeout=5000`.
- Migrações SQL embutidas (`go:embed`), forward-only.
- Acesso somente pelo pacote `internal/store`; nenhum SQL em handlers.

## Alternativas consideradas

| Alternativa | Por que não |
|---|---|
| PostgreSQL | serviço extra, backup e operação desproporcionais |
| bbolt | sem SQL, consultas de cursor/histórico ficam trabalhosas |
| `mattn/go-sqlite3` (CGO) | mais um ponto de CGO; modernc simplifica testes e cross |
| Arquivos JSON | concorrência, integridade e consultas frágeis |
| DuckDB | analítico, não transacional para o caso |

## Consequências

### Positivas
- Zero serviço externo; backup é copiar um arquivo (com `VACUUM INTO`).
- Testes rodam sem CGO e sem infraestrutura.
- WAL permite leitura durante escrita.
- Migrações versionadas no binário.

### Negativas / riscos
- modernc é ~2–3× mais lento que a versão C (irrelevante nesta escala).
- WAL cria `-wal`/`-shm`; backup ingênuo pode corromper.
- Uma única instância de escrita (adequado, mas limita réplicas).

### Mitigações
- Backup sempre com `VACUUM INTO`.
- `BEGIN IMMEDIATE` para escritas concorrentes.
- Teste de `integrity_check` no CI.
- Suporte oficial a 1 réplica (`Q-013`).
