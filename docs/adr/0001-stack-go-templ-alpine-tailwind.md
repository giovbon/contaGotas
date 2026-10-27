---
title: ADR-0001 — Stack Go + templ + Alpine + Tailwind
status: aceito
date: 2026-10-26
---

# ADR-0001 — Stack Go + templ + Alpine + Tailwind

## Contexto

O sistema roda em ARM64 de baixo recurso (SBC/VPS pequena), atrás de Cloudflare, com uma única pessoa mantendo. Precisa de: binário único, sem runtime Node, sem banco externo, UI server-side com interatividade pontual, e build reproduzível.

## Decisão

- **Go 1.24+** com `net/http` e mux nativo.
- **templ** para views server-side (type-safe, sem reflexão).
- **Alpine.js** vendorizado para interatividade local (upload, preview, edição).
- **Tailwind** compilado no build via CLI; CSS embutido com `go:embed`.
- Sem SPA, sem framework JS, sem CDN no runtime.

## Alternativas consideradas

| Alternativa | Por que não |
|---|---|
| React/Vue SPA + API JSON | dobra a superfície, exige Node no build, pior consumo e complexidade |
| HTMX | viável, mas Alpine cobre os casos locais sem servidor extra por interação |
| `html/template` puro | sem tipagem de componentes; templ dá composição e checagem |
| Bootstrap/Material | peso e opinião visual; Tailwind permite CSS mínimo tree-shaken |
| Serverless | contraria o requisito self-hosted/offline e o agendador in-process |

## Consequências

### Positivas
- Um artefato para deploy; sem Node/Python no runtime.
- Consumo previsível, cold start baixo.
- Views e handlers no mesmo repositório, fáceis de revisar.
- CSP restritiva (`script-src 'self'`) viável.

### Negativas / riscos
- `templ generate` adiciona passo de build.
- Alpine exige disciplina para não virar SPA acidental.
- Tailwind precisa de `@source` correto para não podar classes.

### Mitigações
- Build em Docker multi-stage com versões pinadas.
- JS progressivo: formulários funcionam sem Alpine.
- Teste de presença das classes críticas no CSS final.
