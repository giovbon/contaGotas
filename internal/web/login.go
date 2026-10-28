package web

import (
	"errors"
	"math"
	"net/http"
	"strconv"

	"github.com/giovani/readingdrip/internal/auth"
	"github.com/giovani/readingdrip/internal/web/views"
)

const loginBodyLimit int64 = 64 << 10

// handleLoginPage mostra o formulário. Quem já tem sessão volta para o destino
// seguro (ou para a biblioteca).
func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if s.Sessions == nil {
		http.Error(w, "autenticação indisponível", http.StatusServiceUnavailable)
		return
	}
	next := auth.SafeNext(r.URL.Query().Get("next"))
	if s.Sessions.Authenticated(r) {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	s.renderLogin(w, r, http.StatusOK, next, "")
}

// handleLogin valida a senha com rate limit por IP e cria a sessão.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.Sessions == nil {
		http.Error(w, "autenticação indisponível", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, loginBodyLimit)
	if err := r.ParseForm(); err != nil {
		var grande *http.MaxBytesError
		if errors.As(err, &grande) {
			http.Error(w, "formulário acima do limite", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "formulário inválido", http.StatusBadRequest)
		return
	}

	ip := s.Sessions.ClientIP(r)
	if ok, espera := s.Sessions.Allowed(ip); !ok {
		segundos := int(math.Ceil(espera.Seconds()))
		if segundos < 1 {
			segundos = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(segundos))
		s.logger().Warn("login bloqueado por rate limit", "ip", ip)
		s.renderLogin(w, r, http.StatusTooManyRequests, auth.SafeNext(r.PostFormValue("next")),
			"Muitas tentativas. Tente novamente em "+strconv.Itoa(segundos)+" s.")
		return
	}

	next := auth.SafeNext(r.PostFormValue("next"))
	if !s.Sessions.CheckPassword(r.PostFormValue("senha")) {
		s.Sessions.Fail(ip)
		s.logger().Warn("login recusado", "ip", ip)
		s.renderLogin(w, r, http.StatusUnauthorized, next, "Senha incorreta.")
		return
	}

	s.Sessions.Reset(ip)
	s.Sessions.SetCookie(w, r)
	s.logger().Info("login efetuado", "ip", ip)
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// handleLogout apaga o cookie e volta ao formulário.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if s.Sessions == nil {
		http.Error(w, "autenticação indisponível", http.StatusServiceUnavailable)
		return
	}
	s.Sessions.ClearCookie(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) renderLogin(w http.ResponseWriter, r *http.Request, status int, next, erro string) {
	s.render(w, r, status, views.Login(views.LoginProps{Next: auth.SafeNext(next), Erro: erro}))
}
