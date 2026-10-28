package links

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

var segredo = []byte(strings.Repeat("k", MinSecretBytes))

var agora = time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)

func novoSigner(t *testing.T, quando time.Time) *Signer {
	t.Helper()
	s, err := New(Config{Secret: segredo, BaseURL: "https://leitura.exemplo.com", Now: func() time.Time { return quando }})
	if err != nil {
		t.Fatalf("criar signer: %v", err)
	}
	return s
}

func tokenDaURL(t *testing.T, bruta string) (*url.URL, string) {
	t.Helper()
	u, err := url.Parse(bruta)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	return u, u.Query().Get("t")
}

func TestLinksURLsEEscopos(t *testing.T) {
	s := novoSigner(t, agora)

	leitor := s.ReadURL(42, DefaultReadTTL)
	u, token := tokenDaURL(t, leitor)
	if u.Path != "/r/42" || token == "" {
		t.Fatalf("URL do leitor = %q", leitor)
	}
	if !strings.HasPrefix(leitor, "https://leitura.exemplo.com/r/42?t=") {
		t.Errorf("URL do leitor = %q", leitor)
	}
	if err := s.Verify(ScopeRead, 42, token); err != nil {
		t.Errorf("token do leitor deveria valer: %v", err)
	}
	if err := s.Verify(ScopeDone, 42, token); !errors.Is(err, ErrTokenInvalido) {
		t.Errorf("escopo trocado = %v", err)
	}
	if err := s.Verify(ScopeRead, 43, token); !errors.Is(err, ErrTokenInvalido) {
		t.Errorf("chunk trocado = %v", err)
	}

	lido := s.DoneURL(42, DefaultDoneTTL)
	u, token = tokenDaURL(t, lido)
	if u.Path != "/api/actions/chunks/42/done" {
		t.Fatalf("URL de marcar lido = %q", lido)
	}
	if err := s.Verify(ScopeDone, 42, token); err != nil {
		t.Errorf("token de marcar lido deveria valer: %v", err)
	}
	if err := s.Verify(ScopeRead, 42, token); !errors.Is(err, ErrTokenInvalido) {
		t.Errorf("leitor com token de ação = %v", err)
	}
}

func TestLinksExpiracao(t *testing.T) {
	quando := agora
	s := novoSigner(t, quando)
	token := s.Token(ScopeRead, 7, time.Hour)

	if err := s.Verify(ScopeRead, 7, token); err != nil {
		t.Fatalf("token recém-criado: %v", err)
	}

	// Exatamente no instante de expiração já não vale.
	s.now = func() time.Time { return quando.Add(time.Hour) }
	if err := s.Verify(ScopeRead, 7, token); !errors.Is(err, ErrTokenInvalido) {
		t.Errorf("token expirado = %v", err)
	}

	// Um segundo antes continua valendo.
	s.now = func() time.Time { return quando.Add(time.Hour - time.Second) }
	if err := s.Verify(ScopeRead, 7, token); err != nil {
		t.Errorf("token dentro da validade = %v", err)
	}
}

func TestLinksPadroesDeValidade(t *testing.T) {
	s := novoSigner(t, agora)

	if exp := expiracaoDe(t, s.Token(ScopeRead, 1, 0)); !exp.Equal(agora.Add(DefaultReadTTL)) {
		t.Errorf("expiração do leitor = %v", exp)
	}
	if exp := expiracaoDe(t, s.Token(ScopeDone, 1, 0)); !exp.Equal(agora.Add(DefaultDoneTTL)) {
		t.Errorf("expiração da ação = %v", exp)
	}
	if exp := expiracaoDe(t, s.Token(ScopeDone, 1, 2*time.Hour)); !exp.Equal(agora.Add(2 * time.Hour)) {
		t.Errorf("expiração explícita = %v", exp)
	}
}

func expiracaoDe(t *testing.T, token string) time.Time {
	t.Helper()
	texto, _, ok := strings.Cut(token, ".")
	if !ok {
		t.Fatalf("token sem expiração: %q", token)
	}
	exp, err := strconv.ParseInt(texto, 10, 64)
	if err != nil {
		t.Fatalf("expiração inválida em %q", token)
	}
	return time.Unix(exp, 0)
}

func TestLinksTokenInvalido(t *testing.T) {
	s := novoSigner(t, agora)
	valido := s.Token(ScopeRead, 5, time.Hour)
	_, sig := tokenDaURL(t, s.ReadURL(5, time.Hour))

	adulterada := sig[:len(sig)-1] + "0"
	if adulterada == sig {
		adulterada = sig[:len(sig)-1] + "1"
	}

	casos := map[string]string{
		"vazio":          "",
		"sem ponto":      strings.ReplaceAll(valido, ".", ""),
		"so exp":         "1700000000.",
		"so sig":         "." + sig,
		"exp invalida":   "abc." + sig,
		"exp negativa":   "-1." + sig,
		"exp zero":       "0." + sig,
		"sig adulterada": strings.TrimSuffix(valido, sig) + adulterada,
		"pontos extras":  valido + ".x",
	}

	for nome, token := range casos {
		t.Run(nome, func(t *testing.T) {
			if err := s.Verify(ScopeRead, 5, token); !errors.Is(err, ErrTokenInvalido) {
				t.Errorf("token %q = %v", token, err)
			}
		})
	}

	var nulo *Signer
	if err := nulo.Verify(ScopeRead, 5, valido); !errors.Is(err, ErrTokenInvalido) {
		t.Errorf("signer nulo = %v", err)
	}
	if token := nulo.Token(ScopeRead, 5, time.Hour); token != "" {
		t.Errorf("token de signer nulo = %q", token)
	}
}

func TestLinksSegredoEBase(t *testing.T) {
	if _, err := New(Config{Secret: []byte("curto"), BaseURL: "https://x"}); err == nil {
		t.Error("segredo curto deveria falhar")
	}

	bases := map[string]string{
		"https://leitura.exemplo.com":    "https://leitura.exemplo.com",
		"https://leitura.exemplo.com/":   "https://leitura.exemplo.com",
		"http://localhost:8473/":         "http://localhost:8473",
		"https://exemplo.com/reading":    "https://exemplo.com/reading",
		"  https://exemplo.com/drip//  ": "https://exemplo.com/drip",
	}
	for entrada, esperado := range bases {
		s, err := New(Config{Secret: segredo, BaseURL: entrada})
		if err != nil {
			t.Errorf("base %q: %v", entrada, err)
			continue
		}
		if s.base != esperado {
			t.Errorf("base %q = %q, quer %q", entrada, s.base, esperado)
		}
	}

	invalidas := []string{"", "   ", "/r/1", "ftp://exemplo.com", "https://", "https://x/?a=1", "https://x/#f", "://x"}
	for _, base := range invalidas {
		if _, err := New(Config{Secret: segredo, BaseURL: base}); err == nil {
			t.Errorf("base %q deveria falhar", base)
		}
	}
}

func TestLinksMesmoTokenComMesmoRelogio(t *testing.T) {
	s := novoSigner(t, agora)
	a := s.Token(ScopeRead, 9, time.Hour)
	b := s.Token(ScopeRead, 9, time.Hour)
	if a != b {
		t.Errorf("tokens deveriam ser iguais: %q != %q", a, b)
	}
	if c := s.Token(ScopeRead, 9, 2*time.Hour); c == a {
		t.Error("TTL diferente deveria mudar o token")
	}
}
