---
title: ADRs
status: ativo
updated: 2026-10-26
---

# Architecture Decision Records

Cada decisão estrutural tem um arquivo próprio. ADRs são **imutáveis após aceite**: para mudar, crie um novo e marque o antigo como `substituído`.

## Índice

| ADR | Decisão | Status |
|---|---|---|
| [0001](./0001-stack-go-templ-alpine-tailwind.md) | Stack Go + templ + Alpine + Tailwind em binário único | aceito |
| [0002](./0002-modelo-de-blocos-normalizado.md) | Particionar sobre blocos normalizados | aceito |
| [0003](./0003-pdf-com-go-fitz-cgo.md) | PDF via go-fitz com CGO isolado | aceito |
| [0004](./0004-plano-global-persistido.md) | Plano global persistido e determinístico | aceito |
| [0005](./0005-ntfy-self-hosted-mensagem-completa.md) | ntfy self-hosted com mensagem completa + link | aceito |
| [0006](./0006-documentos-em-paralelo.md) | Documentos em paralelo | aceito |
| [0007](./0007-sqlite-modernc-wal.md) | SQLite modernc em WAL | aceito |
| [0008](./0008-build-arm64-distroless.md) | Imagem ARM64 distroless | aceito |
| [0009](./0009-dois-modos-de-ritmo.md) | Dois modos de ritmo | aceito |
| [0010](./0010-cloudflare-dominio-proprio-links-assinados.md) | Cloudflare + domínio próprio + links assinados | aceito |
| [0011](./0011-paragrafos-longos-em-blocos.md) | Parágrafos longos pré-divididos em blocos | aceito |
| [0012](./0012-epub-zip-xhtml.md) | EPUB com archive/zip e x/net/html | aceito |
| [0013](./0013-sessao-hmac-argon2id.md) | Sessão por cookie HMAC e senha única com Argon2id | aceito |
| [0014](./0014-ui-css-js-proprios.md) | UI da FASE-1 com templ e CSS/JS próprios (Alpine/Tailwind adiados) | aceito |

## Template

```markdown
---
title: ADR-0000 — Título
status: proposto | aceito | substituído | obsoleto
date: AAAA-MM-DD
supersedes: 
superseded_by: 
---

# ADR-0000 — Título

## Contexto
Qual problema, restrição ou força em jogo.

## Decisão
O que foi decidido, em uma frase, e os detalhes necessários para implementar.

## Alternativas consideradas
| Alternativa | Por que não |
|---|---|

## Consequências
### Positivas
### Negativas / riscos
### Mitigações

## Referências
```

## Regras

1. Um ADR por decisão. Não agrupe “stack” e “deploy”.
2. Contexto sem opinião; decisão no imperativo.
3. Sempre listar alternativas reais — inclusive “não fazer nada”.
4. Consequências negativas são obrigatórias.
5. Não editar ADR aceito; criar substituto e atualizar o índice.
