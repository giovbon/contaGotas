// Package auth implementa a autenticação de sessão da UI: senha única
// (APP_PASSWORD) conferida com Argon2id e cookie stateless assinado com
// APP_SECRET (ADR-0013).
//
// O pacote não guarda sessões no banco: o cookie carrega a validade e a
// assinatura HMAC. Trocar APP_SECRET ou APP_PASSWORD invalida todas as sessões.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	// CookieName é o nome do cookie de sessão da UI.
	CookieName = "rd_session"
	// DefaultTTL é a validade padrão da sessão (docs/web-ui.md).
	DefaultTTL = 7 * 24 * time.Hour
	// DefaultMaxAttempts é o número de falhas de login permitidas por janela.
	DefaultMaxAttempts = 5
	// DefaultWindow é a janela do rate limit de login.
	DefaultWindow = time.Minute
	// MinSecretBytes é o tamanho mínimo de APP_SECRET (docs/operations.md).
	MinSecretBytes = 32

	saltBytes  = 16
	hashBytes  = 32
	maxEntries = 8192

	sessionVersao = "v1"
	sessionRotulo = "readingdrip/session/v1\x00"
)

// ErrEmptyPassword indica APP_PASSWORD vazia.
var ErrEmptyPassword = errors.New("auth: APP_PASSWORD vazia")

// ErrWeakSecret indica APP_SECRET menor que o mínimo.
var ErrWeakSecret = fmt.Errorf("auth: APP_SECRET precisa de pelo menos %d bytes", MinSecretBytes)

// Argon2Params controla o custo do hash. O zero value usa os parâmetros de
// produção: 19 MiB, 2 iterações e 1 thread (OWASP, cabe no orçamento de RSS).
type Argon2Params struct {
	Memory  uint32 // KiB
	Time    uint32
	Threads uint8
}

func (p Argon2Params) normalizar() (Argon2Params, error) {
	if p == (Argon2Params{}) {
		return Argon2Params{Memory: 19 * 1024, Time: 2, Threads: 1}, nil
	}
	if p.Time < 1 || p.Threads < 1 || p.Memory < 8*uint32(p.Threads) {
		return Argon2Params{}, fmt.Errorf("auth: parâmetros Argon2id inválidos: %+v", p)
	}
	return p, nil
}

// Config são as entradas de New. Password e Secret vêm do ambiente; os demais
// campos têm padrões seguros.
type Config struct {
	Password         string
	Secret           []byte
	TTL              time.Duration
	BehindCloudflare bool
	MaxAttempts      int
	Window           time.Duration
	Argon2           Argon2Params
	Now              func() time.Time
	Random           io.Reader
}

// Authenticator confere a senha e assina/valida cookies de sessão. É seguro
// para uso concorrente.
type Authenticator struct {
	salt   []byte
	hash   []byte
	key    []byte
	ttl    time.Duration
	params Argon2Params
	cf     bool
	now    func() time.Time
	limit  *limiter
}

// New deriva o hash da senha e a chave do cookie. A senha em claro não é
// guardada.
func New(cfg Config) (*Authenticator, error) {
	if cfg.Password == "" {
		return nil, ErrEmptyPassword
	}
	if len(cfg.Secret) < MinSecretBytes {
		return nil, ErrWeakSecret
	}

	params, err := cfg.Argon2.normalizar()
	if err != nil {
		return nil, err
	}
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	agora := cfg.Now
	if agora == nil {
		agora = time.Now
	}
	random := cfg.Random
	if random == nil {
		random = rand.Reader
	}

	salt := make([]byte, saltBytes)
	if _, err := io.ReadFull(random, salt); err != nil {
		return nil, fmt.Errorf("auth: gerar salt: %w", err)
	}

	return &Authenticator{
		salt:   salt,
		hash:   argon2.IDKey([]byte(cfg.Password), salt, params.Time, params.Memory, params.Threads, hashBytes),
		key:    derivarChave(cfg.Secret, cfg.Password),
		ttl:    ttl,
		params: params,
		cf:     cfg.BehindCloudflare,
		now:    agora,
		limit:  newLimiter(cfg.MaxAttempts, cfg.Window, agora),
	}, nil
}

// derivarChave separa a chave do cookie de outros usos de APP_SECRET (links
// assinados) e liga a sessão à senha: trocar qualquer um invalida os cookies.
func derivarChave(secret []byte, password string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(sessionRotulo))
	mac.Write([]byte(password))
	return mac.Sum(nil)
}

// CheckPassword compara a senha em tempo constante, com o mesmo custo para
// senha certa e errada.
func (a *Authenticator) CheckPassword(password string) bool {
	candidato := argon2.IDKey([]byte(password), a.salt, a.params.Time, a.params.Memory, a.params.Threads, hashBytes)
	return subtle.ConstantTimeCompare(candidato, a.hash) == 1
}

// Authenticated informa se a request traz um cookie de sessão válido.
func (a *Authenticator) Authenticated(r *http.Request) bool {
	if r == nil {
		return false
	}
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return false
	}
	return a.validar(cookie.Value)
}

// SetCookie grava o cookie de sessão. HttpOnly e SameSite=Lax sempre; Secure
// quando a request chega por HTTPS (direto ou com X-Forwarded-Proto https).
func (a *Authenticator) SetCookie(w http.ResponseWriter, r *http.Request) {
	expira := a.now().Add(a.ttl).Truncate(time.Second)
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    a.assinar(expira),
		Path:     "/",
		Expires:  expira,
		MaxAge:   int(a.ttl / time.Second),
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearCookie expira o cookie no cliente.
func (a *Authenticator) ClearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// Middleware protege rotas de gestão. Sem sessão, redireciona para /login
// (RF-074), preservando o destino apenas em GET/HEAD e apenas se ele for um
// caminho local.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.Authenticated(r) {
			next.ServeHTTP(w, r)
			return
		}
		destino := "/login"
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			if alvo := SafeNext(r.URL.RequestURI()); alvo != "/" {
				destino += "?next=" + url.QueryEscape(alvo)
			}
		}
		http.Redirect(w, r, destino, http.StatusSeeOther)
	})
}

// Allowed informa se o IP pode tentar login e, quando não pode, quanto falta
// para a janela liberar.
func (a *Authenticator) Allowed(ip string) (bool, time.Duration) {
	return a.limit.allowed(ip)
}

// Fail registra uma senha errada para o IP.
func (a *Authenticator) Fail(ip string) { a.limit.fail(ip) }

// Reset zera as falhas do IP depois de um login bem-sucedido.
func (a *Authenticator) Reset(ip string) { a.limit.reset(ip) }

// ClientIP devolve o IP do cliente. Com BEHIND_CLOUDFLARE=true confia em
// CF-Connecting-IP; caso contrário usa apenas o endereço da conexão.
func (a *Authenticator) ClientIP(r *http.Request) string {
	if r == nil {
		return "desconhecido"
	}
	if a.cf {
		if ip := net.ParseIP(strings.TrimSpace(r.Header.Get("CF-Connecting-IP"))); ip != nil {
			return ip.String()
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}
	if ip := net.ParseIP(r.RemoteAddr); ip != nil {
		return ip.String()
	}
	return "desconhecido"
}

func (a *Authenticator) assinar(expira time.Time) string {
	payload := sessionVersao + "." + strconv.FormatInt(expira.Unix(), 10)
	mac := hmac.New(sha256.New, a.key)
	mac.Write([]byte(payload))
	return payload + "." + urlSafe(mac.Sum(nil))
}

func (a *Authenticator) validar(valor string) bool {
	partes := strings.Split(valor, ".")
	if len(partes) != 3 || partes[0] != sessionVersao {
		return false
	}
	expira, err := strconv.ParseInt(partes[1], 10, 64)
	if err != nil || expira <= a.now().Unix() {
		return false
	}
	payload := partes[0] + "." + partes[1]
	mac := hmac.New(sha256.New, a.key)
	mac.Write([]byte(payload))
	esperado := mac.Sum(nil)
	recebido, err := urlSafeDecode(partes[2])
	if err != nil {
		return false
	}
	return hmac.Equal(recebido, esperado)
}

// SafeNext aceita apenas caminhos locais como destino pós-login. Qualquer
// coisa com esquema, host, barra invertida ou caminho que decodifique para
// //host vira "/".
func SafeNext(raw string) string {
	if raw == "" || temControle(raw) {
		return "/"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" || u.User != nil {
		return "/"
	}
	decodificado, err := url.PathUnescape(u.EscapedPath())
	if err != nil || !strings.HasPrefix(decodificado, "/") || strings.HasPrefix(decodificado, "//") ||
		strings.ContainsRune(decodificado, '\\') || temControle(decodificado) {
		return "/"
	}
	return u.RequestURI()
}

func temControle(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// isHTTPS aceita TLS direto ou o cabeçalho do proxy (Cloudflare).
func isHTTPS(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	proto := r.Header.Get("X-Forwarded-Proto")
	if i := strings.IndexByte(proto, ','); i >= 0 {
		proto = proto[:i]
	}
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}
