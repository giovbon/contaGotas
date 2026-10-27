---
title: Spikes técnicos
status: ativo
updated: 2026-10-26
---

# Spikes

Investigações com prazo, feitas antes de comprometer implementação. Cada spike tem uma pergunta única, um método e um entregável. O resultado vai para o doc da área e, se mudar contrato, para um ADR.

| ID | Pergunta | Bloqueia | Status |
|---|---|---|---|
| `SPIKE-01` | O `go-fitz` expõe fonte/estilo por span de forma confiável em PDF? | extração de PDF (`FASE-0`) | aberto |
| `SPIKE-02` | Renderizar figuras de PDF compensa o custo de memória? | mídia de PDF (`FASE-3`) | aberto |
| `SPIKE-03` | O cross-compile CGO ARM64 produz binário estático funcional? | imagem (`FASE-4`) | aberto |
| `SPIKE-04` | Qual o pior caso de memória ao extrair PDF de 300 páginas? | orçamento de recursos | aberto |

## SPIKE-01 — API do go-fitz

**Pergunta:** conseguimos obter, por página, spans com `font-size`, negrito/itálico e posição (x, y) — o suficiente para inferir headings e colunas?

**Status:** em andamento (estrutura criada, aguardando ambiente com Go+CGO)

**Método:**
1. Fixar a versão do `go-fitz` (v1.24.14 adicionado).
2. Extrair `HTML(page)` e `Text(page)` de 3 PDFs de fixtures.
3. Verificar disponibilidade de API para spans/structured text.
4. Comparar com o resultado esperado (ground truth manual de 1 página).
5. Medir tempo e memória.

**Entregável:** nota em `extraction.md` com a API escolhida e resultados. Registrado em `docs/open-questions.md` Q-010.

**Critério de saída:** headings de 10 páginas identificados com ≥ 90% de acerto. API viável para coluna única e duas colunas.

## SPIKE-02 — Figuras de PDF

**Pergunta:** renderizar páginas/figuras como imagem aumenta muito o custo de memória e disco?

**Método:** renderizar 20 figuras a 150 dpi em ARM64, medir RSS de pico, tempo e tamanho total.

**Entregável:** decisão registrada em `open-questions.md` (`Q-006`).

**Critério de saída:** ≤ 200 MiB de pico e ≤ 20 MB de mídia por documento de 300 páginas.

## SPIKE-03 — Cross-compile ARM64

**Pergunta:** o build `linux/arm64` com CGO e libs do MuPDF gera binário estático que roda em distroless?

**Método:** `docker buildx build --platform linux/arm64`, rodar em runner ARM64 e em qemu; executar `healthcheck`, importar um PDF e um EPUB.

**Entregável:** Dockerfile validado em `operations.md` + job de CI.

**Critério de saída:** imagem ≤ 90 MB, `ldd` vazio, importação funcionando.

## SPIKE-04 — Memória de PDF

**Pergunta:** qual o pico real de memória em PDF de 300 páginas com imagens?

**Método:** `GOMEMLIMIT=96MiB`, medir `/proc/self/status` a cada 500 ms durante extração; repetir com 3 PDFs distintos.

**Entregável:** tabela em `architecture.md` (orçamento de recursos) revisada.

**Critério de saída:** pico ≤ 200 MiB; caso contrário, reduzir concorrência ou processar por páginas.

## Regras

- Spike não vira produto: o código descartável fica em `tools/spike-XX/` e não é importado por `internal/`.
- Prazo máximo de 1 dia por spike; se estourar, registrar o bloqueio em `open-questions.md`.
- Resultado sempre escrito, mesmo que negativo.
