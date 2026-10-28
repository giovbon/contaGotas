---
title: Extração
status: ativo
updated: 2026-10-27
---

# Extração

Objetivo: converter qualquer entrada em `[]blocks.Block` com hierarquia e tempo, preservando o suficiente para renderizar no leitor.

## Contrato

O núcleo comum é `blocks.Block`. Cada extrator devolve blocos **crus** (tipo, nível, texto, HTML); `blocks.Prepare` completa `ord`, palavras, segundos, hash e `heading_id`.

Implementação atual (FASE-0):

```go
// internal/docs
func Detect(name string) (Format, bool)

// internal/docs/markdown
func Extract(src []byte, fallbackTitle string) (*Result, error)

// internal/docs/epub
func Extract(src []byte, fallbackTitle string) (*Result, error)

// internal/docs/pdf (build tag cgo)
func Extract(r io.ReaderAt, size int64, fallbackTitle string) (*Result, error)

type markdown.Result struct {
    Title  string
    Blocks []blocks.Block
}

type epub.Result struct {
    Title    string
    Author   string
    Language string
    Blocks   []blocks.Block
}
```

O contrato completo planejado para EPUB/PDF é:

```go
type Extractor interface {
    Extract(ctx context.Context, src io.ReaderAt, opts Options) (*Result, error)
}

type Result struct {
    Title    string
    Author   string
    Language string
    Blocks   []Block
    Media    []MediaAsset // imagens, capa
}
```

Regras válidas para os três formatos:

1. `ord` contíguo a partir de 0 (atribuído em `Prepare`).
2. `hash = sha256(kind + "\x00" + textNormalizado)[:16]`.
3. Blocos vazios ou só com espaços são descartados (exceto `hr`).
4. `html` é gerado apenas com markup permitido. No Markdown, o goldmark roda sem `WithUnsafe`, então HTML cru é omitido; a injeção de `rel="noopener noreferrer"` em links externos ainda não está implementada nesse extrator. No EPUB, o HTML é reconstruído por whitelist, com `rel` em links e sem `on*`/`script`/`style`.
5. Notas de rodapé viram um bloco `footnote` (Markdown: a lista inteira no fim do documento; EPUB: cada `aside`/`epub:type` de nota). A ligação por referência fica para a FASE-1.
6. Nenhuma transformação depende de rede.
7. Parágrafos acima de `max_para_words` (300) são pré-divididos em blocos `p` alinhados a frases — ver [ADR-0011](./adr/0011-paragrafos-longos-em-blocos.md).


## Markdown (`.md`)

Pipeline: `goldmark` (GFM + footnotes) → AST → walk.

| Nó | Bloco |
|---|---|
| `Heading` | `h1..h6`, `level` |
| `Paragraph` | `p` |
| `List` / `ListItem` | `li` (um bloco por item; listas aninhadas ficam no HTML do item) |
| `FencedCodeBlock` / `CodeBlock` | `code` (texto cru; linguagem preservada apenas no `class` do HTML) |
| `Blockquote` | `quote` |
| `Table` | `table` (HTML completo preservado) |
| `Image` | `figure` quando o parágrafo contém exatamente uma imagem (alt + HTML) |
| `ThematicBreak` | `hr` |
| `FootnoteList` | `footnote` (lista inteira em um bloco, no fim do documento) |
| `HTMLBlock` / `RawHTML` | omitido (goldmark sem `WithUnsafe`) |

Título: primeiro `h1`; se ausente, nome do arquivo sem extensão. Autor e front matter YAML ainda não são extraídos.

Parágrafos com mais de 300 palavras são divididos em vários blocos `p` por `blocks.Prepare`; os pedaços perdem formatação inline (o HTML é reconstruído como `<p>` com texto escapado).


## EPUB (`.epub`)

Pipeline implementado ([ADR-0012](./adr/0012-epub-zip-xhtml.md)):

```
archive/zip → META-INF/container.xml → rootfile (.opf, encoding/xml)
    → metadata (dc:title, dc:creator, dc:language)
    → manifest + spine (ordem de leitura)
    → cada XHTML: x/net/html → walk do <body> → sanitização
```

Regras:
- `linear="no"` é ignorado, exceto quando todo o spine é não linear;
- itens com `properties="nav"` e mídia que não é XHTML/HTML são ignorados;
- o mesmo arquivo não é extraído duas vezes;
- `href` é resolvido contra o diretório do OPF, decodificado e limitado ao ZIP;
- limites: 2 MiB por metadado, 8 MiB por XHTML, 64 MiB de XHTML por EPUB;
- título do OPF; se ausente, primeiro heading; se ainda ausente, nome do arquivo.

Mapeamento HTML → blocos:

| Elemento | Bloco |
|---|---|
| `h1..h6` | heading com `level` |
| `p`, `dt`, `dd` | `p` |
| `li` | `li` (um bloco por item; listas aninhadas no HTML do item) |
| `pre` (`code` interno) | `code` (linguagem via `class="language-*"`) |
| `blockquote` | `quote` |
| `table` | `table` |
| `figure`, imagem isolada | `figure` (alt + legenda; sem `src` por enquanto) |
| `aside[epub:type=footnote/endnote]`, `[role=doc-footnote]` | `footnote` |
| `hr` | `hr` |
| `script`, `style`, `nav`, `head`, `iframe`, `svg`, `math` | ignorado |

Sanitização:
- whitelist de tags e atributos; `on*` nunca é copiado;
- links só com `http`, `https`, `mailto`, `tel`, esquema vazio ou âncora; `javascript:`, `data:`, `file:` são descartados;
- todo link recebe `rel="noopener noreferrer"`;
- texto e atributos são escapados.

Ainda não implementado: copiar imagens/capa para `/data/documents/{id}/media/`, usar o TOC para enriquecer títulos e ignorar páginas de copyright por `epub:type`. O README/índice é ignorado apenas quando marcado como `nav`.

## PDF (`.pdf`)

Pipeline: `go-fitz` (MuPDF) → por página → HTML estruturado → linhas com posição → remoção de cabeçalho/rodapé → colunas → parágrafos → blocos.

### API usada (`SPIKE-01`)

- `go-fitz v1.24.14` **não expõe spans crus**. Expõe `Text(page)` e `HTML(page, header)`.
- O HTML estruturado traz um `<p style="top:...pt;left:...pt;line-height:...pt">` por linha visual, com `font-size` e `font-weight` nos spans. É a fonte de posição e estilo usada pelo extrator.
- Tudo depende de CGO; sem a tag `cgo` o pacote compila com stub e `Extract` retorna erro claro.
- A memória em PDF de 300 páginas ainda será medida no `SPIKE-04`.

### Ordem de leitura e parágrafos

1. Parsear `HTML(page)` com `x/net/html`; cada `<p>` vira uma `Line` com `x`, `y`, `line-height`, tamanho de fonte e negrito.
2. Descartar as bordas da página (4% de cada lado — marcas d’água, numeração de linha) e cabeçalho/rodapé repetidos: texto idêntico em > 40% das páginas nos 10% superiores/inferiores, ou número de página isolado nessa faixa.
3. Detectar colunas por clustering de `x`: origens a até 12pt formam o mesmo grupo; grupos separados por menos de 36pt são fundidos (recuo de primeira linha); grupos residuais vão para a coluna mais próxima. Se a página tem título grande (fonte ≥ 1,4× a mediana do corpo), ela é tratada como folha de rosto: o bloco acima da primeira linha do corpo é lido de cima para baixo (título, autores, resumo) e o restante por colunas. Com 3+ grupos fortes, o layout é irregular e a página inteira cai para a ordem por `y`.
4. Fundir fragmentos da mesma linha visual que o MuPDF emitiu em `<p>` separados (ex.: `3.1` e `Encoder and Decoder Stacks`): `|Δy| ≤ 1pt` e vão horizontal ≤ 120pt.
5. Ordenar por `(coluna, y)`. Páginas são processadas em ordem; a mudança de coluna dentro da página encerra o parágrafo.
6. Juntar linhas em parágrafos:
   - desfazer hifenização (`-` no fim + minúscula → concatena);
   - quebrar quando o vão vertical > 1,4× a entrelinha mediana, quando o tamanho da fonte muda, quando a coluna/página muda (salvo frase em aberto), ou quando a linha anterior termina em pontuação final e a próxima começa com maiúscula e recuo;
   - remover espaços duplos e soft hyphens (`U+00AD`).
7. Detectar títulos por:
   - tamanho de fonte ≥ 1,15× a mediana do corpo **e** (≥ 1,4× **ou** negrito **ou** até 60 caracteres) — blocos de aviso/licença usam a mesma fonte de um título, mas são longos, e não viram título;
   - negrito + linha curta (< 60 caracteres) + numeração (`^\d+(\.\d+)*\.?\s+\p{Lu}`, `^Cap[íi]tulo\b`, `^Parte\b`) ou caixa alta.
   Linhas dominadas por números/símbolos (rótulos de tabela como `0.3 300K 4.33`) são descartadas. Tamanhos maiores que o título da primeira página (× 1,05) são tratados como rótulos de figura e não viram título. Tamanhos distintos são agrupados com tolerância de 8% e ranqueados do maior para o menor (maior → h1); títulos só com numeração usam a profundidade (`1.2.3` → h3).

### Validação com PDFs reais (2026-10-27)

Revisão manual de saída da FASE-0 com três artigos do arXiv. Os arquivos ficam fora do repositório (`/tmp/opencode/pdf-review/`) por causa de licença; os testes automatizados usam as fixtures sintéticas de `testdata/pdf/`.

| Documento | Origem | Páginas | Tamanho | Blocos | Títulos | Palavras | Tempo | Pico RSS |
|---|---|---|---|---|---|---|---|---|
| Attention Is All You Need | arXiv 1706.03762 | 15 | 2,2 MB | 279 | 33 | 6 066 | 0,8 s | 41 MB |
| Deep Residual Learning | arXiv 1512.03385 | 12 | 801 KB | 531 | 36 | 9 639 | 0,4 s | 22 MB |
| Language Models are Few-Shot Learners | arXiv 2005.14165 | 75 | 6,5 MB | 1 518 | 95 | 37 912 | 8,5 s | 325 MB |

Medido em Linux x86_64 (`VmHWM` do processo de teste); ARM64 continua pendente no `SPIKE-04`.

O que a revisão confirmou:

- título do documento extraído do corpo (não da marca d’água do arXiv) nos três casos;
- corpo em duas colunas na ordem certa, com parágrafos contínuos;
- títulos numerados que o MuPDF quebra em `<p>` separados (`3.1` + `Encoder and Decoder Stacks`) voltam a ser um único título;
- cabeçalhos corridos e marcas d’água removidos;
- hifenização remontada (`sequencealigned` etc.).

O que ainda falha:

- rótulos e eixos dentro de figuras vetoriais vazam como parágrafos soltos (`weight layer`, `relu`, `F(x)`); figuras não são separadas do fluxo de texto;
- sumário (GPT-3) vira uma sequência de parágrafos longos e alguns títulos com nível errado;
- grade de autores é lida em ordem aproximada; nomes vizinhos da mesma linha podem ser fundidos;
- notas de rodapé permanecem no fluxo da página;
- memória: `doc.HTML` embute as imagens da página como PNG em base64 — que a FASE-0 não usa. O GPT-3 chega a 325 MB de pico por causa disso; reduzir `fitz.MaxStore` para 32 MB baixa para ~229 MB, e `GOGC`/`GOMEMLIMIT` não mudam o pico (a alocação é do lado C). Ver `Q-016`.

### Fallback e falhas

- Página sem HTML estruturado cai para `Text(page)`; documento inteiro sem HTML usa `blocosDeTexto` (parágrafos separados por linha em branco, títulos só por numeração).
- PDF escaneado: se < 50 caracteres por página em média → `pdf.ErrScanned` (`Q-002`), que o worker traduz para `needs_ocr`.
- PDF inválido, vazio ou sem blocos → erro descritivo para `documents.error`.

### Limitações conhecidas

- Tabelas complexas: preservadas como texto; sem reconstrução de células.
- Fórmulas matemáticas: texto linearizado.
- Notas de rodapé na base da página: mantidas no fluxo, ao fim da página.
- Figuras: ignoradas nesta fase (`SPIKE-02`); rótulos internos de figuras vetoriais podem virar parágrafos.
- Sumário/TOC de PDF: não é detectado nem descartado; entra como texto (ver GPT-3 na revisão real).
- Layouts com 3+ colunas ou caixas: funcionam quando o gutter passa de 36pt; qualidade degradada em layouts irregulares.
- A ordem de leitura depende das posições do HTML do MuPDF; PDFs com o fluxo de conteúdo fora da ordem visual continuam sendo reordenados pelas colunas.
- Pico de memória cresce com páginas que têm imagens grandes, porque `HTML()` as embute em base64 (`Q-016`).

### Implementação

| Arquivo | Papel |
|---|---|
| `internal/docs/pdf/extract_cgo.go` | cópia para temporário, orquestração, fallback e `needs_ocr` |
| `internal/docs/pdf/extract_spans.go` | parse do HTML estruturado, remoção de cabeçalho/rodapé |
| `internal/docs/pdf/col.go` | clustering de colunas e ordenação |
| `internal/docs/pdf/heading.go` | detecção e nivelamento de títulos |
| `internal/docs/pdf/blocks.go` | junção de linhas, parágrafos e fallback textual |

### Despacho por arquivo (FASE-1)

`docs.ExtractFile(ctx, path, fallbackTitle)` detecta o formato pela extensão, lê o arquivo e devolve um `docs.Result` comum (`Title`, `Author`, `Language`, `Blocks`). PDF é lido por `ReaderAt` + tamanho; sem CGO o stub devolve erro claro. A CLI e o worker de ingestão usam esse mesmo caminho.

- `pdf.ErrScanned` é uma sentinela comparável com `errors.Is`; o worker usa isso para marcar `needs_ocr` sem comparar strings.
- O contexto é observado antes da leitura/extração e ao final; cancelamento devolve `context.Canceled` sem marcar o documento como `failed`.
- Metadados vazios não apagam o que já está no documento: título, autor e idioma só são sobrescritos quando a extração encontra valor.

## Normalização de texto (todos os formatos)

```go
func NormalizeText(s string) string {
    s = norm.NFC.String(s)            // unicode
    s = strings.ReplaceAll(s, "\u00ad", "")
    s = collapseSpaces(s)             // exceto em code
    s = strings.TrimSpace(s)
    return s
}

// code: NFC, CRLF → LF, sem colapsar espaços nem remover indentação
func NormalizeCode(s string) string
```

`blocks.Prepare` aplica `NormalizeText`/`NormalizeCode`, divide parágrafos longos, estima `words`/`seconds`, calcula o hash e liga cada bloco ao heading que o possui.


## Mídia

| Origem | Destino | Estado |
|---|---|---|
| EPUB `img` | `media/{n}.{ext}` | planejado para a FASE-1; FASE-0 preserva apenas `alt` |
| PDF figura | `media/page-{n}-{k}.{ext}` | planejado (`SPIKE-02`) |
| Markdown `![alt](url)` | — | remoto não é baixado; vira link |
| Capa | `cover.{ext}` | planejado |

Limite futuro: 200 MB de mídia por documento; excedente é ignorado com aviso.

## Falhas

| Situação | `documents.status` | Ação do usuário |
|---|---|---|
| Formato inválido | `failed` | reenviar |
| PDF sem texto | `needs_ocr` | aviso; sem plano |
| Zero blocos extraídos | `failed` | verificar arquivo |
| EPUB corrompido | `failed` | reenviar |
| Timeout de extração (5 min) | `failed` | reprocessar |

Toda falha grava `documents.error` com mensagem legível e um `event` com o erro técnico.

## Testes

Fixtures em `testdata/`:

| Fixture | Cobre | Estado |
|---|---|---|
| `md/headings.md` | hierarquia, listas, código, tabela, figura, footnote | implementado |
| `md/plain.md` | sem headings | implementado |
| `md/long-para.md` | pré-divisão de parágrafo longo | implementado |
| `md/oversized.md` | bloco de código maior que `max` | implementado |
| `md/unicode.md` | acentos, emoji, pontuação tipográfica | planejado |
| `epub/livro.epub` | spine, metadados, listas, código, tabela, figura, nota | implementado |
| `pdf/coluna-unica.pdf` | parágrafos, headings por fonte, hifenização | implementado |
| `pdf/duas-colunas.pdf` | ordem de leitura em duas colunas | implementado |
| `pdf/escaneado.pdf` | `needs_ocr` | implementado |
| `pdf/headings-10pg.pdf` | 20 headings em 10 páginas | implementado |

Fixtures EPUB são geradas por `go run testdata/epub/generate.go`; fixtures PDF, por `go run testdata/pdf/generate.go`. Os goldens atuais comparam o plano em texto (`testdata/golden/*.plan.txt`); um golden de blocos em JSON fica para a FASE-1.
