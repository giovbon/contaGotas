package auth

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

type relogio struct {
	mu sync.Mutex
	t  time.Time
}

func (r *relogio) now() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.t
}

func (r *relogio) avancar(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.t = r.t.Add(d)
}

func novoAuth(t *testing.T, mut func(*Config)) (*Authenticator, *relogio) {
	t.Helper()
	rel := &relogio{t: t0}
	cfg := Config{
		Password: "senha-correta",
		Secret:   bytes.Repeat([]byte("s"), MinSecretBytes),
		Argon2:   Argon2Params{Memory: 64, Time: 1, Threads: 1},
		Now:      rel.now,
		Random:   bytes.NewReader(bytes.Repeat([]byte("r"), 64)),
	}
	if mut != nil {
		mut(&cfg)
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a, rel
}

func TestCheckPassword(t *testing.T) {
	a, _ := novoAuth(t, nil)
	if !a.CheckPassword("senha-correta") {
		t.Error("senha correta recusada")
	}
	for _, errada := range []string{"", "senha-errada", "SENHA-CORRETA", "senha-correta "} {
		if a.CheckPassword(errada) {
			t.Errorf("senha %q aceita", errada)
		}
	}
}

func TestNewRejeitaConfiguracaoInsegura(t *testing.T) {
	casos := []struct {
		nome string
		cfg  Config
		quer error
	}{
		{"senha vazia", Config{Secret: bytes.Repeat([]byte("s"), MinSecretBytes)}, ErrEmptyPassword},
		{"segredo curto", Config{Password: "x", Secret: []byte("curto")}, ErrWeakSecret},
		{"argon inválido", Config{
			Password: "x",
			Secret:   bytes.Repeat([]byte("s"), MinSecretBytes),
			Argon2:   Argon2Params{Memory: 16, Time: 1, Threads: 8},
		}, nil},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			_, err := New(c.cfg)
			if err == nil {
				t.Fatal("esperado erro")
			}
			if c.quer != nil && !errors.Is(err, c.quer) {
				t.Errorf("erro = %v, quer %v", err, c.quer)
			}
		})
	}

	_, err := New(Config{
		Password: "x",
		Secret:   bytes.Repeat([]byte("s"), MinSecretBytes),
		Random:   erroReader{},
	})
	if err == nil || !strings.Contains(err.Error(), "salt") {
		t.Errorf("falha do leitor aleatório = %v", err)
	}
}

type erroReader struct{}

func (erroReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestCookieSessao(t *testing.T) {
	a, rel := novoAuth(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	a.SetCookie(rr, req)

	cookies := rr.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, quer 1", len(cookies))
	}
	c := cookies[0]
	if c.Name != CookieName || c.Path != "/" || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie = %+v", c)
	}
	if c.Secure {
		t.Error("cookie Secure em HTTP direto")
	}
	if c.MaxAge != int(DefaultTTL/time.Second) {
		t.Errorf("MaxAge = %d", c.MaxAge)
	}
	if !c.Expires.Equal(t0.Add(DefaultTTL)) {
		t.Errorf("Expires = %v", c.Expires)
	}

	autenticada := httptest.NewRequest(http.MethodGet, "/", nil)
	autenticada.AddCookie(&http.Cookie{Name: CookieName, Value: c.Value})
	if !a.Authenticated(autenticada) {
		t.Fatal("cookie recém-criado inválido")
	}

	rel.avancar(DefaultTTL + time.Second)
	if a.Authenticated(autenticada) {
		t.Error("cookie aceito depois de expirar")
	}
}

func TestCookieAdulterado(t *testing.T) {
	a, _ := novoAuth(t, nil)
	rr := httptest.NewRecorder()
	a.SetCookie(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	valido := rr.Result().Cookies()[0].Value

	partes := strings.Split(valido, ".")
	if len(partes) != 3 {
		t.Fatalf("formato do cookie = %q", valido)
	}
	adulterados := []string{
		"",
		"lixo",
		"v1.9999999999",
		"v2." + partes[1] + "." + partes[2],
		"v1.9999999999." + partes[2],
		partes[0] + "." + partes[1] + "." + trocarUltimo(partes[2]),
	}
	for _, valor := range adulterados {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: CookieName, Value: valor})
		if a.Authenticated(req) {
			t.Errorf("cookie adulterado aceito: %q", valor)
		}
	}

	// A assinatura de um cookie válido não vale para outra expiração.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "v1.9999999999." + partes[2]})
	if a.Authenticated(req) {
		t.Error("expiração trocada manteve a assinatura")
	}
}

func trocarUltimo(s string) string {
	if s == "" {
		return "a"
	}
	b := []byte(s)
	if b[len(b)-1] == 'A' {
		b[len(b)-1] = 'B'
	} else {
		b[len(b)-1] = 'A'
	}
	return string(b)
}

func TestCookieSecureEmHTTPS(t *testing.T) {
	a, _ := novoAuth(t, nil)

	comProxy := httptest.NewRequest(http.MethodGet, "/", nil)
	comProxy.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()
	a.SetCookie(rr, comProxy)
	if !rr.Result().Cookies()[0].Secure {
		t.Error("cookie sem Secure com X-Forwarded-Proto: https")
	}

	comTLS := httptest.NewRequest(http.MethodGet, "/", nil)
	comTLS.TLS = &tls.ConnectionState{}
	rr = httptest.NewRecorder()
	a.SetCookie(rr, comTLS)
	if !rr.Result().Cookies()[0].Secure {
		t.Error("cookie sem Secure em TLS direto")
	}
}

func TestClearCookie(t *testing.T) {
	a, _ := novoAuth(t, nil)
	rr := httptest.NewRecorder()
	a.ClearCookie(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	c := rr.Result().Cookies()[0]
	if c.Name != CookieName || c.MaxAge != -1 || c.Value != "" {
		t.Errorf("cookie de logout = %+v", c)
	}
	if !c.Expires.Before(t0) {
		t.Errorf("Expires = %v", c.Expires)
	}
}

func TestMiddleware(t *testing.T) {
	a, _ := novoAuth(t, nil)
	chamou := false
	protegido := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		chamou = true
		w.WriteHeader(http.StatusNoContent)
	}))

	get := httptest.NewRequest(http.MethodGet, "/documents/1?x=1", nil)
	rr := httptest.NewRecorder()
	protegido.ServeHTTP(rr, get)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, quer 303", rr.Code)
	}
	if quer := rr.Header().Get("Location"); quer != "/login?next=%2Fdocuments%2F1%3Fx%3D1" {
		t.Errorf("Location = %q", quer)
	}
	if chamou {
		t.Error("handler chamado sem sessão")
	}

	post := httptest.NewRequest(http.MethodPost, "/documents", nil)
	rr = httptest.NewRecorder()
	protegido.ServeHTTP(rr, post)
	if quer := rr.Header().Get("Location"); quer != "/login" {
		t.Errorf("Location do POST = %q, quer /login", quer)
	}

	raiz := httptest.NewRequest(http.MethodGet, "/", nil)
	rr = httptest.NewRecorder()
	protegido.ServeHTTP(rr, raiz)
	if quer := rr.Header().Get("Location"); quer != "/login" {
		t.Errorf("Location da raiz = %q, quer /login", quer)
	}

	cookieRR := httptest.NewRecorder()
	a.SetCookie(cookieRR, get)
	autenticada := httptest.NewRequest(http.MethodGet, "/documents/1?x=1", nil)
	autenticada.AddCookie(cookieRR.Result().Cookies()[0])
	rr = httptest.NewRecorder()
	protegido.ServeHTTP(rr, autenticada)
	if rr.Code != http.StatusNoContent || !chamou {
		t.Errorf("com sessão: status=%d chamou=%v", rr.Code, chamou)
	}
}

func TestSafeNext(t *testing.T) {
	casos := map[string]string{
		"":                                "/",
		"/":                               "/",
		"/documents/1":                    "/documents/1",
		"/documents/1?duplicado=1":        "/documents/1?duplicado=1",
		"/settings?x=1#top":               "/settings?x=1",
		"https://evil.com":                "/",
		"http://evil.com/x":               "/",
		"//evil.com":                      "/",
		"///evil.com":                     "/",
		"/\\evil.com":                     "/",
		"\\\\evil.com":                    "/",
		"javascript:alert(1)":             "/",
		"login":                           "/",
		"/%2f%2fevil.com":                 "/",
		"/%5cevil.com":                    "/",
		"/ok%2f..%2f..":                   "/ok%2f..%2f..",
		"/a\nLocation:%20https://evil.co": "/",
		"/a\tLocation:%20https://evil.co": "/",
	}
	for entrada, quer := range casos {
		if obtido := SafeNext(entrada); obtido != quer {
			t.Errorf("SafeNext(%q) = %q, quer %q", entrada, obtido, quer)
		}
	}
}

func TestRateLimit(t *testing.T) {
	a, rel := novoAuth(t, func(c *Config) {
		c.MaxAttempts = 2
		c.Window = 30 * time.Second
	})
	const ip = "203.0.113.10"

	for i := 0; i < 2; i++ {
		if ok, _ := a.Allowed(ip); !ok {
			t.Fatalf("tentativa %d bloqueada antes do limite", i+1)
		}
		a.Fail(ip)
	}

	ok, espera := a.Allowed(ip)
	if ok {
		t.Fatalf("IP liberado depois de %d falhas", 2)
	}
	if espera <= 0 || espera > 30*time.Second {
		t.Errorf("espera = %v", espera)
	}

	// Outro IP não herda o bloqueio.
	if ok, _ := a.Allowed("203.0.113.11"); !ok {
		t.Error("outro IP bloqueado")
	}

	// Sucesso zera as falhas.
	a.Reset(ip)
	if ok, _ := a.Allowed(ip); !ok {
		t.Error("IP continua bloqueado após Reset")
	}

	// A janela expira.
	a.Fail(ip)
	a.Fail(ip)
	rel.avancar(31 * time.Second)
	if ok, _ := a.Allowed(ip); !ok {
		t.Error("IP bloqueado depois da janela")
	}
}

func TestClientIP(t *testing.T) {
	a, _ := novoAuth(t, nil)
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "198.51.100.5:4321"
	req.Header.Set("CF-Connecting-IP", "203.0.113.9")
	if ip := a.ClientIP(req); ip != "198.51.100.5" {
		t.Errorf("IP sem proxy = %q", ip)
	}

	cf, _ := novoAuth(t, func(c *Config) { c.BehindCloudflare = true })
	if ip := cf.ClientIP(req); ip != "203.0.113.9" {
		t.Errorf("IP atrás da Cloudflare = %q", ip)
	}
	req.Header.Set("CF-Connecting-IP", "não-é-ip")
	if ip := cf.ClientIP(req); ip != "198.51.100.5" {
		t.Errorf("fallback com header inválido = %q", ip)
	}
	req.Header.Set("CF-Connecting-IP", "2001:db8::1")
	if ip := cf.ClientIP(req); ip != "2001:db8::1" {
		t.Errorf("IPv6 = %q", ip)
	}
}
