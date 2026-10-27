---
title: ADR-0006 — Documentos em paralelo
status: aceito
date: 2026-10-26
---

# ADR-0006 — Documentos em paralelo

## Contexto

O usuário quer ler vários livros ao mesmo tempo. A alternativa natural é uma fila sequencial (um livro por vez), mais simples de compor a mensagem e de manter contexto.

## Decisão

Cada documento tem **plano, pacing e cursor próprios**. Uma run diária seleciona o próximo chunk `pending` de cada documento `ready`, na ordem de `priority`, até `max_docs_per_run`.

A mensagem agrupa os chunks em seções `## 📖 {documento}` separadas por `---`. Se exceder o limite de bytes, o envio é dividido por documento.

## Alternativas consideradas

| Alternativa | Por que não |
|---|---|
| Fila sequencial | impede ler dois livros; objetivo explícito do usuário |
| Rodízio (um doc por dia) | cada livro avança devagar; pior continuidade |
| Um topic ntfy por documento | mais configuração; fica para `Q-011` |
| Mensagens separadas sempre | poluição; agrupar é melhor por padrão |

## Consequências

### Positivas
- Usuário lê o que quiser, no ritmo de cada obra.
- Falha em um documento não bloqueia os outros.
- Pausar um livro não afeta os demais.

### Negativas / riscos
- Mensagem diária maior; risco de truncamento.
- Contexto dividido entre obras (recap por documento resolve).
- Ordem/prioridade precisa ser gerenciável na UI.

### Mitigações
- Limite de bytes + split por documento.
- Recap e breadcrumb sempre por documento.
- `priority` editável e `max_docs_per_run`.
- Aviso na UI quando a mensagem estimada passar do limite.
