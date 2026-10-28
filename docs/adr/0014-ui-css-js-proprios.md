---
title: ADR-0014 — UI da FASE-1 com templ e CSS/JS próprios
status: aceito
date: 2026-10-27
---

# ADR-0014 — UI da FASE-1 com templ e CSS/JS próprios

## Contexto

O [ADR-0001](./0001-stack-go-templ-alpine-tailwind.md) definiu Go + templ + Alpine.js + Tailwind para a UI. A primeira entrega das telas de biblioteca, detalhe e preview do plano é somente leitura: lista, status, tabela e download do original. Não há uploader com progresso, edição de fronteiras nem componentes com estado local que justifiquem Alpine nesta etapa, e Tailwind exigiria Node no build antes de a interface estabilizar.

O requisito de binário único, CSP `script-src 'self'` e funcionamento sem CDN continua valendo, assim como a proibição de dependência nova sem ADR.

## Decisão

- Manter **templ** para todas as páginas e componentes server-side.
- Usar um **CSS próprio** (`internal/web/assets/app.css`) e um **JS próprio** (`internal/web/assets/app.js`), embutidos com `go:embed` e servidos em `/static/`.
- Não incluir Alpine.js nem Tailwind na FASE-1; a interatividade atual (polling de status por recarga da própria página) é feita com `fetch` + `DOMParser`, sem framework.
- Reavaliar Alpine/Tailwind no incremento que introduzir uploader com progresso, preview editável ou edição de fronteiras; a decisão de adotá-los exige revisão deste ADR.

## Alternativas consideradas

| Alternativa | Por que não |
|---|---|
| Tailwind já nesta entrega | adiciona Node e etapa de build para telas ainda em movimento; o CSS atual cabe em um arquivo revisável |
| Alpine.js já nesta entrega | não há estado local complexo; o polling cabe em poucas linhas de JS progressivo |
| CSS/JS inline nas páginas | CSP atual não permite `unsafe-inline`; arquivos externos embutidos preservam `script-src 'self'` |
| CDN para CSS/JS | contraria I-09 e o requisito de operação offline |

## Consequências

### Positivas
- Build Go continua autocontido: sem Node, sem download de runtime, sem CDN.
- CSS/JS versionados e embutidos no binário; CSP restritiva preservada.
- Superfície menor para revisar enquanto as telas mudam.

### Negativas / riscos
- CSS próprio tende a crescer e precisa de disciplina para não virar um framework caseiro.
- Sem Alpine, interações mais ricas (drag-and-drop, edição de fronteiras) exigirão JS manual ou a adoção posterior do Alpine.
- O ADR-0001 permanece aceito para templ e para a direção geral; Alpine/Tailwind ficam adiados, não descartados.

### Mitigações
- Manter o CSS organizado por blocos e sem dependências externas.
- JS progressivo: as telas funcionam sem JavaScript; o polling é melhoria, não requisito.
- Revisar a decisão ao iniciar o uploader com progresso e o editor de plano.

## Referências

- [ADR-0001](./0001-stack-go-templ-alpine-tailwind.md)
- [`docs/web-ui.md`](../web-ui.md)
