package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/giovani/readingdrip/internal/plan"
)

var agora = time.Date(2026, 10, 27, 12, 0, 0, 0, time.FixedZone("BRT", -3*3600))

func docExemplo(titulo, sha string) Document {
	return Document{
		Title:        titulo,
		Format:       "md",
		SourceName:   titulo + ".md",
		SourcePath:   "/data/" + titulo + ".md",
		SourceSHA256: sha,
		CreatedAt:    agora,
		UpdatedAt:    agora,
	}
}

func TestDocumentsCreateGet(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	id, err := s.Documents().Create(ctx, docExemplo("Livro", "sha-1"))
	if err != nil {
		t.Fatalf("criar: %v", err)
	}

	d, err := s.Documents().Get(ctx, id)
	if err != nil {
		t.Fatalf("buscar: %v", err)
	}
	if d == nil {
		t.Fatal("documento não encontrado")
	}
	if d.Title != "Livro" || d.SourceSHA256 != "sha-1" {
		t.Errorf("documento = %+v", d)
	}
	if d.Language != "pt-BR" || d.Status != StatusProcessing {
		t.Errorf("defaults não aplicados: language=%q status=%q", d.Language, d.Status)
	}
	if d.PacingMinutes != 10 || d.WPM != 200 || d.Priority != 100 {
		t.Errorf("defaults numéricos = %d/%d/%d", d.PacingMinutes, d.WPM, d.Priority)
	}
	if d.Difficulty != 1.0 {
		t.Errorf("difficulty = %v", d.Difficulty)
	}
	if !d.CreatedAt.Equal(agora) {
		t.Errorf("created_at = %v, quer %v", d.CreatedAt, agora)
	}
}

func TestDocumentsCreateSemTitulo(t *testing.T) {
	s := abrirTeste(t)
	doc := docExemplo("", "sha")
	if _, err := s.Documents().Create(context.Background(), doc); err == nil {
		t.Fatal("esperado erro para documento sem título")
	}
}

func TestDocumentsCreateSemData(t *testing.T) {
	s := abrirTeste(t)
	doc := docExemplo("X", "sha")
	doc.CreatedAt = time.Time{}
	if _, err := s.Documents().Create(context.Background(), doc); err == nil {
		t.Fatal("esperado erro para documento sem created_at")
	}
}

func TestDocumentsSHAUnico(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	if _, err := s.Documents().Create(ctx, docExemplo("A", "mesmo")); err != nil {
		t.Fatal(err)
	}
	_, err := s.Documents().Create(ctx, docExemplo("B", "mesmo"))
	if err == nil {
		t.Fatal("esperado erro para sha256 duplicado")
	}
	if !strings.Contains(err.Error(), "UNIQUE") && !strings.Contains(strings.ToLower(err.Error()), "unique") {
		t.Errorf("erro = %v, quer violação de unicidade", err)
	}
}

func TestDocumentsBySHA256(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	if _, err := s.Documents().Create(ctx, docExemplo("A", "sha-a")); err != nil {
		t.Fatal(err)
	}
	d, err := s.Documents().BySHA256(ctx, "sha-a")
	if err != nil || d == nil {
		t.Fatalf("buscar por sha: %v, %v", d, err)
	}
	if d.Title != "A" {
		t.Errorf("título = %q", d.Title)
	}

	ausente, err := s.Documents().BySHA256(ctx, "nao-existe")
	if err != nil {
		t.Fatal(err)
	}
	if ausente != nil {
		t.Errorf("esperado nil, veio %+v", ausente)
	}
}

func TestDocumentsListOrdenaPorPrioridade(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	a := docExemplo("A", "a")
	a.Priority = 200
	b := docExemplo("B", "b")
	b.Priority = 10
	for _, d := range []Document{a, b} {
		if _, err := s.Documents().Create(ctx, d); err != nil {
			t.Fatal(err)
		}
	}

	lista, err := s.Documents().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(lista) != 2 || lista[0].Title != "B" || lista[1].Title != "A" {
		t.Fatalf("ordem = %+v", lista)
	}
}

func TestDocumentsUpdateStatusETotais(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	id, err := s.Documents().Create(ctx, docExemplo("A", "a"))
	if err != nil {
		t.Fatal(err)
	}

	depois := agora.Add(time.Hour)
	if err := s.Documents().UpdateStatus(ctx, id, StatusReady, depois); err != nil {
		t.Fatalf("status: %v", err)
	}
	if err := s.Documents().UpdateTotais(ctx, id, 1234, 370, depois); err != nil {
		t.Fatalf("totais: %v", err)
	}

	d, err := s.Documents().Get(ctx, id)
	if err != nil || d == nil {
		t.Fatal(err)
	}
	if d.Status != StatusReady {
		t.Errorf("status = %q", d.Status)
	}
	if d.TotalWords != 1234 || d.TotalSeconds != 370 {
		t.Errorf("totais = %d/%d", d.TotalWords, d.TotalSeconds)
	}
	if !d.UpdatedAt.Equal(depois) {
		t.Errorf("updated_at = %v, quer %v", d.UpdatedAt, depois)
	}
}

func TestDocumentsGetInexistente(t *testing.T) {
	s := abrirTeste(t)
	d, err := s.Documents().Get(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if d != nil {
		t.Fatalf("esperado nil, veio %+v", d)
	}
}

func TestDocumentsSaveExtraction(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	id, err := s.Documents().Create(ctx, docExemplo("Original", "sha-extracao"))
	if err != nil {
		t.Fatal(err)
	}
	extracao := Extraction{
		Title:        "Título Extraído",
		Author:       "Autora",
		Language:     "en",
		TotalWords:   1200,
		TotalSeconds: 360,
		Status:       StatusReady,
	}
	if err := s.Documents().SaveExtraction(ctx, id, extracao, agora); err != nil {
		t.Fatal(err)
	}

	d, err := s.Documents().Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "Título Extraído" || d.Author == nil || *d.Author != "Autora" || d.Language != "en" {
		t.Errorf("metadados = %+v", d)
	}
	if d.TotalWords != 1200 || d.TotalSeconds != 360 || d.Status != StatusReady || d.Error != nil {
		t.Errorf("extração = %+v", d)
	}

	// Campos vazios preservam o que já existe e Error vazio limpa o anterior.
	if err := s.Documents().SaveExtraction(ctx, id, Extraction{Status: StatusFailed, Error: "falhou"}, agora); err != nil {
		t.Fatal(err)
	}
	d, err = s.Documents().Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "Título Extraído" || d.Author == nil || *d.Author != "Autora" || d.Language != "en" {
		t.Errorf("metadados sobrescritos por vazio: %+v", d)
	}
	if d.Error == nil || *d.Error != "falhou" {
		t.Errorf("erro = %v", d.Error)
	}

	if err := s.Documents().SaveExtraction(ctx, id, Extraction{Status: StatusReady}, agora); err != nil {
		t.Fatal(err)
	}
	d, err = s.Documents().Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if d.Error != nil {
		t.Errorf("erro = %q, quer limpo", *d.Error)
	}

	if err := s.Documents().SaveExtraction(ctx, id, Extraction{Status: "invalido"}, agora); err == nil {
		t.Fatal("esperado erro para status inválido")
	}
	if err := s.Documents().SaveExtraction(ctx, id, Extraction{Status: StatusReady}, time.Time{}); err == nil {
		t.Fatal("esperado erro sem data")
	}
}

func TestDocumentsUpdateSource(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	id, err := s.Documents().Create(ctx, docExemplo("Livro", "sha-origem"))
	if err != nil {
		t.Fatal(err)
	}

	final := "/data/documents/1/source.md"
	if err := s.Documents().UpdateSource(ctx, id, "novo-nome.md", final, agora.Add(time.Minute)); err != nil {
		t.Fatalf("atualizar origem: %v", err)
	}
	d, err := s.Documents().Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if d.SourceName != "novo-nome.md" || d.SourcePath != final {
		t.Errorf("origem = %q/%q", d.SourceName, d.SourcePath)
	}
	if !d.UpdatedAt.Equal(agora.Add(time.Minute)) {
		t.Errorf("updated_at = %v", d.UpdatedAt)
	}

	if err := s.Documents().UpdateSource(ctx, id, "", final, agora); err == nil {
		t.Error("esperado erro sem nome")
	}
	if err := s.Documents().UpdateSource(ctx, id, "x.md", final, time.Time{}); err == nil {
		t.Error("esperado erro sem data")
	}
}

func TestDocumentsDelete(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	id := documentoComBlocos(t, s, blocosDeTeste(10, 60))
	params := plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1}
	pl, err := plan.Generate(blocosDeTeste(10, 60), params)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Plans().Create(ctx, id, pl, params, agora); err != nil {
		t.Fatal(err)
	}
	ativo, err := s.Plans().Active(ctx, id)
	if err != nil || ativo == nil {
		t.Fatalf("plano ativo: %v, %v", ativo, err)
	}

	res, err := s.DB().Exec(`INSERT INTO delivery_runs (run_date, scheduled_for, started_at)
		VALUES ('2026-10-27', '2026-10-27T07:00:00-03:00', '2026-10-27T07:00:00-03:00')`)
	if err != nil {
		t.Fatal(err)
	}
	runID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`INSERT INTO delivery_items (run_id, document_id, chunk_id) VALUES (?, ?, ?)`,
		runID, id, ativo.Chunks[0].ID); err != nil {
		t.Fatal(err)
	}

	if err := s.Documents().Delete(ctx, id); err != nil {
		t.Fatalf("excluir: %v", err)
	}

	if d, err := s.Documents().Get(ctx, id); err != nil || d != nil {
		t.Errorf("documento após exclusão = %+v, %v", d, err)
	}
	if n, err := s.Blocks().Count(ctx, id); err != nil || n != 0 {
		t.Errorf("blocos = %d, %v", n, err)
	}
	if versoes, err := s.Plans().Versions(ctx, id); err != nil || len(versoes) != 0 {
		t.Errorf("planos = %v, %v", versoes, err)
	}
	for _, tabela := range []string{"chunks", "delivery_items"} {
		var n int
		if err := s.DB().QueryRow(`SELECT COUNT(*) FROM `+tabela+` WHERE document_id = ?`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s = %d, quer 0", tabela, n)
		}
	}

	if err := s.Documents().Delete(ctx, id); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("segunda exclusão = %v, quer ErrDocumentNotFound", err)
	}
}

func TestDocumentsTransitionStatus(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	id, err := s.Documents().Create(ctx, docExemplo("Transição", "sha-transicao"))
	if err != nil {
		t.Fatal(err)
	}

	mudou, err := s.Documents().TransitionStatus(ctx, id,
		[]DocumentStatus{StatusProcessing}, StatusReady, agora)
	if err != nil || !mudou {
		t.Fatalf("processing→ready = %v, %v", mudou, err)
	}
	d, err := s.Documents().Get(ctx, id)
	if err != nil || d == nil || d.Status != StatusReady {
		t.Fatalf("documento após transição = %+v, %v", d, err)
	}

	mudou, err = s.Documents().TransitionStatus(ctx, id,
		[]DocumentStatus{StatusProcessing}, StatusFailed, agora)
	if err != nil {
		t.Fatalf("transição inválida devolveu erro: %v", err)
	}
	if mudou {
		t.Error("estado atual ready não permite a transição de processing")
	}
	d, err = s.Documents().Get(ctx, id)
	if err != nil || d == nil || d.Status != StatusReady {
		t.Errorf("status mudou sem permissão: %+v, %v", d, err)
	}

	if _, err := s.Documents().TransitionStatus(ctx, id,
		[]DocumentStatus{StatusReady}, "inexistente", agora); err == nil {
		t.Error("esperado erro para status de destino inválido")
	}
	if _, err := s.Documents().TransitionStatus(ctx, id, nil, StatusPaused, agora); err == nil {
		t.Error("esperado erro sem estados de origem")
	}

	mudou, err = s.Documents().TransitionStatus(ctx, 9999,
		[]DocumentStatus{StatusReady}, StatusPaused, agora)
	if err != nil || mudou {
		t.Errorf("documento inexistente = %v, %v", mudou, err)
	}
}
