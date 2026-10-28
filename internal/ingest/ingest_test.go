package ingest_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovani/readingdrip/internal/ingest"
	"github.com/giovani/readingdrip/internal/store"
)

var agora = time.Date(2026, 10, 27, 12, 0, 0, 0, time.FixedZone("BRT", -3*3600))

func abrirServico(t *testing.T) (*store.Store, *ingest.Service) {
	t.Helper()
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("fechar banco: %v", err)
		}
	})
	return s, &ingest.Service{
		Store: s,
		Now:   func() time.Time { return agora },
	}
}

func textoMarkdown() string {
	var b strings.Builder
	b.WriteString("# Livro de Teste\n\n")
	b.WriteString("Abertura do livro com uma frase curta.\n\n")
	for i := 1; i <= 4; i++ {
		fmt.Fprintf(&b, "## Capítulo %d\n\n", i)
		for j := 0; j < 3; j++ {
			fmt.Fprintf(&b, "Parágrafo %d do capítulo %d com algumas palavras para dar corpo ao texto e permitir um plano de leitura razoável.\n\n", j+1, i)
		}
	}
	return b.String()
}

func escrever(t *testing.T, nome, conteudo string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), nome)
	if err := os.WriteFile(path, []byte(conteudo), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func criarDocumento(t *testing.T, s *store.Store, doc store.Document) int64 {
	t.Helper()
	if doc.Format == "" {
		doc.Format = "md"
	}
	if doc.SourceName == "" {
		doc.SourceName = "livro.md"
	}
	if doc.SourceSHA256 == "" {
		doc.SourceSHA256 = "sha-" + doc.SourcePath
	}
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = agora
		doc.UpdatedAt = agora
	}
	id, err := s.Documents().Create(context.Background(), doc)
	if err != nil {
		t.Fatalf("criar documento: %v", err)
	}
	return id
}

func TestProcessMarkdown(t *testing.T) {
	s, svc := abrirServico(t)
	path := escrever(t, "livro.md", textoMarkdown())
	id := criarDocumento(t, s, store.Document{Title: "Nome do arquivo", SourcePath: path})

	res, err := svc.Process(context.Background(), id)
	if err != nil {
		t.Fatalf("processar: %v", err)
	}
	if res.Status != store.StatusReady || res.DocumentID != id {
		t.Errorf("resultado = %+v", res)
	}
	if res.Title != "Livro de Teste" {
		t.Errorf("título = %q", res.Title)
	}
	if res.Blocks == 0 || res.Words == 0 || res.Seconds == 0 {
		t.Errorf("contagens vazias: %+v", res)
	}
	if res.Chunks == 0 || res.PlanVer != 1 {
		t.Errorf("plano = v%d com %d chunks", res.PlanVer, res.Chunks)
	}

	doc, err := s.Documents().Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Status != store.StatusReady {
		t.Errorf("status = %q, quer ready", doc.Status)
	}
	if doc.Error != nil {
		t.Errorf("erro = %q, quer nil", *doc.Error)
	}
	if doc.Title != "Livro de Teste" {
		t.Errorf("título persistido = %q", doc.Title)
	}
	if doc.TotalWords != res.Words || doc.TotalSeconds != res.Seconds {
		t.Errorf("totais = %d/%d, quer %d/%d", doc.TotalWords, doc.TotalSeconds, res.Words, res.Seconds)
	}
	if doc.Language != "pt-BR" {
		t.Errorf("idioma = %q, quer o padrão preservado", doc.Language)
	}

	blocos, err := s.Blocks().List(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocos) != res.Blocks {
		t.Fatalf("blocos = %d, quer %d", len(blocos), res.Blocks)
	}
	for i, b := range blocos {
		if b.Ord != i || b.Hash == "" {
			t.Fatalf("bloco %d = %+v", i, b)
		}
	}

	ativo, err := s.Plans().Active(context.Background(), id)
	if err != nil || ativo == nil {
		t.Fatalf("plano ativo: %v, %v", ativo, err)
	}
	if ativo.ID != res.PlanID || len(ativo.Chunks) != res.Chunks {
		t.Errorf("plano ativo = %d com %d chunks", ativo.ID, len(ativo.Chunks))
	}

	eventos, err := s.Events().List(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(eventos) == 0 || eventos[0].Kind != "plan" {
		t.Fatalf("eventos = %+v", eventos)
	}
}

func TestProcessExtraicaoFalha(t *testing.T) {
	s, svc := abrirServico(t)
	id := criarDocumento(t, s, store.Document{
		Title:      "Sumido",
		SourcePath: filepath.Join(t.TempDir(), "nao-existe.md"),
	})

	_, err := svc.Process(context.Background(), id)
	if err == nil {
		t.Fatal("esperado erro de extração")
	}

	doc, err := s.Documents().Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Status != store.StatusFailed {
		t.Errorf("status = %q, quer failed", doc.Status)
	}
	if doc.Error == nil || !strings.Contains(*doc.Error, "nao-existe.md") {
		t.Errorf("erro persistido = %v", doc.Error)
	}
	if n, _ := s.Blocks().Count(context.Background(), id); n != 0 {
		t.Errorf("blocos = %d, quer 0", n)
	}
	if plano, _ := s.Plans().Active(context.Background(), id); plano != nil {
		t.Errorf("plano criado em falha: %+v", plano)
	}

	eventos, err := s.Events().List(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(eventos) == 0 || eventos[0].Level != store.EventError || eventos[0].Kind != "extract" {
		t.Fatalf("eventos = %+v", eventos)
	}
}

func TestProcessArquivoVazio(t *testing.T) {
	s, svc := abrirServico(t)
	path := escrever(t, "vazio.md", "   \n\n")
	id := criarDocumento(t, s, store.Document{Title: "Vazio", SourcePath: path})

	if _, err := svc.Process(context.Background(), id); err == nil {
		t.Fatal("esperado erro para arquivo vazio")
	}
	doc, err := s.Documents().Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Status != store.StatusFailed || doc.Error == nil {
		t.Errorf("documento = status %q, erro %v", doc.Status, doc.Error)
	}
}

func TestProcessDocumentoPausado(t *testing.T) {
	s, svc := abrirServico(t)
	path := escrever(t, "livro.md", textoMarkdown())
	id := criarDocumento(t, s, store.Document{Title: "Pausado", SourcePath: path, Status: store.StatusPaused})

	_, err := svc.Process(context.Background(), id)
	if !errors.Is(err, ingest.ErrNotProcessable) {
		t.Fatalf("erro = %v, quer ErrNotProcessable", err)
	}
	doc, err := s.Documents().Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Status != store.StatusPaused {
		t.Errorf("status = %q, quer paused preservado", doc.Status)
	}
}

func TestProcessDocumentoInexistente(t *testing.T) {
	_, svc := abrirServico(t)
	if _, err := svc.Process(context.Background(), 999); !errors.Is(err, ingest.ErrNotFound) {
		t.Fatalf("erro = %v, quer ErrNotFound", err)
	}
}

func TestProcessReprocessa(t *testing.T) {
	s, svc := abrirServico(t)
	path := escrever(t, "livro.md", textoMarkdown())
	id := criarDocumento(t, s, store.Document{Title: "Livro", SourcePath: path})

	primeiro, err := svc.Process(context.Background(), id)
	if err != nil {
		t.Fatalf("primeiro processamento: %v", err)
	}
	segundo, err := svc.Process(context.Background(), id)
	if err != nil {
		t.Fatalf("reprocessamento: %v", err)
	}
	if segundo.Blocks != primeiro.Blocks || segundo.Chunks != primeiro.Chunks {
		t.Errorf("reprocessamento divergiu: %+v vs %+v", primeiro, segundo)
	}

	versoes, err := s.Plans().Versions(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(versoes) != 1 || versoes[0].Status != store.PlanActive {
		t.Fatalf("versões = %+v, quer apenas o plano ativo", versoes)
	}
	ativo, err := s.Plans().Active(context.Background(), id)
	if err != nil || ativo == nil {
		t.Fatalf("plano ativo: %v, %v", ativo, err)
	}
	if len(ativo.Chunks) != segundo.Chunks {
		t.Errorf("chunks do plano ativo = %d, quer %d", len(ativo.Chunks), segundo.Chunks)
	}
}

func TestProcessPreservaHistoricoDeEntrega(t *testing.T) {
	s, svc := abrirServico(t)
	path := escrever(t, "livro.md", textoMarkdown())
	id := criarDocumento(t, s, store.Document{Title: "Livro", SourcePath: path})
	ctx := context.Background()

	if _, err := svc.Process(ctx, id); err != nil {
		t.Fatal(err)
	}
	ativo, err := s.Plans().Active(ctx, id)
	if err != nil || ativo == nil {
		t.Fatalf("plano ativo: %v, %v", ativo, err)
	}
	if err := s.Plans().MarkSent(ctx, ativo.Chunks[0].ID, agora); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Process(ctx, id); !errors.Is(err, store.ErrDeliveryHistory) {
		t.Fatalf("erro = %v, quer ErrDeliveryHistory", err)
	}

	depois, err := s.Plans().Active(ctx, id)
	if err != nil || depois == nil {
		t.Fatalf("plano ativo após recusa: %v, %v", depois, err)
	}
	if depois.ID != ativo.ID || depois.Chunks[0].Status != store.ChunkSent {
		t.Errorf("histórico alterado: plano %d, chunk %q", depois.ID, depois.Chunks[0].Status)
	}
	doc, err := s.Documents().Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Status != store.StatusReady {
		t.Errorf("status = %q, quer ready preservado", doc.Status)
	}
}

func TestProcessFinishByDays(t *testing.T) {
	s, svc := abrirServico(t)
	path := escrever(t, "livro.md", textoMarkdown())
	dias := 4
	id := criarDocumento(t, s, store.Document{
		Title:      "Livro",
		SourcePath: path,
		PacingMode: "finish_by_days",
		PacingDays: &dias,
	})

	if _, err := svc.Process(context.Background(), id); err != nil {
		t.Fatalf("processar: %v", err)
	}
	ativo, err := s.Plans().Active(context.Background(), id)
	if err != nil || ativo == nil {
		t.Fatalf("plano ativo: %v, %v", ativo, err)
	}
	if ativo.HorizonDays == nil || *ativo.HorizonDays != dias {
		t.Errorf("horizon_days = %v, quer %d", ativo.HorizonDays, dias)
	}
	if ativo.Params.Mode != "finish_by_days" || ativo.Params.Days != dias {
		t.Errorf("params = %+v", ativo.Params)
	}
}

func TestProcessFinishByDaysSemDias(t *testing.T) {
	s, svc := abrirServico(t)
	path := escrever(t, "livro.md", textoMarkdown())
	id := criarDocumento(t, s, store.Document{
		Title:      "Livro",
		SourcePath: path,
		PacingMode: "finish_by_days",
	})

	_, err := svc.Process(context.Background(), id)
	if err == nil || !strings.Contains(err.Error(), "sem dias") {
		t.Fatalf("erro = %v", err)
	}
	doc, err := s.Documents().Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Status != store.StatusFailed || doc.Error == nil {
		t.Errorf("documento = status %q, erro %v", doc.Status, doc.Error)
	}
}
