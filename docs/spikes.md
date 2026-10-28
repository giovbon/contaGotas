---
title: Spikes técnicos
status: ativo
updated: 2026-10-27
---

# Spikes

Investigações com prazo, feitas antes de comprometer implementação. Cada spike tem uma pergunta única, um método e um entregável. O resultado vai para o doc da área e, se mudar contrato, para um ADR.

| ID | Pergunta | Bloqueia | Status |
|---|---|---|---|
| `SPIKE-01` | O `go-fitz` expõe fonte/estilo por span de forma confiável em PDF? | extração de PDF (`FASE-0`) | concluído (2026-10-27) |
| `SPIKE-02` | Renderizar figuras de PDF compensa o custo de memória? | mídia de PDF (`FASE-3`) | aberto |
| `SPIKE-03` | O cross-compile CGO ARM64 produz binário estático funcional? | imagem (`FASE-4`) | aberto |
| `SPIKE-04` | Qual o pior caso de memória ao extrair PDF de 300 páginas? | orçamento de recursos | parcial (2026-10-27) |

## SPIKE-01 — API do go-fitz

**Pergunta:** conseguimos obter, por página, spans com `font-size`, negrito/itálico e posição (x, y) — o suficiente para inferir headings e colunas?

**Status:** concluído (2026-10-27)

**Método:**
1. Fixar a versão do `go-fitz` (v1.24.14).
2. Extrair `HTML(page, false)` e `Text(page)` das fixtures PDF.
3. Verificar disponibilidade de API para spans/structured text.
4. Comparar com o resultado esperado (ground truth sintético).
5. Medir tempo e memória.

**Resultado:**
- `go-fitz v1.24.14` não expõe spans crus (não há `StructuredText`/`Span`). Expõe `Text(page)` e `HTML(page, header)`.
- O HTML estruturado traz uma linha visual por `<p style="top;left;line-height">`, com `font-size`/`font-weight` nos spans. É suficiente para posição, colunas, títulos e negrito.
- `Text` é o fallback; não carrega posição.
- Tudo exige CGO; sem CGO o pacote usa stub com erro claro.
- Fixture `headings-10pg.pdf`: 20/20 títulos detectados (100%) e nenhum falso positivo.
- `duas-colunas.pdf`: ordem de leitura correta mesmo com o fluxo do conteúdo intercalado.
- Memória de PDF de 300 páginas não foi medida aqui: fica para `SPIKE-04`.

**Entregável:** implementação em `internal/docs/pdf` e nota em [`extraction.md`](./extraction.md). Q-010 fechada com o critério adotado.

**Critério de saída:** headings de 10 páginas identificados com ≥ 90% de acerto. API viável para coluna única e duas colunas. **Atendido** com a fixture sintética (100%).

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

**Resultado parcial (2026-10-27, x86_64):**

| PDF | Páginas | Tamanho | Pico RSS | Tempo |
|---|---|---|---|---|
| Attention (arXiv 1706.03762) | 15 | 2,2 MB | 41 MB | 0,8 s |
| ResNet (arXiv 1512.03385) | 12 | 801 KB | 22 MB | 0,4 s |
| GPT-3 (arXiv 2005.14165) | 75 | 6,5 MB | 325 MB | 8,5 s |

- O pico não acompanha linearmente o número de páginas: acompanha as páginas com imagens grandes. `doc.HTML` embute cada imagem como PNG em base64, mesmo sem uso na FASE-0; a página 6 do GPT-3 (560 KB de HTML) sozinha levou o pico de 77 MB para 189 MB.
- `fitz.MaxStore = 32 MiB` (padrão 256 MiB) baixa o pico do GPT-3 para ~229 MB, sem custo de tempo.
- `GOGC=50`/`20` e `GOMEMLIMIT` não mudam o pico: a alocação é do lado C, fora do heap Go.
- Ainda **não** atende ao critério de ≤ 200 MiB para documentos com figuras grandes. Falta medir em ARM64 e com um PDF de 300 páginas; a hipótese é que o store de 256 MiB imponha um teto, mas páginas isoladas com imagens muito grandes podem estourar. Alternativa em avaliação: extrair texto sem `HTML()` (exigiria API própria sobre o MuPDF). Ver `Q-016`.

## Regras

- Spike não vira produto: o código descartável fica em `tools/spike-XX/` e não é importado por `internal/`.
- Prazo máximo de 1 dia por spike; se estourar, registrar o bloqueio em `open-questions.md`.
- Resultado sempre escrito, mesmo que negativo.
