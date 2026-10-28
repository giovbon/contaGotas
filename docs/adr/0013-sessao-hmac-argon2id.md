---
title: ADR-0013 — Sessão por cookie HMAC e senha única com Argon2id
status: aceito
date: 2026-10-27
---

# ADR-0013 — Sessão por cookie HMAC e senha única com Argon2id

## Contexto

A UI de gestão é de um único usuário e fica atrás de um túnel Cloudflare. Ela precisa de autenticação simples, sem cadastro, sem banco de sessões e sem dependência de serviço externo (I-09). O requisito RF-074 exige senha única e sessão assinada; sem sessão, rotas de gestão redirecionam para `/login`. O orçamento de memória em idle é de 60 MiB e o app roda em ARM64.

## Decisão

1. **Senha única** vinda de `APP_PASSWORD`, sem persistência. Na inicialização o app gera um salt aleatório de 16 bytes e calcula **Argon2id** com `m=19456 KiB`, `t=2`, `p=1`, saída de 32 bytes. A comparação usa `subtle.ConstantTimeCompare`.
2. **Sessão stateless** em cookie `rd_session`, no formato `v1.{exp.Unix}.{HMAC-SHA256}`. A chave é derivada de `APP_SECRET` com `HMAC-SHA256(APP_SECRET, "readingdrip/session/v1\0" + APP_PASSWORD)`, separando-a dos links assinados e ligando a sessão à senha.
3. O cookie tem validade de **7 dias**, `Path=/`, `HttpOnly`, `SameSite=Lax` e `Secure` quando a request chega por HTTPS (TLS direto ou `X-Forwarded-Proto: https`). Não há tabela de sessões; a assinatura e a expiração bastam.
4. **Rate limit em memória**: 5 falhas por minuto por IP. Com `BEHIND_CLOUDFLARE=true` o IP vem de `CF-Connecting-IP`; caso contrário, do endereço da conexão. Login bem-sucedido zera as falhas; bloqueio responde `429` com `Retry-After`.
5. **Rotas de gestão sem sessão** redirecionam com `303` para `/login`. O destino (`?next=`) só é preservado em GET/HEAD e apenas para caminhos locais; qualquer esquema, host, barra invertida ou caminho que decodifique para `//host` vira `/`.
6. A dependência nova é **`golang.org/x/crypto/argon2`**, já prevista em [`architecture.md`](../architecture.md). Nenhuma biblioteca de JWT, sessão ou CSRF é adicionada.

### Contrato do login

| Situação | Resposta |
|---|---|
| `GET /login` | `200` com formulário; com sessão válida, `303` para o destino seguro |
| senha correta | `303` para `next` (ou `/`) + `Set-Cookie` |
| senha errada | `401` com mensagem genérica, sem cookie |
| 5 falhas no minuto | `429` + `Retry-After`, sem cookie |
| formulário inválido | `400`; acima de 64 KiB, `413` |
| autenticação não configurada | `503` |

## Alternativas consideradas

| Alternativa | Por que não |
|---|---|
| bcrypt | limite de 72 bytes e sem dureza de memória; Argon2id é o recomendado atual |
| scrypt/PBKDF2 | mesma família de dependência, mas Argon2id tem parâmetros e resistência a GPU melhores |
| Sessões em tabela SQLite | revogação individual, mas exige escrita, limpeza e transações para um único usuário; o cookie assinado já expira |
| JWT de biblioteca | dependência e superfície maiores para um payload de duas partes |
| Cloudflare Access obrigatório | exige bypass para `/r/*` e `/api/actions/*` e quebra o uso local (`Q-003`) |
| Sessão em memória | some no restart e não escala nem para duas instâncias, sem ganho sobre o cookie assinado |
| Token de sessão opaco em cache | mais uma peça de estado para um cenário de usuário único |

## Consequências

### Positivas

- Sem estado de sessão no banco e sem migração; restart não desloga ninguém.
- Trocar `APP_SECRET` **ou** `APP_PASSWORD` invalida todas as sessões imediatamente.
- Argon2id com 19 MiB cabe no orçamento de memória; o custo só é pago no login.
- Rate limit e cookie não adicionam dependências além de `x/crypto`.

### Negativas / riscos

- Não há revogação individual de sessão: o cookie vale até expirar ou até rotacionar segredo/senha.
- O rate limit é por processo e zera no restart; uma única instância é premissa atual.
- `Secure` depende da detecção de HTTPS; deploy em HTTP puro (não recomendado) gera cookie sem `Secure`.
- Argon2id custa ~19 MiB e algumas dezenas de ms por tentativa; tentativas concorrentes são limitadas pelo rate limit.
- `SameSite=Lax` cobre CSRF de POST entre sites, mas não substitui um token CSRF se o modelo de navegador mudar.

### Mitigações

- Senha forte e `APP_SECRET` de 32+ bytes fora do Git (checklist em [`operations.md`](../operations.md)).
- TTL curto o suficiente para limitar cookie vazado; rotação documentada invalida tudo.
- Bloqueio de login com `Retry-After` e log de IP sem senha nem cookie.
- `Cache-Control: no-store`, CSP e `X-Robots-Tag: noindex` nas rotas de gestão.
- Testes de cookie adulterado/expirado, rate limit, `next` malicioso e flags do cookie.

## Referências

- [`docs/web-ui.md`](../web-ui.md) — rotas e contrato de autenticação.
- [`docs/operations.md`](../operations.md) — `APP_PASSWORD`, `APP_SECRET` e checklist de segurança.
- [`docs/requirements.md`](../requirements.md) — RF-074.
- [`ADR-0010`](./0010-cloudflare-dominio-proprio-links-assinados.md) — túnel, cookies e links assinados.
