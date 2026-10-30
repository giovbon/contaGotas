---
title: Entrega via ntfy
status: ativo
updated: 2026-10-30
---

# Entrega via ntfy

ntfy **self-hosted**, com `message-size-limit` elevado para caber um chunk inteiro. A mensagem leva o texto completo em Markdown e um **link para o leitor** no domínio próprio (Cloudflare).

## Configuração do servidor ntfy

`server.yml` relevante:

```yaml
base-url: "https://ntfy.exemplo.com"
listen-http: ":8080"
cache-file: "/var/cache/ntfy/cache.db"
cache-duration: "48h"
message-size-limit: "64k"        # padrão é 4k
behind-proxy: true
# Com autenticação (opcional):
# auth-file: "/var/cache/ntfy/auth.db"
# auth-default-access: "deny-all"
```

Com `auth-default-access: "deny-all"`, crie um usuário e libere escrita no tópico usado pelo app:

```sh
ntfy user add leitor          # pede a senha
ntfy access leitor leitura write-only
```

> O app fala com o ntfy pela URL **interna** (`http://ntfy:8080`) e usa a URL pública apenas nos links. Isso evita round-trip pela Cloudflare e funciona mesmo se o túnel cair.

## Configuração do app

Lidas do banco a cada tick (`schedule.LoadSettings`, ver [`scheduling.md`](./scheduling.md)):

| Chave | Padrão | Descrição |
|---|---|---|
| `ntfy.base_url` | vazio | URL interna do ntfy, com `http(s)`; obrigatória para enviar |
| `ntfy.topic` | vazio | tópico de publicação; obrigatório para enviar |
| `ntfy.token` | vazio | Bearer opcional; nunca logado nem renderizado |
| `ntfy.username` | vazio | usuário do HTTP Basic; junto com `ntfy.password` tem precedência sobre o token |
| `ntfy.password` | vazio | senha do HTTP Basic; nunca logada nem renderizada |
| `ntfy.priority` | `3` | inteiro de 1 a 5 |
| `ntfy.body_mode` | `full` | `full` ou `preview` |
| `ntfy.max_message_bytes` | `60000` | limite do corpo; mínimo 1.000 |

Sem `base_url` ou `topic`, o tick não cria run e reporta `ntfy não configurado`. Valor inválido (prioridade fora de 1..5, modo desconhecido, limite abaixo do piso) falha o tick com erro citando a chave. `ntfy.username` e `ntfy.password` são um par: um sem o outro falha com erro citando as duas chaves.

## Publicação

`POST {ntfy.base_url}/` com JSON (evita problemas de UTF-8 em headers):

```json
{
  "topic": "leitura",
  "title": "📖 Dom Casmurro · ~9 min",
  "message": "**Dom Casmurro** · Machado de Assis\n`Cap. 32 …`\n\n…",
  "markdown": true,
  "priority": 3,
  "tags": ["books", "open_book"],
  "click": "https://leitura.exemplo.com/r/42?t=TOKEN",
  "actions": [
    {
      "action": "view",
      "label": "Abrir leitor",
      "url": "https://leitura.exemplo.com/r/42?t=TOKEN"
    },
    {
      "action": "http",
      "label": "Marcar lido",
      "url": "https://leitura.exemplo.com/api/actions/chunks/42/done?t=TOKEN",
      "method": "POST"
    }
  ]
}
```

Autenticação: `Authorization: Basic base64(usuário:senha)` quando `ntfy.username`/`ntfy.password` estão configurados; senão `Authorization: Bearer tk_…` com `ntfy.token`. Basic tem precedência sobre o token. Nunca logar token nem senha.

## Modos de corpo

| `ntfy.body_mode` | Comportamento | Quando usar |
|---|---|---|
| `full` (padrão) | corpo completo em Markdown + link | ntfy self-hosted com limite ≥ 64k |
| `preview` | breadcrumb + recap + primeiras 400 palavras + link | mobile lento, mensagens enormes, ntfy público |

O modo é global, com override por documento (`body_mode_override`).

## Agrupamento (documentos em paralelo)

Uma run gera **uma mensagem** com um bloco por documento ativo, na ordem de `priority`:

```markdown
## 📖 Dom Casmurro · Machado de Assis
…

---

## 📖 O Hobbit · J. R. R. Tolkien
…
```

Regras:
- no máximo `schedule.max_docs_per_run` documentos por run (padrão 5); o excedente fica para a próxima;
- título da mensagem: `📚 Leitura de hoje · 3 doses` quando há mais de um; `📖 {documento} · ~{n} min` quando há um;
- se o corpo ultrapassar `ntfy.max_message_bytes`, o app envia **uma mensagem por documento** (o scheduler aplica 1 s de intervalo) e registra os itens separadamente;
- se **um único** chunk ultrapassar o limite, trunca em 60% do limite em fronteira de parágrafo e adiciona `[…continua no leitor](link)`.

## Limite de bytes

| Constante | Valor | Origem |
|---|---|---|
| `message-size-limit` do ntfy | 64 KiB (recomendado) | configuração do servidor |
| `ntfy.max_message_bytes` do app | 60.000 bytes | margem de segurança |
| Limite do Cloudflare no POST | 100 MiB (não relevante aqui) | — |

O tamanho é medido em **bytes UTF-8** do `message`, antes do JSON.

## Retry e classificação de erro

| Situação | Retry? | Backoff |
|---|---|---|
| Timeout / erro de rede | sim | 30 s, 2 min |
| HTTP 5xx | sim | idem |
| HTTP 429 | sim | respeita `Retry-After` |
| HTTP 400/401/403/404 | **não** | falha imediata, erro visível na UI |
| Corpo acima do limite | não | cai para split/truncate antes de enviar |

São **3 tentativas no total** (`DefaultNtfyAttempts`): a primeira mais duas repetições, usando os dois primeiros intervalos da lista de backoff (o padrão tem ainda 10 min para quem aumentar as tentativas). `Retry-After` (segundos ou data HTTP) substitui o backoff daquela tentativa. Cada tentativa incrementa `delivery_items.attempts`. Após a última falha, o item fica `failed` e o chunk **continua `pending`** — ele entra na próxima run.

Exceções: **envio manual** (`POST /documents/{id}/send-now`) e **envio de teste** (`POST /settings/test`) usam `MaxAttempts=1`, porque são síncronos e o usuário vê o resultado — repetir o clique é o retry. Ambos montam o cliente com uma tentativa própria (`Runner.publisherOnce`); o tick continua com o padrão de 3.

## Implementação

`internal/delivery` expõe dois componentes independentes:

- `Compose(doses, opts)` monta `[]Message` sem I/O nem relógio. Cada `Dose` traz o chunk, os blocos do trecho atual, do anterior (recap) e do próximo (rodapé), já recortados pelo chamador. Uma dose vira uma mensagem; várias doses viram uma mensagem agrupada com seções `## 📖 {documento} · {autor}` separadas por `---` (RF-053).
- `NewClient(cfg)` + `Publish(ctx, msg)` publicam via JSON com `markdown: true`, HTTP Basic (usuário/senha) ou `Authorization: Bearer` opcional e retry classificado. `Publish` mede o corpo em bytes UTF-8 e recusa acima de `MaxMessageBytes` antes de qualquer request (`ErrMessageTooLarge`).

Regras aplicadas na composição:

- `max_message_bytes` tem piso de 1.000 bytes (`MinMessageBytes`); abaixo disso `Compose` falha em vez de gerar mensagem sem sentido.
- O link do leitor precisa caber no limite com margem de 200 bytes.
- Acima do limite, o app gera **uma mensagem por documento**; se um único chunk ainda passar, o corpo é cortado em 60% do limite, em fronteira de parágrafo ou palavra, sem quebrar rune UTF-8, e recebe `[…continua no leitor](link)`.
- Truncamento e recap contam **caracteres** (runes), não bytes; o limite de envio conta bytes.
- `body_mode` global pode ser sobrescrito por dose (`Dose.BodyMode`).
- `plan.recap=false` desliga o “Onde paramos” (`Options.RecapDisabled`); o padrão é ligado, mantendo RF-041.
- A composição não loga corpo nem token; quem publica deve logar apenas ids, bytes, status, latência e tentativas.

## Integração com o agendador

`internal/schedule.Runner` lê as configurações, monta as doses a partir do chunk, blocos anteriores/próximos e progresso, chama `Compose` com `body_mode`/`max_message_bytes` e publica com `Client.Publish`. As doses de uma run vão em uma única chamada a `Compose`; se ele devolver várias mensagens (split por limite), o runner espera 1 s entre elas (`Sleep` injetável nos testes). Cada mensagem publicada marca seus chunks e itens na mesma transação. Ver [`scheduling.md`](./scheduling.md).

## Idempotência

- `delivery_runs.run_date` é `UNIQUE` no fuso configurado; a criação usa `ON CONFLICT DO NOTHING` e o tick que perdeu a corrida não reexecuta a run.
- Um lock em processo (`TryLock`) impede dois ticks simultâneos; uma run `running` é retomada no próximo tick antes de criar run nova.
- Item só é marcado `sent` após 2xx; `chunks.status='sent'` e `sent_at` são gravados na mesma transação, que exige o chunk `pending` (DB-08).
- Falha de publicação marca o item `failed` com `attempts` e `bytes`, mas deixa o chunk `pending`: ele entra na próxima run.
- `UNIQUE (run_id, document_id)` garante no máximo uma dose por documento por run; `max_docs_per_run` limita quantos documentos entram.
- Restart no meio do envio: a run é retomada no próximo tick, mas apenas itens `pending` são reenviados. Como o ntfy não oferece idempotency key, existe uma janela rara de duplicação se o processo morrer **depois** do 2xx e **antes** do commit — aceitável e documentado (`RNF-012`).
- Uma mensagem agrupada grava `bytes` igual em todos os itens que ela levou; o `attempts` de cada item vem do resultado do `Publish`.

## Links assinados

`internal/links` assina as URLs das mensagens. O token é stateless e não guarda
nada no banco: `{exp}.{hex}`, onde `exp` é o Unix de expiração e `hex` é
`HMAC-SHA256(chave, "{escopo}:{chunkID}:{exp}")`. A chave é
`HMAC-SHA256(APP_SECRET, "readingdrip/link/v1\x00")`.

| Uso | Escopo | Validade padrão | Chave |
|---|---|---|---|
| Leitor (`GET /r/{id}?t=`) | `r` | 180 dias | `reader.link_ttl_days` |
| Marcar lido (`POST /api/actions/chunks/{id}/done?t=`) | `done` | 30 dias | `reader.done_ttl_days` |

- Cada escopo só vale para o seu uso e para o seu chunk; trocar qualquer um
  invalida o token.
- A verificação é em tempo constante e devolve um único erro
  (`ErrTokenInvalido`), sem distinguir expirado de adulterado.
- `APP_BASE_URL` define o host dos links; `APP_SECRET` é a chave. Trocar o
  segredo invalida todos os links antigos (aceitável, regeneráveis).
- As validades vêm de `Settings.LinkTTLs()`; configuração inválida cai nos
  padrões de `links` e aparece como erro no tick.
- `Runner.Links` recebe `(document, chunk, readerTTL, doneTTL)` e devolve as duas
  URLs. Com `Links == nil` (testes antigos), as mensagens saem sem `click` e sem
  ações.
- O token viaja só na URL: nunca é logado nem renderizado no corpo da mensagem.

## Segurança

- Token e senha do ntfy só no banco (nunca em logs ou HTML).
- O endpoint de ação é `noindex`, sem CORS.
- Se Cloudflare Access estiver na frente do domínio, criar **bypass** para `/r/*` e `/api/actions/*` — ou não usar Access e confiar na auth do app.
- O campo `click` e as ações usam os links assinados descritos acima; a página do leitor nunca loga o corpo lido.

## Envio de teste

`POST /settings/test` envia, com a configuração **salva**:

```markdown
✅ **Teste do Reading Drip**
Se você está lendo isto, o ntfy está configurado.
```

- título da mensagem: `✅ Teste do Reading Drip`;
- **uma** tentativa (`MaxAttempts=1`), sem backoff: o resultado volta na hora e repetir é o retry;
- `context` de 20 s aplicado pelo handler; a página volta com HTTP, latência e bytes, ou o motivo da falha;
- não cria run, item nem evento de entrega; loga apenas status, bytes, latência e tentativas;
- sem `ntfy.base_url`/`topic`, a UI pede para configurar antes de testar (`ErrNtfyNaoConfigurado`).

O teste não usa `Runner.Links`: sai sem `click`, sem ações e sem link de leitor, porque não existe chunk associado. Isso também confirma que o ntfy está acessível sem depender da assinatura.

## Observabilidade

- Log estruturado por envio: `run_id`, `document_id`, `chunk_id`, `bytes`, `status`, `latency_ms`, `attempt`.
- Nunca logar o corpo da mensagem nem o token.
- Contadores em memória expostos em `/healthz` (versão, runs hoje, último erro).

## Testes

| Teste | Ferramenta |
|---|---|
| Payload JSON correto | `httptest.Server` capturando body |
| Autenticação | header `Authorization` presente |
| Basic auth | usuário/senha viram `Authorization: Basic` e têm precedência sobre o token |
| Markdown habilitado | campo `markdown: true` |
| Limite de bytes | corpo acima do limite → split |
| Retry 5xx | 2 falhas + 1 sucesso = 3 tentativas |
| Sem retry 4xx | 1 tentativa |
| Retry-After | 429 com espera de 7 s |
| Idempotência | duas runs no mesmo dia = 1 envio |
| Truncamento | corpo > limite com link preservado, runes íntegros |
| Agrupamento | seções `## 📖 …` e título `📚 Leitura de hoje` |
| Recap | duas últimas frases, ≤ 240 caracteres |
