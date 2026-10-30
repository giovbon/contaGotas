---
title: Changelog
status: ativo
updated: 2026-10-30
---

# Changelog

Append-only, mais recente no topo. Uma entrada por fase ou mudança relevante. Não reescreva entradas antigas.

> **Agentes:** leia apenas a entrada mais recente; não carregue o arquivo inteiro.

## 2026-10-30 — CI/CD para Docker Hub e template de deploy

**Adicionado**
- `.github/workflows/docker.yml`: testa (`go test ./... -race`), compila e publica a imagem `linux/arm64` no Docker Hub a cada push na `main`, tag `v*` e dispatch manual; em PR só valida (build sem push). Tags: `latest`, `<branch>`, `<sha>` e versão; cache GHA no buildx. Segredos `DOCKERHUB_USERNAME`/`DOCKERHUB_TOKEN`.
- `/deploy/compose.template.yml`: template de deploy que usa a imagem publicada (sem build local), com `app` + `cloudflared` + `ntfy`, `read_only`, healthcheck e roteiro de uso no cabeçalho.

**Corrigido**
- [`web-ui.md`](./web-ui.md) e [`context.md`](./context.md): as configurações de plano (WPM, dificuldade, `min_fill`, `max_overshoot`, recap) já estão em `/settings` desde o incremento 1.15 — a doc dizia que ficavam para a FASE-3.

**Documentação**
- [`operations.md`](./operations.md) — seção “CI/CD (GitHub Actions)” e referência ao template.
- [`roadmap.md`](./roadmap.md) — item “configurações de plano” concluído; “pipeline de release” concluído com o workflow.

## 2026-10-30 — autenticação do ntfy por usuário e senha

**Adicionado**
- HTTP Basic no cliente ntfy: novas chaves `ntfy.username`/`ntfy.password`, lidas por `schedule.LoadSettings` e enviadas como `Authorization: Basic`. Quando o par está preenchido, tem precedência sobre `ntfy.token` (Bearer).
- Campos “Usuário (Basic)” e “Senha (Basic)” em `/settings`; a senha nunca é renderizada, em branco mantém a salva e limpar o usuário remove o par. O par incompleto falha a validação citando as duas chaves.
- Entrada comentada de `auth-file`/`auth-default-access` em `deploy/ntfy/server.yml` e comandos `ntfy user add`/`ntfy access` para liberar só escrita no tópico.

**Testes**
- `TestPublishBasicAuth` e `TestPublishBasicTemPrecedencia` no cliente; par usuário/senha em `settings_test` e no fluxo de `/settings`.

**Documentação**
- [`delivery-ntfy.md`](./delivery-ntfy.md) — chaves, precedência, configuração do servidor com auth e testes.
- [`scheduling.md`](./scheduling.md), [`web-ui.md`](./web-ui.md) e [`operations.md`](./operations.md) — chaves, campos da tela, operação e checklist.
## 2026-10-30 — leitura econômica da documentação para agentes

**Adicionado**
- `docs/tasks/`: briefs de tarefa de ~60 linhas, com `README.md`, `TEMPLATE.md` e o primeiro brief (`rf-023.md`).
- `AGENTS.md`: orçamento de ~10 mil tokens de documentação antes do código, leitura por seção e obrigação de criar/atualizar o brief.
- `docs/README.md`: mapa “tarefa → leitura mínima” e trilha completa separada, só para onboarding/revisão de arquitetura.

**Decisões**
- O brief não substitui os docs de área; em conflito, o doc de área vence. Contrato em aberto não entra como fechado.
- O `changelog.md` continua append-only; agentes leem só a entrada mais recente, nunca o arquivo inteiro.
- Nenhum doc de área foi dividido nesta mudança; o ganho vem da leitura direcionada e dos briefs.

**Testes**
- Mudança somente de documentação; nenhum código de produção foi alterado. Links relativos e front matter revisados.

**Documentação**
- [`README.md`](./README.md), [`documentation-pattern.md`](./documentation-pattern.md), [`tasks/`](./tasks/README.md) e `AGENTS.md`.

## 2026-10-30 — regeneração do plano a partir do cursor (RF-024, 1.17)

**Adicionado**
- `plan.GenerateFrom(bs, params, start)` gera o sufixo do documento a partir do bloco `start`, com índices de bloco **absolutos** e `section_path`/`starts_mid_section` derivados do documento inteiro. O alvo de `finish_by_days` usa o tempo restante (segundos restantes ÷ dias restantes).
- `plan.ValidateFrom(bs, pl, p, start)` valida a cobertura de `start` até o fim; `Validate` delega com `start=0` e mantém o contrato anterior.
- `Plans.Replan(ctx, documentID, startBlock, plano, params, agora)`: em uma transação, cria a nova versão (`version = MAX+1`), marca a anterior como `superseded` e grava o novo ritmo (`pacing_mode`, `pacing_minutes`/`pacing_days`). Chunks enviados/ignorados do plano antigo não são tocados.
- `ingest.Service.Replan` + `ReplanRequest`/`ReplanResult`, com sentinelas `ErrNoPlan`, `ErrNothingPending` e `ErrInvalidPacing`. Aceita documentos `ready`/`paused` com plano ativo e ao menos um chunk pendente; o início é o `first_block_id` do primeiro chunk pendente.
- Rota `POST /documents/{id}/plan/regenerate` (sessão) e formulário “Ritmo de leitura” no detalhe: modo, minutos por dia e dias de leitura. Mensagens `?acao=regenerado`, `?aviso=ritmo`, `?aviso=sem-plano` e `?aviso=sem-trechos`.
- `cmd/app` compartilha o mesmo `ingest.Service` entre a fila e o `Replanner` da UI.

**Decisões**
- Regenerar **não** reextrai: opera sobre os blocos existentes e só recalcula o sufixo. O plano antigo permanece consultável como `superseded`.
- A nova versão cobre do cursor até o último bloco; o progresso exibido é o do plano ativo (restante), não o do livro inteiro.
- `Plans.Replan` recebe índices absolutos, diferente de `Create`, que recebe índices 0-based (iguais aos absolutos porque o início é 0).
- UI limita minutos a 1–600 e dias a 1–3650; valores inválidos não persistem e redirecionam com `?aviso=ritmo`.

**Testes**
- `internal/plan/generatefrom_test.go`: sufixo com índices absolutos, caminho de seção herdado, alvo pelo tempo restante e validação de cobertura a partir de `start`.
- `internal/store/plans_test.go`: `Replan` preserva chunks enviados, versiona o plano e atualiza o ritmo; início inválido é recusado.
- `internal/ingest/replan_test.go`: fluxo feliz, `ErrNoPlan`, `ErrNothingPending`, `ErrInvalidPacing` e estados não elegíveis.
- `internal/web/replan_test.go`: formulário, `303 ?acao=regenerado`, avisos de ritmo/ausência de trechos e rota sem sessão.

**Documentação**
- [`segmentation.md`](./segmentation.md#regeneração-a-partir-do-cursor-rf-024) — algoritmo do sufixo e alvo restante; [`data-model.md`](./data-model.md) — transação de `Replan`; [`web-ui.md`](./web-ui.md) — rota e formulário; [`testing.md`](./testing.md) — checklist; [`roadmap.md`](./roadmap.md) — 1.17 e próximo passo; [`context.md`](./context.md) — estado.

## 2026-10-29 — backup pré-migração (RF-092)

**Adicionado**
- `store.OpenWithOptions(ctx, path, store.Options{BackupDir, OnBackup})`: quando há migrações pendentes num banco existente (arquivo não vazio), grava `VACUUM INTO {BackupDir}/pre-migrate-{versão-alvo}.db` **antes** de criar/aplicar qualquer migração. `OnBackup` recebe o caminho final para log. `store.Open` continua equivalente a `OpenWithOptions` sem backup.
- `store.Store.Backup(ctx, dest)`: cria o diretório pai, grava em `dest + ".tmp"` e renomeia para o nome final. Backup existente é tratado como íntegro e preservado (não sobrescreve). Destino vazio ou banco fechado devolvem erro.
- `platform.Config.BackupDir()` (`{APP_DATA_DIR}/backup`); `cmd/app` abre o store com backup e loga “backup pré-migração gravado”.

**Decisões**
- O backup precede até o `CREATE TABLE schema_migrations`; banco novo não gera arquivo e falha no backup aborta o start sem tocar no schema.
- O nome usa a **versão-alvo** (maior migração pendente), seguindo `pre-migrate-{versão}.db` de `data-model.md`. Repetir o mesmo upgrade reaproveita o backup da tentativa anterior.
- `VACUUM INTO` com parâmetro vinculado, seguro com WAL (ADR-0007). O temporário + rename evita que um backup interrompido apareça como válido.

**Testes**
- `internal/store/backup_test.go`: cópia de dados/configurações, preservação de destino existente, destino vazio, backup antes de migração sintética (estado anterior sem a tabela nova e `OnBackup` chamado) e banco novo sem diretório de backup.

**Documentação**
- [`data-model.md`](./data-model.md#migrações) — fluxo de `OpenWithOptions`/`Backup` e nome do arquivo; [`operations.md`](./operations.md#upgrade) — backup automático no upgrade.
- [`testing.md`](./testing.md#store) — casos de backup; [`roadmap.md`](./roadmap.md) — 1.16 e FASE-4 parcial; [`context.md`](./context.md) — estado e próximo passo.

## 2026-10-29 — padrões de plano em `/settings` (1.15)

**Adicionado**
- `internal/planconfig`: `Config`/`Default`/`Load`/`Parse` para `plan.wpm` (50–1000), `plan.difficulty` (0,5–3,0), `plan.min_fill` (0,30–0,95), `plan.max_overshoot` (1,00–3,00) e `plan.recap` (bool). Valores ausentes/vazios caem no padrão; inválidos devolvem erro citando a chave; `NaN`/`Inf` são recusados.
- `/settings` ganhou a seção “Plano”. O formulário é validado inteiro por `schedule.Parse` (que inclui `planconfig.Parse`) e gravado em transação; valor inválido não persiste.
- A importação aplica `plan.wpm`/`plan.difficulty` ao documento novo; a extração lê `plan.min_fill`/`plan.max_overshoot`; `plan.recap=false` desliga o “Onde paramos” na composição (`Options.RecapDisabled`).
- Testes de `planconfig` (padrões, faixas, limites, `Load`), da UI e da importação.

**Decisões**
- Padrões globais valem só para documentos novos (WPM/dificuldade) ou na próxima extração/reprocessamento (min/max); documentos existentes mantêm `params_json` e metadados.
- O pacing (minutos/dias) continua por documento; os padrões não criam novo campo nem migração.

**Documentação**
- [`segmentation.md`](./segmentation.md#parâmetros), [`reading-time.md`](./reading-time.md), [`continuity.md`](./continuity.md), [`delivery-ntfy.md`](./delivery-ntfy.md), [`web-ui.md`](./web-ui.md#configurações-settings), [`roadmap.md`](./roadmap.md) e [`context.md`](./context.md).
## 2026-10-29 — validação da imagem ARM64 e correção do builder

**Adicionado**
- Validação do build cross `docker buildx build --platform linux/arm64 --load .` num host amd64 com Docker 29 + BuildKit e QEMU/binfmt. A imagem `readingdrip:arm64` resultou em `arm64 linux`, **23.965.482 bytes (~22,9 MiB)**.
- Smoke test do container sob emulação: `healthy` com `read_only: true` e `tmpfs /tmp`; `/healthz` e `/readyz` responderam `200`; upload de `.md` chegou a `ready` com 6 blocos/1 chunk/plano v1; original preservado em `/data/documents/1/source.md` com owner `65532:65532`.
- Seção “Validação da imagem (2026-10-29)” em [`operations.md`](./operations.md#validação-da-imagem-2026-10-29) e nota de que o `compose.yaml` exige Compose v2.

**Corrigido**
- `/Dockerfile`: o estágio de build passou de `golang:1.27-bookworm` para `golang:1.27-trixie`. Os objetos pré-compilados do `go-fitz` referenciam `__isoc23_strtol`/`__isoc23_strtoul`, ausentes na glibc 2.36 do bookworm; o link cross falhava com `undefined reference`. O runtime distroless static não muda.
- Goldens `pdf_coluna-unica.plan.txt` e `pdf_duas-colunas.plan.txt` regenerados com as linhas `# aviso:` do incremento 1.14, que estavam faltando e quebravam `go test ./... -race` (`TestPDFGolden`).

**Documentação**
- [`operations.md`](./operations.md) — resultado real do build/smoke test, requisito de binfmt e de Compose v2.
- [`roadmap.md`](./roadmap.md) — item “Dockerfile + imagem arm64” concluído; deploy com Cloudflare segue pendente.
- [`context.md`](./context.md) — estado e próximo passo.

## 2026-10-28 — FASE-2: avisos de pacing inviável

**Adicionado**
- `plan.Params.Warnings(targetSeconds, chunks)`: função pura que deriva avisos de sanidade no modo `finish_by_days` — alvo abaixo de 180 s (“leitura fragmentada”), acima de 3600 s (“mais de 1 h”) e dias de leitura maiores que o número de chunks (“sobram dias sem conteúdo”). `minutes_per_day` nunca gera aviso, e o plano continua válido em todos os casos (RF-026).
- `plan.Format` imprime cada aviso como `# aviso: …`, então o CLI `readingdrip plan` mostra o alerta junto do plano.
- Detalhe (`GET /documents/{id}`) e plano completo (`GET /documents/{id}/plan`) exibem o bloco “Ritmo inviável” com os avisos; `avisosDoPlano` deriva do plano ativo (`Params.Warnings(TargetSeconds, len(Chunks))`), sem novo campo no banco.
- Golden `headings-days` ganhou a linha de aviso; o plano em si não mudou.

**Decisões**
- Os avisos são derivados, não persistidos: o plano e `params_json` continuam os mesmos, e a UI sempre recalcula a partir do alvo e dos chunks.
- Limites exatos de `segmentation.md`: 180 s e 3600 s não avisam; singular/plural é tratado na mensagem.
- O aviso não muda status, não bloqueia entrega e não impede o plano de ser gerado.

**Documentação**
- [`segmentation.md`](./segmentation.md#modos-de-ritmo) — `Params.Warnings` e limites.
- [`web-ui.md`](./web-ui.md) — avisos no detalhe e no plano.
- [`testing.md`](./testing.md#avisos-de-pacing) — casos de limites, plural e UI.
- [`roadmap.md`](./roadmap.md) — item da FASE-2 concluído; [`context.md`](./context.md) — estado atual.

## 2026-10-28 — operações: imagem ARM64, healthcheck e deploy de referência

**Adicionado**
- `/Dockerfile` multi-stage seguindo o [ADR-0008](./adr/0008-build-arm64-distroless.md): `golang:1.27-bookworm` com `gcc-aarch64-linux-gnu` para cross ARM64 (ou `NATIVE=1` em runner ARM64), `templ generate`, `CGO_ENABLED=1` com link estático (`netgo,osusergo`) e runtime `distroless/static-debian12:nonroot`. Argumentos `GO_VERSION`, `NATIVE` e `VERSION`.
- `healthcheck` como subcomando de `cmd/app` (`/readingdrip healthcheck`): consulta `GET /readyz` em `127.0.0.1:${APP_ADDR}`, com timeout de 2,5 s, e aceita só `200`. Necessário porque a imagem distroless não tem shell, `curl` nem `wget`.
- `import _ "time/tzdata"` em `cmd/app`, para o fuso da agenda não depender de tzdata do runtime.
- `/compose.yaml` com `app`, `cloudflared` e `ntfy`: app somente leitura, `tmpfs` em `/tmp`, volume `./data`, `127.0.0.1:8080`, limites de memória/CPU, `depends_on` por healthcheck e `ntfy` lendo `/deploy/ntfy/server.yml`.
- `/.env.example` com `APP_BASE_URL`, `APP_PASSWORD`, `APP_SECRET`, `CF_TUNNEL_TOKEN` e opcionais `RD_IMAGE`/`RD_VERSION`/`TZ`; `.env` e o cache do ntfy entram no `.gitignore`.
- `/deploy/ntfy/server.yml` com `message-size-limit: 64k`, cache de 48 h e `behind-proxy`.
- `/.dockerignore`, limitando o contexto ao necessário para compilar.

**Decisões**
- O healthcheck é o próprio binário (ADR-0008), não um sidecar ou shell adicionado à imagem.
- O `compose.yaml` usa `build:` por padrão e aceita `RD_IMAGE` para deploy de imagem publicada; o app não fica exposto fora de `127.0.0.1`.
- O `cloudflared` espera o app ficar `healthy`, reduzindo 502 no primeiro start.
- `templ generate` roda no build para impedir imagem com view desatualizada, mesmo com `_templ.go` versionados.

**Validação pendente**
- O daemon do Docker não está disponível nesta máquina, então o build da imagem e o teste `healthy` em ARM64 não foram executados. O cross-compile Go sem CGO (`CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./...`) passa, mas não cobre o link CGO/MuPDF. Passo a passo de verificação em [`operations.md`](./operations.md#imagem-dockerfile).

**Documentação**
- [`operations.md`](./operations.md) — arquivos reais, comandos de build/compose, healthcheck, ntfy e bypass de `/static/*` no Access.
- [`testing.md`](./testing.md) — casos de `healthcheckURL` e `runHealthcheck`.
- [`roadmap.md`](./roadmap.md) — incremento 1.13 e estado da FASE-1; [`context.md`](./context.md) — próximo passo.

## 2026-10-28 — FASE-1.12: ações de gestão e histórico

**Adicionado**
- `POST /documents/{id}/pause` e `POST /documents/{id}/resume`: pausam/retomam a entrega sem tocar em plano, chunks ou cursor. A transição acontece no banco com `Documents.TransitionStatus` (origem e destino validados no `UPDATE`), então um tick concorrente não pausa um documento já retomado. Pausar um documento já pausado e retomar um `ready` são no-ops idempotentes; outros estados respondem `?aviso=indisponivel`.
- `POST /documents/{id}/reprocess`: reextrai `ready`, `failed` ou `needs_ocr` sem histórico de entrega. Recusa `processing` (`?aviso=processando`), `paused` (`?aviso=pausado`) e `completed`, e devolve `?aviso=historico` quando `Plans.HasDeliveryHistory` é verdadeiro. A transição para `processing` é condicional; se `Queue.Enqueue` falhar, o status anterior é restaurado (`UpdateStatus`) para não deixar o documento preso sem worker.
- Exclusão em duas etapas: `GET /documents/{id}/delete` mostra a confirmação e `POST /documents/{id}/delete` remove o documento; `DELETE /documents/{id}` atende a API e compartilha o mesmo handler. O store apaga derivados em cascata e `library.Importer.RemoveOriginal` remove o diretório do original; falha ao apagar o diretório só gera log, pois o registro já saiu.
- `GET /history`: últimas 30 runs (`historicoLimite`) com filtro opcional `?documento={id}` (id inválido é ignorado; documento excluído aparece como “Documento removido”). Cada run mostra itens, tentativas, bytes, erros e latência; somas e contadores (enviados/falhas/pulados/pendentes) refletem o filtro ativo. A agregação é feita na camada web, só com leituras de `delivery_runs`/`delivery_items`.
- `internal/store`: `Documents.TransitionStatus(ctx, id, de, para, at) (bool, error)` e `Runs.List(ctx, limit, documentID)` (limite padrão 30, teto 200, ordenação por `run_date`/`id` decrescente e filtro por documento via `EXISTS` em `delivery_items`).
- `internal/library`: `Importer.RemoveOriginal(id)`, confinado a `{APP_DATA_DIR}/documents/{id}`.
- UI: botões pausar/retomar, reprocessar e excluir no detalhe, aviso de documento pausado, página de confirmação de exclusão, link “Histórico” no cabeçalho e aviso `?excluido=1` na biblioteca. Tudo sem JavaScript, compatível com a CSP `script-src 'self'`.
- Cada ação grava um evento `kind='system'` com o id do documento; falha ao gravar o evento não desfaz a ação.

**Decisões**
- A transição de status é condicional no banco (não em memória): o handler não corre o risco de sobrescrever uma mudança feita pelo agendador entre a leitura e a escrita.
- Reprocessar continua proibido com histórico de entrega: os ids de bloco antigos não podem ser reaproveitados (`I-01`, ver `data-model.md#reprocessamento`).
- A exclusão é irreversível e sempre confirmada; o `DELETE` direto existe para API/automação.
- O histórico é somente leitura e não guarda corpo de leitura nem token.

**Documentação**
- [`web-ui.md`](./web-ui.md#ações-de-gestão-documentsid) — rotas, avisos, confirmação de exclusão e página de histórico.
- [`data-model.md`](./data-model.md#gestão-de-documentos-2026-10-28) — `TransitionStatus`, `Runs.List` e remoção do diretório do original.
- [`testing.md`](./testing.md) — casos de HTTP, store e importação.
- [`roadmap.md`](./roadmap.md) — 1.12 concluído, FASE-2 em andamento e próximo passo revisado; [`context.md`](./context.md) — estado atual.

## 2026-10-27 — FASE-1.11: leitor por link assinado e “marcar lido”

**Adicionado**
- `internal/links`: `Signer`/`New` com tokens stateless `{exp}.{hex HMAC-SHA256}` sobre `{escopo}:{chunkID}:{exp}`, chave `HMAC-SHA256(APP_SECRET, "readingdrip/link/v1\x00")`. Escopos `r` (leitor) e `done` (ação), validades padrão de 180 e 30 dias, `ReadURL`/`DoneURL`/`Verify`, normalização de `APP_BASE_URL` e erro único `ErrTokenInvalido` (sem distinguir expirado de adulterado), comparado em tempo constante.
- `GET /r/{chunkID}?t=`: leitor público com breadcrumb, progresso, corpo do trecho, navegação anterior/próximo (cada uma com token próprio), “ver seção completa” em `<details>` quando a seção excede o trecho e “marcar como lido”. `401` para token inválido/expirado, `404` para chunk/documento inexistente e `503` sem `Server.Links`.
- `POST /api/actions/chunks/{id}/done?t=`: marca o chunk como lido de forma idempotente; `204` para o ntfy e `303 /r/{id}?t=…&lido=1` para o formulário (`ui=1` ou `Accept: text/html`).
- `internal/store`: `Blocks.Section(ctx, documentID, heading)` devolve do heading ancestral até o próximo heading de nível ≤, incluindo subseções; `HeadingID < 0` devolve a abertura.
- `internal/schedule`: chaves `reader.link_ttl_days` (padrão 180) e `reader.done_ttl_days` (padrão 30), validadas em 1..3650 por `schedule.Parse`, expostas em `Settings.Reader`/`Settings.LinkTTLs()`. `Runner.Links` passa a receber `(document, chunk, readerTTL, doneTTL)` e a preencher `click` e as ações das mensagens.
- `cmd/app` cria o `links.Signer` com `APP_SECRET`/`APP_BASE_URL` e o injeta no agendador e no servidor web.
- CSS do leitor (`.leitura`, `.leitura-corpo`, `.secao-completa`, `.leitor-nav`, `.leitor-acoes`), legível a partir de 360 px.

**Decisões**
- O token é a credencial do celular: `/r/*` e `/api/actions/*` não usam sessão (I-08, ADR-0010).
- O escopo separa leitura de ação; o mesmo token não serve para os dois e não revela qual parte falhou.
- O redirect do formulário é relativo (`/r/...`) para não desviar para o domínio público quando `APP_BASE_URL` é de produção.
- A página do leitor não loga corpo nem token; o log do “marcar lido” traz só `chunk` e `documento`.
- O envio de teste continua sem link: não há chunk associado nem necessidade do assinador.

**Documentação**
- [`web-ui.md`](./web-ui.md) — telas implementadas, contratos de `/r/{id}` e `POST /done`, TTL do leitor ainda sem campo na UI.
- [`delivery-ntfy.md`](./delivery-ntfy.md#links-assinados) — formato dos tokens, escopos, validades e `Runner.Links`.
- [`scheduling.md`](./scheduling.md) — chaves `reader.*`, `Settings.LinkTTLs()` e passo de montagem dos links na run.
- [`testing.md`](./testing.md) — casos do leitor, ação, `Blocks.Section` e `internal/links`; [`roadmap.md`](./roadmap.md) e [`context.md`](./context.md) — 1.11 concluído.

## 2026-10-27 — FASE-1.10: configurações, envio de teste e “enviar agora”

**Adicionado**
- `GET/POST /settings`: formulário de agenda (`enabled`, dias, horário, fuso, catch-up, documentos por run) e ntfy (URL, tópico, token, prioridade, modo de corpo, limite). O token nunca é renderizado; em branco mantém o salvo e “Remover o token salvo” grava vazio.
- Validação do formulário inteiro com `schedule.Parse` (extraído de `LoadSettings`) e gravação em transação com `Settings.SetMany`: valor inválido não é persistido e o formulário volta com o erro (`422`).
- `POST /settings/test`: envio de teste no ntfy com a configuração salva, uma tentativa e `context` de 20 s; a página volta com HTTP, latência e bytes, ou o motivo da falha.
- `POST /documents/{id}/send-now` e botão “Enviar agora” no detalhe: envio manual síncrono do próximo trecho, uma tentativa e avisos por query string (`enviado`, `sem-trechos`, `ja-enviado`, `ntfy`, `ocupado`, `indisponivel`, `tempo`, `falha`).
- `Runner.SendNow`/`SendTest` e `publisherOnce` (cliente de uma tentativa); `Runner.eventoKind` e o evento `delivery.manual`.
- `internal/store`: `Settings.SetMany`, `Runs.ItemByDocument` e `Runs.Delete` (só run `running`, cascata nos itens).

**Decisões**
- O envio manual usa a run do dia (DB-04): se ela já fechou, nada é reenviado; run `running` de um tick é reaproveitada sem ser fechada; falha antes de qualquer envio descarta a run criada pelo clique para não consumir a run programada.
- Envio manual e de teste compartilham o `runMu` com os ticks (`TryLock`): durante uma entrega, o clique responde “entrega em andamento” em vez de esperar retries.
- Feedback por POST/Redirect/GET com códigos fixos na query string.

**Documentação**
- [`web-ui.md`](./web-ui.md): rotas, telas, contratos de `/settings`, `/settings/test` e `/send-now`.
- [`scheduling.md`](./scheduling.md): regras do envio manual e do teste; [`delivery-ntfy.md`](./delivery-ntfy.md): teste e exceções de retry.
- [`data-model.md`](./data-model.md): novos métodos de repositório e a run do envio manual; [`testing.md`](./testing.md): casos de HTTP, store e agendamento.

## 2026-10-27 — execução local com porta dedicada

**Adicionado**
- `run.sh`: compila `./cmd/app` com `CGO_ENABLED=1` e sobe o app na porta **8473** (ou `PORT`/primeiro argumento), com dados em `./data`.
- Na primeira execução, gera `APP_PASSWORD` e `APP_SECRET` em `.env.local` (chmod 600); variáveis de ambiente têm prioridade e não são persistidas.
- Verificação da porta antes de compilar, com mensagem para usar `PORT=...`.
- `.gitignore` para `.env.local`, `./data` e `./bin`.

**Documentação**
- [`operations.md`](./operations.md) ganhou a seção “Execução local rápida”.

## 2026-10-27 — FASE-1.9: agendador, runs idempotentes e integração com entrega

**Adicionado**
- `internal/schedule`: `LoadSettings` lê `schedule.*` (`enabled`, `days`, `time`, `timezone`, `catchup_days`, `max_docs_per_run`) e `ntfy.*` (`base_url`, `topic`, `token`, `priority`, `body_mode`, `max_message_bytes`) do banco, com padrões e erro citando a chave inválida.
- `Runner.Tick` cria a run do dia somente em dia de leitura e depois do horário, faz catch-up da run perdida mais recente dentro da janela em **dias de leitura** (uma vez por janela) e retoma uma run `running` antes de criar run nova.
- `Runner.Start`/`Stop`: primeiro tick imediato, ticker de 60 s, `TryLock` para não sobrepor ticks, `Stop` cancela e espera; `cmd/app` liga e desliga o agendador.
- `internal/store/runs.go`: `Create` idempotente por `run_date`, `GetByDate`, `Running`, `AddItem`, `ListItems`, `MarkItemSent` (item + chunk na mesma transação, exige chunk `pending`), `MarkItemFailed`, `MarkItemSkipped` e `FinishRun`.
- `Plans.Chunk`, `Plans.PreviousChunk`, `Plans.NextChunk` e `Blocks.Between` para montar corpo, recap e rodapé das doses.
- Testes do store de runs e de `internal/schedule` (settings, horário, dias, catch-up, agrupamento, pausa, limite de documentos, falha 401, retomada, chunk já enviado, start/stop).

**Comportamento**
- Sem `ntfy.base_url`/`topic`, o tick não cria run e devolve `Skipped: "ntfy não configurado"`.
- Falha de publicação marca o item `failed` com `attempts` e `bytes`, mas mantém o chunk `pending`: ele entra na próxima run.
- Contexto cancelado durante o envio deixa a run `running`; no próximo start só itens `pending` são reenviados.
- Run sem documento elegível fecha `sent`; run com falhas fecha `partial`/`failed` e registra evento `delivery`.
- Ao fechar a run por erro (não por cancelamento), itens ainda `pending` viram `failed` com o motivo; o chunk continua `pending` para a próxima run.
- Mensagem agrupada grava o mesmo `bytes` em todos os itens que ela levou.
- Links do leitor seguem desligados (`Runner.Links == nil`): as mensagens saem sem `click`/ações até o incremento do leitor.

**Pendente**
- UI de configurações (agenda/ntfy), envio de teste, “enviar agora”, leitor com token HMAC + “marcar lido”, ações de gestão, histórico na UI, backup pré-migração e Dockerfile/ARM64.

## 2026-10-27 — FASE-1.8: composição da mensagem e cliente ntfy

**Adicionado**
- `internal/delivery/compose.go`: `Compose(doses, opts)` puro e determinístico, sem I/O nem relógio.
  - cabeçalho `**{documento}** · {autor}`, breadcrumb `capítulo › seção`, métricas (`1.842 palavras · ~9 min · 21% do livro`);
  - recap extrativo das duas últimas frases de prosa, limitado a 240 caracteres;
  - heading repetido em `starts_mid_section`; rodapé `⏭️ Amanhã` com prévia de até 140 caracteres ou `✓ Fim da seção`;
  - `body_mode` `full`/`preview` com override por dose; preview padrão de 400 palavras;
  - agrupamento em seções `## 📖 {documento} · {autor}` com separador `---`, título `📚 Leitura de hoje · N doses`;
  - acima de `max_message_bytes`, uma mensagem por documento; chunk único é cortado em 60% do limite, sem quebrar rune UTF-8, com `[…continua no leitor](link)`;
  - guardas: dose sem título, modo inválido, limite abaixo de 1.000 bytes e link que não cabe no limite.
- `internal/delivery/ntfy.go`: `NewClient`/`Publish` com payload JSON (`topic`, `title`, `message`, `markdown: true`, `priority`, `tags`, `click`, `actions`), Bearer opcional e `Content-Type: application/json`.
  - 3 tentativas no total; backoff padrão de 30 s e 2 min entre elas (lista configurável até 10 min);
  - retry em erro de rede, timeout, 5xx e 429; `Retry-After` (segundos ou data HTTP) substitui o backoff;
  - 4xx (exceto 429) falha imediata; corpo acima de `MaxMessageBytes` devolve `ErrMessageTooLarge` sem request;
  - `StatusError` expõe o código e se o erro é transitório; token e corpo nunca entram na mensagem de erro.
- Testes de composição (mensagem única, agrupamento, preview, split, truncamento, UTF-8, recap ≤ 240 caracteres, overrides e erros) e do cliente (payload, auth, 5xx, 4xx, 429/`Retry-After`, falha final, contexto cancelado, limite de tamanho e configuração inválida).

**Comportamento**
- Mensagem agrupada só usa `## 📖` quando há mais de um documento; com um documento, mantém o cabeçalho em negrito de [`continuity.md`](./continuity.md).
- Limites de truncamento e de recap contam caracteres; o limite de envio conta bytes UTF-8.
- `Compose` não publica nem grava estado: a integração com runs e `settings` fica para o próximo incremento.

**Pendente**
- Integração ao scheduler e às configurações, runs idempotentes por `run_date`, envio de teste, “enviar agora”, ações de gestão, leitor com token, backup pré-migração e Dockerfile/ARM64.

## 2026-10-27 — FASE-1.7: entrypoint HTTP e recuperação no start

**Adicionado**
- `cmd/app`: monta o servidor com config por ambiente, store SQLite, `auth.Authenticator`, `library.Importer`, `ingest.Queue` e `internal/web.Server`.
- `internal/platform`: `LoadEnv` (`APP_ADDR`, `APP_DATA_DIR`, `APP_BASE_URL`, `APP_PASSWORD`, `APP_SECRET`, `BEHIND_CLOUDFLARE`, `MAX_UPLOAD_BYTES`, `LOG_LEVEL`), `Config.DBPath()` e logger em texto.
- No start: `CleanupTemp` remove uploads interrompidos e documentos em `processing` são reenfileirados (recuperação após crash/restart).
- No shutdown: `http.Server.Shutdown` com timeout de 15 s, drenagem da fila e fechamento do banco; `SIGINT`/`SIGTERM` disparam o encerramento.
- Timeouts HTTP (`ReadHeaderTimeout` 10 s, leitura/escrita de 10 min para uploads grandes, `IdleTimeout` 120 s).
- Testes de `LoadEnv`/`ParseLogLevel` e de integração do `montar` (health, login, raiz sem sessão, recuperação de `processing` até `ready` com plano e upload autenticado percorrendo importação → fila → detalhe).

**Comportamento**
- Valores inválidos de ambiente falham no start com mensagem clara, em vez de usar o padrão silenciosamente.
- O worker usa contexto próprio (não o do sinal) para que a fila drene no shutdown; o que não concluir fica `processing` e é reenfileirado no próximo start.
- Logs de extração incluem id, blocos e chunks; nunca o conteúdo do documento.

**Pendente**
- Composição da mensagem + cliente ntfy, ações de gestão, configurações, histórico, leitor com token, backup pré-migração e Dockerfile/ARM64.

## 2026-10-27 — FASE-1.6: biblioteca, detalhe, preview do plano e assets

**Adicionado**
- Telas em templ: `GET /` (biblioteca), `GET /documents/{id}` (detalhe + prévia do plano), `GET /documents/{id}/plan` (preview somente leitura) e página de erro (`404`/`500`).
- `GET /documents/{id}/download`: entrega o original como anexo, apenas se `SourcePath` estiver sob `{APP_DATA_DIR}/documents`; caso contrário, `404`.
- `internal/web/views`: `layout.templ`, `login.templ`, `biblioteca.templ`, `detalhe.templ`, `plano.templ`, `erro.templ`, tipos e helpers de formatação.
- `internal/web/assets/app.css` e `app.js` embutidos com `go:embed` e servidos em `/static/` (público, `Cache-Control: public, max-age=3600`).
- Polling de status no detalhe: JS externo consulta a própria página a cada 1,5 s enquanto `processing`, recarrega ao mudar e desiste após ~3 min; sem JS, a página continua funcionando.
- Avisos de importação (`?importados=N&duplicados=M`) e de duplicado (`?duplicado=1`).
- ADR-0014: UI da FASE-1 com CSS/JS próprios; Alpine/Tailwind adiados.
- Testes de UI: biblioteca com/sem sessão, lista, detalhe processing/ready, duplicado, 404, preview do plano, download (sucesso, ausente e fora do diretório) e assets.

**Comportamento**
- Login migrado de `html/template` para templ; o formulário mantém `next` seguro, CSP e `no-store`.
- As rotas de gestão continuam exigindo sessão; `/static/` é público e não usa sessão.
- A UI é somente leitura: pausar, retomar, excluir, reprocessar, enviar agora e editar o plano continuam fora desta entrega.
- `templ` passa a ser dependência direta em `v0.3.1070` (exige Go 1.26+); corrigida a versão inexistente `v0.3.900` na documentação.

**Pendente**
- Composição da mensagem + cliente ntfy, ações de gestão, configurações, histórico, leitor com token e recuperação de `processing` no start.

## 2026-10-27 — FASE-1.5: autenticação de sessão e login

**Adicionado**
- `internal/auth`: senha única (`APP_PASSWORD`) conferida com Argon2id (`m=19456 KiB`, `t=2`, `p=1`, salt de 16 bytes) e comparação em tempo constante.
- Cookie `rd_session` stateless no formato `v1.{exp}.{HMAC-SHA256}`, com chave derivada de `APP_SECRET` + `APP_PASSWORD`, validade de 7 dias, `HttpOnly`, `SameSite=Lax`, `Path=/` e `Secure` quando a request é HTTPS (TLS ou `X-Forwarded-Proto`).
- Rate limit em memória: 5 falhas/min por IP, `CF-Connecting-IP` quando `BEHIND_CLOUDFLARE=true`, `429` com `Retry-After`; login bem-sucedido zera as falhas.
- `GET /login`, `POST /login` e `POST /logout`; página mínima em `html/template` (sem JS), com CSP, `Referrer-Policy: same-origin` e `no-store`.
- Middleware de sessão: rota de gestão sem cookie válido → `303 /login` (RF-074), preservando `?next=` só em GET/HEAD e só para caminho local.
- ADR-0013 e dependência `golang.org/x/crypto/argon2`.

**Comportamento**
- Senha errada → `401` sem cookie; 5 falhas no minuto → `429` antes de conferir a senha.
- `next` com esquema, host, `//host`, barra invertida ou caminho percent-encoded que decodifica para `//host` vira `/`.
- Trocar `APP_SECRET` ou `APP_PASSWORD` invalida todas as sessões; não há tabela de sessões.
- Sem autenticador configurado, as rotas de gestão continuam falhando fechadas com `401`; `Sessions` presente é o caminho de produção.

**Pendente**
- Telas de biblioteca/detalhe em templ, token de leitor, entrega ntfy, recuperação de `processing` no start e `Replan` a partir do cursor.

## 2026-10-27 — FASE-1.4: rota de upload e endpoints de saúde

**Adicionado**
- `internal/web.Server.Handler()` com mux nativo: `POST /documents`, `GET /healthz` e `GET /readyz`.
- Upload multipart em streaming, um ou vários arquivos, reutilizando `library.Importer`; resposta `303` para o documento (um arquivo), para o documento existente com `?duplicado=1` (RF-007) ou para `/?importados=N&duplicados=M` (lote).
- `Middleware` de sessão injetável: sem middleware, as rotas de gestão respondem `401` (fail closed); health/ready continuam públicas.
- Cabeçalhos `Cache-Control: no-store`, `X-Robots-Tag: noindex, nofollow` e `X-Content-Type-Options: nosniff` em todas as respostas.
- Limite de corpo = limite do importador + 1 MiB (ou `Server.MaxBodyBytes`), com o limite exato do arquivo aplicado pelo importador.
- Testes HTTP com `httptest`: upload único, lote, duplicata, lote misto, formato/vazio, 413 do importador e do corpo, sem arquivo, sem sessão, não multipart, 405, health/ready.

**Comportamento**
- `413` para arquivo/corpo acima do limite; `422` para formato inválido, vazio ou formulário sem arquivo; `400` para corpo não multipart; `500` genérico sem vazar detalhe interno.
- O upload responde sem esperar extração: o documento é criado em `processing` e a fila assume.
- Nenhum corpo de leitura é logado; erros internos são registrados com status e causa técnica.

**Pendente**
- Login/sessão (Argon2id + cookie assinado), telas em templ, demais rotas de gestão, recuperação de `processing` no start e `Replan` a partir do cursor.

## 2026-10-27 — FASE-1.3: importação de arquivos e fila do worker

**Adicionado**
- `internal/library`: `Importer.Import(ctx, Entrada)` valida extensão (`.md`, `.markdown`, `.epub`, `.pdf`), grava em temporário calculando `sha256` em streaming, aplica limite de 95 MiB (`DefaultMaxBytes`), recusa vazio e duplicata, move o original para `{root}/documents/{id}/source{ext}` e enfileira a extração.
- `DuplicateError` (com o documento existente) e `TooLargeError` (limite e bytes lidos); sentinelas `ErrDuplicate`, `ErrTooLarge`, `ErrUnsupportedFormat` e `ErrEmptyFile`.
- `Importer.CleanupTemp` remove `.upload-*` órfãos de uploads interrompidos.
- `ingest.Queue`: worker em goroutine própria, uma extração por vez, buffer padrão de 16, `Start`/`Enqueue`/`Close`/`Pending`, drenagem no shutdown, `OnResult` para observação e recuperação de panic por documento.
- `Documents.UpdateSource` e `Documents.Delete` (exclusão transacional de itens de entrega, chunks, planos e blocos; `ErrDocumentNotFound`).
- Testes de importação (sucesso, duplicata, limite, rejeições, nome com caminho, falha de enfileiramento, integração com a fila real), de ciclo de vida/concorrência/erro/panic da fila e de `UpdateSource`/`Delete`.

**Comportamento**
- Falha em qualquer passo após a criação do documento desfaz registro, derivados e diretório do original; não sobra lixo de importações parciais.
- O upload não espera a extração: o documento nasce `processing` e a fila o leva a `ready`/`failed`/`needs_ocr`.
- Dois uploads do mesmo conteúdo: o `UNIQUE(source_sha256)` decide a corrida e o perdedor recebe `DuplicateError` apontando para o vencedor.
- Nome original é reduzido ao nome base (sem caminho) e a extensão no disco é sempre minúscula.

**Pendente**
- Rota `POST /documents` (multipart, sessão, 413) e UI de upload; recuperação no start de documentos presos em `processing`; `Replan` a partir do cursor.

## 2026-10-27 — FASE-1.2: worker de ingestão e planos persistidos

**Adicionado**
- `docs.ExtractFile(ctx, path, fallbackTitle)`: detecção por extensão e despacho para Markdown, EPUB e PDF, devolvendo um `docs.Result` comum com título, autor, idioma e blocos.
- `pdf.ErrScanned`: sentinela para PDF sem camada de texto, comparável com `errors.Is`.
- `Documents.SaveExtraction`: grava título/autor/idioma, totais, status e erro em uma instrução, preservando metadados quando a extração não traz valor.
- `Plans` completo: `Create`, `Active`, `Get`, `Versions`, `NextPending`, `MarkSent`, `MarkSkipped`, `MarkDone`, `Progress`, `HasDeliveryHistory` e `DiscardUndelivered`; validação de cobertura/limites antes de tocar no banco e mapeamento de índice de bloco para id real.
- `Blocks.IDs` para o repositório de planos.
- `internal/ingest`: `Service.Process(ctx, docID)` extrai, normaliza, gera e persiste blocos, plano ativo, totais e status; falhas marcam `failed`/`needs_ocr` e registram evento.
- Testes de extração por arquivo (md, epub, pdf com CGO e contexto cancelado) e do worker (sucesso, falha, arquivo vazio, pausado, reprocessamento, histórico de entrega, `finish_by_days` sem dias e PDF escaneado).

**Comportamento**
- Documento pausado ou concluído não é processado (`ingest.ErrNotProcessable`); id inexistente devolve `ingest.ErrNotFound`.
- PDF escaneado fica `needs_ocr` com mensagem acionável, sem blocos nem plano e evento `warn`.
- Reprocessar reextrai e recria o plano; planos não entregues são descartados. Se houver chunk enviado ou item de run, nada é alterado e o worker devolve `store.ErrDeliveryHistory`.
- A CLI `readingdrip plan` passou a usar `docs.ExtractFile`, eliminando a duplicação do despacho por formato.

**Pendente**
- Fila/worker assíncrono do upload (1 documento por vez), `Replan` a partir do cursor preservando chunks enviados e backup pré-migração.

## 2026-10-27 — FASE-1.1: SQLite, migrações e primeiros repositórios

**Adicionado**
- `internal/store`: `Open(ctx, path)` com PRAGMAs via DSN (`WAL`, `synchronous=NORMAL`, `foreign_keys=ON`, `busy_timeout=5000`), criação do diretório e até 4 conexões.
- `migrations/0001_init.sql` com o schema completo de [`data-model.md`](./data-model.md) (documents, blocks, plans, chunks, delivery_runs, delivery_items, settings, events).
- `migrate.go`: migrações embutidas com `go:embed`, forward-only, uma transação por arquivo, registro em `schema_migrations` e recusa de versão duplicada.
- Repositórios `Documents` (`Create`, `Get`, `BySHA256`, `List`, `UpdateStatus`, `UpdateTotais`), `Blocks` (`Replace`, `List`, `Count`), `Settings` (`Get`, `Set`, `All`) e `Events` (`Add`, `List`).
- Dependência `modernc.org/sqlite` v1.61.0, conforme [ADR-0007](./adr/0007-sqlite-modernc-wal.md).
- Testes de abertura, idempotência das migrações, PRAGMAs, chave estrangeira e invariantes DB-03/DB-07, além de CRUD e mapeamento de `heading_id`.

**Comportamento**
- `Documents.Get`/`BySHA256` devolvem `nil` sem erro quando o registro não existe.
- `Blocks.List` devolve `HeadingID` como índice do bloco (`-1` = sem heading), simétrico ao que a extração entrega.
- O store não chama `time.Now()`: datas são parâmetros do chamador.

**Pendente**
- Backup pré-migração (`VACUUM INTO`) e repositórios de planos/chunks (próximo incremento da FASE-1).

## 2026-10-27 — FASE-0.4: revisão com PDFs reais e fechamento da FASE-0

**Adicionado**
- Revisão manual de três artigos reais do arXiv (Attention, ResNet, GPT-3), registrada em [`extraction.md`](./extraction.md), com páginas, blocos, títulos, tempo e pico de memória.
- `col.go`: `fundirFragmentos` junta pedaços da mesma linha visual emitidos em `<p>` separados (ex.: `3.1` + `Encoder and Decoder Stacks`).
- `col.go`: `ordenarCapa` lê o bloco acima do primeiro parágrafo por `y` (título, autores, resumo) e o restante por colunas; fallback para ordem por `y` quando há 3+ grupos fortes.
- `extract_spans.go`: descarte de linhas nas bordas de 4% da página (marcas d’água e numeração de linha).
- Testes unitários de ordenação de capa e de ranqueamento de títulos.
- `Q-016` aberta: pico de memória de PDF com imagens (`HTML()` embute PNG em base64).

**Corrigido**
- Título do documento não é mais a marca d’água do arXiv: ela cai no descarte de margem e o título vem do corpo.
- Títulos que o MuPDF emite em dois `<p>` (`3.1` + texto) não são mais divididos em título vazio + parágrafo.
- Parágrafos separados por linha em branco não são mais fundidos em um único bloco.
- Níveis de título não são mais poluídos por rótulos de figura com fonte enorme: tamanhos maiores que o título da primeira página são ignorados e tamanhos próximos são agrupados (tolerância de 8%).
- Blocos de aviso/licença com a mesma fonte de um título não viram mais título (fonte pouco maior só conta se a linha for curta ou estiver em negrito).
- Linhas de tabela dominadas por números (`0.3 300K 4.33`) e numeração seguida de minúscula não são mais classificadas como títulos.

**Comportamento**
- FASE-0 concluída: extração Markdown, EPUB e PDF, plano nos dois modos, CLI e revisão manual com três PDFs reais.
- `SPIKE-04` parcial: em x86_64, um PDF de 75 páginas com figuras grandes chega a 325 MB de pico; `fitz.MaxStore=32MB` baixa para ~229 MB e `GOGC` não altera. A medição em ARM64 e com 300 páginas continua pendente.

**Pendente**
- `SPIKE-04` em ARM64 e com PDF de 300 páginas.
- Figuras de PDF (`SPIKE-02`), sumário de PDF e tabelas complexas.

## 2026-10-27 — FASE-0.3: extrator PDF completo e `SPIKE-01`

**Adicionado**
- `internal/docs/pdf`: extração sobre o HTML estruturado do MuPDF, com uma linha visual por `<p>` (`top`, `left`, `line-height`, `font-size`, negrito).
- `extract_spans.go`: parse com `x/net/html` e remoção de cabeçalho/rodapé repetidos (> 40% das páginas) e números de página isolados nas margens.
- `col.go`: detecção de colunas por clustering de `x` (tolerância 12pt, gutter 36pt) e ordenação `(coluna, y)`.
- `heading.go`: títulos por tamanho ≥ 1,15× a mediana do corpo, por negrito + numeração/linha curta, e nivelamento pela ordem dos tamanhos distintos.
- `blocks.go`: junção de linhas em parágrafos, desfazer hifenização, quebra por vão vertical, mudança de coluna/página e recuo; fallback textual quando não há HTML.
- Fixtures determinísticas geradas por `testdata/pdf/generate.go`: `coluna-unica`, `duas-colunas`, `escaneado` e `headings-10pg` (20 títulos em 10 páginas).
- Goldens `testdata/golden/pdf_coluna-unica.plan.txt` e `pdf_duas-colunas.plan.txt`; testes de conteúdo, ordem de leitura, colunas, PDF escaneado e títulos em 10 páginas.
- Spike `cmd/spike-pdf` atualizado para a API `HTML(page, header)` e para amostrar posição/tamanho de fonte.

**Corrigido**
- `go.sum` sem as entradas do `go-fitz` (build CGO falhava).
- Chamada de `doc.HTML` desatualizada e helpers de PDF incompletos que impediam a compilação com CGO.
- Código morto e duplicado em `extract_spans.go`/`col.go`/`heading.go`.

**Comportamento**
- Título do documento vem dos metadados do PDF; se ausente, do primeiro `h1`; senão, do nome do arquivo.
- PDF sem camada de texto (< 50 caracteres/página em média) continua retornando erro `needs_ocr`.
- Sem CGO, o pacote compila pelo stub e o CLI informa que a extração exige `cgo`.
- `SPIKE-01` concluído: o `go-fitz v1.24.14` não expõe spans crus; o HTML estruturado é suficiente. Acurácia de títulos medida em 100% na fixture de 10 páginas.

**Pendente**
- Revisão manual de 3 PDFs reais e benchmark de memória de PDF de 300 páginas em ARM64 (`SPIKE-04`).
- Figuras de PDF (`SPIKE-02`) e tabelas complexas.

## 2026-10-26 — FASE-0.3: preparação para extração PDF

**Adicionado**
- `internal/docs/pdf`: pacote com `extract_cgo.go` (build tag `cgo`) e `extract_stub.go` (build tag `!cgo`), seguindo isolamento CGO via ADR-0003.
- `cmd/spike-pdf`: spike `pdfspike` para investigar API do `go-fitz` (spans, fonte/estilo/posição), memória e comportamento em arm64.
- CLI `plan` agora aceita `.pdf` e roteia para `internal/docs/pdf`. Uso atualizado.
- Dependência `github.com/gen2brain/go-fitz v1.24.14` adicionada ao `go.mod`.
- Diretórios `internal/docs/pdf/spike/`, `testdata/pdf/`, `cmd/spike-pdf/` criados.

**Comportamento**
- Sem CGO, `pdf.Extract` retorna erro claro; com CGO ainda não implementado (aguarda `SPIKE-01`).

**Pendente**
- `SPIKE-01` (fixar versão, testar spans/structured text, memória), extrator PDF (coluna única + duas colunas), golden para PDF, marcos da FASE-0.

## 2026-10-26 — FASE-0.2: extração EPUB

**Adicionado**
- `internal/docs/epub`: leitura de EPUB em memória (`archive/zip` + `x/net/html`), container/OPF/spine, metadados (título, autor, idioma), sanitização de HTML e blocos para headings, parágrafos, listas, código, citações, tabelas, figuras, `hr` e notas.
- CLI `plan` aceita `.epub` além de `.md`.
- Fixture determinística `testdata/epub/livro.epub`, gerador `testdata/epub/generate.go` e golden `testdata/golden/epub.plan.txt`.
- Testes de extração, sanitização, links seguros, spine não linear, fallback de título e erros de ZIP/container/OPF.
- `BenchmarkPlan10kBlocks` (~5 ms e ~4,8 MB por plano de 10 mil blocos no ambiente de desenvolvimento).
- ADR-0012: EPUB com `archive/zip` e `x/net/html`.

**Comportamento**
- Itens `linear="no"` são ignorados quando há itens lineares; itens `properties="nav"` são sempre ignorados.
- Links só sobrevivem com esquemas seguros e recebem `rel="noopener noreferrer"`; `script`, `style`, `iframe`, `svg` e `math` são descartados.
- Imagens preservam apenas `alt`/legenda; cópia de mídia fica para a FASE-1.
- Limites: 2 MiB de metadados, 8 MiB por XHTML, 64 MiB de XHTML por EPUB.

**Ajustado**
- Balanceamento agora move várias unidades entre grupos vizinhos até estabilizar, em vez de uma única unidade.
- `collectUnits` desce nós acima do alvo, não apenas acima do máximo.

**Pendente**
- `SPIKE-01`, extrator PDF, benchmarks.
- Mídia/capa do EPUB, TOC enriquecendo títulos e ligação de footnotes por referência.
- `go test -race` neste ambiente Windows (sem `gcc`); rodar em CI Linux/ARM64.

## 2026-10-26 — FASE-0.1: blocos, plano e extração Markdown

**Adicionado**
- `internal/blocks`: modelo `Block`, hashes SHA-256 de 16 caracteres, normalização NFC, estimador de tempo, divisão de frases/parágrafos, árvore de seções e `SectionPath`.
- `internal/plan`: geração determinística nos modos `minutes_per_day` e `finish_by_days`, empacotamento por unidades, balanceamento, materialização, `Validate` e formatação textual/JSON.
- `internal/docs`: detecção de formato e extrator Markdown com goldmark (GFM + footnotes) para headings, parágrafos, listas, código, tabelas, citações, figuras, `hr` e footnotes.
- `cmd/readingdrip`: CLI `plan` com `--minutes`, `--days`, `--wpm`, `--difficulty`, `--min-fill`, `--max-overshoot`, `--max-para-words` e `--json`; comando `version`.
- Fixtures `md/headings.md`, `md/plain.md`, `md/long-para.md`, `md/oversized.md` e cinco golden plans.
- Testes unitários, de propriedade (300 documentos aleatórios) e golden; `go test ./...` e `CGO_ENABLED=0 go test ./internal/plan/... ./internal/blocks/...` verdes.
- ADR-0011: parágrafos longos são pré-divididos em blocos alinhados a frases.

**Comportamento**
- Parágrafos acima de 300 palavras viram vários blocos `p`; o segmentador nunca corta dentro de um bloco.
- Nós que cabem no alvo são preservados inteiros; nós maiores descem um nível.
- Headings isolados e headings no fim de um nó são transportados para o próximo conteúdo.
- `oversized` marca qualquer chunk acima de `max`, inclusive quando o excesso vem de fechar uma seção.
- `starts_mid_section` e `ends_mid_section` derivam dos blocos vizinhos, não de offsets.

**Corrigido durante a implementação**
- Headings órfãos no fim de um nó/arquivo não podem mais criar chunks terminando em heading no meio do documento.
- Unidades agregadas mantêm métricas coerentes com o intervalo de blocos coberto.

**Pendente**
- Extrator EPUB, `SPIKE-01` e extrator PDF.
- Benchmarks (`BenchmarkPlan10kBlocks`).
- Front matter/autor, `rel="noopener noreferrer"` e ligação de footnotes por referência.
- `go test -race` neste ambiente Windows (sem `gcc`); rodar em CI Linux/ARM64.

## 2026-10-26 — Documentação inicial e decisões

**Adicionado**
- Árvore de documentação em `docs/`, com contexto, requisitos, arquitetura, modelo de dados, segmentação, tempo de leitura, extração, continuidade, entrega ntfy, agendamento, UI, operações, testes, contribuição, roadmap e perguntas em aberto.
- 10 ADRs registrando as decisões estruturantes.

**Decidido**
- Stack Go + templ + Alpine + Tailwind, binário único.
- Particionamento sobre blocos normalizados com âncoras por hash.
- Plano global persistido e determinístico.
- Dois modos de ritmo (`minutes_per_day`, `finish_by_days`).
- Documentos em paralelo, cada um com plano e pacing próprios.
- ntfy self-hosted com corpo completo em Markdown e link para o leitor.
- PDF via `go-fitz` (CGO isolado).
- SQLite `modernc.org/sqlite` em WAL.
- Imagem ARM64 distroless, non-root, read-only.
- Exposição via Cloudflare Tunnel + domínio próprio, com links assinados no leitor.

**Pendente**
- Implementação da `FASE-0` (núcleo headless).
- `SPIKE-01`: confirmar API do `go-fitz` para fonte/estilo.
- `Q-003`: decidir entre Cloudflare Access e auth própria.

## 2026-10-26 — FASE-0.3 (continuação): extrator PDF MVP

**Adicionado**
- `internal/docs/pdf/extract_cgo.go`: extração via `go-fitz` com cópia para temp file, detecção de PDF escaneado (< 50 chars/página), tentativa HTML primeiro com fallback para texto, normalização (NFC, hifenização, espaços), inferência básica de headings por padrão numérico.
- `internal/docs/pdf/pdf_test.go` e `extract_test_stub.go` para testes com/ssem CGO.
- Spike aprimorado em `cmd/spike-pdf` com medição de memória.

**Ajustado**
- `internal/docs/pdf/extract_stub.go` atualizado.
- `docs/extraction.md` com seção PDF FASE-0 detalhada.

**Pendente**
- SPIKE-01 completo (spans com fonte/estilo/posição), detecção de colunas, cabeçalho/rodapé repetidos, fixtures PDF, golden tests, fechar FASE-0.

## 2026-10-26 — FASE-0.3 (continuação 2): detecção de colunas e headings

**Adicionado**
- `internal/docs/pdf/col.go`: detecção de colunas por clustering de posições x, ordenação por coluna+y, utilitários estatísticos.
- `internal/docs/pdf/heading.go`: detecção de headings por tamanho de fonte, negrito e padrões numéricos.
- Função `isRepetido` para detecção de cabeçalho/rodapé repetidos.

**Pendente**
- SPIKE-01 completo, integração com go-fitz spans/fonte, fixtures PDF, golden tests, fechar FASE-0.

## 2026-10-26 — FASE-0.3 (continuação 3): hifenização, junção de linhas e spike aprimorado

**Adicionado**
- `internal/docs/pdf/extract_spans.go`: estrutura para extração com spans
- Funções `desfazerHifenizacao` e `juntarLinhas` seguindo regras de `docs/extraction.md`
- Spike aprimorado para analisar fontes, imagens, links

**Pendente**
- SPIKE-01 completo com PDF real, integração ao fluxo, fixtures, golden tests, fechar FASE-0.

## 2026-10-26 — FASE-0.3 (continuação 4): junção avançada e detecção repetidos

**Adicionado**
- `juntarLinhasComQuebra` e `detectarCabecalhoRodape` seguindo regras de segmentação
- Melhorias na detecção de headings

**Pendente**
- SPIKE-01 completo, fixtures PDF, golden tests, integração final, fechar FASE-0.

## 2026-10-26 — FASE-0.3 (continuação 5): golden tests e integração

**Adicionado**
- `internal/docs/pdf/pdf_golden_test.go` para testes com fixtures PDF
- Atualizações em `docs/testing.md`

**Pendente**
- SPIKE-01 completo com PDF real, criar fixtures, integrar detecção de colunas ao fluxo principal, fechar FASE-0.
