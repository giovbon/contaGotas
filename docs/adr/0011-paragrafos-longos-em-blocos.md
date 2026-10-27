---
title: ADR-0011 — Parágrafos longos em blocos
status: aceito
date: 2026-10-26
---

# ADR-0011 — Parágrafos longos em blocos

## Contexto

Textos reais contêm parágrafos patologicamente longos (traduções, textos técnicos, PDFs mal segmentados). Se um parágrafo sozinho ocupa mais que uma dose, o segmentador precisa cortá-lo. A invariante I-01 proíbe chunks por offset de caractere: um chunk é sempre um intervalo contíguo de blocos com hash nas pontas.

Cortar dentro do bloco no momento do plano exigiria offsets, blocos virtuais ou um schema paralelo — três formas de quebrar a simplicidade do modelo.

## Decisão

Parágrafos com mais de `max_para_words` palavras (padrão **300**) são pré-divididos em vários blocos `p` durante `blocks.Prepare`, sempre em fronteira de frase:

- `SplitSentences` respeita abreviações, iniciais e decimais;
- uma frase sozinha maior que o limite permanece inteira;
- cada pedaço recebe `ord`, `words`, `seconds` e `hash` próprios;
- o HTML dos pedaços é reconstruído como `<p>` com texto escapado (formatação inline do parágrafo original é perdida apenas nesses casos patológicos).

O segmentador continua proibido de cortar dentro de um bloco. Se um pedaço ainda exceder `max`, ele é tratado como indivisível e o chunk vira `oversized`.

## Alternativas consideradas

| Alternativa | Por que não |
|---|---|
| Offset de caractere no bloco | viola I-01; torna retomada e hash frágeis |
| Blocos virtuais só no plano | schema e materialização paralelos; duas fontes de verdade |
| Não dividir e aceitar `oversized` | doses de 20+ minutos em textos reais |
| Reescrever o parágrafo | o sistema não deve alterar o autor |

## Consequências

### Positivas
- Mantém I-01 e o modelo de chunks como intervalo de blocos.
- Um parágrafo gigante nunca força uma dose gigante.
- A divisão é determinística e testável isoladamente.

### Negativas / riscos
- Blocos deixam de ser 1:1 com parágrafos do arquivo original.
- Formatação inline é perdida nos parágrafos divididos.
- O limite de 300 palavras é uma heurística; pode precisar de calibração por idioma/formato.

### Mitigações
- Limite configurável (`MaxParagraphWords`).
- Testes de `SplitSentences` com abreviações, decimais e aspas.
- O HTML original é preservado para todos os parágrafos que não precisam de divisão.

## Referências

- [ADR-0002](./0002-modelo-de-blocos-normalizado.md)
- [Segmentação](../segmentation.md)
- [Extração](../extraction.md)
- `internal/blocks/normalize.go`
