---
title: ADR-0010 — Cloudflare + domínio próprio + links assinados
status: aceito
date: 2026-10-26
---

# ADR-0010 — Cloudflare + domínio próprio + links assinados

## Contexto

O sistema será exposto em um domínio próprio, atrás da Cloudflare. A notificação precisa abrir o leitor no celular sem exigir login toda vez, e o origin não deve expor portas.

## Decisão

1. Exposição via **Cloudflare Tunnel** (`cloudflared`), sem portas abertas no host.
2. `APP_BASE_URL` sempre `https://leitura.exemplo.com`; links e cookies usam essa base.
3. **Leitor acessível por link assinado**: `GET /r/{chunkID}?t={token}`, HMAC-SHA256 com `APP_SECRET`, escopo `r`, TTL padrão 180 dias.
4. Ação “marcar lido”: token com escopo `done`, TTL 30 dias, `POST` only.
5. Auth do app (senha única + sessão) para a UI de gestão; sem Cloudflare Access por padrão.
6. `BEHIND_CLOUDFLARE=true` → confiar em `CF-Connecting-IP` e exigir `X-Forwarded-Proto: https` para cookies `Secure`.
7. Cloudflare **não** cacheia HTML; `/media/*` e assets com hash usam cache imutável.

### Formato dos links

| Uso | URL |
|---|---|
| Leitor | `https://leitura.exemplo.com/r/42?t={hmac}` |
| Marcar lido | `https://leitura.exemplo.com/api/actions/chunks/42/done?t={hmac}` |
| ntfy (publish) | interno: `http://ntfy:8080` |
| ntfy (público) | `https://ntfy.exemplo.com` (só para o app do celular) |

## Alternativas consideradas

| Alternativa | Por que não |
|---|---|
| VPS com portas abertas + Let's Encrypt | mais superfície e manutenção de TLS |
| Tailscale apenas | ótimo para uso pessoal, mas o usuário quer domínio próprio |
| Cloudflare Access obrigatório | exige bypass para as ações do ntfy e piora a UX do link |
| Link sem token + login | fricção a cada notificação no celular |
| Token permanente | vazamento sem expiração |

## Consequências

### Positivas
- Sem inbound ports; TLS e DDoS na borda.
- Link abre direto no leitor, sem senha, com escopo mínimo.
- Funciona bem no celular e pode ser revogado rotacionando `APP_SECRET`.

### Negativas / riscos
- Dependência da Cloudflare (disponibilidade e política de uso).
- Limite de upload de 100 MiB e timeout de 100 s no plano free → upload ≤ 95 MiB e parse assíncrono.
- Token na URL pode vazar via histórico/compartilhamento.
- WAF/bot fight pode bloquear o POST da ação do ntfy.
- `APP_SECRET` comprometido forja links válidos.

### Mitigações
- `Cache-Control: no-store` + `X-Robots-Tag: noindex` no leitor.
- TTL curto para o token de ação; escopo separado.
- Bypass explícito em `/r/*` e `/api/actions/*` quando Access estiver ativo.
- Parse sempre assíncrono; limite de upload no app.
- Rotação de segredo documentada (invalida links, regeneráveis).
- Monitorar 5xx do túnel e manter backup local.
