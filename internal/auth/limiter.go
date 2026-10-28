package auth

import (
	"encoding/base64"
	"sync"
	"time"
)

func urlSafe(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func urlSafeDecode(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }

// limiter conta falhas de login por IP dentro de uma janela deslizante. Fica
// em memória: reiniciar o app zera as tentativas (instância única).
type limiter struct {
	mu         sync.Mutex
	tentativas map[string][]time.Time
	max        int
	window     time.Duration
	now        func() time.Time
}

func newLimiter(max int, window time.Duration, now func() time.Time) *limiter {
	if max <= 0 {
		max = DefaultMaxAttempts
	}
	if window <= 0 {
		window = DefaultWindow
	}
	if now == nil {
		now = time.Now
	}
	return &limiter{
		tentativas: make(map[string][]time.Time),
		max:        max,
		window:     window,
		now:        now,
	}
}

// allowed não consome tentativa: só responde se o IP ainda pode tentar.
func (l *limiter) allowed(ip string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	agora := l.now()
	lista := l.purgar(ip, agora)
	if len(lista) == 0 {
		return true, 0
	}
	if len(lista) >= l.max {
		espera := lista[0].Add(l.window).Sub(agora)
		if espera <= 0 {
			espera = time.Second
		}
		return false, espera
	}
	return true, 0
}

func (l *limiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	agora := l.now()
	lista := append(l.purgar(ip, agora), agora)
	l.tentativas[ip] = lista

	if len(l.tentativas) > maxEntries {
		l.podarMapa(agora)
	}
}

func (l *limiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.tentativas, ip)
}

// purgar remove da lista do IP as tentativas fora da janela e apaga a entrada
// quando ela fica vazia.
func (l *limiter) purgar(ip string, agora time.Time) []time.Time {
	lista := l.tentativas[ip]
	corte := agora.Add(-l.window)
	i := 0
	for i < len(lista) && !lista[i].After(corte) {
		i++
	}
	lista = lista[i:]
	if len(lista) == 0 {
		delete(l.tentativas, ip)
		return nil
	}
	l.tentativas[ip] = lista
	return lista
}

// podarMapa limita a memória do rate limiter. Primeiro remove entradas fora da
// janela; se ainda estiver cheio, descarta a mais antiga.
func (l *limiter) podarMapa(agora time.Time) {
	for ip := range l.tentativas {
		l.purgar(ip, agora)
	}
	for len(l.tentativas) > maxEntries {
		var (
			maisAntigo string
			quando     time.Time
		)
		for ip, lista := range l.tentativas {
			if quando.IsZero() || lista[len(lista)-1].Before(quando) {
				maisAntigo, quando = ip, lista[len(lista)-1]
			}
		}
		if maisAntigo == "" {
			return
		}
		delete(l.tentativas, maisAntigo)
	}
}
