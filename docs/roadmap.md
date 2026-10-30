---
title: Roadmap
status: ativo
updated: 2026-10-29
---

# Roadmap

Fases entregáveis, com critério de saída verificável. Status: `planejado` · `em andamento` · `concluído`.

| Fase | Tema | Status |
|---|---|---|
| `FASE-0` | Núcleo headless (extração + tempo + segmentador) | concluído (2026-10-27) |
| `FASE-1` | MVP utilizável (upload, plano, ntfy, leitor) | em andamento |
| `FASE-2` | Automação (agendador, histórico, retry) | em andamento |
| `FASE-3` | Refinamento (edição de plano, calibração, PDF) | planejado |
| `FASE-4` | Operação (imagem ARM64, backup, hardening) | planejado |

---

## FASE-0 — Núcleo headless

**Objetivo:** provar que o particionamento funciona antes de investir em UI.

- [x] modelo `blocks.Block` + árvore de seções
- [x] estimador de tempo + testes
- [x] extrator Markdown (goldmark) + golden
- [x] extrator EPUB + golden
- [x] `SPIKE-01`: confirmar API do `go-fitz` para fonte/estilo
- [x] extrator PDF (coluna única e duas colunas) + golden
- [x] segmentador: modo A e modo B, empacotamento e balanceamento
- [x] pré-divisão de parágrafo gigante em blocos alinhados a frases
- [x] `Validate` + testes de propriedade
- [x] CLI `readingdrip plan --input arquivo --minutes 10` imprimindo o plano
- [x] benchmarks (`BenchmarkPlan10kBlocks`)
- [x] revisão manual com 3 PDFs reais (arXiv) e fechamento da fase

**Incremento 0.4 (concluído em 2026-10-27):** revisão com três artigos reais do arXiv, ajustes de ordem de leitura em folha de rosto, fusão de títulos quebrados pelo MuPDF, descarte de margens e nivelamento de títulos sem rótulos de figura. Pico de memória medido em x86_64 (`SPIKE-04` parcial, `Q-016`). Ver [`extraction.md`](./extraction.md) e [`changelog.md`](./changelog.md).

**Incremento 0.3 (concluído em 2026-10-27):** extrator PDF completo sobre o HTML estruturado do MuPDF: posição/tamanho por linha, detecção de colunas, remoção de cabeçalho/rodapé, junção de linhas com hifenização, títulos por estilo e fallback textual. Fixtures determinísticas (`coluna-unica`, `duas-colunas`, `escaneado`, `headings-10pg`) e goldens de plano. `SPIKE-01` concluído; ver [`extraction.md`](./extraction.md) e [`spikes.md`](./spikes.md).

**Incremento 0.1 (2026-10-26):** `internal/blocks`, `internal/plan`, `internal/docs/markdown` e `cmd/readingdrip` funcionando; 5 goldens de Markdown (incluindo modo B e bloco oversized); 300 casos de propriedade. Ver [`changelog.md`](./changelog.md).

**Incremento 0.2 (2026-10-26):** extrator EPUB com `archive/zip` + `x/net/html`, sanitização de HTML, metadados, spine, fixture determinística e golden `epub.plan.txt`; CLI aceita `.epub`. Ver [ADR-0012](./adr/0012-epub-zip-xhtml.md).


**Saída:** rodar o CLI em 3 fixtures reais e obter planos que “fazem sentido para ler” em revisão manual, com todos os testes verdes.

**Riscos:** qualidade do PDF em duas colunas; API do `go-fitz`; tempo de extração em ARM64.

---

## FASE-1 — MVP utilizável

- [x] SQLite + migrações + repositórios
- [x] upload multipart com limite e deduplicação por sha256
- [x] worker de extração (1 por vez) + status
- [x] persistência de plano e chunks
- [x] UI: biblioteca, detalhe, preview do plano
- [x] configurações: ntfy, agenda e plano (WPM, dificuldade, min/max, recap)
- [x] entrypoint HTTP (`cmd/app`) com wiring de config, store, auth, importador e fila
- [x] composição da mensagem (breadcrumb, recap, rodapé)
- [x] cliente ntfy com retry (sem envio de teste nem “enviar agora”)
- [x] agendador in-process + runs idempotentes + catch-up + retomada
- [x] envio de teste + “enviar agora”
- [x] leitor com token assinado + “marcar lido”
- [x] autenticação por senha + sessão
- [x] ações de gestão + histórico (1.12)
- [x] Dockerfile + imagem arm64 (build cross + smoke test em 2026-10-29)
- [ ] deploy atrás de Cloudflare Tunnel (compose e doc prontos em 1.13; imagem validada em 1.13; falta rodar no host com Compose v2)

**Validação 1.13 (2026-10-29):** com o Docker disponível, o build cross ARM64 foi executado e passou depois de trocar o builder de `bookworm` para `trixie` — os objetos pré-compilados do `go-fitz` usam `__isoc23_strtol`/`strtoul`, ausentes na glibc 2.36. A imagem resultante tem **23.965.482 bytes (~22,9 MiB)**, arquitetura `arm64`, e o container subiu `healthy` com `read_only` + `tmpfs /tmp`; `/healthz` e `/readyz` responderam `200`, e o fluxo upload `.md` → `ready` → plano v1 funcionou com o original preservado em `/data/documents/1/source.md` (owner `65532`). O deploy completo com `compose.yaml` e Cloudflare Tunnel segue pendente: o host de validação não tem o plugin Compose v2 nem credenciais do túnel.

**Incremento 1.13 (entregue em 2026-10-28, validação pendente):** imagem ARM64 e deploy de referência. `/Dockerfile` multi-stage com cross `gcc-aarch64-linux-gnu` (ou `NATIVE=1` em runner ARM64), `templ generate`, CGO com link estático e runtime distroless non-root; `cmd/app healthcheck` consulta `/readyz` (a imagem não tem shell) e `cmd/app` embute `time/tzdata`. `/compose.yaml` sobe app, `cloudflared` e `ntfy`, com app `read_only`, `/tmp` em tmpfs, volume `/data`, porta só em `127.0.0.1` e `depends_on` por healthcheck; `/deploy/ntfy/server.yml` e `/.env.example` completam o pacote. O build da imagem ainda **não foi executado** (daemon do Docker indisponível); ver [`operations.md`](./operations.md#imagem-dockerfile).

**Incremento 1.12 (concluído em 2026-10-28):** ações de gestão e histórico. `POST /documents/{id}/pause` e `/resume` mudam só `documents.status` com `Documents.TransitionStatus` (origem validada no banco); `POST /documents/{id}/reprocess` reextrai quando não há histórico e restaura o status se a fila recusar; exclusão em duas etapas (`GET` confirmação, `POST /documents/{id}/delete` ou `DELETE /documents/{id}`) remove derivados em cascata e o diretório via `library.Importer.RemoveOriginal`. `GET /history` lista as últimas 30 runs como `Runs.List` (filtro por documento), com itens, tentativas, bytes, erros e latência. Cada ação grava evento `system`. Ver [`web-ui.md`](./web-ui.md#ações-de-gestão-documentsid) e [`data-model.md`](./data-model.md#gestão-de-documentos-2026-10-28).

**Incremento 1.11 (concluído em 2026-10-27):** leitor público por link assinado (`GET /r/{chunkID}?t=`), “marcar lido” (`POST /api/actions/chunks/{id}/done?t=`), navegação anterior/próximo e “ver seção completa”. `internal/links` assina tokens stateless `{exp}.{HMAC-SHA256}` com escopos `r`/`done` e chave derivada de `APP_SECRET`; as validades saem de `reader.link_ttl_days` (180) e `reader.done_ttl_days` (30). `Runner.Links` passa a preencher `click` e as ações do ntfy; o teste continua sem link. O POST responde `204` para o ntfy e `303` de volta ao leitor (`ui=1`/`Accept: text/html`). Ver [`web-ui.md`](./web-ui.md#leitor-rchunkid), [`delivery-ntfy.md`](./delivery-ntfy.md#links-assinados) e [`scheduling.md`](./scheduling.md#configuração).

**Incremento 1.10 (concluído em 2026-10-27):** UI de configurações (`GET/POST /settings`), envio de teste (`POST /settings/test`) e envio manual (`POST /documents/{id}/send-now`). O formulário cobre agenda e ntfy, valida com o mesmo `schedule.Parse` do agendador, grava em transação (`Settings.SetMany`) e nunca renderiza o token. O envio de teste e o manual são síncronos, de uma tentativa, e devolvem o resultado por query string (PRG): o manual usa a run do dia, reaproveita run `running` de um tick, recusa run já fechada e descarta a run que ele mesmo criou quando a publicação falha antes de qualquer envio. Ver [`web-ui.md`](./web-ui.md#configurações-settings), [`scheduling.md`](./scheduling.md#envio-manual) e [`delivery-ntfy.md`](./delivery-ntfy.md#envio-de-teste).

**Incremento 1.9 (concluído em 2026-10-27):** agendador em processo com `internal/schedule`: lê `schedule.*` e `ntfy.*` do banco, cria runs idempotentes por `run_date`, respeita dias/horário/fuso, aplica catch-up em dias de leitura e retoma run `running` após restart reenviando só itens `pending`. O tick é serializado por `TryLock` e roda na goroutine do ticker; `Stop` cancela e espera. Falha de publicação mantém o chunk `pending`; item `sent` grava chunk na mesma transação. `cmd/app` inicia e encerra o agendador. Ver [`scheduling.md`](./scheduling.md) e [`delivery-ntfy.md`](./delivery-ntfy.md).

**Incremento 1.8 (concluído em 2026-10-27):** `internal/delivery` com composição pura (`Compose`) e cliente ntfy (`NewClient`/`Publish`). A composição monta cabeçalho, breadcrumb, métricas, recap extrativo (duas frases, ≤ 240 caracteres), heading repetido, corpo `full`/`preview`, rodapé “Amanhã” e link do leitor; agrupa documentos em seções `## 📖 …` e, acima de `max_message_bytes`, gera uma mensagem por documento, truncando em 60% do limite com link. O cliente publica JSON com Bearer opcional, 3 tentativas, backoff 30 s/2 min e `Retry-After` para 429, sem retry em 4xx. Ver [`delivery-ntfy.md`](./delivery-ntfy.md) e [`continuity.md`](./continuity.md).

**Incremento 1.7 (concluído em 2026-10-27):** entrypoint HTTP em `cmd/app`: configuração por ambiente (`internal/platform`), store SQLite, autenticação, importador, fila de extração e `web.Server`. No start, limpa temporários de upload e reenfileira documentos presos em `processing`; no shutdown, para o servidor, drena a fila e fecha o banco. Logs em texto com `LOG_LEVEL`. Ver [`operations.md`](./operations.md).

**Incremento 1.6 (concluído em 2026-10-27):** telas somente leitura em templ: biblioteca (`GET /`), detalhe (`GET /documents/{id}`), preview do plano (`GET /documents/{id}/plan`), download do original (`GET /documents/{id}/download`) e página de erro. CSS/JS próprios embutidos e servidos em `/static/`; polling do status `processing` por recarga da própria página, com JS progressivo. Login migrado de `html/template` para templ. Assets e UI seguem a CSP `script-src 'self'`; Alpine/Tailwind ficam adiados ([ADR-0014](./adr/0014-ui-css-js-proprios.md)). Ver [`web-ui.md`](./web-ui.md).

**Incremento 1.5 (concluído em 2026-10-27):** autenticação de sessão com senha única (`APP_PASSWORD`) conferida por Argon2id, cookie stateless `rd_session` assinado com chave derivada de `APP_SECRET` + senha (7 dias, `HttpOnly`, `SameSite=Lax`, `Secure` em HTTPS), rate limit de 5 falhas/min por IP e rotas `GET/POST /login` + `POST /logout`. Rotas de gestão sem sessão redirecionam para `/login` (RF-074), com `?next=` restrito a caminho local. Página de login mínima em `html/template`, CSP e `Referrer-Policy`. Ver [ADR-0013](./adr/0013-sessao-hmac-argon2id.md) e [`web-ui.md`](./web-ui.md).

**Incremento 1.4 (concluído em 2026-10-27):** `internal/web` com `POST /documents` (multipart em streaming, um ou vários arquivos, dedup, 303/400/413/422), `/healthz` e `/readyz`, cabeçalhos `no-store`/`nosniff` e middleware de sessão injetável (sem middleware → 401). A request não espera extração. Telas, login e demais rotas ficam para os próximos incrementos. Ver [`web-ui.md`](./web-ui.md).

**Incremento 1.3 (concluído em 2026-10-27):** importação de arquivos e fila do worker: `internal/library` valida formato, calcula `sha256` em streaming, aplica limite de 95 MiB, recusa duplicata com o documento existente, grava `{data}/documents/{id}/source.*` e desfaz tudo em falha. `ingest.Queue` processa um documento por vez (buffer 16), drena no shutdown e sobrevive a erro/panic por documento. A rota HTTP multipart fica para o incremento 1.4. Ver [`data-model.md`](./data-model.md) e [`changelog.md`](./changelog.md).

**Incremento 1.2 (concluído em 2026-10-27):** pipeline de ingestão: `docs.ExtractFile` despacha md/epub/pdf para um `Result` comum, PDF escaneado usa a sentinela `pdf.ErrScanned`, e `internal/ingest` extrai, normaliza, gera e persiste blocos + plano ativo + status (`ready`, `failed`, `needs_ocr`) com evento. Reprocessar descarta planos não entregues (`Plans.DiscardUndelivered`) e recusa documentos com histórico de entrega. Ver [`data-model.md`](./data-model.md) e [`changelog.md`](./changelog.md).

**Incremento 1.1 (concluído em 2026-10-27):** `internal/store` com SQLite (`modernc.org/sqlite`), migrações embutidas forward-only, schema completo e repositórios de documentos, blocos, configurações, eventos, planos e chunks. Backup pré-migração fica para o start do app. Ver [`data-model.md`](./data-model.md).

**Saída:** três documentos ativos, envio manual funcionando no celular, leitor abrindo por link sem login, container rodando em ARM64 com < 100 MiB.

---

## FASE-2 — Automação

**Adiantado na FASE-1 (2026-10-28):** histórico de runs e pausar/retomar por documento entraram no incremento 1.12, junto das demais ações de gestão.

- [x] ticker + calendário + timezone/DST (1.9)
- [x] idempotência por `run_date` (1.9)
- [x] catch-up (1.9)
- [x] lock de run + retomada após restart (1.9)
- [x] retry com backoff e classificação de erro (1.8)
- [x] histórico de runs e itens (adiantado em 1.12)
- [x] pausar/retomar por documento (adiantado em 1.12)
- [x] limite `max_docs_per_run` (1.9)
- [x] avisos de pacing inviável (1.14)

**Incremento 1.14 (concluído em 2026-10-28):** avisos de pacing inviável (RF-026). `Params.Warnings` deriva alertas puros para `finish_by_days` (alvo < 180 s, > 3600 s ou dias maiores que os chunks), o CLI imprime `# aviso:`, e detalhe/plano mostram “Ritmo inviável”. Nada é persistido e o plano continua válido. Ver [`segmentation.md`](./segmentation.md#modos-de-ritmo).

**Saída:** uma semana inteira de entregas automáticas sem intervenção, incluindo um restart forçado no meio. Todos os itens estão entregues; falta essa validação de campo para marcar a fase como concluída.

---

## FASE-3 — Refinamento

- [ ] edição de fronteiras (mesclar/dividir/mover)
- [ ] regenerar plano a partir do cursor
- [ ] simulação de parâmetros no preview
- [ ] calibração de `difficulty` por feedback
- [ ] hardening de PDF (3+ colunas, notas, figuras)
- [ ] `SPIKE-02`: renderização de figuras de PDF
- [ ] recaps opcionais por LLM (desligado por padrão)
- [ ] busca full-text (FTS5)
- [ ] exportar plano em Markdown/CSV

**Saída:** usuário consegue corrigir um plano ruim sem reimportar o documento.

---

## FASE-4 — Operação

- [ ] backup automático + restore testado
- [ ] `/healthz` e `/readyz` completos
- [ ] métricas leves (contadores em memória)
- [ ] rotação de `APP_SECRET` com regeneração de links
- [ ] checklist de segurança aplicado
- [ ] documentação de upgrade
- [x] pipeline de release (GitHub Actions → Docker Hub, foco arm64; 2026-10-30)
- [ ] teste de carga com 50 documentos

**Saída:** upgrade de versão sem perda de dados e restore validado em container limpo.

---

## Fora do roadmap

- OCR (`Q-002`)
- multi-usuário (`Q-008`)
- envio para Kindle
- apps nativos
- TTS

## Próximo passo imediato

Validar a imagem ARM64 e o deploy em um host com Docker: `docker buildx build --platform linux/arm64 ...`, subir o `compose.yaml` e conferir `docker inspect --format '{{.State.Health.Status}}'` = `healthy`, acesso pelo Cloudflare Tunnel e upload/envio de ponta a ponta. Com isso a FASE-1 fecha. O Dockerfile, o healthcheck, o `compose.yaml`, o ntfy de referência e o `.env.example` já estão prontos (1.13); o build não roda nesta máquina porque o daemon do Docker não está disponível. Depois da validação, a FASE-1 fecha; a FASE-2 só depende da semana de entregas automáticas com restart no meio. `SPIKE-04` segue aberto e não bloqueia.
