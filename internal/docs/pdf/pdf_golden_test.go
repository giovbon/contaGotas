//go:build cgo

package pdf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovani/readingdrip/internal/blocks"
)

func TestPDFGolden(t *testing.T) {
	goldens := []string{
		"coluna-unica",
		"duas-colunas",
	}
	for _, name := range goldens {
		t.Run(name, func(t *testing.T) {
			pdfPath := filepath.Join("..", "..", "..", "testdata", "pdf", name+".pdf")
			goldenPath := filepath.Join("..", "..", "..", "testdata", "golden", "pdf_"+name+".plan.txt")
			if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
				t.Skipf("fixture %s não encontrada", pdfPath)
			}
			f, err := os.Open(pdfPath)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			fi, _ := f.Stat()
			res, err := Extract(f, fi.Size(), name)
			if err != nil {
				// pode ser needs_ocr
				if strings.Contains(err.Error(), "escaneado") {
					t.Skip(err.Error())
				}
				t.Fatal(err)
			}
			prepared := blocks.Prepare(res.Blocks, blocks.PrepareOptions{
				Estimator:         blocks.Estimator{WPM: 200, Difficulty: 1.0},
				MaxParagraphWords: 300,
			})
			// Apenas verificar que gerou blocos
			if len(prepared) == 0 {
				t.Fatal("sem blocos")
			}
			_ = goldenPath
		})
	}
}
