package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/giovani/readingdrip/internal/store"
	"github.com/giovani/readingdrip/internal/web/views"
)

// historicoLimite é quantas runs a página mostra (docs/web-ui.md#histórico).
const historicoLimite = 30

// handleHistorico lista as runs mais recentes com o detalhe por documento.
// O filtro `?documento={id}` mostra só as runs que tiveram aquele documento;
// o filtro inválido é ignorado.
func (s *Server) handleHistorico(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil {
		http.Error(w, "banco indisponível", http.StatusServiceUnavailable)
		return
	}
	ctx := r.Context()

	filtro := queryID(r, "documento")
	docs, err := s.Store.Documents().List(ctx)
	if err != nil {
		s.logger().Error("listar documentos do histórico", "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível carregar o histórico")
		return
	}
	runs, err := s.Store.Runs().List(ctx, historicoLimite, filtro)
	if err != nil {
		s.logger().Error("listar runs", "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível carregar o histórico")
		return
	}

	titulos := make(map[int64]string, len(docs))
	for _, d := range docs {
		titulos[d.ID] = d.Title
	}

	props := views.HistoricoProps{
		Documentos: docs,
		Filtro:     filtro,
		Runs:       make([]views.HistoricoRun, 0, len(runs)),
	}
	for _, run := range runs {
		itens, err := s.Store.Runs().ListItems(ctx, run.ID)
		if err != nil {
			s.logger().Error("listar itens da run", "run", run.ID, "erro", err)
			s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível carregar o histórico")
			return
		}
		props.Runs = append(props.Runs, montarRunHistorico(run, itens, filtro, titulos))
	}

	s.render(w, r, http.StatusOK, views.Historico(props))
}

// montarRunHistorico agrega os itens de uma run. Com filtro, só os itens do
// documento aparecem e as somas refletem apenas ele.
func montarRunHistorico(run store.DeliveryRun, itens []store.DeliveryItem, filtro int64, titulos map[int64]string) views.HistoricoRun {
	out := views.HistoricoRun{Run: run}
	for _, item := range itens {
		if filtro > 0 && item.DocumentID != filtro {
			continue
		}
		titulo := titulos[item.DocumentID]
		if strings.TrimSpace(titulo) == "" {
			titulo = "Documento removido"
		}
		out.Itens = append(out.Itens, views.HistoricoItem{Item: item, Titulo: titulo})
		if item.Bytes != nil {
			out.Bytes += *item.Bytes
		}
		switch item.Status {
		case store.ItemSent:
			out.Enviados++
		case store.ItemFailed:
			out.Falhas++
		case store.ItemSkipped:
			out.Pulados++
		case store.ItemPending:
			out.Pendentes++
		}
	}
	if run.FinishedAt != nil {
		if latencia := run.FinishedAt.Sub(run.StartedAt); latencia > 0 {
			out.Latencia = latencia
		}
	}
	return out
}

// queryID lê um id positivo da query; qualquer outra coisa (vazio, texto,
// negativo) vira 0, que significa “sem filtro”.
func queryID(r *http.Request, chave string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get(chave)), 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
