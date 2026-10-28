package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func clienteTeste(t *testing.T, base string, ajustes func(*ClientConfig)) *Client {
	t.Helper()
	cfg := ClientConfig{
		BaseURL:     base,
		Topic:       "leitura",
		Token:       "tk_segredo",
		Priority:    3,
		Tags:        []string{"books", "open_book"},
		MaxAttempts: 3,
		Backoff:     []time.Duration{time.Second, 2 * time.Second},
		Sleep:       func(context.Context, time.Duration) error { return nil },
	}
	if ajustes != nil {
		ajustes(&cfg)
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestPublishPayload(t *testing.T) {
	var recebido payload
	var auth, tipo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		tipo = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&recebido); err != nil {
			t.Errorf("decodificar payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	msg := Message{
		Title: "📖 Livro",
		Body:  "corpo **markdown**",
		Click: "https://leitura.exemplo.com/r/1",
		Actions: []Action{
			{Action: "view", Label: "Abrir leitor", URL: "https://leitura.exemplo.com/r/1"},
			{Action: "http", Label: "Marcar lido", URL: "https://leitura.exemplo.com/api/actions/chunks/1/done", Method: "POST"},
		},
	}
	res, err := clienteTeste(t, srv.URL, nil).Publish(context.Background(), msg)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != 200 || res.Attempts != 1 || res.Bytes != len(msg.Body) {
		t.Errorf("resultado = %+v", res)
	}
	if auth != "Bearer tk_segredo" {
		t.Errorf("Authorization = %q", auth)
	}
	if tipo != "application/json" {
		t.Errorf("Content-Type = %q", tipo)
	}
	if recebido.Topic != "leitura" || recebido.Title != msg.Title || recebido.Message != msg.Body {
		t.Errorf("payload = %+v", recebido)
	}
	if !recebido.Markdown || recebido.Priority != 3 {
		t.Errorf("markdown/prioridade = %v / %d", recebido.Markdown, recebido.Priority)
	}
	if len(recebido.Tags) != 2 || recebido.Tags[0] != "books" {
		t.Errorf("tags = %v", recebido.Tags)
	}
	if recebido.Click != msg.Click || len(recebido.Actions) != 2 {
		t.Errorf("click/actions = %q / %+v", recebido.Click, recebido.Actions)
	}
}

func TestPublishRetry5xx(t *testing.T) {
	var tentativas atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tentativas.Add(1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var mu sync.Mutex
	var esperas []time.Duration
	c := clienteTeste(t, srv.URL, func(cfg *ClientConfig) {
		cfg.Sleep = func(_ context.Context, d time.Duration) error {
			mu.Lock()
			esperas = append(esperas, d)
			mu.Unlock()
			return nil
		}
	})
	res, err := c.Publish(context.Background(), Message{Body: "oi"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempts != 3 || tentativas.Load() != 3 {
		t.Errorf("attempts = %d, requisições = %d", res.Attempts, tentativas.Load())
	}
	if len(esperas) != 2 || esperas[0] != time.Second || esperas[1] != 2*time.Second {
		t.Errorf("esperas = %v", esperas)
	}
	if res.Status != 200 || res.Bytes != 2 {
		t.Errorf("resultado = %+v", res)
	}
}

func TestPublishSemRetry4xx(t *testing.T) {
	var tentativas atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tentativas.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	dormiu := false
	c := clienteTeste(t, srv.URL, func(cfg *ClientConfig) {
		cfg.Sleep = func(context.Context, time.Duration) error {
			dormiu = true
			return nil
		}
	})
	_, err := c.Publish(context.Background(), Message{Body: "oi"})
	if err == nil {
		t.Fatal("esperava erro")
	}
	var se *StatusError
	if !errors.As(err, &se) || se.Code != 401 {
		t.Errorf("erro = %v", err)
	}
	if tentativas.Load() != 1 || dormiu {
		t.Errorf("tentativas = %d, dormiu = %v", tentativas.Load(), dormiu)
	}
}

func TestPublishRetryAfter(t *testing.T) {
	var tentativas atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tentativas.Add(1) == 1 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var espera time.Duration
	c := clienteTeste(t, srv.URL, func(cfg *ClientConfig) {
		cfg.Sleep = func(_ context.Context, d time.Duration) error {
			espera = d
			return nil
		}
	})
	res, err := c.Publish(context.Background(), Message{Body: "oi"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempts != 2 || espera != 7*time.Second {
		t.Errorf("attempts = %d, espera = %v", res.Attempts, espera)
	}
}

func TestPublishFalhaFinal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := clienteTeste(t, srv.URL, nil)
	res, err := c.Publish(context.Background(), Message{Body: "oi"})
	if err == nil {
		t.Fatal("esperava erro")
	}
	if res.Attempts != 3 {
		t.Errorf("attempts = %d", res.Attempts)
	}
}

func TestPublishContextoCancelado(t *testing.T) {
	var tentativas atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tentativas.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancelar := context.WithCancel(context.Background())
	c := clienteTeste(t, srv.URL, func(cfg *ClientConfig) {
		cfg.Sleep = func(context.Context, time.Duration) error { return context.Canceled }
	})
	_, err := c.Publish(ctx, Message{Body: "oi"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("erro = %v", err)
	}
	if tentativas.Load() != 1 {
		t.Errorf("requisições = %d", tentativas.Load())
	}
	cancelar()
}

func TestPublishGrandeDemais(t *testing.T) {
	var tentativas atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tentativas.Add(1)
	}))
	defer srv.Close()

	c := clienteTeste(t, srv.URL, func(cfg *ClientConfig) { cfg.MaxMessageBytes = 4 })
	_, err := c.Publish(context.Background(), Message{Body: "12345"})
	if !errors.Is(err, ErrMessageTooLarge) {
		t.Errorf("erro = %v", err)
	}
	if tentativas.Load() != 0 {
		t.Errorf("houve requisição para mensagem grande demais")
	}
}

func TestNewClientValida(t *testing.T) {
	casos := []ClientConfig{
		{BaseURL: "", Topic: "x"},
		{BaseURL: "ftp://ntfy", Topic: "x"},
		{BaseURL: "http://ntfy", Topic: ""},
		{BaseURL: "http://", Topic: "x"},
	}
	for _, cfg := range casos {
		if _, err := NewClient(cfg); err == nil {
			t.Errorf("esperava erro para %+v", cfg)
		}
	}
	if _, err := NewClient(ClientConfig{BaseURL: "http://ntfy", Topic: "x"}); err != nil {
		t.Errorf("configuração válida recusada: %v", err)
	}
}

func TestDormirContexto(t *testing.T) {
	if err := dormirContexto(context.Background(), 0); err != nil {
		t.Errorf("espera zero = %v", err)
	}
	ctx, cancelar := context.WithCancel(context.Background())
	cancelar()
	if err := dormirContexto(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("contexto cancelado = %v", err)
	}
}

func TestRetryAfterHTTPDate(t *testing.T) {
	quando := time.Now().Add(5 * time.Second).UTC().Format(http.TimeFormat)
	if d := retryAfter(quando); d <= 0 || d > 6*time.Second {
		t.Errorf("retryAfter(date) = %v", d)
	}
	if d := retryAfter(""); d != 0 {
		t.Errorf("retryAfter(vazio) = %v", d)
	}
	if d := retryAfter("abc"); d != 0 {
		t.Errorf("retryAfter(inválido) = %v", d)
	}
}
