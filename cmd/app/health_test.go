package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthcheckURL(t *testing.T) {
	casos := []struct {
		nome string
		addr string
		quer string
		erro bool
	}{
		{nome: "vazio usa padrão", addr: "", quer: "http://127.0.0.1:8080/readyz"},
		{nome: "sem host", addr: ":8080", quer: "http://127.0.0.1:8080/readyz"},
		{nome: "qualquer interface", addr: "0.0.0.0:9090", quer: "http://127.0.0.1:9090/readyz"},
		{nome: "IPv6 qualquer", addr: "[::]:8081", quer: "http://127.0.0.1:8081/readyz"},
		{nome: "loopback", addr: "127.0.0.1:1234", quer: "http://127.0.0.1:1234/readyz"},
		{nome: "host nomeado", addr: "localhost:7000", quer: "http://localhost:7000/readyz"},
		{nome: "IPv6 loopback", addr: "[::1]:8080", quer: "http://[::1]:8080/readyz"},
		{nome: "sem porta", addr: "localhost", erro: true},
		{nome: "vazio explícito", addr: "   ", quer: "http://127.0.0.1:8080/readyz"},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			got, err := healthcheckURL(caso.addr)
			if caso.erro {
				if err == nil {
					t.Fatalf("healthcheckURL(%q) = %q, quer erro", caso.addr, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("healthcheckURL(%q): %v", caso.addr, err)
			}
			if got != caso.quer {
				t.Errorf("healthcheckURL(%q) = %q, quer %q", caso.addr, got, caso.quer)
			}
		})
	}
}

func TestRunHealthcheck(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			t.Errorf("caminho = %q, quer /readyz", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer servidor.Close()

	addr := servidor.Listener.Addr().String()
	getenv := func(chave string) string {
		if chave == "APP_ADDR" {
			return addr
		}
		return ""
	}
	if err := runHealthcheck(getenv, servidor.Client()); err != nil {
		t.Fatalf("healthcheck: %v", err)
	}
}

func TestRunHealthcheckNaoPronto(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "banco indisponível", http.StatusServiceUnavailable)
	}))
	defer servidor.Close()

	getenv := func(string) string { return servidor.Listener.Addr().String() }
	if err := runHealthcheck(getenv, servidor.Client()); err == nil {
		t.Fatal("healthcheck com 503 deveria falhar")
	}
}

func TestRunHealthcheckEnderecoInvalido(t *testing.T) {
	getenv := func(string) string { return "sem-porta" }
	if err := runHealthcheck(getenv, http.DefaultClient); err == nil {
		t.Fatal("APP_ADDR inválido deveria falhar")
	}
}

func TestRunHealthcheckGetenvNulo(t *testing.T) {
	if err := runHealthcheck(nil, http.DefaultClient); err == nil {
		t.Fatal("getenv nulo deveria falhar")
	}
}
