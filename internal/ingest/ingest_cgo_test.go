//go:build cgo

package ingest_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/giovani/readingdrip/internal/docs/pdf"
	"github.com/giovani/readingdrip/internal/store"
)

func TestProcessPDFEscaneado(t *testing.T) {
	s, svc := abrirServico(t)
	id := criarDocumento(t, s, store.Document{
		Title:      "Escaneado",
		Format:     "pdf",
		SourceName: "escaneado.pdf",
		SourcePath: "../../testdata/pdf/escaneado.pdf",
	})

	_, err := svc.Process(context.Background(), id)
	if !errors.Is(err, pdf.ErrScanned) {
		t.Fatalf("erro = %v, quer pdf.ErrScanned", err)
	}

	doc, err := s.Documents().Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Status != store.StatusNeedsOCR {
		t.Errorf("status = %q, quer needs_ocr", doc.Status)
	}
	if doc.Error == nil || !strings.Contains(*doc.Error, "OCR") {
		t.Errorf("mensagem = %v, quer menção a OCR", doc.Error)
	}
	if n, _ := s.Blocks().Count(context.Background(), id); n != 0 {
		t.Errorf("blocos = %d, quer 0", n)
	}
	if plano, _ := s.Plans().Active(context.Background(), id); plano != nil {
		t.Errorf("plano criado para PDF escaneado: %+v", plano)
	}

	eventos, err := s.Events().List(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(eventos) == 0 || eventos[0].Level != store.EventWarn || eventos[0].Kind != "extract" {
		t.Fatalf("eventos = %+v", eventos)
	}
}
