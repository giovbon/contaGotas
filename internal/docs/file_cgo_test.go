//go:build cgo

package docs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/giovani/readingdrip/internal/docs"
	"github.com/giovani/readingdrip/internal/docs/pdf"
)

// TestExtractFilePDF garante o despacho para o extrator de PDF. O PDF
// escaneado devolve a sentinela comparável com errors.Is.
func TestExtractFilePDF(t *testing.T) {
	res, err := docs.ExtractFile(context.Background(), "../../testdata/pdf/coluna-unica.pdf", "fallback")
	if err != nil {
		t.Fatalf("extrair: %v", err)
	}
	if len(res.Blocks) == 0 {
		t.Error("nenhum bloco extraído")
	}

	_, err = docs.ExtractFile(context.Background(), "../../testdata/pdf/escaneado.pdf", "fallback")
	if !errors.Is(err, pdf.ErrScanned) {
		t.Fatalf("erro = %v, quer pdf.ErrScanned", err)
	}
}
