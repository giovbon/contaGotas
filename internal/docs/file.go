package docs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/giovani/readingdrip/internal/blocks"
	"github.com/giovani/readingdrip/internal/docs/epub"
	"github.com/giovani/readingdrip/internal/docs/markdown"
	"github.com/giovani/readingdrip/internal/docs/pdf"
)

// Result é o resultado comum da extração de um arquivo, independente do
// formato. Metadados ausentes ficam vazios: o chamador decide o que preservar.
type Result struct {
	Title    string
	Author   string
	Language string
	Blocks   []blocks.Block
}

// ExtractFile lê `path` e despacha para o extrator do formato detectado pela
// extensão. `fallbackTitle` é usado quando o arquivo não traz título próprio.
//
// PDFs sem camada de texto devolvem `pdf.ErrScanned` (comparável com
// `errors.Is`), para o chamador marcar o documento como `needs_ocr`.
func ExtractFile(ctx context.Context, path, fallbackTitle string) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	format, ok := Detect(path)
	if !ok {
		return nil, fmt.Errorf("docs: formato não suportado: %q", filepath.Ext(path))
	}

	var res *Result
	switch format {
	case FormatPDF:
		r, err := extrairPDF(ctx, path, fallbackTitle)
		if err != nil {
			return nil, err
		}
		res = r
	default:
		src, err := lerArquivo(ctx, path)
		if err != nil {
			return nil, err
		}
		switch format {
		case FormatMarkdown:
			r, err := markdown.Extract(src, fallbackTitle)
			if err != nil {
				return nil, err
			}
			res = &Result{Title: r.Title, Blocks: r.Blocks}
		case FormatEPUB:
			r, err := epub.Extract(src, fallbackTitle)
			if err != nil {
				return nil, err
			}
			res = &Result{Title: r.Title, Author: r.Author, Language: r.Language, Blocks: r.Blocks}
		default:
			return nil, fmt.Errorf("docs: formato não suportado: %q", format)
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func extrairPDF(ctx context.Context, path, fallbackTitle string) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("docs: abrir %s: %w", filepath.Base(path), err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("docs: medir %s: %w", filepath.Base(path), err)
	}
	r, err := pdf.Extract(f, info.Size(), fallbackTitle)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &Result{Title: r.Title, Author: r.Author, Language: r.Language, Blocks: r.Blocks}, nil
}

// lerArquivo lê o conteúdo com cancelamento observado antes e depois da
// leitura (a leitura em si não é interrompível).
func lerArquivo(ctx context.Context, path string) ([]byte, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("docs: ler %s: %w", filepath.Base(path), err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return src, nil
}
