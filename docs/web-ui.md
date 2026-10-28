---
title: Interface web
status: ativo
updated: 2026-10-27
---

# Interface web

templ no servidor e CSS/JS próprios embutidos no binário ([ADR-0014](./adr/0014-ui-css-js-proprios.md)). Sem SPA, sem CDN e sem Node no build nesta fase. Alpine.js e Tailwind ficam adiados para as telas interativas (uploader com progresso e edição de plano).

## Rotas

| Método | Rota | Auth | Função |
|---|---|---|---|
| `GET` | `/` | sessão | Biblioteca |
| `GET` | `/login` | pública | Formulário |
| `POST` | `/login` | pública | Cria sessão (rate limit) |
| `POST` | `/logout` | sessão | Encerra |
| `POST` | `/documents` | sessão | Upload multipart |
| `GET` | `/documents/{id}` | sessão | Detalhe + status + plano |
| `DELETE` | `/documents/{id}` | sessão | Exclui |
| `POST` | `/documents/{id}/pause` | sessão | Pausa |
| `POST` | `/documents/{id}/resume` | sessão | Retoma |
| `POST` | `/documents/{id}/reprocess` | sessão | Reprocessa |
| `POST` | `/documents/{id}/send-now` | sessão | Envio manual |
| `GET` | `/documents/{id}/plan` | sessão | Preview do plano |
| `GET` | `/documents/{id}/download` | sessão | Baixa o original |
| `GET` | `/static/` | pública | CSS/JS embutidos |
| `POST` | `/documents/{id}/plan/regenerate` | sessão | Nova versão a partir do cursor |
| `PATCH` | `/chunks/{id}` | sessão | Mescla/divide/move fronteira |
| `GET` | `/settings` | sessão | Configurações |
| `POST` | `/settings` | sessão | Salva |
| `POST` | `/settings/test` | sessão | Envia teste ntfy |
| `GET` | `/history` | sessão | Runs e itens |
| `GET` | `/r/{chunkID}` | **token** | Leitor |
| `POST` | `/api/actions/chunks/{id}/done` | **token** | Marca lido |
| `GET` | `/media/{docID}/{file}` | sessão ou token | Imagens |
| `GET` | `/healthz` | pública | Liveness |
| `GET` | `/readyz` | pública | Readiness (DB) |

Rotas HTML sem sessão precisam de `Cache-Control: no-store` e `X-Robots-Tag: noindex, nofollow`; `/static/` é a exceção pública com cache curto.

### Implementação atual (2026-10-27)

Já implementado em `internal/web`:

- `GET /login`, `POST /login` e `POST /logout` com sessão real de `internal/auth` (Argon2id + cookie HMAC; ver [ADR-0013](./adr/0013-sessao-hmac-argon2id.md)).
- `GET /` — biblioteca em templ: lista de documentos com título, formato, autor, palavras, duração, badge de status, aviso de erro e formulário de upload multipart.
- `GET /documents/{id}` — detalhe em templ: metadados, status, aviso de duplicado, botão de download do original e prévia do plano ativo.
- `GET /documents/{id}/plan` — preview do plano em página própria, com a mesma tabela da prévia (somente leitura).
- `GET /documents/{id}/download` — entrega o original como `Content-Disposition: attachment`, somente se o caminho estiver sob `{APP_DATA_DIR}/documents`.
- `GET /settings` — configurações em templ: agenda, ntfy e token mascarado; avisa quando a configuração salva tem valor inválido.
- `POST /settings` — valida o formulário inteiro com `schedule.Parse` e grava em uma transação; valor inválido não é persistido.
- `POST /settings/test` — publica a mensagem de teste com a configuração salva (uma tentativa) e volta com status, latência e bytes.
- `POST /documents/{id}/send-now` — envio manual do próximo trecho, síncrono e de uma tentativa, registrado na run do dia.
- `GET /r/{chunkID}` — leitor público autenticado por token: corpo do trecho, navegação anterior/próximo, “ver seção completa” e “marcar como lido”.
- `POST /api/actions/chunks/{id}/done` — marca o trecho como lido; o ntfy recebe `204` e o formulário do leitor volta para a página com `303`.
- `GET /static/` — CSS e JS embutidos (`app.css`, `app.js`); público, com `Cache-Control: public, max-age=3600`.
- `POST /documents` (upload multipart em streaming), protegido pelo middleware de sessão. Sem `Server.Sessions` nem `Server.Auth`, as rotas de gestão falham fechadas com `401` (configuração incompleta); com `Sessions`, sem cookie válido a resposta é `303 /login`.
- `GET /healthz` e `GET /readyz` (ping no SQLite), públicas.
- Cabeçalhos comuns: `Cache-Control: no-store`, `X-Robots-Tag: noindex, nofollow`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin` e CSP `default-src 'self'; img-src 'self' data:; script-src 'self'; style-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'`.

As rotas de ação de gestão (`DELETE`, `pause`, `resume`, `reprocess`, `PATCH /chunks/{id}`, histórico e `/media/*`) ainda não existem. O leitor e a API de ações do ntfy já estão implementados.

Contrato do upload:

| Situação | Resposta |
|---|---|
| 1 arquivo novo | `303 /documents/{id}` |
| 1 duplicado | `303 /documents/{id}?duplicado=1` (id do existente, RF-007) |
| lote | `303 /?importados={n}&duplicados={m}` |
| arquivo > limite | `413` |
| formato inválido ou vazio | `422` |
| corpo não multipart | `400` |
| sem arquivo no formulário | `422` |
| falha interna | `500` |
| sem sessão | `303 /login` (RF-074) |

Contrato do login:

| Situação | Resposta |
|---|---|
| `GET /login` | `200` com formulário; com sessão válida, `303` para o destino seguro |
| senha correta | `303` para `next` (ou `/`) + cookie `rd_session` |
| senha errada | `401`, sem cookie |
| 5 falhas/minuto por IP | `429` + `Retry-After`, sem cookie |
| formulário inválido | `400`; acima de 64 KiB, `413` |
| autenticação não configurada | `503` |

O limite do corpo do upload é o limite do importador + 1 MiB para o envelope multipart (ou `Server.MaxBodyBytes`); o limite exato do arquivo é aplicado pelo importador. A request não espera extração: o documento nasce `processing`.

A página de login, a biblioteca, o detalhe, o preview do plano e a página de erro usam templ. O CSS e o JS são arquivos próprios embutidos (`internal/web/assets`) e servidos em `/static/`; não há JavaScript inline, o que mantém a CSP em `script-src 'self'`. A biblioteca e o detalhe funcionam sem JS; o único JS é o polling de status descrito abaixo.


## Telas

### Biblioteca (`/`)

Implementado:

- lista de documentos com título, autor, formato, palavras, duração e status;
- badge de status (`processando`, `pronto`, `pausado`, `concluído`, `falhou`, `precisa OCR`);
- aviso de importação (`?importados=N&duplicados=M`) e de erro de extração;
- upload multipart clássico (sem drag-and-drop e sem progresso individual).

Ainda não implementado: grade de capas, drag-and-drop com progresso e ações rápidas (pausar, abrir plano, excluir).

### Detalhe (`/documents/{id}`)

Implementado:

- metadados e estatísticas: formato, palavras, tempo total, data de adição e versão/número de trechos do plano;
- badge de status, aviso de `processing`, `failed`/`needs_ocr` e aviso de duplicado (`?duplicado=1`);
- botão “Baixar original” quando o arquivo está sob `{APP_DATA_DIR}/documents`;
- botão “Enviar agora” quando o documento está `ready` e tem plano ativo;
- aviso do envio manual (`?enviado=1`, `?aviso=…`) ou a falha de publicação (`?aviso=falha&motivo=…`);
- prévia do plano ativo com `seq`, seção, palavras, minutos e status, com link para o preview completo;
- polling de status: com JS, a própria página é consultada a cada 1,5 s enquanto o status é `processing`; ao mudar, recarrega; desiste após ~3 min. Sem JS, basta recarregar.

Ainda não implementado: edição de metadados e ritmo, “regenerar a partir do cursor”, avisos de `oversized`/`stale` e ações de pausa/exclusão.

### Preview do plano (`/documents/{id}/plan`)

Implementado (somente leitura):

- tabela com `seq`, seção, palavras, minutos e status por chunk;
- indicação da versão ativa e mensagem quando ainda não há plano.

Ainda não implementado: paginação/virtualização, expansão para ler o texto do chunk, edição de fronteiras (mesclar/dividir/mover) e simulação de `min_fill`/`max_overshoot`.

### Leitor (`/r/{chunkID}`)

Implementado:

- layout de leitura (largura ~68ch, tipografia confortável, tema claro/escuro, utilizável a partir de 360 px);
- cabeçalho com breadcrumb (documento › seção) e progresso do livro;
- corpo do trecho renderizado a partir do HTML seguro dos blocos;
- “ver seção completa” em `<details>`, exibido só quando a seção do heading ancestral vai além do trecho;
- navegação anterior/próximo, cada uma com token de leitura próprio (desabilitada nas pontas);
- “marcar como lido” por POST assinado; depois de lido, mostra a data e esconde o botão;
- link de volta para o documento (exige sessão);
- CSS embutido, sem chamadas externas e sem JavaScript.

Contrato de `/r/{chunkID}`:

| Situação | Resposta |
|---|---|
| token `r` válido para um chunk existente | `200` com a página do leitor |
| token ausente, malformado, adulterado, de outro escopo/chunk ou expirado | `401` com a página “Este link expirou ou é inválido” |
| chunk inexistente | `404` |
| documento do chunk removido | `404` |
| banco ou assinador indisponível | `503` |

O token **não** é ecoado em lugar nenhum além das URLs de navegação e do formulário; o corpo do trecho nunca é registrado no log. A página responde com `Cache-Control: no-store` e `X-Robots-Tag: noindex, nofollow`.

Contrato de `/api/actions/chunks/{id}/done`:

| Situação | Resposta |
|---|---|
| POST com token `done` válido | `204 No Content` (ntfy) |
| mesmo POST com `ui=1` ou `Accept: text/html` | `303 /r/{id}?t={novo token de leitura}&lido=1` |
| token ausente/inválido/expirado ou de outro escopo | `401` com a página de link inválido |
| chunk inexistente | `404` |
| repetição do POST | mesma resposta; `done_at` não muda de estado |
| banco ou assinador indisponível | `503` |

Só `POST`: abrir a URL no navegador responde `405`. Marcar de novo é idempotente — o `UPDATE` só exige que o chunk exista.

### Configurações (`/settings`)

Implementado: agenda e ntfy, com envio de teste.

- **Agenda**: liga/desliga (`schedule.enabled`), dias da semana (`schedule.days`, checkbox por dia), horário (`schedule.time`, `<input type="time">`), fuso IANA (`schedule.timezone`), catch-up em dias de leitura (`schedule.catchup_days`) e documentos por run (`schedule.max_docs_per_run`).
- **ntfy**: URL base (`ntfy.base_url`), tópico (`ntfy.topic`), token (`ntfy.token`, campo `password`), prioridade 1–5, modo de corpo (`full`/`preview`) e limite do corpo (`ntfy.max_message_bytes`, mínimo 1.000).
- O token **nunca** é renderizado: o campo nasce vazio com o placeholder “token salvo”. Em branco mantém o token atual; a caixa “Remover o token salvo” grava vazio.
- A validação é a mesma do agendador (`schedule.Parse`), aplicada ao formulário inteiro antes de gravar; tudo entra em uma transação (`Settings.SetMany`). Valor inválido reexibe o formulário com o erro e não persiste nada.
- `GET /settings` mostra os valores salvos (ou os padrões) e avisa quando a configuração salva é inválida, sem impedir a correção.
- “Enviar teste” (`POST /settings/test`) publica a mensagem de teste no ntfy com a configuração **salva**, em uma única tentativa; a página volta com HTTP, latência e bytes, ou o motivo da falha.

As validades dos links já são configuráveis pelas chaves `reader.link_ttl_days` (padrão 180) e `reader.done_ttl_days` (padrão 30), mas ainda não têm campo nesta tela; ver [`scheduling.md`](./scheduling.md#configurações).

Ainda não implementado: plano (WPM, min/max, recaps), campos do leitor na UI e troca de senha.

Contrato de `/settings`:

| Situação | Resposta |
|---|---|
| `GET` | `200` com o formulário preenchido |
| `POST` válido | `303 /settings?salvo=1` |
| `POST` com valor inválido | `422` com o formulário e o erro (nada gravado) |
| formulário malformado | `400`; acima de 64 KiB, `413` |
| sem banco | `503` |
| sem sessão | `303 /login` (RF-074) |

Contrato de `/settings/test`:

| Situação | Resposta |
|---|---|
| envio 2xx | `303 /settings?teste=ok&http=…&ms=…&bytes=…` |
| falha | `303 /settings?teste=erro&motivo=…` (motivo truncado em 200 bytes) |
| envio indisponível (sem agendador) | `503` |
| sem sessão | `303 /login` |

Contrato de `/documents/{id}/send-now`:

| Situação | Resposta |
|---|---|
| trecho enviado | `303 /documents/{id}?enviado=1` |
| sem trechos pendentes | `303 …?aviso=sem-trechos` |
| run do dia já fechada | `303 …?aviso=ja-enviado` |
| ntfy não configurado | `303 …?aviso=ntfy` |
| entrega em andamento | `303 …?aviso=ocupado` |
| documento não `ready`/inexistente | `303 …?aviso=indisponivel` (id malformado: `404`) |
| tempo esgotado | `303 …?aviso=tempo` |
| falha de publicação | `303 …?aviso=falha&motivo=…` |
| envio indisponível (sem agendador) | `503` |
| sem sessão | `303 /login` |

O envio manual é síncrono e de **uma tentativa** (`envioTimeout` de 30 s), para o clique ter resultado imediato; repetir o clique é o “retry”. Os detalhes da run do dia estão em [`scheduling.md`](./scheduling.md#envio-manual).

### Histórico (`/history`)

- últimas 30 runs: data, status, `late`, itens, bytes, latência;
- detalhe por documento com erro e número de tentativas;
- filtro por documento.

## Autenticação

Modelo de dois níveis:

| Nível | Quem | Como |
|---|---|---|
| Sessão | UI de gestão | senha única (`APP_PASSWORD`) → cookie assinado `HttpOnly; Secure (HTTPS); SameSite=Lax`, validade 7 dias |
| Token | links do ntfy | HMAC na URL, escopo `r` ou `done`, expiração configurável |

- Senha com **Argon2id** (`m=19456 KiB`, `t=2`, `p=1`, salt de 16 bytes), comparada em tempo constante.
- Cookie `rd_session` stateless: `v1.{expiração}.{HMAC-SHA256}`; a chave deriva de `APP_SECRET` + `APP_PASSWORD`, então trocar qualquer um invalida as sessões.
- Rate limit: **5 falhas/min por IP**; `BEHIND_CLOUDFLARE=true` usa `CF-Connecting-IP`, senão o endereço da conexão. Bloqueio responde `429` com `Retry-After`; sucesso zera as falhas.
- Sem sessão, rota de gestão → `303 /login`. `?next=` só é preservado em GET/HEAD e só para caminho local (sem esquema, host, `//host` ou barra invertida).
- Rotas de gestão exigem sessão; `/r/*` e `/api/actions/*` exigem token válido.
- `APP_SECRET` assina cookie e tokens; trocar invalida sessões e links.
- Opcional: Cloudflare Access na frente de tudo, com bypass para `/r/*` e `/api/actions/*` (`Q-003`).

Detalhes e alternativas: [ADR-0013](./adr/0013-sessao-hmac-argon2id.md). O formulário usa `SameSite=Lax` e POST; não há token CSRF separado nesta versão.

## JavaScript

O único JS servido é `internal/web/assets/app.js` (embutido em `/static/app.js`):

| Componente | Onde | Responsabilidade |
|---|---|---|
| `pollStatus` | detalhe | consulta a própria página a cada 1,5 s enquanto `data-status="processing"`; recarrega quando o status muda e desiste após ~3 min |

Regras:
- CSP `script-src 'self'`: nenhum script inline, nenhum CDN;
- JS progressivo: sem JS, biblioteca, upload clássico e detalhe continuam funcionando;
- nenhum dado sensível no cliente (o cookie de sessão é `HttpOnly`).

Alpine.js fica adiado ([ADR-0014](./adr/0014-ui-css-js-proprios.md)). Quando entrar, os componentes previstos são `uploader`, `planTable`, `boundaryEditor`, `settingsForm` e `themeToggle`, vendorizados sem CDN.

## CSS

- CSS próprio em `internal/web/assets/app.css`, embutido com `go:embed` e servido em `/static/app.css`;
- tema claro/escuro por `prefers-color-scheme`, com `prefers-reduced-motion`;
- sem Node, sem Tailwind e sem hash no nome nesta fase; `Cache-Control: public, max-age=3600` (ADR-0014);
- Tailwind, se adotado depois, entra com CLI no build e hash no nome para cache imutável.

## Cloudflare

| Assunto | Decisão |
|---|---|
| TLS | terminado na Cloudflare; origem HTTP interna |
| Proxy | `cloudflared tunnel` (sem portas abertas) |
| IP real | `CF-Connecting-IP` confiado só com `BEHIND_CLOUDFLARE=true` |
| Cache | HTML `no-store`; `/media/*` (futuro) e assets com cache curto |
| Upload | limite de 95 MiB (margem dos 100 MiB do plano free) |
| Timeout | parse sempre assíncrono (origem responde < 1 s) |
| Bot fight | bypass em `/api/actions/*` para o ntfy conseguir POSTAR |
| Cookies | `Secure` + `X-Forwarded-Proto: https` respeitado |

## Acessibilidade

- contraste AA, foco visível, `prefers-reduced-motion`;
- leitor com landmarks (`main`, `nav`), headings hierárquicos, `lang` correto;
- botões com `aria-label`; imagens com `alt` vindo da extração;
- navegação completa por teclado no preview do plano.

## Estados de erro

| Código | UI |
|---|---|
| 401 token inválido/expirado | página “Este link expirou ou é inválido”, com botão para a biblioteca |
| 401 senha errada | mensagem genérica no formulário de login |
| 429 login | “muitas tentativas”, com `Retry-After` em segundos |
| 404 | documento/chunk não encontrado |
| 413 | arquivo acima do limite, com o limite exibido |
| 422 | arquivo inválido (`needs_ocr`, corrompido) |
| 500 | mensagem genérica na página; detalhe técnico só no log do servidor |
