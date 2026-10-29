// Package ingest executa a extração de um documento e persiste o resultado:
// blocos, totais, plano ativo e status. É o worker chamado pelo upload.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/giovani/readingdrip/internal/blocks"
	"github.com/giovani/readingdrip/internal/docs"
	"github.com/giovani/readingdrip/internal/docs/pdf"
	"github.com/giovani/readingdrip/internal/plan"
	"github.com/giovani/readingdrip/internal/planconfig"
	"github.com/giovani/readingdrip/internal/store"
)

var (
	// ErrNotFound indica um id de documento inexistente.
	ErrNotFound = errors.New("ingest: documento não encontrado")
	// ErrNotProcessable indica um documento em estado que não deve ser
	// sobrescrito pelo worker (pausado ou concluído).
	ErrNotProcessable = errors.New("ingest: documento não pode ser processado")
)

// Service processa documentos um por vez. O chamador controla a concorrência.
type Service struct {
	Store             *store.Store
	Now               func() time.Time // injetado nos testes; nunca usado pelo store
	MaxParagraphWords int              // 0 → 300 (ver blocks.Prepare)
}

// Result resume o que foi persistido por um Process bem-sucedido.
type Result struct {
	DocumentID int64
	Status     store.DocumentStatus
	Title      string
	Blocks     int
	Words      int
	Seconds    int
	PlanID     int64
	PlanVer    int
	Chunks     int
}

// Process extrai o arquivo original do documento, normaliza os blocos, gera e
// persiste o plano ativo e grava o status final.
//
// Em falha de extração o documento fica `failed` (ou `needs_ocr` para PDF sem
// camada de texto) com a mensagem no campo `error`, e o erro original é
// devolvido. Reprocessamentos descartam planos ainda não entregues; se houver
// histórico de entrega, nada é alterado e o erro é `store.ErrDeliveryHistory`.
func (s *Service) Process(ctx context.Context, documentID int64) (*Result, error) {
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
	if doc.Status == store.StatusPaused || doc.Status == store.StatusCompleted {
		return nil, fmt.Errorf("%w: status %q", ErrNotProcessable, doc.Status)
	}

	historico, err := s.Store.Plans().HasDeliveryHistory(ctx, doc.ID)
	if err != nil {
		return nil, err
	}
	if historico {
		return nil, fmt.Errorf("ingest: documento %d não pode ser reextraído: %w", documentID, store.ErrDeliveryHistory)
	}

	res, err := docs.ExtractFile(ctx, doc.SourcePath, doc.Title)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		status := store.StatusFailed
		if errors.Is(err, pdf.ErrScanned) {
			status = store.StatusNeedsOCR
		}
		msg := err.Error()
		if status == store.StatusNeedsOCR {
			msg = "PDF sem camada de texto: converta o arquivo com OCR antes de subir. " + msg
		}
		falha := s.falhar(ctx, doc, status, msg, agora)
		return nil, errors.Join(fmt.Errorf("ingest: extrair documento %d: %w", documentID, err), falha)
	}

	prepared := blocks.Prepare(res.Blocks, blocks.PrepareOptions{
		Estimator:         blocks.Estimator{WPM: doc.WPM, Difficulty: doc.Difficulty},
		MaxParagraphWords: s.MaxParagraphWords,
	})
	if len(prepared) == 0 {
		err := errors.New("ingest: extração não gerou blocos de conteúdo")
		falha := s.falhar(ctx, doc, store.StatusFailed, err.Error(), agora)
		return nil, errors.Join(err, falha)
	}

	padroes, err := planconfig.Load(ctx, s.Store.Settings())
	if err != nil {
		falha := s.falhar(ctx, doc, store.StatusFailed, err.Error(), agora)
		return nil, errors.Join(fmt.Errorf("ingest: ler padrões de plano: %w", err), falha)
	}
	params, err := paramsDoDocumento(doc, padroes)
	if err != nil {
		falha := s.falhar(ctx, doc, store.StatusFailed, err.Error(), agora)
		return nil, errors.Join(err, falha)
	}

	pl, err := plan.Generate(prepared, params)
	if err != nil {
		falha := s.falhar(ctx, doc, store.StatusFailed, err.Error(), agora)
		return nil, errors.Join(fmt.Errorf("ingest: gerar plano do documento %d: %w", documentID, err), falha)
	}

	if err := s.Store.Plans().DiscardUndelivered(ctx, doc.ID); err != nil {
		if errors.Is(err, store.ErrDeliveryHistory) {
			return nil, fmt.Errorf("ingest: documento %d não pode ser reextraído: %w", documentID, err)
		}
		falha := s.falhar(ctx, doc, store.StatusFailed, err.Error(), agora)
		return nil, errors.Join(err, falha)
	}
	if err := s.Store.Blocks().Replace(ctx, doc.ID, prepared); err != nil {
		falha := s.falhar(ctx, doc, store.StatusFailed, err.Error(), agora)
		return nil, errors.Join(err, falha)
	}

	criado, err := s.Store.Plans().Create(ctx, doc.ID, pl, params, agora)
	if err != nil {
		falha := s.falhar(ctx, doc, store.StatusFailed, err.Error(), agora)
		return nil, errors.Join(err, falha)
	}

	totalWords, totalSeconds := 0, 0
	for _, b := range prepared {
		totalWords += b.Words
		totalSeconds += b.Seconds
	}

	extraida := store.Extraction{
		Title:        res.Title,
		Author:       res.Author,
		Language:     res.Language,
		TotalWords:   totalWords,
		TotalSeconds: totalSeconds,
		Status:       store.StatusReady,
	}
	if err := s.Store.Documents().SaveExtraction(ctx, doc.ID, extraida, agora); err != nil {
		falha := s.falhar(ctx, doc, store.StatusFailed, err.Error(), agora)
		return nil, errors.Join(err, falha)
	}

	s.evento(ctx, store.EventInfo, "plan", doc.ID,
		fmt.Sprintf("documento processado: %d blocos, %d chunks (plano v%d)",
			len(prepared), len(pl.Chunks), criado.Version), agora)

	titulo := res.Title
	if titulo == "" {
		titulo = doc.Title
	}
	return &Result{
		DocumentID: doc.ID,
		Status:     store.StatusReady,
		Title:      titulo,
		Blocks:     len(prepared),
		Words:      totalWords,
		Seconds:    totalSeconds,
		PlanID:     criado.ID,
		PlanVer:    criado.Version,
		Chunks:     len(pl.Chunks),
	}, nil
}

func (s *Service) agora() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// paramsDoDocumento traduz os fatores de ritmo gravados no documento e os
// padrões globais de plano para os parâmetros do segmentador.
func paramsDoDocumento(doc *store.Document, padroes planconfig.Config) (plan.Params, error) {
	params := plan.Params{
		Mode:             plan.Mode(doc.PacingMode),
		Minutes:          doc.PacingMinutes,
		WPM:              doc.WPM,
		Difficulty:       doc.Difficulty,
		MinFill:          padroes.MinFill,
		MaxOvershoot:     padroes.MaxOvershoot,
		SectionAlignment: true,
	}
	switch params.Mode {
	case plan.ModeMinutesPerDay:
		params.Days = 0
	case plan.ModeFinishByDays:
		params.Minutes = 0
		if doc.PacingDays == nil || *doc.PacingDays <= 0 {
			return plan.Params{}, fmt.Errorf("ingest: finish_by_days sem dias de leitura no documento %d", doc.ID)
		}
		params.Days = *doc.PacingDays
	default:
		return plan.Params{}, fmt.Errorf("ingest: modo de ritmo desconhecido %q no documento %d", doc.PacingMode, doc.ID)
	}
	return params, nil
}

// falhar grava o estado de erro e o evento correspondente, preservando os
// metadados e totais que já estavam no documento.
func (s *Service) falhar(ctx context.Context, doc *store.Document, status store.DocumentStatus, msg string, agora time.Time) error {
	campos := store.Extraction{
		Title:        doc.Title,
		Language:     doc.Language,
		TotalWords:   doc.TotalWords,
		TotalSeconds: doc.TotalSeconds,
		Status:       status,
		Error:        msg,
	}
	if doc.Author != nil {
		campos.Author = *doc.Author
	}
	erroStatus := s.Store.Documents().SaveExtraction(ctx, doc.ID, campos, agora)

	level := store.EventError
	if status == store.StatusNeedsOCR {
		level = store.EventWarn
	}
	erroEvento := s.Store.Events().Add(ctx, store.Event{
		At:         agora,
		Level:      level,
		Kind:       "extract",
		DocumentID: &doc.ID,
		Message:    msg,
	})
	return errors.Join(erroStatus, erroEvento)
}

func (s *Service) evento(ctx context.Context, level, kind string, docID int64, msg string, agora time.Time) {
	id := docID
	_ = s.Store.Events().Add(ctx, store.Event{
		At: agora, Level: level, Kind: kind, DocumentID: &id, Message: msg,
	})
}
