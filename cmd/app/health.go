package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/giovani/readingdrip/internal/platform"
)

// healthTimeout é o tempo máximo do healthcheck do container. O Docker usa
// --timeout=3s; manter o cliente abaixo disso evita que o processo seja morto
// antes de relatar o motivo.
const healthTimeout = 2500 * time.Millisecond

// runHealthcheck consulta /readyz no próprio processo. É usado pelo
// HEALTHCHECK do container, já que a imagem distroless não tem shell, curl
// nem wget (ADR-0008).
func runHealthcheck(getenv func(string) string, client *http.Client) error {
	if getenv == nil {
		return errors.New("healthcheck: getenv nulo")
	}
	if client == nil {
		client = &http.Client{Timeout: healthTimeout}
	}

	url, err := healthcheckURL(getenv("APP_ADDR"))
	if err != nil {
		return err
	}
	return consultarSaude(client, url)
}

// healthcheckURL monta a URL local de /readyz a partir de APP_ADDR. Endereços
// sem host (`:8080`), com `0.0.0.0` ou `[::]` são normalizados para 127.0.0.1,
// pois o healthcheck roda dentro do mesmo container.
func healthcheckURL(addr string) (string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = platform.DefaultAddr
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("healthcheck: APP_ADDR inválido: %q", addr)
	}
	if port == "" {
		return "", fmt.Errorf("healthcheck: APP_ADDR sem porta: %q", addr)
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/readyz", nil
}

// consultarSaude aceita somente 200; qualquer outro status significa que o
// processo ainda não está pronto (banco indisponível, por exemplo).
func consultarSaude(client *http.Client, url string) error {
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("healthcheck: %s: %w", url, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: %s respondeu %d", url, resp.StatusCode)
	}
	return nil
}
