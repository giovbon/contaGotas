//go:build cgo

package pdf

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gen2brain/go-fitz"

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
//
// O pipeline usa o HTML estruturado do MuPDF (posição e tamanho de fonte por
// linha), remove cabeçalho/rodapé repetidos, detecta colunas e junta linhas em
// parágrafos. Se o HTML não estiver disponível, cai para texto puro.
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
				if _, err := tmp.Write(buf[:n]); err != nil {
					tmp.Close()
					return nil, err
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
	if meta := doc.Metadata(); meta != nil {
		res.Title = primeiroNaoVazio(meta["title"], fallbackTitle)
		res.Author = strings.Trim(meta["author"], "\x00 \t\r\n")
	}

	paginas := make([][]Line, 0, doc.NumPage())
	larguras := make([]float64, 0, doc.NumPage())
	alturas := make([]float64, 0, doc.NumPage())
	totalChars := 0
	htmlOK := false

	for i := 0; i < doc.NumPage(); i++ {
		if text, err := doc.Text(i); err == nil {
			totalChars += len([]rune(strings.TrimSpace(text)))
		}
		altura, largura := 842.0, 595.0
		if b, err := doc.Bound(i); err == nil {
			altura = float64(b.Max.Y)
			largura = float64(b.Max.X)
		}
		var lines []Line
		if src, err := doc.HTML(i, false); err == nil && src != "" {
			lines, err = extrairLinhasHTML(src, i)
			if err != nil {
				lines = nil
			}
		}
		if len(lines) > 0 {
			htmlOK = true
		}
		paginas = append(paginas, lines)
		larguras = append(larguras, largura)
		alturas = append(alturas, altura)
	}

	if totalChars < doc.NumPage()*50 {
		return nil, ErrScanned
	}

	var raw []blocks.Block
	if htmlOK {
		paginas = removerCabecalhoRodape(paginas, larguras, alturas)
		var todas []Line
		for _, pg := range paginas {
			todas = append(todas, ordenarLinhas(pg)...)
		}
		raw = montarBlocos(todas)
	}

	if len(raw) == 0 {
		var sb strings.Builder
		for i := 0; i < doc.NumPage(); i++ {
			text, err := doc.Text(i)
			if err != nil {
				continue
			}
			sb.WriteString(text)
			sb.WriteString("\n\n")
		}
		raw = blocosDeTexto(sb.String())
	}

	if len(raw) == 0 {
		return nil, fmt.Errorf("pdf: nenhum bloco extraído")
	}

	if res.Title == "" || res.Title == fallbackTitle {
		for _, b := range raw {
			if b.Kind == blocks.KindH1 {
				res.Title = b.Text
				break
			}
		}
	}
	res.Blocks = raw
	return res, nil
}

func primeiroNaoVazio(vals ...string) string {
	for _, v := range vals {
		if v = strings.Trim(v, "\x00 \t\r\n"); v != "" {
			return v
		}
	}
	return ""
}
