---
title: Testes
status: ativo
updated: 2026-10-30
---

# Testes

O risco do projeto está concentrado no **segmentador** e na **extração**. A estratégia prioriza esses dois com testes determinísticos e fixtures reais; o resto usa testes de integração com dublês.

> **Agentes:** leia só a seção do componente afetado; não carregue o arquivo inteiro.

## Pirâmide

| Nível | Escopo | Ferramenta |
|---|---|---|
| Unitário | blocos, tempo, fronteiras, recap, calendário | `testing` |
| Golden | extração e plano completo | `testdata/golden` |
| Propriedade | invariantes do plano | `testing/quick` ou gerador próprio |
| Integração | store + HTTP + ntfy | `httptest`, SQLite em `t.TempDir()` |
| E2E manual | fluxo completo com ntfy real | checklist por release |

## Comandos

```sh
go test ./...                      # tudo (CGO ligado)
CGO_ENABLED=0 go test ./internal/plan/... ./internal/blocks/...
go test ./internal/plan -run TestGolden -update   # regenera golden de plano
go test ./internal/docs/pdf -run TestPDFGolden -update  # regenera golden de PDF
go test ./... -race
go test ./... -coverprofile=cover.out && go tool cover -func=cover.out
go test ./internal/plan -bench . -benchmem
```

## Golden files

Formato determinístico e legível:

```
testdata/golden/headings.plan.txt
```

```
# doc: testdata/md/headings.md  mode=minutes_per_day  target=120s  min=72s  max=168s
000  [0..14]  2m22s       397w  A arte de ler devagar  start=0 end=0
001  [15..27] 2m18s       370w  A arte de ler devagar › Capítulo 1 — O começo › 1.2 Um exemplo concreto  start=0 end=0
# total: 2 chunks, 4m40s, coverage=ok
```

Regras:
- caminho e parâmetros no cabeçalho;
- uma linha por chunk com `seq`, intervalo, duração, palavras, breadcrumb, flags e `oversized=1` quando aplicável;
- rodapé com totais e validação;
- `-update` só em PR que explique a mudança de comportamento.

Casos obrigatórios:

| Golden | Fixture | Cobre | Estado |
|---|---|---|---|
| `headings` | `md/headings.md` | hierarquia, listas, código, tabela, figura | implementado |
| `plain` | `md/plain.md` | sem headings | implementado |
| `long-para` | `md/long-para.md` | pré-divisão de parágrafo | implementado |
| `oversized` | `md/oversized.md` | código maior que o máximo | implementado |
| `headings-days` | `md/headings.md` | modo `finish_by_days` | implementado |
| `epub` | `epub/livro.epub` | spine, sanitização, listas, código, tabela, figura | implementado |
| `pdf-coluna-unica` | `pdf/coluna-unica.pdf` | headings por fonte, hifenização | implementado |
| `pdf-duas-colunas` | `pdf/duas-colunas.pdf` | ordem de leitura | implementado |
| `parallel` | dois docs | composição da mensagem | planejado |

## Testes de propriedade

Gerar florestas de nós aleatórias (tamanho, profundidade, tipos) e verificar:

| Propriedade | Invariante |
|---|---|
| Cobertura | todo bloco em exatamente um chunk |
| Contiguidade | `first[n+1] = last[n] + 1` |
| Sem heading final | `kind(last) != h*`, exceto heading como último bloco do documento |
| Indivisível | nenhum corte dentro de `code/table/li/quote/figure` |
| Faixa | `dur <= max` exceto `oversized`; chunks curtos são permitidos quando inevitáveis |
| Flags | `starts_mid_section` / `ends_mid_section` coerentes com os blocos vizinhos |
| Hashes | `first_hash`/`last_hash` conferem com os blocos |
| Determinismo | gerar duas vezes → bytes iguais |
| Estabilidade | reordenar mapas internos não muda o resultado |

Semântica: se uma propriedade falhar, o teste imprime o plano completo em formato golden para reprodução.

## EPUB

`internal/docs/epub` cobre:

- container/OPF/`spine`, `linear="no"`, itens `nav` e mídia não XHTML;
- título/autor/idioma e fallback de título;
- headings, parágrafos, listas, código, citação, tabela, figura, `hr` e nota;
- links seguros, `rel="noopener noreferrer"` e remoção de `script`/`style`/`on*`;
- ZIP inválido, container/OPF ausente, spine vazio, zero blocos e `href` fora do pacote.

A fixture `testdata/epub/livro.epub` é determinística e pode ser regenerada com:

```sh
go run testdata/epub/generate.go
```

## PDF

`internal/docs/pdf` cobre, com CGO:

- parse do HTML estruturado do MuPDF (`top`/`left`/`line-height`/`font-size`);
- remoção de cabeçalho/rodapé repetidos, de número de página isolado e das bordas de 4%;
- detecção de colunas e ordem de leitura em duas colunas;
- ordem de capa/folha de rosto e fusão de fragmentos da mesma linha;
- junção de linhas em parágrafos, hifenização e quebra por vão vertical;
- títulos por tamanho de fonte e por negrito + numeração, com ranqueamento que ignora rótulos de figura;
- `needs_ocr` para PDF sem camada de texto, arquivo vazio e PDF inválido;
- fixture de 10 páginas com 20 títulos (critério do `SPIKE-01`, 100% detectados).

Além das fixtures, a revisão de saída da FASE-0 usou três PDFs reais do arXiv (não versionados): resultados e limitações em [`extraction.md`](./extraction.md).

As fixtures PDF são determinísticas e podem ser regeneradas com:

```sh
go run testdata/pdf/generate.go
```

Sem CGO, o pacote compila pelo stub e `TestExtractStub` verifica o erro claro.

## Determinismo

- `plan.Generate` não recebe `time.Time` nem `rand`.
- Proibido iterar `map` para produzir ordem; sempre ordenar.
- Teste `TestDeterminism` roda 100 vezes em processo e compara hashes.

## Avisos de pacing

- `Params.Warnings` só avisa no modo `finish_by_days`; `minutes_per_day` nunca gera aviso.
- Limites exatos: alvo de 180 s e 3600 s não avisam; 179 s e 3601 s avisam.
- Dias maiores que chunks geram “Sobram N dias sem conteúdo”; dias iguais ou menores não geram.
- Singular/plural: “1 dia”, “Sobra 1 dia”, “Sobram 2 dias”.
- `plan.Format` escreve cada aviso como `# aviso: …`; goldens com `finish_by_days` refletem a linha.
- Detalhe e plano completo mostram “Ritmo inviável” para documento `finish_by_days` com 30 dias e texto curto.

## Regeneração a partir do cursor

- `GenerateFrom(bs, params, 2)` começa no bloco 2, termina no último e mantém `seq`/hashes contíguos; `ValidateFrom` aceita a cobertura parcial.
- O alvo de `finish_by_days` usa só o tempo restante (155 s ÷ 5 = 31 s); `minutes_per_day` mantém os minutos escolhidos.
- `section_path` e `starts_mid_section` vêm do documento inteiro (bloco no meio do Capítulo 1 continua com o breadcrumb).
- `GenerateFrom(..., 0)` equivale a `Generate`; início negativo ou igual ao total devolve erro.
- `Plans.Replan` cria a versão 2, preserva os chunks enviados no plano `superseded` e atualiza o ritmo do documento; início inválido não altera plano nem ritmo.
- `ingest.Replan` a partir do segundo chunk, documento pausado, ritmo inválido (`ErrInvalidPacing`), sem plano (`ErrNoPlan`), sem pendentes (`ErrNothingPending`) e id inexistente (`ErrNotFound`).
- HTTP: formulário no detalhe, `303 ?acao=regenerado`, primeiro bloco = cursor, ritmo novo no documento e aviso na página; ritmo inválido → `?aviso=ritmo`, sem pendentes → `?aviso=sem-trechos`; rota sem sessão → `303 /login`.

## Store

- Banco em `t.TempDir()` com migrações reais (não mock).
- Teste de migração: criar schema antigo, aplicar migrações, validar dados.
- Backup: `Store.Backup` copia dados e configurações para o destino, cria diretório pai e não sobrescreve um arquivo existente; destino vazio devolve erro.
- Backup pré-migração: `migrar` com migração sintética grava `pre-migrate-{versão}.db`, chama `OnBackup`, aplica a migração no banco e deixa o backup sem a tabela nova (estado anterior).
- `OpenWithOptions` num banco novo não cria o diretório de backup.
- `PRAGMA integrity_check` ao final de cada teste.
- Concorrência: 10 goroutines criando runs no mesmo dia → exatamente 1 linha.
- Runs: `Create` idempotente por `run_date`, `Running` devolve a mais recente, `AddItem` recusa segunda dose do mesmo documento e `MarkItemSent` grava item+chunk ou faz rollback com `ErrChunkNotPending`.
- Falha e pulo: `MarkItemFailed` mantém o chunk `pending`; `MarkItemSkipped` só muda o item; `FinishRun` recusa status final inválido.
- `ItemByDocument` devolve `nil` sem erro para documento ausente da run; `Delete` remove só run `running` (cascata nos itens) e preserva run fechada.
- `Settings.SetMany` grava o lote em transação: chave vazia aborta tudo, valor vazio é gravado como vazio e lote vazio é no-op.
- `Blocks.Section` devolve do heading ancestral até o próximo heading de nível igual ou maior, incluindo subseções; sem heading anterior, devolve a abertura; índice que não é heading e índice fora do documento devolvem erro.
- `Documents.TransitionStatus` troca o status só a partir de uma origem permitida, atualiza `updated_at` e devolve `false` sem erro quando o status atual não está na lista (corrida com o worker ou ação repetida); lista de origem vazia, origem/destino inválidos ou data zero devolvem erro; id inexistente devolve `false`.
- `Runs.List` ordena por `run_date`/`id` decrescente, aplica o limite padrão 30 e o teto 200, ignora documento negativo e, com `documentID > 0`, devolve só runs com item daquele documento (`EXISTS` em `delivery_items`).

## Ingestão

`internal/ingest` é testado com banco real em `t.TempDir()` e arquivos Markdown temporários; `Service.Now` é injetado (nunca o relógio real).

- Markdown feliz: status `ready`, título extraído, blocos contíguos, totais, plano ativo com chunks e evento `plan`.
- Falhas: arquivo inexistente e Markdown vazio → `failed` + `error` + evento `extract`; nada de plano.
- Documento pausado/concluído → `ingest.ErrNotProcessable` sem alterar status; id inexistente → `ingest.ErrNotFound`.
- Reprocessamento: recria blocos e plano ativo; planos não entregues somem (uma única versão).
- Histórico: após `MarkSent`, reprocessar devolve `store.ErrDeliveryHistory` e preserva plano, chunk enviado e status.
- `finish_by_days` grava `horizon_days`; sem `pacing_days` → `failed`.
- PDF escaneado (build tag `cgo`): `needs_ocr`, mensagem com OCR, sem blocos/plano, evento `warn`.

## Importação e fila

`internal/library` usa banco e diretório de dados em `t.TempDir()`; a fila falsa registra os ids enfileirados.

- Markdown feliz: linha `processing`, `source{ext}` com os bytes originais, `sha256` correto e id na fila.
- Duplicata: segundo upload do mesmo conteúdo devolve `DuplicateError` com o documento existente, sem segunda linha nem arquivo.
- Limite: `MaxBytes` pequeno → `TooLargeError` e nenhum documento/temporário; extensão inválida e arquivo vazio também são recusados.
- Nome `../../x.md` é reduzido ao nome base; o caminho final fica dentro de `{root}/documents`.
- Falha de enfileiramento desfaz documento e diretório.
- `Importer.RemoveOriginal` apaga só `{root}/documents/{id}`, é idempotente com diretório ausente e recusa id inválido.
- Integração com `ingest.Queue` real: documento importado em `processing` vira `ready` com plano ativo.
- Fila: ordem FIFO, no máximo 1 processamento simultâneo, erros/panic não matam o worker, `Enqueue` respeita contexto com buffer cheio e o ciclo Start/Close é idempotente.

## Autenticação (sessão)

`internal/auth` é testado com Argon2id barato (`Memory: 64 KiB`, `Time: 1`) e relógio injetado; os cookies são gerados e validados por `httptest`.

- senha correta aceita; vazia, com caixa diferente ou com espaço extra recusada;
- `APP_PASSWORD` vazia, `APP_SECRET` curto e parâmetros Argon2id inválidos são rejeitados;
- cookie recém-criado válido; expirado, com `v1` trocado, expiração alterada, assinatura alterada ou formato inválido recusado;
- flags `HttpOnly`, `SameSite=Lax`, `Path=/` e `MaxAge` de 7 dias; `Secure` só com TLS ou `X-Forwarded-Proto: https`;
- rate limit: 5 falhas bloqueiam o IP, outro IP não herda, sucesso zera, janela expira, `Retry-After` positivo;
- `ClientIP`: `CF-Connecting-IP` só com `BehindCloudflare`; fallback para `RemoteAddr`;
- `SafeNext`: aceita caminhos locais e rejeita esquema, host, `//host`, barra invertida, controle e caminho percent-encoded que decodifica para `//host`;
- middleware: GET preserva `?next=`, POST não preserva, raiz vai para `/login` e cookie válido passa.

## HTTP

`internal/web` é testado com `httptest.NewServer` sobre o mux real, banco e importador em `t.TempDir()`. O upload usa um middleware pass-through; login/logout usam um `auth.Authenticator` real com Argon2id barato.

Implementado:

- upload de um arquivo → `303 /documents/{id}`, documento em `processing`, original no disco e id na fila;
- lote de 3 → `303 /?importados=3&duplicados=0`;
- mesma carga duas vezes → `303 /documents/{id}?duplicado=1`, sem segundo documento;
- lote com duplicata → contadores corretos;
- `.txt`, arquivo vazio e formulário sem arquivo → `422`;
- arquivo acima do limite do importador e corpo acima de `MaxBodyBytes` → `413` sem sobras;
- corpo não multipart → `400`; `GET /documents` → `405`; sem middleware → `401`;
- `/healthz` e `/readyz` (200 e 503 sem banco);
- cabeçalhos `no-store`, `nosniff`, `Referrer-Policy` e CSP.

Login:

- `GET /login` → `200` com formulário e `next` preservado; com sessão → `303` para o destino;
- senha correta → `303` + cookie; senha errada → `401` sem cookie;
- 5 falhas → `429` com `Retry-After` e sem cookie;
- `next` externo cai para `/`; rota de gestão sem sessão → `303 /login`; cookie adulterado também;
- logout → `303 /login` e cookie expirado; `POST /documents` com sessão chega ao handler (`400` sem multipart);
- corpo de login acima de 64 KiB → `413`; autenticação ausente → `503`.

Telas (templ + CSS/JS embutidos):

- biblioteca sem sessão → `303 /login`; com sessão → `200` com formulário de upload, vazio e lista;
- lista com dois documentos → títulos, badges `ready`/`processing` e avisos `?importados=2&duplicados=1`;
- detalhe em `processing` → `data-status`, `data-status-url`, aviso de extração e link de download, sem plano;
- detalhe `ready` → plano ativo, versão e link “Ver plano completo”;
- `?duplicado=1` → aviso de arquivo já existente;
- `GET /documents/{id}/plan` → tabela do plano e mensagem quando não há plano;
- `GET /documents/9999`, id inválido e id negativo → `404` com página de erro;
- download → `200` com o conteúdo original, `Content-Disposition: attachment` e `Content-Type` coerente;
- original removido ou caminho fora de `{APP_DATA_DIR}/documents` → `404`;
- `/static/app.css` e `/static/app.js` → `200`, tipos corretos e `max-age`; arquivo inexistente → `404`; `app.js` contém o polling de `processing`.

Leitor e ação do ntfy:

- sem `Server.Links` → `503` no leitor e na ação;
- token ausente, vazio, malformado, adulterado, de outro escopo ou expirado → `401` com a página “Este link expirou ou é inválido”, sem vazar o corpo;
- token válido para chunk inexistente → `404`;
- leitor de um trecho → `200` com corpo, breadcrumb, link do documento, progresso, anterior/próximo e formulário `ui=1`; toda URL `/r/` leva token;
- primeiro trecho sem anterior e com próximo;
- “ver seção completa” aparece exatamente quando o trecho começa ou termina no meio da seção;
- POST com token `done` → `204` e `done_at` gravado; repetição → `204` sem perder o estado;
- POST com `ui=1` ou `Accept: text/html` → `303 /r/{id}?t=…&lido=1`, com token de leitura válido; a página volta mostrando a data e sem o botão.

Configurações e envio:

- `/settings`, `POST /settings`, `POST /settings/test` e `POST /documents/{id}/send-now` sem sessão → `303 /login`;
- `GET /settings` nos padrões → formulário completo, dias marcados e placeholder “token salvo”, sem renderizar o token;
- `POST /settings` válido → `303 /settings?salvo=1` e valores efetivos em `schedule.LoadSettings` (dias, horário, catch-up, ntfy);
- valor inválido (`horario=25:00`) → `422`, erro citando `schedule.time`, nenhum valor gravado e formulário preservado;
- token em branco mantém o salvo; “remover o token salvo” grava vazio;
- teste ok → `303 /settings?teste=ok&http=…&ms=…&bytes=…` e resumo na página; erro → `?teste=erro&motivo=…`, incluindo `ErrNtfyNaoConfigurado` e HTTP 401;
- `send-now` ok → `303 /documents/{id}?enviado=1` com aviso no detalhe; cada erro mapeia um `aviso` (`sem-trechos`, `ja-enviado`, `ntfy`, `ocupado`, `indisponivel`, `tempo`, `falha` com motivo);
- id malformado → `404`; sem `Sender` → `503`;
- botão “Enviar agora” só aparece em documento `ready` com plano ativo.

Ações de gestão e histórico:

- `pause`, `resume`, `reprocess`, `GET/POST /documents/{id}/delete`, `DELETE /documents/{id}` e `/history` sem sessão → `303 /login`;
- pausar `ready` → `303 /documents/{id}?acao=pausado`, status `paused` e evento `system`; pausar de novo repete o redirect sem erro; retomar → `?acao=retomado` e `ready`; retomar de novo é inofensivo;
- pausar documento em `processing` → `?aviso=indisponivel` e status preservado;
- detalhe `ready` mostra pausar, reprocessar e excluir; detalhe `paused` mostra o aviso “Documento pausado”, retomar e excluir, sem oferecer pausar de novo;
- reprocessar `ready` → `?acao=reprocessando`, status `processing` e id na fila; segundo clique em `processing` → `?aviso=processando`;
- reprocessar com chunk já entregue → `?aviso=historico`, status e fila intactos;
- falha de `Queue.Enqueue` → `500` e status anterior restaurado (nada fica preso em `processing`);
- confirmação de exclusão → `200` com “Excluir documento?”, ação `POST` e “Cancelar”; `POST` → `303 /?excluido=1`, documento e diretório do original removidos, e a biblioteca exibe o aviso; repetir → `404`;
- `DELETE /documents/{id}` remove o documento e responde `303`;
- `/history` sem runs → “Nenhuma entrega registrada ainda.”; com run enviada → data, status “Enviada”, título do documento, “Enviado”, bytes em KiB, filtro por documento e link do detalhe;
- filtro por documento sem runs → vazio; filtro do próprio documento e filtro inválido (`documento=abc`) mantêm a listagem;
- run falha → status “Falhou”, erro da run, erro do item e número de tentativas.

## Links assinados

`internal/links` cobre 100% das instruções com relógio injetado:

- `ReadURL`/`DoneURL` montam `/r/{id}` e `/api/actions/chunks/{id}/done` com `t=` e escopo correto;
- token do leitor não vale na ação e vice-versa; chunk ou escopo trocados → `ErrTokenInvalido`;
- expiração exata (`now == exp`) já invalida; um segundo antes ainda vale;
- TTLs padrão por escopo (180/30 dias) e TTL explícito;
- tokens malformados (sem ponto, `exp` inválido/negativo/zero, assinatura adulterada, pontos extras) → mesmo erro, sem oráculo;
- `New` recusa segredo curto e `APP_BASE_URL` sem esquema/host, com query/fragmento ou vazia; a base é normalizada sem barra final e preserva caminho;
- mesmo relógio e TTL → token idêntico; TTL diferente muda o token.

## Configuração e entrypoint

`internal/platform`:

- `LoadEnv` usa os padrões documentados (`APP_ADDR`, `APP_DATA_DIR`, `APP_BASE_URL`, `MAX_UPLOAD_BYTES`) e lê `APP_PASSWORD`, `APP_SECRET`, `BEHIND_CLOUDFLARE` e `LOG_LEVEL`;
- valores inválidos (`MAX_UPLOAD_BYTES`, booleano, `LOG_LEVEL`, `APP_BASE_URL` sem http(s)/host) devolvem erro em vez de cair no padrão;
- `DBPath()` monta `{APP_DATA_DIR}/db/app.db`;
- `ParseLogLevel` aceita `debug`, `info`, `warn`/`warning` e `error`.

`cmd/app` (integração, sem subir porta fixa):

- `montar` abre store e auth reais em `t.TempDir()`, inicia a fila e serve `/healthz`, `/readyz`, `/login` e `/` (303 sem sessão);
- documento deixado em `processing` com original no disco é reenfileirado no start e termina `ready` com plano ativo;
- upload autenticado multipart percorre o caminho real (login → importação → fila → `ready`) e o detalhe exibe o plano;
- `documentosPendentes` filtra só `processing` e preserva a ordem da lista.
- `healthcheckURL` normaliza `APP_ADDR` vazio/`0.0.0.0`/`[::]` para `127.0.0.1`, preserva host e IPv6 explícitos e recusa endereço sem porta.
- `runHealthcheck` chama `/readyz`, aceita só `200` e devolve erro em `503`, endereço inválido ou `getenv` nulo.

## Composição da mensagem

`internal/delivery` (puro, sem I/O):

- mensagem única com cabeçalho, breadcrumb, métricas, corpo em Markdown, rodapé e link;
- recap só a partir do segundo chunk, com as duas últimas frases de prosa e ≤ 240 caracteres (inclusive com acentos);
- `starts_mid_section` repete o heading; `ends_mid_section` traz `⏭️ Amanhã` com prévia de até 140 caracteres; caso contrário, `✓ Fim da seção`;
- mais de uma dose gera seções `## 📖 {documento} · {autor}` e título `📚 Leitura de hoje · N doses`;
- corpo acima do limite → uma mensagem por documento; chunk sozinho acima do limite → corte em 60% do limite, sem quebrar rune, com `[…continua no leitor](link)`;
- `body_mode=preview` corta em 400 palavras (ou no valor configurado) e preserva o link;
- erros: nenhuma dose, título vazio, modo inválido, limite abaixo de 1.000 bytes e link maior que o limite.

## ntfy

`NewClient`/`Publish` contra `httptest.Server`:

- payload JSON com `topic`, `title`, `message`, `markdown: true`, `priority`, `tags`, `click` e `actions`;
- `Authorization: Bearer` só quando há token e `Content-Type: application/json`;
- 5xx: até 3 tentativas no total, com os backoffs injetados; sucesso encerra antes;
- 401/400: uma tentativa, sem dormir; `StatusError.Code` preservado;
- 429: respeita `Retry-After` em segundos e em data HTTP;
- corpo acima de `MaxMessageBytes`: `ErrMessageTooLarge`, sem request;
- contexto cancelado durante a espera: devolve `context.Canceled`;
- configuração inválida (base URL vazia/não http(s), topic vazio) falha no `NewClient`.

A composição não é testada com rede, e o cliente não é testado com conteúdo real: o payload é capturado e comparado por campos.

## Agendamento

`internal/schedule` usa banco real em `t.TempDir()`, `platform.FakeClock` e um `Publisher` falso (sem rede) que registra as mensagens; `Sleep` é injetado e não espera de verdade.

- configuração: padrões, leitura de todas as chaves e tabela de inválidos por chave (erro sem correção silenciosa);
- antes do horário, dia não configurado e `enabled=false` → nenhuma run;
- dia de leitura no horário → run criada, um chunk enviado, run `sent`; segundo tick não repete;
- duas doses → uma mensagem agrupada com dois chunks;
- catch-up: segunda 08:00 cria a run de hoje e a de sexta com `late=1`; janela conta dias de leitura; tick seguinte não repete;
- sem `ntfy.base_url`/`topic` → `Skipped: "ntfy não configurado"` e nenhuma run;
- documento `paused` fica de fora e `max_docs_per_run` limita a seleção;
- falha de publicação (401) → item `failed`, chunk `pending`, run `failed`;
- retomada: run `running` reenvia só itens `pending`; item de chunk já `sent` vira `skipped`;
- `Start` roda o primeiro tick imediatamente e `Stop` espera e é idempotente.

Envio manual e teste (`manual_test.go`):

- `SendNow` cria a run do dia, marca item/chunk e registra `delivery.manual`; o tick seguinte não repete;
- segundo clique com a run já fechada → `ErrJaEnviadoHoje`; sem trechos → `ErrSemTrechos` sem criar run;
- falha de publicação descarta a run criada, mantém o chunk `pending` e permite nova tentativa; run de tick anterior continua `running`;
- item `failed` é reenviado; item `sent`/`skipped` e chunk já não pendente → `ErrJaEnviadoHoje`;
- lock ocupado → `ErrEnvioEmAndamento`; id inválido → `ErrDocumentoNaoEncontrado`; documento em `processing` → `ErrDocumentoNaoPronto`; sem ntfy/configuração inválida não cria run;
- dose inválida e título vazio descartam a run; `context` nulo funciona;
- `SendTest` publica sem chunks e propaga erro; sem ntfy → `ErrNtfyNaoConfigurado`.

## Relógio

`platform.Clock` falso controla o ticker. Nenhum teste depende do relógio real.

## Fixtures e licença

- Textos de domínio público (Machado de Assis, Eça de Queirós) ou gerados.
- EPUB e PDF criados por scripts determinísticos (`testdata/epub/generate.go` e `testdata/pdf/generate.go`).
- Nenhum arquivo com direitos autorais no repositório.
- Tamanho total de `testdata` < 15 MiB.

## Benchmarks

```go
func BenchmarkPlan10kBlocks(b *testing.B)
func BenchmarkExtractPDF300p(b *testing.B)
func BenchmarkComposeMessage5Docs(b *testing.B)
```

Metas: plano de 10k blocos < 50 ms; composição < 5 ms. Medir alocação (`-benchmem`) e travar regressões no CI.

## Cobertura

| Pacote | Meta |
|---|---|
| `internal/plan` | ≥ 90% |
| `internal/blocks` | ≥ 85% |
| `internal/docs` | ≥ 75% |
| `internal/store` | ≥ 80% |
| `internal/delivery` | ≥ 80% |
| `internal/schedule` | ≥ 85% |
| `internal/links` | ≥ 95% |
| `internal/web` | ≥ 60% |

Cobertura é piso, não meta: o que importa são os cenários de borda de [`segmentation.md`](./segmentation.md).

## CI

```yaml
jobs:
  test:
    steps:
      - go test ./... -race
      - go vet ./...
      - CGO_ENABLED=0 go test ./internal/plan/... ./internal/blocks/...
      - go test ./... -coverprofile=cover.out
  build-arm64:
    steps:
      - docker buildx build --platform linux/arm64 -t readingdrip:ci .
```

`golangci-lint` opcional (não bloqueante no início).

## E2E manual (por release)

- [ ] Subir com volume vazio, criar senha, logar.
- [ ] Importar 1 MD, 1 EPUB, 1 PDF (nativo e escaneado).
- [ ] Conferir plano nos dois modos.
- [ ] Editar uma fronteira e regenerar.
- [ ] Enviar teste pelo ntfy.
- [ ] “Enviar agora” com 3 documentos ativos.
- [ ] Reiniciar o container no meio da run.
- [ ] Abrir link do leitor no celular (sem sessão).
- [ ] Marcar como lido.
- [ ] Backup + restore em container limpo.
