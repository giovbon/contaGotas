---
title: Contexto do projeto
status: ativo
updated: 2026-10-28
---

# Contexto

Pacote de contexto para humanos e agentes. **Se você só pode ler um arquivo, leia este.** Ele não substitui os documentos de área; aponta para eles.

## O que é

**Reading Drip** (nome provisório, `Q-001`) é um sistema self-hosted que recebe arquivos `.md`, `.pdf` e `.epub`, extrai o conteúdo em uma árvore de blocos, particiona em doses coerentes de leitura e envia **uma mensagem ntfy por dia de leitura**, com o texto completo em Markdown e um link para o leitor web.

Cada documento tem seu próprio plano e seu próprio ritmo. Vários documentos podem ser lidos em paralelo: a mensagem diária traz um trecho de cada um.

## Decisões fechadas

| ID | Decisão | Referência |
|---|---|---|
| D-01 | Go + templ; binário único com assets embutidos. CSS/JS próprios na FASE-1; Alpine/Tailwind adiados | [ADR-0001](./adr/0001-stack-go-templ-alpine-tailwind.md), [ADR-0014](./adr/0014-ui-css-js-proprios.md) |
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
| Linguagem | Go 1.26+ (templ exige) |
| HTTP | `net/http` (mux nativo) |
| Views | `templ` (server-side) |
| Interatividade | JS próprio (polling de status); Alpine adiado para upload/preview/edição |
| CSS | próprio, embutido (`go:embed`); Tailwind adiado |
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
| Código | **FASE-1 em andamento** — FASE-0 concluída; store, ingestão, importação, fila, upload, sessão, telas, entrypoint, composição, cliente ntfy, agendador, configurações/entrega manual, leitor por link assinado, ações de gestão, histórico, Dockerfile/compose, healthcheck e avisos de pacing prontos |
| Fase atual | `FASE-1` — MVP utilizável; falta validar o build da imagem ARM64 e o deploy num host com Docker, além das configurações de plano |
| Próximo marco | validar `docker buildx --platform linux/arm64` + `compose.yaml` + Cloudflare Tunnel; `SPIKE-04` (memória ARM64) segue aberto e não bloqueia a FASE-1 |

## Próximo passo

Validar a imagem ARM64 e o deploy em um host com Docker (o daemon não está disponível na máquina de desenvolvimento). O `/Dockerfile` já implementa o [ADR-0008](./adr/0008-build-arm64-distroless.md) — cross ARM64 com CGO estático ou `NATIVE=1`, runtime distroless non-root — e o `cmd/app` responde a `healthcheck` consultando `/readyz`, já que a imagem não tem shell. O `/compose.yaml` sobe app, `cloudflared` e `ntfy`, com `read_only`, `tmpfs` em `/tmp`, volume `/data`, porta só em `127.0.0.1` e `depends_on` por healthcheck; `/deploy/ntfy/server.yml` e `/.env.example` completam o pacote. Os avisos de pacing inviável (1.14) fecham os itens da FASE-2: `Params.Warnings` aponta alvo abaixo de 3 min, acima de 1 h ou dias sobrando no `finish_by_days`, no CLI e nas telas de detalhe/plano. Ainda falta a semana de entregas automáticas sem intervenção para marcar a FASE-2 como concluída. Roteiro de verificação em [`operations.md`](./operations.md#imagem-dockerfile). As ações de gestão e o histórico (1.12), o leitor com token assinado (1.11), as configurações de agenda/ntfy e os envios (1.10), o agendador (1.9), a composição/cliente ntfy (1.8), a autenticação de sessão ([ADR-0013](./adr/0013-sessao-hmac-argon2id.md)), as telas ([ADR-0014](./adr/0014-ui-css-js-proprios.md)), o entrypoint HTTP (`cmd/app`), a importação, a extração Markdown/EPUB/PDF e o plano nos dois modos estão prontos. As configurações de plano ficam para a FASE-3. O pico de memória de PDF com figuras grandes continua em aberto (`Q-016`, `SPIKE-04`).
