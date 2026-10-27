---
title: Interface web
status: ativo
updated: 2026-10-26
---

# Interface web

templ no servidor, Alpine.js pontual, Tailwind compilado no build. Sem SPA, sem CDN, sem build de JS no runtime.

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

Rotas sem sessão precisam de `Cache-Control: no-store` e `X-Robots-Tag: noindex, nofollow`.

## Telas

### Biblioteca (`/`)

- grade de capas com título, autor, formato, progresso e status;
- badge de status (`processando`, `pronto`, `pausado`, `falhou`, `precisa OCR`);
- upload por drag-and-drop com múltiplos arquivos e progresso individual;
- ações rápidas: pausar, enviar agora, abrir plano, excluir (com confirmação).

### Detalhe (`/documents/{id}`)

- metadados editáveis;
- ritmo (modo + valor) com prévia do alvo;
- estatísticas: palavras, tempo total, chunks, conclusão estimada;
- timeline do plano: lista de chunks com `seq`, seção, minutos, status;
- botão “regenerar a partir do cursor”;
- avisos (chunk oversized, pacing inviável, plano `stale`).

### Preview do plano (`/documents/{id}/plan`)

- tabela virtualizada (Alpine) com 10 itens por página;
- por chunk: intervalo de blocos, seção, palavras, minutos, flags;
- expansão para ler o texto do chunk antes de enviar;
- edição: mesclar com o próximo, dividir em um bloco escolhido, mover bloco de fronteira;
- simulação: alterar `min_fill`/`max_overshoot` e ver o resultado **sem salvar**.

### Leitor (`/r/{chunkID}`)

- layout de leitura (largura ~68ch, tipografia confortável, tema claro/escuro);
- cabeçalho com breadcrumb e progresso;
- recap, corpo, rodapé;
- “ver seção completa” (expansível, carrega o restante da seção);
- navegação anterior/próximo;
- “marcar como lido”;
- funciona offline após carregar (CSS/JS embutidos, sem chamadas externas).

### Configurações (`/settings`)

Seções: ntfy (URL, topic, token, prioridade, modo de corpo, limite), agenda (dias, hora, fuso, catch-up), plano (WPM, min/max, recaps), leitor (TTL de link), segurança (trocar senha).

### Histórico (`/history`)

- últimas 30 runs: data, status, `late`, itens, bytes, latência;
- detalhe por documento com erro e número de tentativas;
- filtro por documento.

## Autenticação

Modelo de dois níveis:

| Nível | Quem | Como |
|---|---|---|
| Sessão | UI de gestão | senha única (`APP_PASSWORD`) → cookie assinado `HttpOnly; Secure; SameSite=Lax`, validade 7 dias |
| Token | links do ntfy | HMAC na URL, escopo `r` ou `done`, expiração configurável |

- Senha com Argon2id, comparada em tempo constante.
- Rate limit: 5 tentativas/min por IP.
- Rotas de gestão exigem sessão; `/r/*` e `/api/actions/*` exigem token válido.
- `APP_SECRET` assina cookie e tokens; trocar invalida sessões e links.
- Opcional: Cloudflare Access na frente de tudo, com bypass para `/r/*` e `/api/actions/*` (`Q-003`).

## Alpine.js

Componentes pequenos, estado local, sem store global:

| Componente | Onde | Responsabilidade |
|---|---|---|
| `uploader` | biblioteca | fila, progresso via XHR, retry, erros |
| `pollStatus` | detalhe | polling de 1,5 s até `ready`/`failed` |
| `planTable` | preview | paginação, expansão, seleção de bloco |
| `boundaryEditor` | preview | mesclar/dividir com confirmação |
| `settingsForm` | configurações | validação e “testar conexão” |
| `themeToggle` | leitor | claro/escuro persistido em `localStorage` |

Regras:
- Alpine vendorizado (`web/static/alpine.min.js`), sem CDN;
- nenhum dado sensível em `x-data` (token do ntfy nunca vai ao cliente);
- CSP `script-src 'self'` — sem `unsafe-inline`; handlers via `x-on`.
- JS progressivo: sem Alpine, upload e formulários continuam funcionando (form submit clássico).

## Tailwind

- `web/static/input.css` com `@import "tailwindcss"` e `@source` apontando para `web/views/**/*.templ`;
- build no Docker: `npx @tailwindcss/cli -i … -o web/static/app.css --minify`;
- CSS final embutido via `go:embed`, hash no nome para cache imutável;
- dark mode por classe (`data-theme`), sem flash (script inline mínimo permitido por hash na CSP).

## Cloudflare

| Assunto | Decisão |
|---|---|
| TLS | terminado na Cloudflare; origem HTTP interna |
| Proxy | `cloudflared tunnel` (sem portas abertas) |
| IP real | `CF-Connecting-IP` confiado só com `BEHIND_CLOUDFLARE=true` |
| Cache | HTML `no-store`; `/media/*` e assets com hash `immutable` |
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
| 401 token inválido/expirado | página “link expirado” com botão para gerar novo (requer sessão) |
| 404 | documento/chunk não encontrado |
| 413 | arquivo acima do limite, com o limite exibido |
| 422 | arquivo inválido (`needs_ocr`, corrompido) |
| 500 | mensagem genérica + `request_id` para correlacionar no log |
