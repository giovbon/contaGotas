package ingest_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/giovani/readingdrip/internal/ingest"
	"github.com/giovani/readingdrip/internal/store"
)

type processorFunc func(ctx context.Context, documentID int64) (*ingest.Result, error)

func (f processorFunc) Process(ctx context.Context, documentID int64) (*ingest.Result, error) {
	return f(ctx, documentID)
}

// processadorFake registra ordem, concorrência e erros por documento.
type processadorFake struct {
	mu     sync.Mutex
	ids    []int64
	ativos int
	pico   int
	erros  map[int64]error
	panico map[int64]bool
	atraso time.Duration
}

func (p *processadorFake) Process(ctx context.Context, id int64) (*ingest.Result, error) {
	p.mu.Lock()
	p.ativos++
	if p.ativos > p.pico {
		p.pico = p.ativos
	}
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.ativos--
		p.mu.Unlock()
	}()

	if p.atraso > 0 {
		time.Sleep(p.atraso)
	}

	p.mu.Lock()
	p.ids = append(p.ids, id)
	erro := p.erros[id]
	panico := p.panico[id]
	p.mu.Unlock()

	if panico {
		panic("falha simulada")
	}
	if erro != nil {
		return nil, erro
	}
	return &ingest.Result{DocumentID: id, Status: store.StatusReady}, nil
}

func TestQueueProcessaEmSequencia(t *testing.T) {
	proc := &processadorFake{atraso: time.Millisecond}
	q := ingest.NewQueue(proc, 4)

	var (
		mu     sync.Mutex
		vistos []int64
		erros  []error
	)
	q.OnResult = func(id int64, res *ingest.Result, err error) {
		mu.Lock()
		defer mu.Unlock()
		vistos = append(vistos, id)
		erros = append(erros, err)
	}

	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{1, 2, 3} {
		if err := q.Enqueue(context.Background(), id); err != nil {
			t.Fatalf("enfileirar %d: %v", id, err)
		}
	}
	q.Close()

	proc.mu.Lock()
	ordem, pico := append([]int64(nil), proc.ids...), proc.pico
	proc.mu.Unlock()

	if len(ordem) != 3 || ordem[0] != 1 || ordem[1] != 2 || ordem[2] != 3 {
		t.Fatalf("ordem = %v, quer [1 2 3]", ordem)
	}
	if pico != 1 {
		t.Errorf("processamentos simultâneos = %d, quer 1", pico)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(vistos) != 3 {
		t.Fatalf("resultados = %v", vistos)
	}
	for i, err := range erros {
		if err != nil {
			t.Errorf("resultado %d com erro: %v", i, err)
		}
	}
}

func TestQueueCicloDeVida(t *testing.T) {
	q := ingest.NewQueue(processorFunc(func(context.Context, int64) (*ingest.Result, error) {
		return &ingest.Result{}, nil
	}), 1)

	if err := q.Enqueue(context.Background(), 1); !errors.Is(err, ingest.ErrQueueNotStarted) {
		t.Fatalf("enqueue antes do start: %v, quer ErrQueueNotStarted", err)
	}
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := q.Start(context.Background()); !errors.Is(err, ingest.ErrQueueStarted) {
		t.Fatalf("start repetido: %v, quer ErrQueueStarted", err)
	}
	q.Close()
	q.Close() // idempotente

	if err := q.Enqueue(context.Background(), 1); !errors.Is(err, ingest.ErrQueueClosed) {
		t.Fatalf("enqueue após close: %v, quer ErrQueueClosed", err)
	}
	if err := q.Enqueue(context.Background(), 0); err == nil {
		t.Fatal("esperado erro para id inválido")
	}
}

func TestQueueErroEPanicoNaoParamOWorker(t *testing.T) {
	proc := &processadorFake{
		erros:  map[int64]error{2: errors.New("extração falhou")},
		panico: map[int64]bool{3: true},
	}
	q := ingest.NewQueue(proc, 4)

	var (
		mu    sync.Mutex
		erros = map[int64]error{}
	)
	q.OnResult = func(id int64, _ *ingest.Result, err error) {
		mu.Lock()
		defer mu.Unlock()
		erros[id] = err
	}

	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{1, 2, 3, 4} {
		if err := q.Enqueue(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	q.Close()

	proc.mu.Lock()
	total := len(proc.ids)
	proc.mu.Unlock()
	if total != 4 {
		t.Fatalf("processados = %d, quer 4 (o worker deve continuar após erro)", total)
	}

	mu.Lock()
	defer mu.Unlock()
	if erros[1] != nil || erros[4] != nil {
		t.Errorf("documentos 1/4 deveriam passar: %+v", erros)
	}
	if erros[2] == nil || !strings.Contains(erros[2].Error(), "extração falhou") {
		t.Errorf("erro do documento 2 = %v", erros[2])
	}
	if erros[3] == nil || !strings.Contains(erros[3].Error(), "panic") {
		t.Errorf("erro do documento 3 = %v", erros[3])
	}
}

func TestQueueEnqueueRespeitaContexto(t *testing.T) {
	bloqueio := make(chan struct{})
	iniciou := make(chan struct{})
	var umaVez sync.Once
	proc := processorFunc(func(context.Context, int64) (*ingest.Result, error) {
		umaVez.Do(func() { close(iniciou) })
		<-bloqueio
		return &ingest.Result{}, nil
	})
	q := ingest.NewQueue(proc, 2)
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := q.Enqueue(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	<-iniciou // worker preso no documento 1
	if err := q.Enqueue(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	if err := q.Enqueue(context.Background(), 3); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := q.Enqueue(ctx, 4); !errors.Is(err, context.Canceled) {
		t.Fatalf("enqueue com buffer cheio: %v, quer context.Canceled", err)
	}

	close(bloqueio)
	q.Close()
}
