// Package links assina e confere os links do leitor e das ações do ntfy
// (ADR-0010). O token é stateless: `{expiração}.{HMAC-SHA256}` sobre
// `escopo:id:expiração`, com a chave derivada de APP_SECRET. Trocar o segredo
// invalida todos os links.
package links

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Escopos dos tokens. Cada escopo só vale para o seu uso.
const (
	// ScopeRead abre o leitor (`GET /r/{chunkID}`).
	ScopeRead = "r"
	// ScopeDone marca o chunk como lido (`POST /api/actions/chunks/{id}/done`).
	ScopeDone = "done"
)

// Validades padrão (docs/web-ui.md e docs/delivery-ntfy.md).
const (
	DefaultReadTTL = 180 * 24 * time.Hour
	DefaultDoneTTL = 30 * 24 * time.Hour
	// MinSecretBytes é o mínimo de APP_SECRET aceito (docs/operations.md).
	MinSecretBytes = 32

	rotuloChave = "readingdrip/link/v1\x00"
	maxExpBytes = 12
)

// ErrTokenInvalido cobre token ausente, malformado, adulterado, de outro
// escopo/chunk ou expirado. A mensagem não diferencia os casos.
var ErrTokenInvalido = errors.New("links: token inválido ou expirado")

// Config são as entradas de New.
type Config struct {
	Secret  []byte
	BaseURL string
	Now     func() time.Time
}

// Signer assina e confere tokens. É seguro para uso concorrente.
type Signer struct {
	key  []byte
	base string
	now  func() time.Time
}

// New valida o segredo e a URL base e deriva a chave dos links.
func New(cfg Config) (*Signer, error) {
	if len(cfg.Secret) < MinSecretBytes {
		return nil, fmt.Errorf("links: APP_SECRET precisa de pelo menos %d bytes", MinSecretBytes)
	}
	base, err := normalizarBase(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	agora := cfg.Now
	if agora == nil {
		agora = time.Now
	}

	mac := hmac.New(sha256.New, cfg.Secret)
	mac.Write([]byte(rotuloChave))
	return &Signer{key: mac.Sum(nil), base: base, now: agora}, nil
}

// ReadURL devolve a URL do leitor para o chunk.
func (s *Signer) ReadURL(chunkID int64, ttl time.Duration) string {
	return s.base + "/r/" + strconv.FormatInt(chunkID, 10) + "?t=" + s.Token(ScopeRead, chunkID, ttl)
}

// DoneURL devolve a URL de “marcar lido” para o chunk.
func (s *Signer) DoneURL(chunkID int64, ttl time.Duration) string {
	return s.base + "/api/actions/chunks/" + strconv.FormatInt(chunkID, 10) + "/done?t=" +
		s.Token(ScopeDone, chunkID, ttl)
}

// Token assina o par escopo/chunk com validade ttl a partir do relógio do
// Signer. ttl <= 0 usa a validade padrão do escopo.
func (s *Signer) Token(scope string, chunkID int64, ttl time.Duration) string {
	if s == nil {
		return ""
	}
	if ttl <= 0 {
		ttl = DefaultReadTTL
		if scope == ScopeDone {
			ttl = DefaultDoneTTL
		}
	}
	exp := s.now().Add(ttl).Unix()
	return strconv.FormatInt(exp, 10) + "." + s.assinar(scope, chunkID, exp)
}

// Verify confere o token do escopo e do chunk. Devolve ErrTokenInvalido para
// qualquer divergência — nunca detalha qual.
func (s *Signer) Verify(scope string, chunkID int64, token string) error {
	if s == nil {
		return ErrTokenInvalido
	}
	token = strings.TrimSpace(token)
	expTexto, sig, ok := strings.Cut(token, ".")
	if !ok || expTexto == "" || sig == "" || len(expTexto) > maxExpBytes {
		return ErrTokenInvalido
	}
	exp, err := strconv.ParseInt(expTexto, 10, 64)
	if err != nil || exp <= 0 {
		return ErrTokenInvalido
	}
	esperado := s.assinar(scope, chunkID, exp)
	// Compara antes de checar a validade para não distinguir “expirado” de
	// “assinatura errada” pelo tempo de resposta.
	if !hmac.Equal([]byte(sig), []byte(esperado)) {
		return ErrTokenInvalido
	}
	if s.now().Unix() >= exp {
		return ErrTokenInvalido
	}
	return nil
}

func (s *Signer) assinar(scope string, chunkID, exp int64) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(scope))
	mac.Write([]byte(":"))
	mac.Write([]byte(strconv.FormatInt(chunkID, 10)))
	mac.Write([]byte(":"))
	mac.Write([]byte(strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

func normalizarBase(bruto string) (string, error) {
	bruto = strings.TrimSpace(bruto)
	if bruto == "" {
		return "", errors.New("links: APP_BASE_URL vazia")
	}
	u, err := url.Parse(bruto)
	if err != nil {
		return "", fmt.Errorf("links: APP_BASE_URL inválida: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("links: APP_BASE_URL precisa de http ou https: %q", bruto)
	}
	if u.Host == "" {
		return "", fmt.Errorf("links: APP_BASE_URL sem host: %q", bruto)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("links: APP_BASE_URL não pode ter query nem fragmento: %q", bruto)
	}
	return strings.TrimRight(u.String(), "/"), nil
}
