---
title: Contexto do projeto
status: ativo
updated: 2026-10-26
---

# Contexto

Pacote de contexto para humanos e agentes. **Se você só pode ler um arquivo, leia este.** Ele não substitui os documentos de área; aponta para eles.

## O que é

**Reading Drip** (nome provisório, `Q-001`) é um sistema self-hosted que recebe arquivos `.md`, `.pdf` e `.epub`, extrai o conteúdo em uma árvore de blocos, particiona em doses coerentes de leitura e envia **uma mensagem ntfy por dia de leitura**, com o texto completo em Markdown e um link para o leitor web.

Cada documento tem seu próprio plano e seu próprio ritmo. Vários documentos podem ser lidos em paralelo: a mensagem diária traz um trecho de cada um.

## Decisões fechadas

| ID | Decisão | Referência |
|---|---|---|
| D-01 | Go + templ + Alpine.js + Tailwind; binário único com assets embutidos | [ADR-0001](./adr/0001-stack-go-templ-alpine-tailwind.md) |
| D-02 | Particionamento opera sobre **blocos normalizados**, nunca sobre offsets de texto | [ADR-0002](./adr/0002-modelo-de-blocos-normalizado.md) |
| D-03 | PDF via `go-fitz` (MuPDF), CGO isolado atrás de build tag | [ADR-0003](./adr/0003-pdf-com-go-fitz-cgo.md) |
| D-04 | **Plano global persistido** e determinístico (não incremental) | [ADR-0004](./adr/0004-plano-global-persistido.md) |
| D-05 | ntfy **self-hosted**, mensagem completa em Markdown com `message-size-limit` elevado; link para o leitor como complemento | [ADR-0005](./adr/0005-ntfy-self-hosted-mensagem-completa.md) |
| D-06 | Documentos em **paralelo**, cada um com plano e pacing próprios | [ADR-0006](./adr/0006-documentos-em-paralelo.md) |
| D-07 | Dois modos de ritmo: `minutes_per_day` e `finish_by_days` | [ADR-0009](./adr/0009-dois-modos-de-ritmo.md) |
| D-08 | SQLite (`modernc.org/sqlite`) em WAL, migrações embutidas | [ADR-0007](./adr/0007-sqlite-modernc-wal.md) |
| D-09 | Imagem ARM64 multi-stage, distroless, non-root, read-only | [ADR-0008](./adr/0008-build-arm64-distroless.md) |
| D-10 | Exposição via **Cloudflare Tunnel + domínio próprio**; leitor acessível por **link assinado** | [ADR-0010](./adr/0010-cloudflare-dominio-proprio-links-assinados.md) |

## Invariantes (não negociáveis sem ADR)

- **I-01** — Um chunk é sempre um intervalo contíguo de blocos identificado por `first_block_id`/`last_block_id` **e** pelos hashes das pontas. Nunca por offset de caractere.
- **I-02** — Nenhum chunk termina em heading; heading sempre pertence ao bloco seguinte. Exceção: heading órfão como último bloco do documento.
- **I-03** — Blocos indivisíveis (código, tabela, item de lista, citação, figura) não são cortados, exceto quando sozinhos excedem o máximo — e nesse caso o chunk é marcado `oversized`.
- **I-04** — O plano é uma **função pura** de `(blocos, parâmetros)`: mesmo input gera exatamente o mesmo output.
- **I-05** — A cobertura do plano é total e sem sobreposição: todo bloco pertence a exatamente um chunk.
- **I-06** — O cursor de cada documento avança **somente após entrega 2xx** do chunk.
- **I-07** — Uma run de entrega é idempotente por `(run_date)` no fuso configurado; restart não reenvia.
- **I-08** — Links de leitor e ações são assinados (HMAC) e escopados; não exigem sessão no celular.
- **I-09** — O sistema funciona sem serviços externos além do ntfy. Sem telemetria, sem CDN obrigatória, sem LLM obrigatório.
- **I-10** — O original enviado é preservado e nunca sobrescrito.

> `oversized` marca qualquer chunk acima de `max`: um bloco indivisível grande demais **ou** um chunk que precisou ultrapassar o máximo para fechar uma seção inteira.


## Stack

| Camada | Escolha |
|---|---|
| Linguagem | Go 1.24+ |
| HTTP | `net/http` (mux nativo) |
| Views | `templ` (server-side) |
| Interatividade | Alpine.js (pontual: upload, preview, edição) |
| CSS | Tailwind (CLI no build, CSS final embutido) |
| Banco | SQLite via `modernc.org/sqlite` (WAL) |
| PDF | `go-fitz` (MuPDF, CGO) |
| EPUB | `archive/zip` + `encoding/xml` + `golang.org/x/net/html` |
| Markdown | `goldmark` (GFM + footnotes) |
| Agendador | ticker in-process (sem lib de cron) |
| Deploy | Cloudflare Tunnel + container ARM64 |

## Números-alvo

| Métrica | Alvo |
|---|---|
| RSS em idle | ≤ 60 MiB |
| Pico de RSS ao extrair PDF | ≤ 200 MiB |
| Imagem do container | ≤ 90 MiB |
| Binário | ≤ 60 MiB (CGO estático) |
| Cold start | ≤ 500 ms |
| Extração de PDF de 300 páginas | ≤ 15 s em ARM64 |
| Entrega ntfy | ≤ 5 s p95 |

## Não-objetivos (v1)

- Multi-usuário e compartilhamento entre contas.
- OCR de PDF escaneado (`Q-002`).
- Sincronização com Kindle/Kobo.
- Leitura em voz alta / TTS.
- Resumos por LLM como dependência obrigatória (`Q-004`).
- Edição de conteúdo do documento (só de fronteiras de plano).

## Estado atual

| Item | Estado |
|---|---|
| Documentação e decisões | **concluída** (2026-10-26) |
| Código | **em andamento** — FASE-0.2: `blocks`, `plan`, extratores Markdown/EPUB, CLI e testes |
| Fase atual | `FASE-0` — núcleo headless ([roadmap](./roadmap.md)) |
| Próximo marco | `SPIKE-01` + extrator PDF; depois benchmarks |

## Próximo passo

Fechar a FASE-0 com `SPIKE-01`, extrator PDF e benchmarks. O plano em Markdown e EPUB já funciona nos dois modos. Ver [`roadmap.md`](./roadmap.md), [`extraction.md`](./extraction.md) e [`segmentation.md`](./segmentation.md).
