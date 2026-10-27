---
title: ADR-0005 — ntfy self-hosted com mensagem completa + link
status: aceito
date: 2026-10-26
---

# ADR-0005 — ntfy self-hosted com mensagem completa + link

## Contexto

Um chunk de 10 minutos tem ~12–14 KB. O ntfy.sh limita mensagens a 4.096 bytes e anexos expiram em 3 h. O usuário opera ntfy self-hosted e terá domínio próprio atrás de Cloudflare.

Três estratégias eram possíveis: corpo completo, anexo `.md` ou link para o leitor.

## Decisão

1. **ntfy self-hosted** com `message-size-limit: 64k`.
2. Corpo completo em Markdown (`markdown: true`) com breadcrumb, recap e rodapé.
3. **Link para o leitor** no domínio próprio via `click` + ação “Abrir leitor”.
4. Ação “Marcar como lido” via `http` action para endpoint assinado.
5. Fallback automático para `preview` + link quando o corpo exceder `max_message_bytes`.

## Alternativas consideradas

| Alternativa | Por que não |
|---|---|
| Anexo `.md` | leitura desconfortável no celular; expira em 3 h |
| Só link | perde a leitura imediata na notificação |
| Só corpo | sem contexto completo, sem imagens, sem navegação |
| Várias mensagens por dia | polui a central de notificações |

## Consequências

### Positivas
- Leitura imediata, sem abrir nada.
- Link resolve contexto, imagens e trechos truncados.
- Ação “marcar lido” fecha o ciclo de progresso.
- Independente do ntfy.sh.

### Negativas / riscos
- Exige operar ntfy próprio (backup, upgrade, exposição).
- Link depende de Cloudflare/Tunnel disponível.
- Token na URL pode vazar se compartilhado.
- Notificação muito longa pode ser truncada por clientes antigos.

### Mitigações
- Modo `preview` configurável.
- Split por documento quando ultrapassar o limite.
- Token escopado, com TTL e regenerável.
- `noindex` e rate limit no leitor.
