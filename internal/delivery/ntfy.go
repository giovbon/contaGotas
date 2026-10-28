package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultNtfyAttempts é o total de tentativas por mensagem.
	DefaultNtfyAttempts = 3
	// DefaultNtfyMaxBytes é o limite aplicado antes de enviar.
	DefaultNtfyMaxBytes = 60_000
	corpoErroMaximo     = 4 << 10
)

// ErrMessageTooLarge indica corpo acima do limite local; não há retry.
var ErrMessageTooLarge = errors.New("ntfy: mensagem acima do limite")

// StatusError é uma resposta HTTP não-2xx do ntfy. Token e corpo da
// requisição nunca entram na mensagem de erro.
type StatusError struct {
	Code int
}

func (e *StatusError) Error() string {
	return "ntfy: resposta HTTP " + strconv.Itoa(e.Code)
}

// Temporario informa se vale retry (5xx e 429).
func (e *StatusError) Temporario() bool {
	return e.Code == http.StatusTooManyRequests || e.Code >= 500
}

// ClientConfig configura o cliente ntfy.
type ClientConfig struct {
	BaseURL         string
	Topic           string
	Token           string
	Priority        int
	Tags            []string
	MaxAttempts     int
	Backoff         []time.Duration
	MaxMessageBytes int
	HTTPClient      *http.Client
	// Sleep é injetável nos testes; o padrão respeita o contexto.
	Sleep func(context.Context, time.Duration) error
}

// Client publica mensagens no ntfy.
type Client struct {
	baseURL    string
	topic      string
	token      string
	priority   int
	tags       []string
	tentativas int
	backoff    []time.Duration
	maxBytes   int
	http       *http.Client
	sleep      func(context.Context, time.Duration) error
}

// NewClient valida a configuração e aplica padrões.
func NewClient(cfg ClientConfig) (*Client, error) {
	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" {
		return nil, errors.New("ntfy: base URL vazia")
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("ntfy: base URL inválida: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("ntfy: base URL precisa ser http(s) com host: %q", base)
	}
	topic := strings.TrimSpace(cfg.Topic)
	if topic == "" {
		return nil, errors.New("ntfy: topic vazio")
	}

	tentativas := cfg.MaxAttempts
	if tentativas <= 0 {
		tentativas = DefaultNtfyAttempts
	}
	backoff := cfg.Backoff
	if len(backoff) == 0 {
		backoff = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute}
	}
	maxBytes := cfg.MaxMessageBytes
	if maxBytes <= 0 {
		maxBytes = DefaultNtfyMaxBytes
	}
	clienteHTTP := cfg.HTTPClient
	if clienteHTTP == nil {
		clienteHTTP = &http.Client{Timeout: 30 * time.Second}
	}
	dormir := cfg.Sleep
	if dormir == nil {
		dormir = dormirContexto
	}

	return &Client{
		baseURL:    strings.TrimRight(base, "/"),
		topic:      topic,
		token:      strings.TrimSpace(cfg.Token),
		priority:   cfg.Priority,
		tags:       append([]string(nil), cfg.Tags...),
		tentativas: tentativas,
		backoff:    append([]time.Duration(nil), backoff...),
		maxBytes:   maxBytes,
		http:       clienteHTTP,
		sleep:      dormir,
	}, nil
}

// SendResult resume um envio bem-sucedido.
type SendResult struct {
	Status   int
	Bytes    int
	Attempts int
	Latency  time.Duration
}

type payload struct {
	Topic    string   `json:"topic"`
	Title    string   `json:"title,omitempty"`
	Message  string   `json:"message"`
	Markdown bool     `json:"markdown"`
	Priority int      `json:"priority,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Click    string   `json:"click,omitempty"`
	Actions  []Action `json:"actions,omitempty"`
}

// Publish envia a mensagem com retry para erros transitórios. Erros 4xx
// (exceto 429) e corpo grande demais não são repetidos.
func (c *Client) Publish(ctx context.Context, msg Message) (SendResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(msg.Body) == "" {
		return SendResult{}, errors.New("ntfy: mensagem vazia")
	}
	if len(msg.Body) > c.maxBytes {
		return SendResult{}, fmt.Errorf("%w: %d bytes (limite %d)", ErrMessageTooLarge, len(msg.Body), c.maxBytes)
	}
	if err := ctx.Err(); err != nil {
		return SendResult{}, err
	}

	corpo, err := json.Marshal(payload{
		Topic:    c.topic,
		Title:    strings.TrimSpace(msg.Title),
		Message:  msg.Body,
		Markdown: true,
		Priority: c.priority,
		Tags:     c.tags,
		Click:    strings.TrimSpace(msg.Click),
		Actions:  msg.Actions,
	})
	if err != nil {
		return SendResult{}, fmt.Errorf("ntfy: montar JSON: %w", err)
	}

	var ultimo error
	ultimaLatencia := time.Duration(0)
	for tentativa := 1; tentativa <= c.tentativas; tentativa++ {
		inicio := time.Now()
		status, espera, err := c.enviar(ctx, corpo)
		latencia := time.Since(inicio)
		ultimaLatencia = latencia
		if err == nil {
			return SendResult{Status: status, Bytes: len(msg.Body), Attempts: tentativa, Latency: latencia}, nil
		}
		ultimo = err

		var se *StatusError
		if errors.As(err, &se) && !se.Temporario() {
			return SendResult{Status: se.Code, Attempts: tentativa, Latency: latencia}, err
		}
		if tentativa == c.tentativas {
			break
		}
		if espera <= 0 {
			espera = c.espera(tentativa - 1)
		}
		if err := c.sleep(ctx, espera); err != nil {
			return SendResult{Attempts: tentativa, Latency: latencia}, err
		}
	}
	return SendResult{Attempts: c.tentativas, Latency: ultimaLatencia}, fmt.Errorf("ntfy: falha após %d tentativas: %w", c.tentativas, ultimo)
}

func (c *Client) enviar(ctx context.Context, corpo []byte) (int, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/", bytes.NewReader(corpo))
	if err != nil {
		return 0, 0, fmt.Errorf("ntfy: criar request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("ntfy: enviar: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, corpoErroMaximo))

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		se := &StatusError{Code: resp.StatusCode}
		return resp.StatusCode, retryAfter(resp.Header.Get("Retry-After")), se
	}
	return resp.StatusCode, 0, nil
}

func (c *Client) espera(indice int) time.Duration {
	if len(c.backoff) == 0 {
		return 0
	}
	if indice >= len(c.backoff) {
		return c.backoff[len(c.backoff)-1]
	}
	return c.backoff[indice]
}

func retryAfter(bruto string) time.Duration {
	bruto = strings.TrimSpace(bruto)
	if bruto == "" {
		return 0
	}
	if segundos, err := strconv.Atoi(bruto); err == nil && segundos > 0 {
		return time.Duration(segundos) * time.Second
	}
	if quando, err := http.ParseTime(bruto); err == nil {
		if espera := time.Until(quando); espera > 0 {
			return espera
		}
	}
	return 0
}

func dormirContexto(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
