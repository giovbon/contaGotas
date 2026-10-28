package web_test

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/giovani/readingdrip/internal/auth"
)

const senhaTeste = "abre-te-sesamo"

func ambienteComLogin(t *testing.T) (*ambiente, *auth.Authenticator) {
	t.Helper()
	s, err := auth.New(auth.Config{
		Password: senhaTeste,
		Secret:   []byte(strings.Repeat("k", auth.MinSecretBytes)),
		Argon2:   auth.Argon2Params{Memory: 64, Time: 1, Threads: 1},
		Now:      func() time.Time { return agora },
	})
	if err != nil {
		t.Fatalf("criar autenticação: %v", err)
	}
	return novoAmbienteConfig(t, nil, s), s
}

func postForm(t *testing.T, a *ambiente, caminho string, valores url.Values, cookies ...*http.Cookie) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, a.http.URL+caminho, strings.NewReader(valores.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := semRedirect.Do(req)
	if err != nil {
		t.Fatalf("post %s: %v", caminho, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func getComCookies(t *testing.T, a *ambiente, caminho string, cookies ...*http.Cookie) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, a.http.URL+caminho, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := semRedirect.Do(req)
	if err != nil {
		t.Fatalf("get %s: %v", caminho, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func entrar(t *testing.T, a *ambiente, next string) *http.Cookie {
	t.Helper()
	resp := postForm(t, a, "/login", url.Values{"senha": {senhaTeste}, "next": {next}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: status = %d, quer 303", resp.StatusCode)
	}
	cookies := resp.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies do login = %d, quer 1", len(cookies))
	}
	return cookies[0]
}

func TestLoginPagina(t *testing.T) {
	a, _ := ambienteComLogin(t)
	resp := getComCookies(t, a, "/login?next=/documents/2")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, quer 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q", ct)
	}
	corpo := lerCorpo(t, resp)
	for _, trecho := range []string{`action="/login"`, `name="senha"`, `value="/documents/2"`} {
		if !strings.Contains(corpo, trecho) {
			t.Errorf("página sem %q", trecho)
		}
	}
	if resp.Header.Get("Content-Security-Policy") == "" {
		t.Error("CSP ausente")
	}
	if resp.Header.Get("Referrer-Policy") != "same-origin" {
		t.Errorf("Referrer-Policy = %q", resp.Header.Get("Referrer-Policy"))
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", resp.Header.Get("Cache-Control"))
	}
}

func TestLoginSucesso(t *testing.T) {
	a, _ := ambienteComLogin(t)
	resp := postForm(t, a, "/login", url.Values{
		"senha": {senhaTeste},
		"next":  {"/documents/7?duplicado=1"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, quer 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/documents/7?duplicado=1" {
		t.Errorf("Location = %q", loc)
	}
	cookies := resp.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, quer 1", len(cookies))
	}
	c := cookies[0]
	if c.Name != auth.CookieName || c.Path != "/" || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie = %+v", c)
	}
	if c.Value == "" {
		t.Error("cookie vazio")
	}

	// Com sessão, o formulário não aparece: volta para o destino.
	jaLogado := getComCookies(t, a, "/login?next=/documents/9", c)
	if jaLogado.StatusCode != http.StatusSeeOther || jaLogado.Header.Get("Location") != "/documents/9" {
		t.Errorf("login já autenticado = %d %q", jaLogado.StatusCode, jaLogado.Header.Get("Location"))
	}

	logout := postForm(t, a, "/logout", url.Values{}, c)
	if logout.StatusCode != http.StatusSeeOther || logout.Header.Get("Location") != "/login" {
		t.Errorf("logout = %d %q", logout.StatusCode, logout.Header.Get("Location"))
	}
	limpo := logout.Cookies()
	if len(limpo) != 1 || limpo[0].MaxAge != -1 {
		t.Errorf("cookie de logout = %+v", limpo)
	}
}

func TestLoginSenhaErrada(t *testing.T) {
	a, _ := ambienteComLogin(t)
	resp := postForm(t, a, "/login", url.Values{"senha": {"errada"}})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, quer 401", resp.StatusCode)
	}
	if cookies := resp.Cookies(); len(cookies) != 0 {
		t.Errorf("cookie criado com senha errada: %+v", cookies)
	}
	if corpo := lerCorpo(t, resp); !strings.Contains(corpo, "Senha incorreta") {
		t.Errorf("corpo = %q", corpo)
	}
}

func TestLoginRateLimit(t *testing.T) {
	a, _ := ambienteComLogin(t)
	for i := 0; i < 5; i++ {
		resp := postForm(t, a, "/login", url.Values{"senha": {"errada"}})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("tentativa %d = %d, quer 401", i+1, resp.StatusCode)
		}
	}

	resp := postForm(t, a, "/login", url.Values{"senha": {senhaTeste}})
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("sexta tentativa = %d, quer 429", resp.StatusCode)
	}
	if cookies := resp.Cookies(); len(cookies) != 0 {
		t.Errorf("cookie criado durante bloqueio: %+v", cookies)
	}
	espera, err := strconv.Atoi(resp.Header.Get("Retry-After"))
	if err != nil || espera < 1 {
		t.Errorf("Retry-After = %q (%v)", resp.Header.Get("Retry-After"), err)
	}
	if corpo := lerCorpo(t, resp); !strings.Contains(corpo, "Muitas tentativas") {
		t.Errorf("corpo = %q", corpo)
	}
}

func TestLoginNextSeguro(t *testing.T) {
	a, _ := ambienteComLogin(t)
	casos := map[string]string{
		"/settings":           "/settings",
		"/documents/4?x=1":    "/documents/4?x=1",
		"https://evil.com":    "/",
		"//evil.com":          "/",
		"/\\evil.com":         "/",
		"javascript:alert(1)": "/",
		"":                    "/",
	}
	for next, quer := range casos {
		resp := postForm(t, a, "/login", url.Values{"senha": {senhaTeste}, "next": {next}})
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("next %q: status = %d", next, resp.StatusCode)
		}
		if loc := resp.Header.Get("Location"); loc != quer {
			t.Errorf("next %q → Location %q, quer %q", next, loc, quer)
		}
	}
}

func TestRotaProtegidaRedirecionaParaLogin(t *testing.T) {
	a, _ := ambienteComLogin(t)

	resp := postForm(t, a, "/documents", url.Values{})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login" {
		t.Fatalf("POST /documents sem sessão = %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	resp = postForm(t, a, "/logout", url.Values{})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login" {
		t.Fatalf("POST /logout sem sessão = %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	cookie := entrar(t, a, "/")
	resp = postForm(t, a, "/documents", url.Values{}, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /documents com sessão = %d, quer 400 (passou pela auth)", resp.StatusCode)
	}

	adulterado := &http.Cookie{Name: cookie.Name, Value: cookie.Value + "x"}
	resp = postForm(t, a, "/documents", url.Values{}, adulterado)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login" {
		t.Fatalf("cookie adulterado = %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestLoginJaAutenticadoComNextExterno(t *testing.T) {
	a, _ := ambienteComLogin(t)
	cookie := entrar(t, a, "/")
	resp := getComCookies(t, a, "/login?next=https://evil.com", cookie)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
		t.Errorf("next externo com sessão = %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestLoginSemAutenticador(t *testing.T) {
	a := novoAmbiente(t, nil)
	for _, resp := range []*http.Response{
		getComCookies(t, a, "/login"),
		postForm(t, a, "/login", url.Values{"senha": {"x"}}),
	} {
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("status sem autenticador = %d, quer 503", resp.StatusCode)
		}
	}
}

func TestLoginCorpoGrande(t *testing.T) {
	a, _ := ambienteComLogin(t)
	req, err := http.NewRequest(http.MethodPost, a.http.URL+"/login",
		strings.NewReader("senha="+strings.Repeat("a", 70<<10)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := semRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, quer 413", resp.StatusCode)
	}
}

func lerCorpo(t *testing.T, resp *http.Response) string {
	t.Helper()
	var b strings.Builder
	if _, err := io.Copy(&b, resp.Body); err != nil {
		t.Fatalf("ler corpo: %v", err)
	}
	return b.String()
}
