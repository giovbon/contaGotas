// Package markdown converte Markdown em blocos normalizados.
package markdown

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"github.com/giovani/readingdrip/internal/blocks"
)

// Result é o resultado da extração de um Markdown.
type Result struct {
	Title  string
	Blocks []blocks.Block
}

// Extract converte Markdown (GFM + footnotes) em blocos.
func Extract(src []byte, fallbackTitle string) (*Result, error) {
	if len(bytes.TrimSpace(src)) == 0 {
		return nil, fmt.Errorf("markdown: arquivo vazio")
	}

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Footnote),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)

	doc := md.Parser().Parse(text.NewReader(src))
	res := &Result{}

	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		switch v := n.(type) {
		case *ast.Heading:
			kind := headingKind(v.Level)
			txt := textOf(v, src)
			if txt == "" {
				continue
			}
			if v.Level == 1 && res.Title == "" {
				res.Title = txt
			}
			res.Blocks = append(res.Blocks, blocks.Block{
				Kind:  kind,
				Level: v.Level,
				Text:  txt,
				HTML:  renderNode(md, src, v),
			})

		case *ast.Paragraph:
			if img := singleImage(v); img != nil {
				res.Blocks = append(res.Blocks, blocks.Block{
					Kind: blocks.KindFigure,
					Text: textOf(img, src),
					HTML: renderNode(md, src, v),
				})
				continue
			}
			res.Blocks = append(res.Blocks, blocks.Block{
				Kind: blocks.KindParagraph,
				Text: textOf(v, src),
				HTML: renderNode(md, src, v),
			})

		case *ast.List:
			for item := v.FirstChild(); item != nil; item = item.NextSibling() {
				li, ok := item.(*ast.ListItem)
				if !ok {
					continue
				}
				res.Blocks = append(res.Blocks, blocks.Block{
					Kind: blocks.KindListItem,
					Text: textOf(li, src),
					HTML: renderNode(md, src, li),
				})
			}

		case *ast.FencedCodeBlock:
			res.Blocks = append(res.Blocks, blocks.Block{
				Kind: blocks.KindCode,
				Text: codeText(v, src),
				HTML: renderNode(md, src, v),
			})

		case *ast.CodeBlock:
			res.Blocks = append(res.Blocks, blocks.Block{
				Kind: blocks.KindCode,
				Text: codeText(v, src),
				HTML: renderNode(md, src, v),
			})

		case *ast.Blockquote:
			res.Blocks = append(res.Blocks, blocks.Block{
				Kind: blocks.KindQuote,
				Text: textOf(v, src),
				HTML: renderNode(md, src, v),
			})

		case *ast.ThematicBreak:
			res.Blocks = append(res.Blocks, blocks.Block{
				Kind: blocks.KindHR,
				HTML: renderNode(md, src, v),
			})

		case *extast.Table:
			res.Blocks = append(res.Blocks, blocks.Block{
				Kind: blocks.KindTable,
				Text: textOf(v, src),
				HTML: renderNode(md, src, v),
			})

		case *extast.FootnoteList:
			res.Blocks = append(res.Blocks, blocks.Block{
				Kind: blocks.KindFootnote,
				Text: textOf(v, src),
				HTML: renderNode(md, src, v),
			})
		}
	}

	if res.Title == "" {
		res.Title = fallbackTitle
	}
	if len(res.Blocks) == 0 {
		return nil, fmt.Errorf("markdown: nenhum bloco extraído")
	}
	return res, nil
}

func headingKind(level int) blocks.Kind {
	switch level {
	case 2:
		return blocks.KindH2
	case 3:
		return blocks.KindH3
	case 4:
		return blocks.KindH4
	case 5:
		return blocks.KindH5
	case 6:
		return blocks.KindH6
	default:
		return blocks.KindH1
	}
}

func renderNode(md goldmark.Markdown, src []byte, n ast.Node) string {
	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, src, n); err != nil {
		return ""
	}
	return buf.String()
}

func codeText(n ast.Node, src []byte) string {
	var b strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(src))
	}
	return b.String()
}

func singleImage(p *ast.Paragraph) *ast.Image {
	var img *ast.Image
	for c := p.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *ast.Image:
			if img != nil {
				return nil
			}
			img = v
		case *ast.Link:
			if v.ChildCount() != 1 {
				return nil
			}
			im, ok := v.FirstChild().(*ast.Image)
			if !ok || img != nil {
				return nil
			}
			img = im
		default:
			return nil
		}
	}
	return img
}

// textOf extrai texto puro de um nó, sem markup.
func textOf(n ast.Node, src []byte) string {
	var b strings.Builder
	writeText(n, src, &b)
	return strings.Join(strings.Fields(b.String()), " ")
}

func writeText(n ast.Node, src []byte, b *strings.Builder) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *ast.Text:
			b.Write(v.Segment.Value(src))
			b.WriteByte(' ')
			if v.SoftLineBreak() || v.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(v.Value)
			b.WriteByte(' ')
		case *ast.AutoLink:
			b.Write(v.Label(src))
			b.WriteByte(' ')
		case *ast.RawHTML:
			// HTML cru é omitido (goldmark sem WithUnsafe)
		default:
			writeText(c, src, b)
		}
	}
}
