---
title: ADR-0002 — Modelo de blocos normalizado
status: aceito
date: 2026-10-26
---

# ADR-0002 — Modelo de blocos normalizado

## Contexto

Markdown, EPUB e PDF têm estruturas diferentes. Particionar por caracteres, offsets ou páginas quebra frases, headings e blocos de código, e torna o plano frágil a reprocessamentos. O mesmo algoritmo precisa servir aos três formatos.

## Decisão

Todo formato é convertido para uma lista ordenada de `Block` (heading, parágrafo, item, código, tabela, citação, figura, nota), com:

- `ord` contíguo;
- `kind`, `level`, `html`;
- `words` e `seconds`;
- `hash` do texto normalizado;
- `heading_id` ligando ao nó da árvore.

O particionamento opera **exclusivamente** sobre blocos; chunks guardam `first_block_id`/`last_block_id` e os hashes das pontas.

## Alternativas consideradas

| Alternativa | Por que não |
|---|---|
| Offset de caractere no texto extraído | quebra no meio de palavra; invalida ao reprocessar |
| Página de PDF / arquivo do EPUB | granularidade errada; capítulos variam |
| AST nativa de cada formato | três algoritmos, três conjuntos de bugs |
| Texto puro com separador `\n\n` | perde tipo, heading e estrutura |

## Consequências

### Positivas
- Um segmentador para todos os formatos, testável isoladamente.
- Âncoras estáveis por hash; detecta plano obsoleto.
- Renderização no leitor reaproveita o `html` do bloco.
- Facilita preview e edição de fronteiras.

### Negativas / riscos
- Extração precisa ser boa o suficiente para não gerar blocos ruins.
- PDF sem estrutura confiável produz headings heurísticos.
- Armazenamento duplica texto (`text` + `html`).

### Mitigações
- Golden tests por formato com fixtures reais.
- Heurísticas de PDF documentadas e calibráveis.
- Compressão do SQLite e retenção consciente.
