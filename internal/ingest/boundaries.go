package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/giovani/readingdrip/internal/store"
)

var (
	// ErrChunkNotFound indica um chunk inexistente na edição de fronteiras.
	ErrChunkNotFound = errors.New("ingest: trecho não encontrado")
	// ErrChunkNotEditable indica chunk fora do plano ativo ou já enviado/pulado.
	ErrChunkNotEditable = errors.New("ingest: trecho não pode ser editado")
	// ErrInvalidBoundary indica uma operação que quebraria as invariantes do
	// plano (intervalo inválido, corte em heading, vizinho vazio).
	ErrInvalidBoundary = errors.New("ingest: fronteira inválida")
)

// BoundaryRequest é o pedido de edição de fronteira (RF-023).
type BoundaryRequest struct {
	Action    string // merge | split | move
	Block     int    // split: índice absoluto do primeiro bloco do novo trecho
	Direction string // move: next (padrão) | prev
	Blocks    int    // move: quantidade (padrão 1)
}

// BoundaryResult resume a edição aplicada ao plano ativo.
type BoundaryResult struct {
	DocumentID int64
	PlanID     int64
	Version    int
	Chunks     int
	Action     string
}

// EditBoundary mescla, divide ou move blocos de um chunk pendente do plano
// ativo, sem reextrair o documento e sem afetar chunks já entregues (RF-023).
//
// A edição muta o plano ativo no lugar; o store revalida as invariantes antes
// do commit. Só documentos `ready`/`paused` são editáveis.
func (s *Service) EditBoundary(ctx context.Context, chunkID int64, req BoundaryRequest) (*BoundaryResult, error) {
	if s.Store == nil {
		return nil, errors.New("ingest: serviço sem store")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	agora := s.agora()

	chunk, err := s.Store.Plans().Chunk(ctx, chunkID)
	if err != nil {
		return nil, err
	}
	if chunk == nil {
		return nil, fmt.Errorf("%w: %d", ErrChunkNotFound, chunkID)
	}
	doc, err := s.Store.Documents().Get(ctx, chunk.DocumentID)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("%w: %d", ErrNotFound, chunk.DocumentID)
	}
	switch doc.Status {
	case store.StatusReady, store.StatusPaused:
	default:
		return nil, fmt.Errorf("%w: status %q", ErrNotProcessable, doc.Status)
	}

	edit := store.BoundaryEdit{
		Action:    store.BoundaryAction(req.Action),
		Block:     req.Block,
		Direction: store.MoveDirection(req.Direction),
		Blocks:    req.Blocks,
	}
	res, err := s.Store.Plans().EditBoundary(ctx, chunkID, edit)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrBoundaryChunkNotFound):
			return nil, fmt.Errorf("%w: %v", ErrChunkNotFound, err)
		case errors.Is(err, store.ErrBoundaryPlanNotActive),
			errors.Is(err, store.ErrBoundaryChunkNotPending):
			return nil, fmt.Errorf("%w: %v", ErrChunkNotEditable, err)
		case errors.Is(err, store.ErrBoundaryInvalid):
			return nil, fmt.Errorf("%w: %v", ErrInvalidBoundary, err)
		default:
			return nil, err
		}
	}

	s.evento(ctx, store.EventInfo, "plan", doc.ID,
		fmt.Sprintf("fronteira editada: %s no chunk %d (%d trechos no plano v%d)",
			req.Action, chunkID, len(res.Plan.Chunks), res.Plan.Version), agora)

	return &BoundaryResult{
		DocumentID: doc.ID,
		PlanID:     res.Plan.ID,
		Version:    res.Plan.Version,
		Chunks:     len(res.Plan.Chunks),
		Action:     string(res.Action),
	}, nil
}
