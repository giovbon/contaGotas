// Package web expõe as rotas HTTP de gestão: login/logout, upload, saúde e os
// cabeçalhos de segurança. As telas de biblioteca e detalhe entram nos
// próximos incrementos.
package web

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/giovani/readingdrip/internal/auth"
	"github.com/giovani/readingdrip/internal/library"
	"github.com/giovani/readingdrip/internal/links"
	"github.com/giovani/readingdrip/internal/store"
)

// Middleware envolve handlers (sessão, logging, etc.). O chamador injeta a
// autenticação; sem middleware, as rotas de gestão são negadas.
type Middleware func(http.Handler) http.Handler

// Server agrega as dependências das rotas.
type Server struct {
	Store        *store.Store
	Importer     *library.Importer
	Sessions     *auth.Authenticator // nil → usa Auth; sem nenhum, 401 (fail closed)
	Auth         Middleware          // middleware extra para testes de pass-through
	Sender       Sender              // nil → envio manual e teste respondem 503
	Links        *links.Signer       // nil → leitor e ações respondem 503
	MaxBodyBytes int64               // 0 → limite do importador + 1 MiB de multipart
	Logger       *slog.Logger
}

// Handler monta o mux com as rotas implementadas.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("POST /login", s.handleLogin)
	mux.Handle("GET /{$}", s.protegido(http.HandlerFunc(s.handleBiblioteca)))
	mux.Handle("GET /documents/{id}", s.protegido(http.HandlerFunc(s.handleDocumento)))
	mux.Handle("GET /documents/{id}/plan", s.protegido(http.HandlerFunc(s.handlePlano)))
	mux.Handle("GET /documents/{id}/download", s.protegido(http.HandlerFunc(s.handleDownload)))
	mux.Handle("POST /documents/{id}/send-now", s.protegido(http.HandlerFunc(s.handleEnviarAgora)))
	mux.Handle("GET /settings", s.protegido(http.HandlerFunc(s.handleConfiguracoes)))
	mux.Handle("POST /settings", s.protegido(http.HandlerFunc(s.handleSalvarConfiguracoes)))
	mux.Handle("POST /settings/test", s.protegido(http.HandlerFunc(s.handleTestarNtfy)))
	mux.HandleFunc("GET /r/{id}", s.handleLeitor)
	mux.HandleFunc("POST /api/actions/chunks/{id}/done", s.handleMarcarLido)
	mux.Handle("GET /static/", staticHandler())
	mux.Handle("POST /logout", s.protegido(http.HandlerFunc(s.handleLogout)))
	mux.Handle("POST /documents", s.protegido(http.HandlerFunc(s.handleUpload)))
	return comHeaders(mux)
}

func (s *Server) protegido(h http.Handler) http.Handler {
	if s.Sessions != nil {
		return s.Sessions.Middleware(h)
	}
	if s.Auth != nil {
		return s.Auth(h)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "não autenticado", http.StatusUnauthorized)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil {
		http.Error(w, "banco indisponível", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.Store.Ping(ctx); err != nil {
		s.logger().Error("readiness falhou", "erro", err)
		http.Error(w, "banco indisponível", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

// comHeaders aplica as políticas comuns: nada de cache para HTML/erros, sem
// indexação por buscadores e CSP restritiva (docs/web-ui.md).
func comHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; "+
				"base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		h.ServeHTTP(w, r)
	})
}

func (s *Server) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}
