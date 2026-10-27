//go:build cgo

package pdf

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/gen2brain/go-fitz"
	"golang.org/x/text/unicode/norm"

	"github.com/giovani/readingdrip/internal/blocks"
)

// Result é o resultado da extração de PDF.
type Result struct {
	Title    string
	Author   string
	Language string
	Blocks   []blocks.Block
}

// Extract converte PDF em blocos normalizados.
func Extract(r io.ReaderAt, size int64, fallbackTitle string) (*Result, error) {
	if size == 0 {
		return nil, fmt.Errorf("pdf: arquivo vazio")
	}

	tmp, err := os.CreateTemp("", "readingdrip-*.pdf")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if rs, ok := r.(io.Reader); ok {
		if _, err := io.Copy(tmp, rs); err != nil {
			tmp.Close()
			return nil, err
		}
	} else {
		buf := make([]byte, 32*1024)
		offset := int64(0)
		for {
			n, readErr := r.ReadAt(buf, offset)
			if n > 0 {
				if _, err2 := tmp.Write(buf[:n]); err2 != nil {
					tmp.Close()
					return nil, err2
				}
				offset += int64(n)
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				tmp.Close()
				return nil, readErr
			}
		}
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}

	doc, err := fitz.New(tmpName)
	if err != nil {
		return nil, fmt.Errorf("pdf: %w", err)
	}
	defer doc.Close()

	if doc.NumPage() == 0 {
		return nil, fmt.Errorf("pdf: sem páginas")
	}

	res := &Result{Title: fallbackTitle}
	var allBlocks []blocks.Block

	totalChars := 0
	for i := 0; i < doc.NumPage(); i++ {
		text, err := doc.Text(i)
		if err != nil {
			continue
		}
		totalChars += len(strings.TrimSpace(text))
	}

	if totalChars < doc.NumPage()*50 {
		return nil, fmt.Errorf("pdf: sem camada de texto (provável PDF escaneado) — OCR não suportado na v1")
	}

	for i := 0; i < doc.NumPage(); i++ {
		pageBlocks, err := extractPage(doc, i)
		if err != nil {
			continue
		}
		allBlocks = append(allBlocks, pageBlocks...)
	}

	if len(allBlocks) == 0 {
		return nil, fmt.Errorf("pdf: nenhum bloco extraído")
	}

	res.Blocks = allBlocks
	if res.Title == "" {
		res.Title = fallbackTitle
	}
	return res, nil
}

func extractPage(doc *fitz.Document, pageNum int) ([]blocks.Block, error) {
	html, err := doc.HTML(pageNum)
	if err == nil && html != "" {
		return extractFromHTML(html)
	}

	text, err := doc.Text(pageNum)
	if err != nil {
		return nil, err
	}
	return extractFromText(text)
}

func extractFromHTML(html string) ([]blocks.Block, error) {
	var blocksOut []blocks.Block

	// Headings primeiro para manter ordem
	hre := regexp.MustCompile(`(?is)<h([1-6])[^>]*>(.*?)</h[1-6]>`)
	hmatches := hre.FindAllStringSubmatch(html, -1)
	for _, m := range hmatches {
		if len(m) < 3 {
			continue
		}
		level, _ := strconv.Atoi(m[1])
		content := stripTags(m[2])
		content = normalizePDFText(content)
		if content == "" {
			continue
		}
		kind := headingKindPDF(level)
		blocksOut = append(blocksOut, blocks.Block{
			Kind:  kind,
			Level: level,
			Text:  content,
			HTML:  fmt.Sprintf("<h%d>%s</h%d>", level, escapeHTML(content), level),
		})
	}

	// Parágrafos
	pre := regexp.MustCompile(`(?is)<pre[^>]*>(.*?)</pre>`)
	html = pre.ReplaceAllString(html, " ")
	cre := regexp.MustCompile(`(?is)<code[^>]*>(.*?)</code>`)
	html = cre.ReplaceAllString(html, " ")
	tre := regexp.MustCompile(`(?is)<table[^>]*>.*?</table>`)
	html = tre.ReplaceAllString(html, " ")

	re := regexp.MustCompile(`(?is)<p[^>]*>(.*?)</p>`)
	matches := re.FindAllStringSubmatch(html, -1)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		content := stripTags(m[1])
		content = normalizePDFText(content)
		if content == "" {
			continue
		}
		// Skip se já parece heading
		if isHeadingLike(content) {
			continue
		}
		blocksOut = append(blocksOut, blocks.Block{
			Kind: blocks.KindParagraph,
			Text: content,
			HTML: "<p>" + escapeHTML(content) + "</p>",
		})
	}

	return blocksOut, nil
}

func extractFromText(text string) ([]blocks.Block, error) {
	var blocksOut []blocks.Block
	text = norm.NFC.String(text)
	paras := regexp.MustCompile(`\n\s*\n`).Split(text, -1)
	for _, p := range paras {
		p = normalizePDFText(p)
		if p == "" {
			continue
		}
		if isHeadingLike(p) && len(p) < 80 {
			level := inferHeadingLevel(p)
			blocksOut = append(blocksOut, blocks.Block{
				Kind:  headingKindPDF(level),
				Level: level,
				Text:  p,
				HTML:  fmt.Sprintf("<h%d>%s</h%d>", level, escapeHTML(p), level),
			})
			continue
		}
		blocksOut = append(blocksOut, blocks.Block{
			Kind: blocks.KindParagraph,
			Text: p,
			HTML: "<p>" + escapeHTML(p) + "</p>",
		})
	}
	return blocksOut, nil
}

func headingKindPDF(level int) blocks.Kind {
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

func isHeadingLike(s string) bool {
	if len(s) > 100 {
		return false
	}
	// Padrões numéricos como 1., 1.1, Capítulo 1
	if regexp.MustCompile(`^(\d+(\.\d+)*\.?\s+|[Cc]ap[íi]tulo\s+\d+|[Pp]arte\s+\d+)`).MatchString(s) {
		return true
	}
	return false
}

func inferHeadingLevel(s string) int {
	if regexp.MustCompile(`^\d+\.\d+\.`).MatchString(s) {
		return 3
	}
	if regexp.MustCompile(`^\d+\.\d+`).MatchString(s) {
		return 2
	}
	if regexp.MustCompile(`^\d+\.`).MatchString(s) {
		return 1
	}
	return 1
}

func normalizePDFText(s string) string {
	s = norm.NFC.String(s)
	s = strings.ReplaceAll(s, "\u00ad", "")
	s = strings.ReplaceAll(s, "\u00a0", " ")
	// Desfazer hifenização
	s = regexp.MustCompile(`-\n([a-záàâãéèêíïóôõöúçñ])`).ReplaceAllString(s, "$1")
	s = regexp.MustCompile(`-\r\n([a-záàâãéèêíïóôõöúçñ])`).ReplaceAllString(s, "$1")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

func stripTags(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	return re.ReplaceAllString(s, "")
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

func isRepetido(texto string, contagem map[string]int, totalPaginas int) bool {
	if texto == "" || len(texto) < 3 {
		return false
	}
	c := contagem[texto]
	return c > 0 && float64(c)/float64(totalPaginas) > 0.4
}

func desfazerHifenizacao(s string) string {
	// Desfazer hifenização no final da linha: palavra-\npalavra -> palavra-palavra quando segunda começa com minúscula
	s = regexp.MustCompile(`([a-záàâãéèêíïóôõöúçñ])-\n\s*([a-záàâãéèêíïóôõöúçñ])`).ReplaceAllString(s, "$1$2")
	s = regexp.MustCompile(`([a-záàâãéèêíïóôõöúçñ])-\r\n\s*([a-záàâãéèêíïóôõöúçñ])`).ReplaceAllString(s, "$1$2")
	return s
}

func juntarLinhas(linhas []string, medianLineHeight float64) []string {
	if len(linhas) == 0 {
		return nil
	}
	var result []string
	var current strings.Builder
	current.WriteString(strings.TrimSpace(linhas[0]))

	for i := 1; i < len(linhas); i++ {
		line := strings.TrimSpace(linhas[i])
		if line == "" {
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
			continue
		}
		// Verificar se é novo parágrafo
		prev := current.String()
		if len(prev) > 0 {
			last := prev[len(prev)-1]
			// Termina com pontuação final e próxima começa com maiúscula -> novo parágrafo
			if (last == '.' || last == '!' || last == '?' || last == ':') && len(line) > 0 && line[0] >= 'A' && line[0] <= 'Z' {
				result = append(result, current.String())
				current.Reset()
				current.WriteString(line)
				continue
			}
		}
		// Continuar parágrafo
		if current.Len() > 0 && !strings.HasSuffix(prev, " ") {
			current.WriteString(" ")
		}
		current.WriteString(line)
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}
	return result
}

func detectarCabecalhoRodape(paginas [][]string) map[string]bool {
	contagem := make(map[string]int)
	for _, pag := range paginas {
		// Verificar primeiras e últimas linhas
		if len(pag) > 0 {
			if l := normalizePDFText(pag[0]); len(l) < 80 {
				contagem[l]++
			}
		}
		if len(pag) > 1 {
			if l := normalizePDFText(pag[len(pag)-1]); len(l) < 80 {
				contagem[l]++
			}
		}
	}
	repetidos := make(map[string]bool)
	total := len(paginas)
	for k, v := range contagem {
		if float64(v)/float64(total) > 0.4 {
			repetidos[k] = true
		}
	}
	return repetidos
}

func juntarLinhasComQuebra(linhas []string) []string {
	if len(linhas) == 0 {
		return nil
	}
	var result []string
	var buf strings.Builder
	buf.WriteString(strings.TrimSpace(linhas[0]))
	for i := 1; i < len(linhas); i++ {
		line := strings.TrimSpace(linhas[i])
		if line == "" {
			if buf.Len() > 0 {
				result = append(result, buf.String())
				buf.Reset()
			}
			continue
		}
		prev := buf.String()
		if len(prev) > 0 {
			last := prev[len(prev)-1]
			if (last == '.' || last == '!' || last == '?' || last == ':' || last == ';') && len(line) > 0 && line[0] >= 'A' && line[0] <= 'Z' {
				result = append(result, prev)
				buf.Reset()
				buf.WriteString(line)
				continue
			}
		}
		if buf.Len() > 0 {
			buf.WriteString(" ")
		}
		buf.WriteString(line)
	}
	if buf.Len() > 0 {
		result = append(result, buf.String())
	}
	return result
}
