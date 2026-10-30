package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/giovani/readingdrip/internal/blocks"
	"github.com/giovani/readingdrip/internal/plan"
)

func blocosDeTeste(n, segundos int) []blocks.Block {
	out := make([]blocks.Block, n)
	for i := range out {
		out[i] = blocks.Block{
			Ord:   i,
			Kind:  blocks.KindParagraph,
			Text:  "Parágrafo de teste.",
			HTML:  "<p>Parágrafo de teste.</p>",
			Words: 3,
			Hash:  "h" + string(rune('a'+i)),
		}
		out[i].Seconds = segundos
	}
	return out
}

func documentoComBlocos(t *testing.T, s *Store, lista []blocks.Block) int64 {
	t.Helper()
	ctx := context.Background()
	id, err := s.Documents().Create(ctx, docExemplo("Livro", "sha-planos"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Blocks().Replace(ctx, id, lista); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPlansCreateActive(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	id := documentoComBlocos(t, s, blocosDeTeste(10, 60))
	params := plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1, WPM: 200, Difficulty: 1, MinFill: 0.6, MaxOvershoot: 1.4}
	pl, err := plan.Generate(blocosDeTeste(10, 60), params)
	if err != nil {
		t.Fatalf("gerar plano: %v", err)
	}

	criado, err := s.Plans().Create(ctx, id, pl, params, agora)
	if err != nil {
		t.Fatalf("persistir plano: %v", err)
	}
	if criado.Version != 1 || criado.Status != PlanActive {
		t.Errorf("plano = %+v", criado)
	}

	ativo, err := s.Plans().Active(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if ativo == nil {
		t.Fatal("plano ativo não encontrado")
	}
	if ativo.ID != criado.ID || ativo.Version != 1 {
		t.Errorf("ativo = %+v", ativo)
	}
	if len(ativo.Chunks) != len(pl.Chunks) {
		t.Fatalf("chunks = %d, quer %d", len(ativo.Chunks), len(pl.Chunks))
	}
	for i, c := range ativo.Chunks {
		if c.Seq != i || c.Status != ChunkPending {
			t.Errorf("chunk %d = %+v", i, c)
		}
		if c.Words != pl.Chunks[i].Words || c.Seconds != pl.Chunks[i].Seconds {
			t.Errorf("chunk %d não preservou palavras/segundos", i)
		}
	}
	if ativo.Params.Minutes != 1 || ativo.Params.Mode != plan.ModeMinutesPerDay {
		t.Errorf("params = %+v", ativo.Params)
	}
	if ativo.TargetSeconds != 60 || ativo.PacingMode != string(plan.ModeMinutesPerDay) {
		t.Errorf("alvo/modo = %d/%q", ativo.TargetSeconds, ativo.PacingMode)
	}
}

func TestPlansNovaVersaoSubstituiAtiva(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	id := documentoComBlocos(t, s, blocosDeTeste(10, 60))
	params := plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1}
	pl, err := plan.Generate(blocosDeTeste(10, 60), params)
	if err != nil {
		t.Fatal(err)
	}
	primeiro, err := s.Plans().Create(ctx, id, pl, params, agora)
	if err != nil {
		t.Fatal(err)
	}
	segundo, err := s.Plans().Create(ctx, id, pl, params, agora.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if segundo.Version != 2 {
		t.Errorf("versão = %d, quer 2", segundo.Version)
	}

	ativo, err := s.Plans().Active(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if ativo.ID != segundo.ID {
		t.Errorf("ativo = %d, quer %d", ativo.ID, segundo.ID)
	}

	antigo, err := s.Plans().Get(ctx, primeiro.ID)
	if err != nil {
		t.Fatal(err)
	}
	if antigo.Status != PlanSuperseded {
		t.Errorf("plano antigo = %q, quer superseded", antigo.Status)
	}

	versoes, err := s.Plans().Versions(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(versoes) != 2 || versoes[0].Version != 2 || versoes[1].Version != 1 {
		t.Fatalf("versões = %+v", versoes)
	}
}

func TestPlansReplanPreservaChunksEnviados(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	blocos := blocosDeTeste(10, 60)
	id := documentoComBlocos(t, s, blocos)
	primeiroParams := plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1}
	pl, err := plan.Generate(blocos, primeiroParams)
	if err != nil {
		t.Fatal(err)
	}
	primeiro, err := s.Plans().Create(ctx, id, pl, primeiroParams, agora)
	if err != nil {
		t.Fatal(err)
	}
	if primeiro, err = s.Plans().Get(ctx, primeiro.ID); err != nil {
		t.Fatal(err)
	}
	if len(primeiro.Chunks) != 10 {
		t.Fatalf("chunks = %d, quer 10", len(primeiro.Chunks))
	}

	for _, seq := range []int{0, 1} {
		if err := s.Plans().MarkSent(ctx, primeiro.Chunks[seq].ID, agora); err != nil {
			t.Fatalf("marcar chunk %d enviado: %v", seq, err)
		}
	}

	ids, err := s.Blocks().IDs(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	restanteParams := plan.Params{Mode: plan.ModeFinishByDays, Days: 7}
	restante, err := plan.GenerateFrom(blocos, restanteParams, 2)
	if err != nil {
		t.Fatal(err)
	}
	novo, err := s.Plans().Replan(ctx, id, 2, restante, restanteParams, agora.Add(time.Hour))
	if err != nil {
		t.Fatalf("replanejar: %v", err)
	}
	if novo.Version != 2 {
		t.Errorf("versão = %d, quer 2", novo.Version)
	}

	ativo, err := s.Plans().Active(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if ativo.ID != novo.ID || len(ativo.Chunks) != len(restante.Chunks) {
		t.Fatalf("ativo = %+v", ativo)
	}
	if got := ativo.Chunks[0].FirstBlockID; got != ids[2] {
		t.Errorf("primeiro bloco = %d, quer %d (cursor)", got, ids[2])
	}
	if got := ativo.Chunks[len(ativo.Chunks)-1].LastBlockID; got != ids[len(ids)-1] {
		t.Errorf("último bloco = %d, quer %d", got, ids[len(ids)-1])
	}
	for _, c := range ativo.Chunks {
		if c.Status != ChunkPending {
			t.Errorf("chunk novo %d com status %q", c.Seq, c.Status)
		}
	}

	// O plano antigo continua com os chunks entregues, sem reescrita.
	antigo, err := s.Plans().Get(ctx, primeiro.ID)
	if err != nil {
		t.Fatal(err)
	}
	if antigo.Status != PlanSuperseded {
		t.Errorf("plano antigo = %q, quer superseded", antigo.Status)
	}
	if antigo.Chunks[0].Status != ChunkSent || antigo.Chunks[1].Status != ChunkSent {
		t.Errorf("chunks enviados do plano antigo = %q/%q",
			antigo.Chunks[0].Status, antigo.Chunks[1].Status)
	}
	if antigo.Chunks[0].SentAt == nil {
		t.Error("chunk enviado perdeu sent_at")
	}

	doc, err := s.Documents().Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PacingMode != string(plan.ModeFinishByDays) || doc.PacingDays == nil || *doc.PacingDays != 7 {
		t.Errorf("ritmo = %q/%v, quer finish_by_days/7", doc.PacingMode, doc.PacingDays)
	}
	if doc.PacingMinutes != 10 {
		t.Errorf("minutos preservados = %d, quer 10", doc.PacingMinutes)
	}
}

func TestPlansReplanRecusaInicioInvalido(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	blocos := blocosDeTeste(5, 60)
	id := documentoComBlocos(t, s, blocos)
	params := plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1}
	pl, err := plan.Generate(blocos, params)
	if err != nil {
		t.Fatal(err)
	}
	primeiro, err := s.Plans().Create(ctx, id, pl, params, agora)
	if err != nil {
		t.Fatal(err)
	}

	novoParams := plan.Params{Mode: plan.ModeFinishByDays, Days: 3}
	restante, err := plan.GenerateFrom(blocos, novoParams, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Plans().Replan(ctx, id, 99, restante, novoParams, agora); err == nil {
		t.Fatal("esperado erro de início fora do documento")
	}

	ativo, err := s.Plans().Active(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if ativo.ID != primeiro.ID {
		t.Errorf("plano ativo mudou após falha: %d, quer %d", ativo.ID, primeiro.ID)
	}
	doc, err := s.Documents().Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PacingMode != string(plan.ModeMinutesPerDay) {
		t.Errorf("ritmo mudou após falha: %q", doc.PacingMode)
	}
}

func TestPlansCoberturaInvalida(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	id := documentoComBlocos(t, s, blocosDeTeste(10, 60))

	params := plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1}
	pl, err := plan.Generate(blocosDeTeste(10, 60), params)
	if err != nil {
		t.Fatal(err)
	}
	// Abre um buraco entre o primeiro e o segundo chunk.
	pl.Chunks[1].FirstBlock = pl.Chunks[0].LastBlock + 2

	if _, err := s.Plans().Create(ctx, id, pl, params, agora); err == nil {
		t.Fatal("esperado erro de cobertura")
	}
	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM plans WHERE document_id = ?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("planos gravados = %d, quer 0", n)
	}
}

func TestPlansCursorEProgresso(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	id := documentoComBlocos(t, s, blocosDeTeste(10, 60))
	params := plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1}
	pl, err := plan.Generate(blocosDeTeste(10, 60), params)
	if err != nil {
		t.Fatal(err)
	}
	criado, err := s.Plans().Create(ctx, id, pl, params, agora)
	if err != nil {
		t.Fatal(err)
	}

	proximo, err := s.Plans().NextPending(ctx, id)
	if err != nil || proximo == nil {
		t.Fatalf("próximo: %v, %v", proximo, err)
	}
	if proximo.Seq != 0 {
		t.Fatalf("seq = %d, quer 0", proximo.Seq)
	}

	if err := s.Plans().MarkSent(ctx, proximo.ID, agora); err != nil {
		t.Fatalf("marcar enviado: %v", err)
	}
	// DB-08: chunk enviado não é reescrito.
	if err := s.Plans().MarkSent(ctx, proximo.ID, agora); err == nil {
		t.Fatal("esperado erro ao reenviar chunk já enviado")
	}

	seguinte, err := s.Plans().NextPending(ctx, id)
	if err != nil || seguinte == nil {
		t.Fatalf("próximo 2: %v, %v", seguinte, err)
	}
	if seguinte.Seq != 1 {
		t.Errorf("seq = %d, quer 1", seguinte.Seq)
	}
	if err := s.Plans().MarkSkipped(ctx, seguinte.ID); err != nil {
		t.Fatalf("pular: %v", err)
	}

	progresso, err := s.Plans().Progress(ctx, criado.ID)
	if err != nil {
		t.Fatal(err)
	}
	if progresso.Total != len(pl.Chunks) || progresso.Enviados != 1 || progresso.Pulados != 1 {
		t.Errorf("progresso = %+v", progresso)
	}
	if progresso.Pct <= 0 {
		t.Errorf("percentual = %v", progresso.Pct)
	}

	if err := s.Plans().MarkDone(ctx, proximo.ID, agora.Add(time.Hour)); err != nil {
		t.Fatalf("marcar lido: %v", err)
	}
	ativo, err := s.Plans().Active(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if ativo.Chunks[0].DoneAt == nil {
		t.Error("done_at não gravado")
	}
	if ativo.Chunks[0].SentAt == nil {
		t.Error("sent_at não gravado")
	}
}

func TestPlansCreateSemBlocos(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	id, err := s.Documents().Create(ctx, docExemplo("Vazio", "sha-vazio"))
	if err != nil {
		t.Fatal(err)
	}
	pl := &plan.Plan{
		Mode:          plan.ModeMinutesPerDay,
		TargetSeconds: 60,
		Chunks:        []plan.Chunk{{Seq: 0, FirstBlock: 0, LastBlock: 0}},
	}
	_, err = s.Plans().Create(ctx, id, pl, plan.Params{Minutes: 1}, agora)
	if err == nil {
		t.Fatal("esperado erro para documento sem blocos")
	}
	if !strings.Contains(err.Error(), "blocos") {
		t.Errorf("erro = %v", err)
	}
}

func TestPlansHorizonDays(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	id := documentoComBlocos(t, s, blocosDeTeste(10, 60))
	params := plan.Params{Mode: plan.ModeFinishByDays, Days: 5}
	pl, err := plan.Generate(blocosDeTeste(10, 60), params)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Plans().Create(ctx, id, pl, params, agora); err != nil {
		t.Fatal(err)
	}

	ativo, err := s.Plans().Active(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if ativo.HorizonDays == nil || *ativo.HorizonDays != 5 {
		t.Errorf("horizon_days = %v, quer 5", ativo.HorizonDays)
	}
	if ativo.Params.Days != 5 || ativo.Params.Mode != plan.ModeFinishByDays {
		t.Errorf("params = %+v", ativo.Params)
	}
}

func TestPlansHasDeliveryHistory(t *testing.T) {
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

	historico, err := s.Plans().HasDeliveryHistory(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if historico {
		t.Fatal("plano recém-criado não tem histórico")
	}

	ativo, err := s.Plans().Active(ctx, id)
	if err != nil || ativo == nil {
		t.Fatalf("plano ativo: %v, %v", ativo, err)
	}
	if err := s.Plans().MarkSent(ctx, ativo.Chunks[0].ID, agora); err != nil {
		t.Fatal(err)
	}

	historico, err = s.Plans().HasDeliveryHistory(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !historico {
		t.Fatal("chunk enviado deveria contar como histórico")
	}
	if err := s.Plans().DiscardUndelivered(ctx, id); !errors.Is(err, ErrDeliveryHistory) {
		t.Fatalf("descartar com histórico: %v, quer ErrDeliveryHistory", err)
	}
}
