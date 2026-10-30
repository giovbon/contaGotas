package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/giovani/readingdrip/internal/ingest"
)

// handleRegenerarPlano cria uma nova versão do plano a partir do primeiro
// trecho pendente, com o ritmo enviado pelo formulário (RF-024). Os trechos já
// enviados continuam no plano anterior e não são reenviados.
func (s *Server) handleRegenerarPlano(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.documentoDaAcao(w, r)
	if !ok {
		return
	}
	if s.Replanner == nil {
		http.Error(w, "regeneração de plano indisponível", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, configBodyLimit)
	if err := r.ParseForm(); err != nil {
		var grande *http.MaxBytesError
		if errors.As(err, &grande) {
			http.Error(w, "formulário acima do limite", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "formulário inválido", http.StatusBadRequest)
		return
	}

	req := ingest.ReplanRequest{
		Mode:    strings.TrimSpace(r.PostFormValue("modo")),
		Minutes: inteiroDoFormulario(r.PostFormValue("minutos")),
		Days:    inteiroDoFormulario(r.PostFormValue("dias")),
	}
	_, err := s.Replanner.Replan(r.Context(), doc.ID, req)
	if err != nil {
		switch {
		case errors.Is(err, ingest.ErrInvalidPacing):
			redirecionarAviso(w, r, doc.ID, "ritmo")
		case errors.Is(err, ingest.ErrNoPlan):
			redirecionarAviso(w, r, doc.ID, "sem-plano")
		case errors.Is(err, ingest.ErrNothingPending):
			redirecionarAviso(w, r, doc.ID, "sem-trechos")
		case errors.Is(err, ingest.ErrNotProcessable):
			redirecionarAviso(w, r, doc.ID, "indisponivel")
		case errors.Is(err, ingest.ErrNotFound):
			s.renderErro(w, r, http.StatusNotFound, "Documento não encontrado")
		default:
			s.logger().Error("regenerar plano", "id", doc.ID, "erro", err)
			s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível regenerar o plano")
		}
		return
	}

	redirecionarAcao(w, r, doc.ID, "regenerado")
}

// inteiroDoFormulario converte um campo de texto em inteiro; valor ausente ou
// inválido vira 0 e é recusado pela validação do serviço.
func inteiroDoFormulario(v string) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0
	}
	return n
}
