package store

import (
	"context"
	"errors"
	"testing"

	"github.com/giovani/readingdrip/internal/blocks"
	"github.com/giovani/readingdrip/internal/plan"
)

// blocosTipados monta blocos com kind/segundos escolhidos, mantendo Ord e hash
// contíguos para o gerador.
func blocosTipados(kinds []blocks.Kind, segundos int) []blocks.Block {
	out := make([]blocks.Block, len(kinds))
	for i, k := range kinds {
		out[i] = blocks.Block{
			Ord:     i,
			Kind:    k,
			Level:   2,
			Text:    "Bloco " + string(rune('A'+i)),
			HTML:    "<p>Bloco</p>",
			Words:   3,
			Seconds: segundos,
			Hash:    "h" + string(rune('a'+i)),
		}
	}
	return out
}

func criarPlanoDeTeste(t *testing.T, s *Store, bs []blocks.Block, params plan.Params) (int64, *Plan) {
	t.Helper()
	ctx := context.Background()
	id := documentoComBlocos(t, s, bs)
	pl, err := plan.Generate(bs, params)
	if err != nil {
		t.Fatalf("gerar plano: %v", err)
	}
	if _, err := s.Plans().Create(ctx, id, pl, params, agora); err != nil {
		t.Fatalf("persistir plano: %v", err)
	}
	ativo, err := s.Plans().Active(ctx, id)
	if err != nil || ativo == nil {
		t.Fatalf("plano ativo: %+v, %v", ativo, err)
	}
	return id, ativo
}

func blocosDoChunk(t *testing.T, s *Store, c Chunk, documentID int64) (int, int) {
	t.Helper()
	ids, err := s.Blocks().IDs(context.Background(), documentID)
	if err != nil {
		t.Fatal(err)
	}
	primeiro, ultimo := -1, -1
	for i, id := range ids {
		if id == c.FirstBlockID {
			primeiro = i
		}
		if id == c.LastBlockID {
			ultimo = i
		}
	}
	if primeiro < 0 || ultimo < primeiro {
		t.Fatalf("chunk %d com intervalo inválido %d..%d", c.ID, primeiro, ultimo)
	}
	return primeiro, ultimo
}

func verificarSeqContiguo(t *testing.T, plano *Plan) {
	t.Helper()
	for i, c := range plano.Chunks {
		if c.Seq != i {
			t.Errorf("chunk %d com seq %d", i, c.Seq)
		}
	}
}

func TestEditBoundaryMerge(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	id, ativo := criarPlanoDeTeste(t, s, blocosDeTeste(10, 60),
		plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1})

	primeiro := ativo.Chunks[0]
	segundo := ativo.Chunks[1]

	res, err := s.Plans().EditBoundary(ctx, primeiro.ID, BoundaryEdit{Action: BoundaryMerge})
	if err != nil {
		t.Fatalf("mesclar: %v", err)
	}
	if res.Plan.Version != 1 || len(res.Plan.Chunks) != 9 {
		t.Fatalf("plano = v%d com %d chunks, quer v1 com 9", res.Plan.Version, len(res.Plan.Chunks))
	}
	verificarSeqContiguo(t, res.Plan)

	mesclado := res.Plan.Chunks[0]
	if mesclado.ID != primeiro.ID {
		t.Errorf("id = %d, quer o do primeiro chunk %d", mesclado.ID, primeiro.ID)
	}
	if mesclado.FirstBlockID != primeiro.FirstBlockID || mesclado.LastBlockID != segundo.LastBlockID {
		t.Errorf("pontas = %d..%d, quer %d..%d", mesclado.FirstBlockID,
			mesclado.LastBlockID, primeiro.FirstBlockID, segundo.LastBlockID)
	}
	if mesclado.Words != primeiro.Words+segundo.Words || mesclado.Seconds != primeiro.Seconds+segundo.Seconds {
		t.Errorf("métricas = %dw/%ds, quer %dw/%ds", mesclado.Words, mesclado.Seconds,
			primeiro.Words+segundo.Words, primeiro.Seconds+segundo.Seconds)
	}
	if !mesclado.Oversized {
		t.Error("chunk de 120 s com alvo de 60 s deveria ficar oversized")
	}
	if got := res.Plan.Chunks[1].ID; got == segundo.ID {
		t.Error("o chunk absorvido não deveria continuar no plano")
	}

	recarregado, err := s.Plans().Active(ctx, id)
	if err != nil || recarregado == nil || len(recarregado.Chunks) != 9 {
		t.Fatalf("plano recarregado = %+v, %v", recarregado, err)
	}
}

func TestEditBoundarySplit(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	id, ativo := criarPlanoDeTeste(t, s, blocosDeTeste(6, 30),
		plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1})
	if len(ativo.Chunks) != 3 {
		t.Fatalf("chunks = %d, o teste precisa de 3", len(ativo.Chunks))
	}

	alvo := ativo.Chunks[0]
	primeiro, _ := blocosDoChunk(t, s, alvo, id)

	res, err := s.Plans().EditBoundary(ctx, alvo.ID, BoundaryEdit{
		Action: BoundarySplit,
		Block:  primeiro + 1,
	})
	if err != nil {
		t.Fatalf("dividir: %v", err)
	}
	if len(res.Plan.Chunks) != 4 {
		t.Fatalf("chunks = %d, quer 4", len(res.Plan.Chunks))
	}
	verificarSeqContiguo(t, res.Plan)

	a := res.Plan.Chunks[0]
	b := res.Plan.Chunks[1]
	if a.ID != alvo.ID {
		t.Errorf("primeiro pedaço perdeu o id: %d, quer %d", a.ID, alvo.ID)
	}
	if b.ID == 0 || b.ID == a.ID {
		t.Errorf("segundo pedaço com id inválido %d", b.ID)
	}
	if a.Seconds != 30 || b.Seconds != 30 {
		t.Errorf("durações = %d/%d, quer 30/30", a.Seconds, b.Seconds)
	}
	if !b.StartsMidSection {
		t.Error("segundo pedaço deveria começar no meio da seção")
	}
	if !a.EndsMidSection {
		t.Error("primeiro pedaço deveria terminar no meio da seção")
	}
	if _, err := s.Plans().Active(ctx, id); err != nil {
		t.Fatal(err)
	}
}

func TestEditBoundarySplitRecusaHeading(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	bs := blocosTipados([]blocks.Kind{
		blocks.KindParagraph, blocks.KindH2, blocks.KindParagraph, blocks.KindParagraph,
		blocks.KindParagraph, blocks.KindParagraph,
	}, 20)
	id, ativo := criarPlanoDeTeste(t, s, bs,
		plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1})
	if len(ativo.Chunks) < 2 {
		t.Fatalf("chunks = %d, o teste precisa de pelo menos 2", len(ativo.Chunks))
	}
	alvo := ativo.Chunks[0]
	primeiro, ultimo := blocosDoChunk(t, s, alvo, id)
	if ultimo-primeiro < 2 {
		t.Fatalf("chunk [%d..%d] precisa de um heading interno", primeiro, ultimo)
	}

	// Cortar antes do parágrafo seguinte ao heading deixaria o heading no fim.
	if _, err := s.Plans().EditBoundary(ctx, alvo.ID, BoundaryEdit{
		Action: BoundarySplit,
		Block:  primeiro + 2,
	}); !errors.Is(err, ErrBoundaryInvalid) {
		t.Fatalf("erro = %v, quer ErrBoundaryInvalid", err)
	}
	depois, err := s.Plans().Active(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(depois.Chunks) != len(ativo.Chunks) {
		t.Errorf("plano mudou após recusa: %d chunks, quer %d", len(depois.Chunks), len(ativo.Chunks))
	}

	// Cortar antes do heading é permitido: o título abre o segundo pedaço.
	if _, err := s.Plans().EditBoundary(ctx, alvo.ID, BoundaryEdit{
		Action: BoundarySplit,
		Block:  primeiro + 1,
	}); err != nil {
		t.Fatalf("dividir antes do heading: %v", err)
	}
}

func TestEditBoundaryMove(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	id, ativo := criarPlanoDeTeste(t, s, blocosDeTeste(4, 30),
		plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1})
	if len(ativo.Chunks) != 2 {
		t.Fatalf("chunks = %d, quer 2", len(ativo.Chunks))
	}

	res, err := s.Plans().EditBoundary(ctx, ativo.Chunks[0].ID, BoundaryEdit{
		Action:    BoundaryMove,
		Direction: MoveNext,
		Blocks:    1,
	})
	if err != nil {
		t.Fatalf("mover do próximo: %v", err)
	}
	primeiro, ultimo := blocosDoChunk(t, s, res.Plan.Chunks[0], id)
	if primeiro != 0 || ultimo != 2 {
		t.Errorf("chunk editado = [%d..%d], quer [0..2]", primeiro, ultimo)
	}
	primeiro, ultimo = blocosDoChunk(t, s, res.Plan.Chunks[1], id)
	if primeiro != 3 || ultimo != 3 {
		t.Errorf("vizinho = [%d..%d], quer [3..3]", primeiro, ultimo)
	}
	verificarSeqContiguo(t, res.Plan)

	res, err = s.Plans().EditBoundary(ctx, res.Plan.Chunks[1].ID, BoundaryEdit{
		Action:    BoundaryMove,
		Direction: MovePrev,
		Blocks:    1,
	})
	if err != nil {
		t.Fatalf("mover do anterior: %v", err)
	}
	primeiro, ultimo = blocosDoChunk(t, s, res.Plan.Chunks[0], id)
	if primeiro != 0 || ultimo != 1 {
		t.Errorf("anterior = [%d..%d], quer [0..1]", primeiro, ultimo)
	}
	primeiro, ultimo = blocosDoChunk(t, s, res.Plan.Chunks[1], id)
	if primeiro != 2 || ultimo != 3 {
		t.Errorf("chunk editado = [%d..%d], quer [2..3]", primeiro, ultimo)
	}

	// O vizinho nunca pode ficar vazio.
	if _, err := s.Plans().EditBoundary(ctx, res.Plan.Chunks[0].ID, BoundaryEdit{
		Action:    BoundaryMove,
		Direction: MoveNext,
		Blocks:    2,
	}); !errors.Is(err, ErrBoundaryInvalid) {
		t.Fatalf("erro = %v, quer ErrBoundaryInvalid", err)
	}
}

func TestEditBoundaryMoveRecusaHeading(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	bs := blocosTipados([]blocks.Kind{
		blocks.KindParagraph, blocks.KindH2, blocks.KindParagraph,
		blocks.KindParagraph, blocks.KindParagraph,
	}, 20)
	id, ativo := criarPlanoDeTeste(t, s, bs,
		plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1})
	if len(ativo.Chunks) < 2 {
		t.Fatalf("chunks = %d, quer pelo menos 2", len(ativo.Chunks))
	}

	// Mover 1 bloco do anterior para o segundo chunk faria o anterior terminar
	// no heading (bloco 1).
	if _, err := s.Plans().EditBoundary(ctx, ativo.Chunks[1].ID, BoundaryEdit{
		Action:    BoundaryMove,
		Direction: MovePrev,
		Blocks:    1,
	}); !errors.Is(err, ErrBoundaryInvalid) {
		t.Fatalf("erro = %v, quer ErrBoundaryInvalid", err)
	}

	// Mover 2 blocos é seguro: o anterior termina no parágrafo do bloco 0.
	if _, err := s.Plans().EditBoundary(ctx, ativo.Chunks[1].ID, BoundaryEdit{
		Action:    BoundaryMove,
		Direction: MovePrev,
		Blocks:    2,
	}); err != nil {
		t.Fatalf("mover 2 blocos: %v", err)
	}
	if _, err := s.Plans().Active(ctx, id); err != nil {
		t.Fatal(err)
	}
}

func TestEditBoundaryRecusaNaoPendente(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	_, ativo := criarPlanoDeTeste(t, s, blocosDeTeste(6, 30),
		plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1})
	if len(ativo.Chunks) < 3 {
		t.Fatalf("chunks = %d, quer pelo menos 3", len(ativo.Chunks))
	}
	if err := s.Plans().MarkSent(ctx, ativo.Chunks[0].ID, agora); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Plans().EditBoundary(ctx, ativo.Chunks[0].ID,
		BoundaryEdit{Action: BoundarySplit, Block: 1}); !errors.Is(err, ErrBoundaryChunkNotPending) {
		t.Fatalf("chunk enviado: erro = %v, quer ErrBoundaryChunkNotPending", err)
	}
	// Mesclar um chunk pendente com o anterior enviado também é recusado.
	if _, err := s.Plans().EditBoundary(ctx, ativo.Chunks[1].ID,
		BoundaryEdit{Action: BoundaryMove, Direction: MovePrev, Blocks: 1}); !errors.Is(err, ErrBoundaryChunkNotPending) {
		t.Fatalf("vizinho enviado: erro = %v, quer ErrBoundaryChunkNotPending", err)
	}

	enviado, err := s.Plans().Chunk(ctx, ativo.Chunks[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if enviado.Status != ChunkSent || enviado.SentAt == nil {
		t.Errorf("chunk enviado foi reescrito: %+v", enviado)
	}
}

func TestEditBoundaryRecusaPlanoNaoAtivo(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	id, ativo := criarPlanoDeTeste(t, s, blocosDeTeste(6, 30),
		plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1})

	params := plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1}
	pl, err := plan.Generate(blocosDeTeste(6, 30), params)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Plans().Create(ctx, id, pl, params, agora); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Plans().EditBoundary(ctx, ativo.Chunks[0].ID,
		BoundaryEdit{Action: BoundaryMerge}); !errors.Is(err, ErrBoundaryPlanNotActive) {
		t.Fatalf("erro = %v, quer ErrBoundaryPlanNotActive", err)
	}
}

func TestEditBoundaryChunkInexistente(t *testing.T) {
	s := abrirTeste(t)
	if _, err := s.Plans().EditBoundary(context.Background(), 4242,
		BoundaryEdit{Action: BoundaryMerge}); !errors.Is(err, ErrBoundaryChunkNotFound) {
		t.Fatalf("erro = %v, quer ErrBoundaryChunkNotFound", err)
	}
}

func TestEditBoundaryIntervaloInvalidoNaoAlteraPlano(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()
	_, ativo := criarPlanoDeTeste(t, s, blocosDeTeste(6, 30),
		plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1})
	alvo := ativo.Chunks[0]

	casos := []BoundaryEdit{
		{Action: BoundarySplit, Block: 0},
		{Action: BoundarySplit, Block: 999},
		{Action: BoundaryMove, Direction: MoveNext, Blocks: 3},
		{Action: BoundaryMove, Direction: "lado", Blocks: 1},
		{Action: BoundaryMove, Direction: MovePrev, Blocks: 1},
	}
	for _, edit := range casos {
		if _, err := s.Plans().EditBoundary(ctx, alvo.ID, edit); err == nil {
			t.Errorf("edit %+v: esperado erro", edit)
		}
	}
	depois, err := s.Plans().Active(ctx, ativo.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	if depois.ID != ativo.ID || len(depois.Chunks) != len(ativo.Chunks) {
		t.Errorf("plano mudou após falhas: %+v", depois)
	}
}
