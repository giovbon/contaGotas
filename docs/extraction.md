---
title: Extração
status: ativo
updated: 2026-10-26
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

Pipeline: `go-fitz` (MuPDF) → por página → HTML/estruturado → normalização → blocos.

### Ordem de leitura e parágrafos

1. Extrair spans com posição, tamanho de fonte e flags de estilo.
2. Detectar colunas por clustering de `x` (gap horizontal consistente por várias linhas). Se ≥ 2 colunas, ler coluna a coluna.
3. Ordenar linhas por `(coluna, y)`.
4. Descartar cabeçalho/rodapé repetidos: linha com texto idêntico em > 40% das páginas, ou número de página isolado no topo/rodapé.
5. Juntar linhas em parágrafos:
   - desfazer hifenização (`-\n` + minúscula → concatena);
   - quebra de parágrafo se o gap vertical > 1,4× a altura média da linha, ou se a linha anterior termina em pontuação final e a próxima começa com maiúscula e há recuo;
   - remover espaços duplos e soft hyphens (`U+00AD`).
6. Detectar headings por:
   - tamanho de fonte ≥ 1,15× a mediana do corpo, **ou**
   - negrito + linha curta (< 60 caracteres) + padrão de numeração (`^\d+(\.\d+)*\s`, `^Cap[íi]tulo\b`, `^Parte\b`).
   - nível inferido pela ordem dos tamanhos distintos (maior → h1).

### Limitações conhecidas

- PDF escaneado (sem texto): se < 5% dos caracteres esperados por página, marcar `needs_ocr` (`Q-002`).
- Tabelas complexas: preservadas como texto; sem reconstrução de células.
- Fórmulas matemáticas: texto linearizado.
- Notas de rodapé na base da página: mantidas no fluxo, ao fim da página.
- PDFs com layout de revista (3+ colunas, caixas): qualidade degradada — validar com fixture.

### Spike obrigatório (`SPIKE-01`)

Antes de implementar, confirmar na versão fixada do `go-fitz`:

- a API que expõe spans com **tamanho de fonte e flags** (HTML/estruturado vs. texto plano);
- comportamento com `CGO_ENABLED=1` em `linux/arm64` com as libs estáticas do módulo;
- consumo de memória em PDF de 300 páginas.

Resultado do spike vai para `open-questions.md` e, se mudar a abordagem, para um ADR.

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
| `pdf/coluna-unica.pdf` | parágrafos, headings por fonte | planejado |
| `pdf/duas-colunas.pdf` | ordem de leitura | planejado |
| `pdf/escaneado.pdf` | `needs_ocr` | planejado |

Fixtures EPUB são geradas por `go run testdata/epub/generate.go`. Os goldens atuais comparam o plano em texto (`testdata/golden/*.plan.txt`); um golden de blocos em JSON fica para a FASE-1.

## PDF (`.pdf`) — implementação FASE-0

### API
```go
// internal/docs/pdf
func Extract(r io.ReaderAt, size int64, fallbackTitle string) (*Result, error)
```
Resultado segue o mesmo padrão (`Title`, `Author`, `Language`, `Blocks`). CGO isolado via build tags `cgo`/`!cgo`.

### Pipeline atual (MVP)
1. Arquivo copiado para temporário (go-fitz requer path).
2. Heurística de PDF escaneado: se < 50 chars médios por página → erro `needs_ocr` (não suportado na v1).
3. Por página: tentar `HTML(pageNum)` primeiro; senão `Text(pageNum)`.
4. HTML: extrair `<p>`, `<h1>..h6>` com `stripTags` + normalização.
5. Texto: dividir por parágrafos vazios.
6. Normalização: NFC, remover soft hyphen, desfazer hifenização simples `-\n` + minúscula, colapsar espaços.

### Pendentes (SPIKE-01)
- Spans com fonte/tamanho/flags para detecção confiável de headings por estilo
- Detecção de colunas por clustering de `x`
- Cabeçalho/rodapé repetidos
- Ordem de leitura correta em duas colunas
- Consumo de memória em PDF 300 páginas
- Fixtures `testdata/pdf/coluna-unica.pdf`, `testdata/pdf/duas-colunas.pdf`, `testdata/pdf/escaneado.pdf`
