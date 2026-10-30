package ingest_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/giovani/readingdrip/internal/ingest"
	"github.com/giovani/readingdrip/internal/plan"
	"github.com/giovani/readingdrip/internal/store"
)

func textoLongo() string {
	return strings.Repeat(textoMarkdown(), 2)
}

func TestReplanAPartirDoCursor(t *testing.T) {
	s, svc := abrirServico(t)
	ctx := context.Background()
	id := criarDocumento(t, s, store.Document{
		Title:         "Replanejado",
		SourcePath:    escrever(t, "replan.md", textoLongo()),
		PacingMinutes: 1,
	})
	if _, err := svc.Process(ctx, id); err != nil {
		t.Fatalf("processar: %v", err)
	}

	antigo, err := s.Plans().Active(ctx, id)
	if err != nil || antigo == nil {
		t.Fatalf("plano ativo: %+v, %v", antigo, err)
	}
	if len(antigo.Chunks) < 2 {
		t.Fatalf("chunks = %d, o teste precisa de pelo menos 2", len(antigo.Chunks))
	}
	if err := s.Plans().MarkSent(ctx, antigo.Chunks[0].ID, agora); err != nil {
		t.Fatalf("marcar enviado: %v", err)
	}
	cursor := antigo.Chunks[1]

	res, err := svc.Replan(ctx, id, ingest.ReplanRequest{Mode: string(plan.ModeFinishByDays), Days: 5})
	if err != nil {
		t.Fatalf("replanejar: %v", err)
	}
	if res.Version != 2 || res.Chunks == 0 {
		t.Errorf("resultado = %+v", res)
	}
	if res.StartBlock == 0 {
		t.Errorf("início = %d, o cursor deveria estar depois do bloco 0", res.StartBlock)
	}

	ativo, err := s.Plans().Active(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if ativo.ID != res.PlanID || ativo.Version != 2 {
		t.Fatalf("ativo = %+v", ativo)
	}
	if got := ativo.Chunks[0].FirstBlockID; got != cursor.FirstBlockID {
		t.Errorf("primeiro bloco do novo plano = %d, quer %d", got, cursor.FirstBlockID)
	}
	if got, want := ativo.Params.Mode, plan.ModeFinishByDays; got != want {
		t.Errorf("modo do plano = %q, quer %q", got, want)
	}
	if ativo.Params.Days != 5 {
		t.Errorf("dias = %d, quer 5", ativo.Params.Days)
	}

	// O chunk enviado não é reescrito nem volta a pendente.
	planoAntigo, err := s.Plans().Get(ctx, antigo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if planoAntigo.Status != store.PlanSuperseded {
		t.Errorf("plano antigo = %q, quer superseded", planoAntigo.Status)
	}
	if planoAntigo.Chunks[0].Status != store.ChunkSent {
		t.Errorf("chunk enviado = %q, quer sent", planoAntigo.Chunks[0].Status)
	}

	doc, err := s.Documents().Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PacingMode != string(plan.ModeFinishByDays) || doc.PacingDays == nil || *doc.PacingDays != 5 {
		t.Errorf("ritmo = %q/%v, quer finish_by_days/5", doc.PacingMode, doc.PacingDays)
	}
}

func TestReplanAceitaDocumentoPausado(t *testing.T) {
	s, svc := abrirServico(t)
	ctx := context.Background()
	id := criarDocumento(t, s, store.Document{
		Title:         "Pausado",
		SourcePath:    escrever(t, "pausado.md", textoLongo()),
		PacingMinutes: 1,
	})
	if _, err := svc.Process(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := s.Documents().UpdateStatus(ctx, id, store.StatusPaused, agora); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Replan(ctx, id, ingest.ReplanRequest{Mode: string(plan.ModeMinutesPerDay), Minutes: 2}); err != nil {
		t.Fatalf("replanejar pausado: %v", err)
	}
}

func TestReplanRecusaRitmoInvalido(t *testing.T) {
	s, svc := abrirServico(t)
	ctx := context.Background()
	id := criarDocumento(t, s, store.Document{Title: "Ritmo", SourcePath: escrever(t, "ritmo.md", textoLongo())})
	if _, err := svc.Process(ctx, id); err != nil {
		t.Fatal(err)
	}
	antes, err := s.Plans().Active(ctx, id)
	if err != nil {
		t.Fatal(err)
	}

	casos := []ingest.ReplanRequest{
		{Mode: string(plan.ModeMinutesPerDay), Minutes: 0},
		{Mode: string(plan.ModeMinutesPerDay), Minutes: ingest.MaxMinutesPerDay + 1},
		{Mode: string(plan.ModeFinishByDays), Days: 0},
		{Mode: string(plan.ModeFinishByDays), Days: ingest.MaxPacingDays + 1},
		{Mode: "semanal"},
	}
	for _, req := range casos {
		if _, err := svc.Replan(ctx, id, req); !errors.Is(err, ingest.ErrInvalidPacing) {
			t.Errorf("req %+v: erro = %v, quer ErrInvalidPacing", req, err)
		}
	}

	depois, err := s.Plans().Active(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if depois.ID != antes.ID {
		t.Errorf("plano mudou após ritmo inválido: %d, quer %d", depois.ID, antes.ID)
	}
}

func TestReplanSemTrechosPendentes(t *testing.T) {
	s, svc := abrirServico(t)
	ctx := context.Background()
	id := criarDocumento(t, s, store.Document{Title: "Fim", SourcePath: escrever(t, "fim.md", textoLongo())})
	if _, err := svc.Process(ctx, id); err != nil {
		t.Fatal(err)
	}
	plano, err := s.Plans().Active(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range plano.Chunks {
		if err := s.Plans().MarkSent(ctx, c.ID, agora); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := svc.Replan(ctx, id, ingest.ReplanRequest{Mode: string(plan.ModeMinutesPerDay), Minutes: 10}); !errors.Is(err, ingest.ErrNothingPending) {
		t.Fatalf("erro = %v, quer ErrNothingPending", err)
	}
}

func TestReplanDocumentoSemPlano(t *testing.T) {
	s, svc := abrirServico(t)
	ctx := context.Background()
	id := criarDocumento(t, s, store.Document{Title: "Sem plano", SourcePath: escrever(t, "sem.md", textoLongo())})
	if ok, err := s.Documents().TransitionStatus(ctx, id, []store.DocumentStatus{store.StatusProcessing}, store.StatusReady, agora); err != nil || !ok {
		t.Fatalf("marcar ready: ok=%v err=%v", ok, err)
	}

	if _, err := svc.Replan(ctx, id, ingest.ReplanRequest{Mode: string(plan.ModeMinutesPerDay), Minutes: 10}); !errors.Is(err, ingest.ErrNoPlan) {
		t.Fatalf("erro = %v, quer ErrNoPlan", err)
	}
}

func TestReplanDocumentoInexistente(t *testing.T) {
	_, svc := abrirServico(t)
	if _, err := svc.Replan(context.Background(), 12345, ingest.ReplanRequest{Mode: string(plan.ModeMinutesPerDay), Minutes: 10}); !errors.Is(err, ingest.ErrNotFound) {
		t.Fatalf("erro = %v, quer ErrNotFound", err)
	}
}
