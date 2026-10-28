package ingest

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	// ErrQueueNotStarted indica enqueue antes de Start.
	ErrQueueNotStarted = errors.New("ingest: fila não iniciada")
	// ErrQueueStarted indica Start repetido.
	ErrQueueStarted = errors.New("ingest: fila já iniciada")
	// ErrQueueClosed indica enqueue depois de Close.
	ErrQueueClosed = errors.New("ingest: fila encerrada")
)

// Processor é o que a fila executa. *Service implementa.
type Processor interface {
	Process(ctx context.Context, documentID int64) (*Result, error)
}

// Queue processa documentos em sequência (uma extração por vez) em uma
// goroutine própria. O upload enfileira e responde sem esperar o parse.
type Queue struct {
	processor Processor
	fila      chan int64
	parar     chan struct{}

	mu        sync.Mutex
	iniciada  bool
	encerrada bool
	wg        sync.WaitGroup

	// OnResult é chamado no fim de cada documento, na goroutine do worker.
	// Deve ser rápido e não bloquear.
	OnResult func(documentID int64, res *Result, err error)
}

// NewQueue cria a fila com buffer de documentos pendentes. Buffer <= 0 usa 16
// (ver docs/architecture.md).
func NewQueue(p Processor, buffer int) *Queue {
	if buffer <= 0 {
		buffer = 16
	}
	return &Queue{
		processor: p,
		fila:      make(chan int64, buffer),
		parar:     make(chan struct{}),
	}
}

// Start inicia a goroutine do worker. O contexto cancela o processamento em
// andamento; Close é quem devolve. Só pode ser chamado uma vez.
func (q *Queue) Start(ctx context.Context) error {
	if q.processor == nil {
		return errors.New("ingest: fila sem processador")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	if q.encerrada {
		return ErrQueueClosed
	}
	if q.iniciada {
		return ErrQueueStarted
	}
	q.iniciada = true
	q.wg.Add(1)
	go q.loop(ctx)
	return nil
}

// Enqueue entrega um documento ao worker. Bloqueia apenas se o buffer estiver
// cheio, respeitando o contexto do chamador.
func (q *Queue) Enqueue(ctx context.Context, documentID int64) error {
	if documentID <= 0 {
		return fmt.Errorf("ingest: id de documento inválido: %d", documentID)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	q.mu.Lock()
	switch {
	case q.encerrada:
		q.mu.Unlock()
		return ErrQueueClosed
	case !q.iniciada:
		q.mu.Unlock()
		return ErrQueueNotStarted
	}
	fila, parar := q.fila, q.parar
	q.mu.Unlock()

	select {
	case fila <- documentID:
		return nil
	case <-parar:
		return ErrQueueClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close sinaliza o fim, drena o que já está no buffer e espera o documento em
// andamento terminar. É idempotente.
func (q *Queue) Close() {
	q.mu.Lock()
	if q.encerrada {
		q.mu.Unlock()
		q.wg.Wait()
		return
	}
	q.encerrada = true
	close(q.parar)
	q.mu.Unlock()
	q.wg.Wait()
}

// Pending informa quantos documentos aguardam no buffer.
func (q *Queue) Pending() int { return len(q.fila) }

func (q *Queue) loop(ctx context.Context) {
	defer q.wg.Done()
	for {
		select {
		case id := <-q.fila:
			q.executar(ctx, id)
		case <-q.parar:
			q.drenar(ctx)
			return
		}
	}
}

// drenar processa o que já foi aceito antes do Close, sem esperar novos
// enqueues (a fila está fechada para o chamador a partir daqui).
func (q *Queue) drenar(ctx context.Context) {
	for {
		select {
		case id := <-q.fila:
			q.executar(ctx, id)
		default:
			return
		}
	}
}

func (q *Queue) executar(ctx context.Context, id int64) {
	var (
		res *Result
		err error
	)
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("ingest: panic ao processar documento %d: %v", id, r)
		}
		if q.OnResult != nil {
			q.OnResult(id, res, err)
		}
	}()
	res, err = q.processor.Process(ctx, id)
}
