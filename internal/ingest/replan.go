package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/giovani/readingdrip/internal/plan"
	"github.com/giovani/readingdrip/internal/planconfig"
	"github.com/giovani/readingdrip/internal/store"
)

var (
	// ErrNoPlan indica um documento sem plano ativo para regenerar.
	ErrNoPlan = errors.New("ingest: documento sem plano ativo")
	// ErrNothingPending indica que não há trecho pendente: o cursor já passou
	// do fim e não existe restante para replanejar.
	ErrNothingPending = errors.New("ingest: documento sem trechos pendentes")
	// ErrInvalidPacing indica ritmo inválido no pedido de regeneração.
	ErrInvalidPacing = errors.New("ingest: ritmo inválido")
)

// Limites do ritmo aceitos na regeneração. Valem só para o pedido da UI; o
// segmentador em si não limita minutos/dias.
const (
	MaxMinutesPerDay = 600
	MaxPacingDays    = 3650
)

// ReplanRequest é o ritmo pedido para a nova versão do plano.
type ReplanRequest struct {
	Mode    string // minutes_per_day | finish_by_days
	Minutes int    // usado no modo minutes_per_day
	Days    int    // usado no modo finish_by_days
}

// ReplanResult resume a versão criada por Replan.
type ReplanResult struct {
	DocumentID  int64
	PlanID      int64
	Version     int
	Chunks      int
	StartBlock  int
	TotalBlocks int
}

// Replan regenera o plano a partir do cursor, preservando o histórico: a nova
// versão cobre do primeiro trecho pendente até o fim, com o tempo restante, e
// o ritmo do documento é atualizado na mesma transação (RF-024). Nenhum chunk
// já enviado é reescrito ou reenviado.
//
// Só documentos `ready` ou `paused` com plano ativo são replanejados.
func (s *Service) Replan(ctx context.Context, documentID int64, req ReplanRequest) (*ReplanResult, error) {
	if s.Store == nil {
		return nil, errors.New("ingest: serviço sem store")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	agora := s.agora()

	doc, err := s.Store.Documents().Get(ctx, documentID)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("%w: %d", ErrNotFound, documentID)
	}
	switch doc.Status {
	case store.StatusReady, store.StatusPaused:
	default:
		return nil, fmt.Errorf("%w: status %q", ErrNotProcessable, doc.Status)
	}

	ativo, err := s.Store.Plans().Active(ctx, documentID)
	if err != nil {
		return nil, err
	}
	if ativo == nil {
		return nil, fmt.Errorf("%w (documento %d)", ErrNoPlan, documentID)
	}

	cursor, err := s.Store.Plans().NextPending(ctx, documentID)
	if err != nil {
		return nil, err
	}
	if cursor == nil {
		return nil, fmt.Errorf("%w (documento %d)", ErrNothingPending, documentID)
	}

	ids, err := s.Store.Blocks().IDs(ctx, documentID)
	if err != nil {
		return nil, err
	}
	inicio := -1
	for i, id := range ids {
		if id == cursor.FirstBlockID {
			inicio = i
			break
		}
	}
	if inicio < 0 {
		return nil, fmt.Errorf("ingest: bloco %d do cursor não existe no documento %d",
			cursor.FirstBlockID, documentID)
	}

	blocos, err := s.Store.Blocks().List(ctx, documentID)
	if err != nil {
		return nil, err
	}
	if inicio >= len(blocos) {
		return nil, fmt.Errorf("ingest: cursor do documento %d fora dos blocos", documentID)
	}

	padroes, err := planconfig.Load(ctx, s.Store.Settings())
	if err != nil {
		return nil, fmt.Errorf("ingest: ler padrões de plano: %w", err)
	}
	params, err := paramsDoReplan(doc, padroes, req)
	if err != nil {
		return nil, err
	}

	pl, err := plan.GenerateFrom(blocos, params, inicio)
	if err != nil {
		return nil, fmt.Errorf("ingest: regenerar plano do documento %d: %w", documentID, err)
	}

	criado, err := s.Store.Plans().Replan(ctx, documentID, inicio, pl, params, agora)
	if err != nil {
		return nil, err
	}

	s.evento(ctx, store.EventInfo, "plan", doc.ID,
		fmt.Sprintf("plano regenerado a partir do bloco %d: %d chunks (v%d)",
			inicio, len(pl.Chunks), criado.Version), agora)

	return &ReplanResult{
		DocumentID:  doc.ID,
		PlanID:      criado.ID,
		Version:     criado.Version,
		Chunks:      len(pl.Chunks),
		StartBlock:  inicio,
		TotalBlocks: len(blocos),
	}, nil
}

// paramsDoReplan traduz o pedido e os padrões globais para os parâmetros do
// segmentador. WPM/dificuldade vêm do documento, como na extração.
func paramsDoReplan(doc *store.Document, padroes planconfig.Config, req ReplanRequest) (plan.Params, error) {
	params := plan.Params{
		Mode:             plan.Mode(req.Mode),
		WPM:              doc.WPM,
		Difficulty:       doc.Difficulty,
		MinFill:          padroes.MinFill,
		MaxOvershoot:     padroes.MaxOvershoot,
		SectionAlignment: true,
	}
	switch params.Mode {
	case plan.ModeMinutesPerDay:
		if req.Minutes < 1 || req.Minutes > MaxMinutesPerDay {
			return plan.Params{}, fmt.Errorf("%w: minutos por dia deve estar entre 1 e %d",
				ErrInvalidPacing, MaxMinutesPerDay)
		}
		params.Minutes = req.Minutes
		params.Days = 0
	case plan.ModeFinishByDays:
		if req.Days < 1 || req.Days > MaxPacingDays {
			return plan.Params{}, fmt.Errorf("%w: dias de leitura deve estar entre 1 e %d",
				ErrInvalidPacing, MaxPacingDays)
		}
		params.Minutes = 0
		params.Days = req.Days
	default:
		return plan.Params{}, fmt.Errorf("%w: modo %q desconhecido", ErrInvalidPacing, req.Mode)
	}
	return params, nil
}
