---
title: Entrega via ntfy
status: ativo
updated: 2026-10-26
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
```

> O app fala com o ntfy pela URL **interna** (`http://ntfy:8080`) e usa a URL pública apenas nos links. Isso evita round-trip pela Cloudflare e funciona mesmo se o túnel cair.

## Publicação

`POST {ntfy.base_url}/` com JSON (evita problemas de UTF-8 em headers):

```json
{
  "topic": "leitura",
  "title": "📖 Dom Casmurro · 32.4 · ~9 min",
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

Autenticação: `Authorization: Bearer tk_…` quando o ntfy exigir token. Nunca logar o token.

## Modos de corpo

| `ntfy.body_mode` | Comportamento | Quando usar |
|---|---|---|
| `full` (padrão) | corpo completo em Markdown + link | ntfy self-hosted com limite ≥ 64k |
| `preview` | breadcrumb + recap + primeiras 400 palavras + link | mobile lento, mensagens enormes, ntfy público |

O modo é global, com override por documento (`body_mode_override`).

## Agrupamento (documentos em paralelo)

Uma run gera **uma mensagem** com um bloco por documento ativo, na ordem de `priority`:

```markdown
## 📖 Dom Casmurro
…

---

## 📖 O Hobbit
…
```

Regras:
- no máximo `schedule.max_docs_per_run` documentos por run (padrão 5); o excedente fica para a próxima;
- título da mensagem: `📚 Leitura de hoje · 3 doses` quando há mais de um; `📖 {documento} · ~{n} min` quando há um;
- se o corpo ultrapassar `ntfy.max_message_bytes`, o app envia **uma mensagem por documento** (mesmo topic, 1 s de intervalo) e registra os itens separadamente;
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
| Timeout / erro de rede | sim | 30 s, 2 min, 10 min |
| HTTP 5xx | sim | idem |
| HTTP 429 | sim | respeita `Retry-After` |
| HTTP 400/401/403/404 | **não** | falha imediata, erro visível na UI |
| Corpo acima do limite | não | cai para split/truncate antes de enviar |

Cada tentativa incrementa `delivery_items.attempts`. Após a última falha, o item fica `failed` e o chunk **continua `pending`** — ele entra na próxima run.

## Idempotência

- `delivery_runs.run_date` é `UNIQUE` no fuso configurado.
- Um lock em processo (`sync.Mutex` + linha de run `running`) impede duas runs simultâneas.
- Item só é marcado `sent` após 2xx; `chunks.status='sent'` e `sent_at` são gravados na mesma transação.
- Restart no meio do envio: a run é retomada no próximo tick, mas apenas itens `pending` são reenviados. Como o ntfy não oferece idempotency key, existe uma janela rara de duplicação se o processo morrer **depois** do 2xx e **antes** do commit — aceitável e documentado (`RNF-012`).

## Segurança

- Token do ntfy só no banco (nunca em logs ou HTML).
- Links assinados: `HMAC-SHA256(APP_SECRET, "r:{chunkID}:{exp}")`, TTL padrão 180 dias, escopo só-leitura.
- Ação “marcar lido”: token separado com escopo `done`, TTL 30 dias, `POST` only.
- `APP_SECRET` rotacionável; rotação invalida links antigos (aceitável, regeneráveis).
- Endpoint de ação é `noindex`, sem CORS, com rate limit por IP.
- Se Cloudflare Access estiver na frente do domínio, criar **bypass** para `/r/*` e `/api/actions/*` — ou não usar Access e confiar na auth do app.

## Envio de teste

`POST /settings/test` envia:

```markdown
✅ **Teste do Reading Drip**
Se você está lendo isto, o ntfy está configurado.
🔗 [Abrir leitor de teste](…)
```

Mostra status HTTP, latência e bytes na UI.

## Observabilidade

- Log estruturado por envio: `run_id`, `document_id`, `chunk_id`, `bytes`, `status`, `latency_ms`, `attempt`.
- Nunca logar o corpo da mensagem nem o token.
- Contadores em memória expostos em `/healthz` (versão, runs hoje, último erro).

## Testes

| Teste | Ferramenta |
|---|---|
| Payload JSON correto | `httptest.Server` capturando body |
| Autenticação | header `Authorization` presente |
| Markdown habilitado | campo `markdown: true` |
| Limite de bytes | corpo acima do limite → split |
| Retry 5xx | 2 falhas + 1 sucesso = 3 tentativas |
| Sem retry 4xx | 1 tentativa |
| Idempotência | duas runs no mesmo dia = 1 envio |
| Truncamento | corpo > limite com link preservado |
