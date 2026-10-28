package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/giovani/readingdrip/internal/blocks"
	"github.com/giovani/readingdrip/internal/links"
	"github.com/giovani/readingdrip/internal/schedule"
	"github.com/giovani/readingdrip/internal/web/views"
)

// handleLeitor renderiza o trecho de um chunk acessado por link assinado
// (ADR-0010). Não exige sessão: o token é a credencial, restrito ao chunk e
// com validade própria.
func (s *Server) handleLeitor(w http.ResponseWriter, r *http.Request) {
	id, ok := idDaRota(r)
	if !ok {
		s.renderErro(w, r, http.StatusNotFound, "Trecho não encontrado")
		return
	}
	if s.Store == nil {
		http.Error(w, "banco indisponível", http.StatusServiceUnavailable)
		return
	}
	if s.Links == nil {
		http.Error(w, "leitor indisponível", http.StatusServiceUnavailable)
		return
	}
	if err := s.Links.Verify(links.ScopeRead, id, r.URL.Query().Get("t")); err != nil {
		s.renderLeitorInvalido(w, r)
		return
	}

	ctx := r.Context()
	chunk, err := s.Store.Plans().Chunk(ctx, id)
	if err != nil {
		s.logger().Error("buscar chunk do leitor", "chunk", id, "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível carregar o trecho")
		return
	}
	if chunk == nil {
		s.renderErro(w, r, http.StatusNotFound, "Trecho não encontrado")
		return
	}
	doc, err := s.Store.Documents().Get(ctx, chunk.DocumentID)
	if err != nil {
		s.logger().Error("buscar documento do leitor", "documento", chunk.DocumentID, "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível carregar o trecho")
		return
	}
	if doc == nil {
		s.renderErro(w, r, http.StatusNotFound, "Documento não encontrado")
		return
	}

	corpo, err := s.Store.Blocks().Between(ctx, doc.ID, chunk.FirstBlockID, chunk.LastBlockID)
	if err != nil {
		s.logger().Error("buscar corpo do leitor", "chunk", id, "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível carregar o trecho")
		return
	}
	if len(corpo) == 0 {
		s.renderErro(w, r, http.StatusNotFound, "Trecho sem conteúdo")
		return
	}

	secao, err := s.Store.Blocks().Section(ctx, doc.ID, corpo[0].HeadingID)
	if err != nil {
		s.logger().Error("buscar seção do leitor", "chunk", id, "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível carregar o trecho")
		return
	}

	prog, err := s.Store.Plans().Progress(ctx, chunk.PlanID)
	if err != nil {
		s.logger().Error("progresso do leitor", "plano", chunk.PlanID, "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível carregar o trecho")
		return
	}

	leitorTTL, lidoTTL := s.linkTTLs(r)
	props := views.LeitorProps{
		Doc:          *doc,
		Chunk:        *chunk,
		Corpo:        corpo,
		Secao:        secao,
		MostrarSecao: secaoMaiorQue(corpo, secao),
		Percent:      int(prog.Pct + 0.5),
		Acao:         s.Links.DoneURL(chunk.ID, lidoTTL) + "&ui=1",
		Voltar:       "/documents/" + strconv.FormatInt(doc.ID, 10),
		LidoEm:       chunk.DoneAt,
		MarcadoAgora: queryBool(r, "lido"),
	}

	if anterior, err := s.Store.Plans().PreviousChunk(ctx, chunk); err != nil {
		s.logger().Error("chunk anterior do leitor", "chunk", id, "erro", err)
	} else if anterior != nil {
		props.Anterior = &views.LeitorVizinho{
			URL: s.Links.ReadURL(anterior.ID, leitorTTL),
			Seq: anterior.Seq,
		}
	}
	if proximo, err := s.Store.Plans().NextChunk(ctx, chunk); err != nil {
		s.logger().Error("próximo chunk do leitor", "chunk", id, "erro", err)
	} else if proximo != nil {
		props.Proximo = &views.LeitorVizinho{
			URL: s.Links.ReadURL(proximo.ID, leitorTTL),
			Seq: proximo.Seq,
		}
	}

	s.render(w, r, http.StatusOK, views.Leitor(props))
}

// handleMarcarLido registra a leitura do chunk. O ntfy chama com POST e recebe
// 204; o formulário do leitor manda `ui=1` e volta para a página do trecho.
func (s *Server) handleMarcarLido(w http.ResponseWriter, r *http.Request) {
	id, ok := idDaRota(r)
	if !ok {
		s.renderErro(w, r, http.StatusNotFound, "Trecho não encontrado")
		return
	}
	if s.Store == nil {
		http.Error(w, "banco indisponível", http.StatusServiceUnavailable)
		return
	}
	if s.Links == nil {
		http.Error(w, "leitor indisponível", http.StatusServiceUnavailable)
		return
	}
	if err := s.Links.Verify(links.ScopeDone, id, r.URL.Query().Get("t")); err != nil {
		s.renderLeitorInvalido(w, r)
		return
	}

	ctx := r.Context()
	chunk, err := s.Store.Plans().Chunk(ctx, id)
	if err != nil {
		s.logger().Error("buscar chunk para marcar lido", "chunk", id, "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível marcar como lido")
		return
	}
	if chunk == nil {
		s.renderErro(w, r, http.StatusNotFound, "Trecho não encontrado")
		return
	}
	if err := s.Store.Plans().MarkDone(ctx, id, time.Now()); err != nil {
		s.logger().Error("marcar chunk como lido", "chunk", id, "erro", err)
		s.renderErro(w, r, http.StatusInternalServerError, "Não foi possível marcar como lido")
		return
	}
	s.logger().Info("chunk marcado como lido", "chunk", id, "documento", chunk.DocumentID)

	if queryBool(r, "ui") || querHTML(r) {
		// Redirect relativo: a página do leitor é servida pelo mesmo host, e
		// assim um APP_BASE_URL de produção não desvia o teste local.
		leitorTTL, _ := s.linkTTLs(r)
		alvo := "/r/" + strconv.FormatInt(id, 10) + "?t=" + s.Links.Token(links.ScopeRead, id, leitorTTL) + "&lido=1"
		http.Redirect(w, r, alvo, http.StatusSeeOther)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// linkTTLs lê as validades configuradas; configuração inválida cai nos
// padrões do pacote links.
func (s *Server) linkTTLs(r *http.Request) (leitor, lido time.Duration) {
	st, err := schedule.LoadSettings(r.Context(), s.Store.Settings())
	if err != nil {
		return 0, 0
	}
	return st.LinkTTLs()
}

func (s *Server) renderLeitorInvalido(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusUnauthorized, views.LeitorInvalido())
}

// secaoMaiorQue informa se a seção completa vai além do trecho entregue.
func secaoMaiorQue(corpo, secao []blocks.Block) bool {
	if len(corpo) == 0 || len(secao) == 0 {
		return false
	}
	return secao[0].Ord < corpo[0].Ord || secao[len(secao)-1].Ord > corpo[len(corpo)-1].Ord
}

func querHTML(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/html")
}
