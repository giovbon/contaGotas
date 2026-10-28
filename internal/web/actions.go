package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/giovani/readingdrip/internal/store"
	"github.com/giovani/readingdrip/internal/web/views"
)

// handlePausar tira o documento da próxima run sem tocar no plano nem no
// cursor (RF-004, RF-065). Só `ready` pode pausar; pausar de novo é inofensivo.
func (s *Server) handlePausar(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.documentoDaAcao(w, r)
	if !ok {
		return
	}
	if doc.Status == store.StatusPaused {
		redirecionarAcao(w, r, doc.ID, "pausado")
		return
	}
	if doc.Status != store.StatusReady {
		redirecionarAviso(w, r, doc.ID, "indisponivel")
		return
	}

	mudou, err := s.Store.Documents().TransitionStatus(r.Context(), doc.ID,
		[]store.DocumentStatus{store.StatusReady}, store.StatusPaused, time.Now())
	if err != nil {
		s.logger().Error("pausar documento", "id", doc.ID, "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível pausar o documento")
		return
	}
	if !mudou {
		redirecionarAviso(w, r, doc.ID, "indisponivel")
		return
	}
	s.registrarEvento(r.Context(), doc.ID, store.EventInfo, "documento pausado")
	redirecionarAcao(w, r, doc.ID, "pausado")
}

// handleRetomar devolve o documento à fila de entrega; o cursor continua no
// próximo chunk pendente.
func (s *Server) handleRetomar(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.documentoDaAcao(w, r)
	if !ok {
		return
	}
	if doc.Status == store.StatusReady {
		redirecionarAcao(w, r, doc.ID, "retomado")
		return
	}
	if doc.Status != store.StatusPaused {
		redirecionarAviso(w, r, doc.ID, "indisponivel")
		return
	}

	mudou, err := s.Store.Documents().TransitionStatus(r.Context(), doc.ID,
		[]store.DocumentStatus{store.StatusPaused}, store.StatusReady, time.Now())
	if err != nil {
		s.logger().Error("retomar documento", "id", doc.ID, "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível retomar o documento")
		return
	}
	if !mudou {
		redirecionarAviso(w, r, doc.ID, "indisponivel")
		return
	}
	s.registrarEvento(r.Context(), doc.ID, store.EventInfo, "documento retomado")
	redirecionarAcao(w, r, doc.ID, "retomado")
}

// handleReprocessar reextrai o original. Documentos com histórico de entrega
// não são reextraídos: os ids de bloco antigos não podem ser reaproveitados
// (ver docs/data-model.md#reprocessamento).
func (s *Server) handleReprocessar(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.documentoDaAcao(w, r)
	if !ok {
		return
	}
	if s.Queue == nil {
		http.Error(w, "reprocessamento indisponível", http.StatusServiceUnavailable)
		return
	}
	if doc.Status == store.StatusProcessing {
		redirecionarAviso(w, r, doc.ID, "processando")
		return
	}
	if doc.Status == store.StatusPaused {
		redirecionarAviso(w, r, doc.ID, "pausado")
		return
	}
	switch doc.Status {
	case store.StatusReady, store.StatusFailed, store.StatusNeedsOCR:
	default:
		redirecionarAviso(w, r, doc.ID, "indisponivel")
		return
	}

	historico, err := s.Store.Plans().HasDeliveryHistory(r.Context(), doc.ID)
	if err != nil {
		s.logger().Error("consultar histórico do documento", "id", doc.ID, "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível reprocessar o documento")
		return
	}
	if historico {
		redirecionarAviso(w, r, doc.ID, "historico")
		return
	}

	anterior := doc.Status
	mudou, err := s.Store.Documents().TransitionStatus(r.Context(), doc.ID,
		[]store.DocumentStatus{store.StatusReady, store.StatusFailed, store.StatusNeedsOCR},
		store.StatusProcessing, time.Now())
	if err != nil {
		s.logger().Error("preparar reprocessamento", "id", doc.ID, "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível reprocessar o documento")
		return
	}
	if !mudou {
		redirecionarAviso(w, r, doc.ID, "indisponivel")
		return
	}

	if err := s.Queue.Enqueue(r.Context(), doc.ID); err != nil {
		// A fila não aceitou: devolve o status para não deixar o documento
		// preso em `processing` sem worker.
		if erroStatus := s.Store.Documents().UpdateStatus(r.Context(), doc.ID, anterior, time.Now()); erroStatus != nil {
			s.logger().Error("reverter status após falha na fila",
				"id", doc.ID, "status", anterior, "erro", erroStatus)
		}
		s.logger().Error("enfileirar reprocessamento", "id", doc.ID, "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível reprocessar o documento")
		return
	}

	// O worker recusa documentos pausados ou concluídos; os estados aceitos
	// acima já garantem que a extração vai rodar.
	s.registrarEvento(r.Context(), doc.ID, store.EventInfo, "documento reenfileirado para reprocessamento")
	redirecionarAcao(w, r, doc.ID, "reprocessando")
}

// handleConfirmarExclusao mostra a confirmação antes da ação destrutiva. O
// formulário usa POST porque a UI não depende de JavaScript.
func (s *Server) handleConfirmarExclusao(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.documentoDaAcao(w, r)
	if !ok {
		return
	}
	s.render(w, r, http.StatusOK, views.ConfirmarExclusao(views.ExclusaoProps{Doc: *doc}))
}

// handleExcluir remove o documento e os dados derivados (em cascata no store) e
// depois o diretório do original. Atende ao formulário (POST) e à API
// (DELETE /documents/{id}).
func (s *Server) handleExcluir(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.documentoDaAcao(w, r)
	if !ok {
		return
	}

	if err := s.Store.Documents().Delete(r.Context(), doc.ID); err != nil {
		if errors.Is(err, store.ErrDocumentNotFound) {
			s.renderErro(w, r, http.StatusNotFound, "Documento não encontrado")
			return
		}
		s.logger().Error("excluir documento", "id", doc.ID, "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível excluir o documento")
		return
	}

	if s.Importer != nil {
		if err := s.Importer.RemoveOriginal(doc.ID); err != nil {
			// O registro já saiu; sobra apenas o diretório no disco. Loga sem
			// falhar a ação, que já surtiu efeito para o usuário.
			s.logger().Warn("remover arquivos do documento", "id", doc.ID, "erro", err)
		}
	}

	s.registrarEvento(r.Context(), doc.ID, store.EventInfo, fmt.Sprintf("documento excluído: %q", doc.Title))
	http.Redirect(w, r, "/?excluido=1", http.StatusSeeOther)
}

// documentoDaAcao resolve o documento da rota, respondendo 404/500/503 antes de
// qualquer ação de gestão.
func (s *Server) documentoDaAcao(w http.ResponseWriter, r *http.Request) (*store.Document, bool) {
	id, ok := idDaRota(r)
	if !ok {
		s.renderErro(w, r, http.StatusNotFound, "Documento não encontrado")
		return nil, false
	}
	if s.Store == nil {
		http.Error(w, "banco indisponível", http.StatusServiceUnavailable)
		return nil, false
	}
	doc, err := s.Store.Documents().Get(r.Context(), id)
	if err != nil {
		s.logger().Error("buscar documento para ação", "id", id, "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível carregar o documento")
		return nil, false
	}
	if doc == nil {
		s.renderErro(w, r, http.StatusNotFound, "Documento não encontrado")
		return nil, false
	}
	return doc, true
}

// registrarEvento grava um evento de sistema; falha no log é apenas avisada,
// nunca desfaz a ação que já foi persistida.
func (s *Server) registrarEvento(ctx context.Context, documentID int64, nivel, mensagem string) {
	id := documentID
	err := s.Store.Events().Add(ctx, store.Event{
		At:         time.Now(),
		Level:      nivel,
		Kind:       "system",
		DocumentID: &id,
		Message:    mensagem,
	})
	if err != nil {
		s.logger().Warn("registrar evento de gestão", "id", documentID, "erro", err)
	}
}

func redirecionarAcao(w http.ResponseWriter, r *http.Request, id int64, acao string) {
	http.Redirect(w, r, destinoEnvio(id, "acao", acao, ""), http.StatusSeeOther)
}

func redirecionarAviso(w http.ResponseWriter, r *http.Request, id int64, aviso string) {
	http.Redirect(w, r, destinoEnvio(id, "aviso", aviso, ""), http.StatusSeeOther)
}
