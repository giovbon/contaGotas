package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/giovani/readingdrip/internal/blocks"
	"github.com/giovani/readingdrip/internal/ingest"
	"github.com/giovani/readingdrip/internal/store"
	"github.com/giovani/readingdrip/internal/web/views"
)

// pedidoFronteira é o corpo JSON de PATCH /chunks/{id} (RF-023).
type pedidoFronteira struct {
	Acao    string `json:"acao"`
	Bloco   int    `json:"bloco"`
	Direcao string `json:"direcao"`
	Blocos  int    `json:"blocos"`
}

// respostaFronteira resume a edição aplicada.
type respostaFronteira struct {
	DocumentID int64  `json:"document_id"`
	PlanID     int64  `json:"plan_id"`
	Version    int    `json:"version"`
	Chunks     int    `json:"chunks"`
	Acao       string `json:"acao"`
}

// handleEditarFronteira atende a API PATCH /chunks/{id} com corpo JSON.
func (s *Server) handleEditarFronteira(w http.ResponseWriter, r *http.Request) {
	chunk, ok := s.chunkDaRota(w, r)
	if !ok {
		return
	}
	if s.Editor == nil {
		escreverErroJSON(w, http.StatusServiceUnavailable, "edição de fronteira indisponível")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, configBodyLimit)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var pedido pedidoFronteira
	if err := dec.Decode(&pedido); err != nil {
		escreverErroJSON(w, http.StatusBadRequest, "corpo JSON inválido")
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		escreverErroJSON(w, http.StatusBadRequest, "corpo JSON com dados excedentes")
		return
	}

	res, err := s.Editor.EditBoundary(r.Context(), chunk.ID, ingest.BoundaryRequest{
		Action:    pedido.Acao,
		Block:     pedido.Bloco,
		Direction: pedido.Direcao,
		Blocks:    pedido.Blocos,
	})
	if err != nil {
		s.responderErroFronteira(w, r, chunk, err, true)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(respostaFronteira{
		DocumentID: res.DocumentID,
		PlanID:     res.PlanID,
		Version:    res.Version,
		Chunks:     res.Chunks,
		Acao:       res.Action,
	})
}

// handleMesclarChunk junta o chunk com o próximo pelo formulário (sem JS).
func (s *Server) handleMesclarChunk(w http.ResponseWriter, r *http.Request) {
	s.editarFronteiraForm(w, r, ingest.BoundaryRequest{Action: string(store.BoundaryMerge)})
}

// handleDividirChunk divide o chunk antes do bloco informado.
func (s *Server) handleDividirChunk(w http.ResponseWriter, r *http.Request) {
	chunk, ok := s.chunkDaRota(w, r)
	if !ok || !s.formularioFronteiraOK(w, r) {
		return
	}
	s.executarEdicaoForm(w, r, chunk, ingest.BoundaryRequest{
		Action: string(store.BoundarySplit),
		Block:  inteiroDoFormulario(r.PostFormValue("bloco")),
	})
}

// handleMoverChunk move blocos entre o chunk e um vizinho pendente.
func (s *Server) handleMoverChunk(w http.ResponseWriter, r *http.Request) {
	chunk, ok := s.chunkDaRota(w, r)
	if !ok || !s.formularioFronteiraOK(w, r) {
		return
	}
	s.executarEdicaoForm(w, r, chunk, ingest.BoundaryRequest{
		Action:    string(store.BoundaryMove),
		Direction: strings.TrimSpace(r.PostFormValue("direcao")),
		Blocks:    inteiroDoFormulario(r.PostFormValue("blocos")),
	})
}

// editarFronteiraForm atende as ações sem parâmetros (mesclar).
func (s *Server) editarFronteiraForm(w http.ResponseWriter, r *http.Request, req ingest.BoundaryRequest) {
	chunk, ok := s.chunkDaRota(w, r)
	if !ok || !s.formularioFronteiraOK(w, r) {
		return
	}
	s.executarEdicaoForm(w, r, chunk, req)
}

func (s *Server) formularioFronteiraOK(w http.ResponseWriter, r *http.Request) bool {
	if s.Editor == nil {
		http.Error(w, "edição de fronteira indisponível", http.StatusServiceUnavailable)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, configBodyLimit)
	if err := r.ParseForm(); err != nil {
		var grande *http.MaxBytesError
		if errors.As(err, &grande) {
			http.Error(w, "formulário acima do limite", http.StatusRequestEntityTooLarge)
			return false
		}
		http.Error(w, "formulário inválido", http.StatusBadRequest)
		return false
	}
	return true
}

func (s *Server) executarEdicaoForm(w http.ResponseWriter, r *http.Request, chunk *store.Chunk, req ingest.BoundaryRequest) {
	res, err := s.Editor.EditBoundary(r.Context(), chunk.ID, req)
	if err != nil {
		s.responderErroFronteira(w, r, chunk, err, false)
		return
	}
	acao := "fundido"
	switch res.Action {
	case string(store.BoundarySplit):
		acao = "dividido"
	case string(store.BoundaryMove):
		acao = "movido"
	}
	http.Redirect(w, r, "/documents/"+strconv.FormatInt(res.DocumentID, 10)+"/plan?acao="+acao, http.StatusSeeOther)
}

// responderErroFronteira traduz os sentinelas do serviço para a API (JSON) e
// para os formulários (redirect com aviso).
func (s *Server) responderErroFronteira(w http.ResponseWriter, r *http.Request, chunk *store.Chunk, err error, api bool) {
	status := http.StatusInternalServerError
	mensagem := "não foi possível editar a fronteira"
	switch {
	case errors.Is(err, ingest.ErrChunkNotFound):
		status, mensagem = http.StatusNotFound, "trecho não encontrado"
	case errors.Is(err, ingest.ErrChunkNotEditable):
		status, mensagem = http.StatusConflict, "só trechos pendentes podem ser editados"
	case errors.Is(err, ingest.ErrNotProcessable):
		status, mensagem = http.StatusConflict, "documento não está em leitura"
	case errors.Is(err, ingest.ErrInvalidBoundary):
		status, mensagem = http.StatusUnprocessableEntity, "fronteira inválida"
	case errors.Is(err, ingest.ErrNotFound):
		status, mensagem = http.StatusNotFound, "documento não encontrado"
	}
	if status >= http.StatusInternalServerError {
		s.logger().Error("editar fronteira", "chunk", chunk.ID, "erro", err)
	}
	if api {
		escreverErroJSON(w, status, mensagem)
		return
	}

	switch status {
	case http.StatusNotFound:
		s.renderErro(w, r, http.StatusNotFound, "Trecho não encontrado")
	case http.StatusConflict:
		switch {
		case errors.Is(err, ingest.ErrNotProcessable):
			redirecionarAviso(w, r, chunk.DocumentID, "estado")
		default:
			redirecionarAviso(w, r, chunk.DocumentID, "nao-pendente")
		}
	case http.StatusUnprocessableEntity:
		redirecionarAviso(w, r, chunk.DocumentID, "fronteira")
	default:
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível editar a fronteira")
	}
}

func escreverErroJSON(w http.ResponseWriter, status int, mensagem string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"erro": mensagem})
}

// chunkDaRota resolve o chunk da rota, respondendo 404/500/503.
func (s *Server) chunkDaRota(w http.ResponseWriter, r *http.Request) (*store.Chunk, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		s.renderErro(w, r, http.StatusNotFound, "Trecho não encontrado")
		return nil, false
	}
	if s.Store == nil {
		http.Error(w, "banco indisponível", http.StatusServiceUnavailable)
		return nil, false
	}
	chunk, err := s.Store.Plans().Chunk(r.Context(), id)
	if err != nil {
		s.logger().Error("buscar chunk para edição", "id", id, "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível carregar o trecho")
		return nil, false
	}
	if chunk == nil {
		s.renderErro(w, r, http.StatusNotFound, "Trecho não encontrado")
		return nil, false
	}
	return chunk, true
}

// edicoesDoPlano monta os controles de fronteira dos chunks pendentes: faixa
// de blocos, cortes válidos e vizinhos pendentes que podem ceder blocos.
func (s *Server) edicoesDoPlano(ctx context.Context, plano *store.Plan) (map[int64]views.ChunkEdicao, error) {
	ids, err := s.Store.Blocks().IDs(ctx, plano.DocumentID)
	if err != nil {
		return nil, err
	}
	bs, err := s.Store.Blocks().List(ctx, plano.DocumentID)
	if err != nil {
		return nil, err
	}
	if len(bs) == 0 || len(ids) != len(bs) {
		return nil, nil
	}
	indice := make(map[int64]int, len(ids))
	for i, id := range ids {
		indice[id] = i
	}

	edicoes := make(map[int64]views.ChunkEdicao)
	for i, c := range plano.Chunks {
		if c.Status != store.ChunkPending {
			continue
		}
		primeiro, okPrimeiro := indice[c.FirstBlockID]
		ultimo, okUltimo := indice[c.LastBlockID]
		if !okPrimeiro || !okUltimo || primeiro > ultimo || ultimo >= len(bs) {
			continue
		}

		ed := views.ChunkEdicao{BlocoInicio: primeiro, BlocoFim: ultimo}
		if i+1 < len(plano.Chunks) && plano.Chunks[i+1].Status == store.ChunkPending {
			ed.PodeJuntar = true
			if n := blocosDoChunk(plano.Chunks[i+1], indice); n > 1 {
				ed.MaxMoverProximo = n - 1
			}
		}
		if i > 0 && plano.Chunks[i-1].Status == store.ChunkPending {
			if n := blocosDoChunk(plano.Chunks[i-1], indice); n > 1 {
				ed.MaxMoverAnterior = n - 1
			}
		}
		for b := primeiro + 1; b <= ultimo; b++ {
			if bs[b-1].Kind.IsHeading() {
				continue
			}
			ed.Fronteiras = append(ed.Fronteiras, views.Fronteira{
				Bloco:  b,
				Rotulo: rotuloDoBloco(bs[b]),
			})
		}
		edicoes[c.ID] = ed
	}
	return edicoes, nil
}

func blocosDoChunk(c store.Chunk, indice map[int64]int) int {
	primeiro, okPrimeiro := indice[c.FirstBlockID]
	ultimo, okUltimo := indice[c.LastBlockID]
	if !okPrimeiro || !okUltimo || ultimo < primeiro {
		return 0
	}
	return ultimo - primeiro + 1
}

func rotuloDoBloco(b blocks.Block) string {
	texto := strings.Join(strings.Fields(b.Text), " ")
	if texto == "" {
		texto = string(b.Kind)
	}
	if r := []rune(texto); len(r) > 60 {
		texto = string(r[:57]) + "…"
	}
	if b.Kind.IsHeading() {
		return "título: " + texto
	}
	return texto
}
