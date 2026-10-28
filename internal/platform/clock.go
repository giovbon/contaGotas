package platform

import (
	"sync"
	"time"
)

// Clock é a fonte de tempo do agendador. Todo código de agendamento recebe um
// Clock em vez de chamar time.Now diretamente (docs/scheduling.md).
type Clock interface {
	Now() time.Time
}

// SystemClock usa o relógio do processo.
type SystemClock struct{}

// Now devolve o instante atual.
func (SystemClock) Now() time.Time { return time.Now() }

// FakeClock é um relógio controlável para testes. É seguro para uso
// concorrente.
type FakeClock struct {
	mu    sync.Mutex
	agora time.Time
}

// NewFakeClock cria um relógio parado em t.
func NewFakeClock(t time.Time) *FakeClock { return &FakeClock{agora: t} }

// Now devolve o instante atual do relógio falso.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.agora
}

// Set muda o instante atual.
func (c *FakeClock) Set(t time.Time) {
	c.mu.Lock()
	c.agora = t
	c.mu.Unlock()
}

// Advance avança o relógio em d.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.agora = c.agora.Add(d)
	c.mu.Unlock()
}
