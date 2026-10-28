package schedule

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/giovani/readingdrip/internal/blocks"
	"github.com/giovani/readingdrip/internal/delivery"
	"github.com/giovani/readingdrip/internal/plan"
	"github.com/giovani/readingdrip/internal/platform"
	"github.com/giovani/readingdrip/internal/store"
)

var quando = time.Date(2026, 10, 26, 8, 0, 0, 0, time.FixedZone("BRT", -3*3600))

func publicarConfigurado(t *testing.T, s *store.Store) {
	t.Helper()
	definir(t, s, KeyNtfyBaseURL, "http://ntfy:8080")
	definir(t, s, KeyNtfyTopic, "leitura")
}

func semCatchup(t *testing.T, s *store.Store) {
	t.Helper()
	definir(t, s, KeyCatchupDays, "0")
}

type publicadorFake struct {
	mu        sync.Mutex
	mensagens []delivery.Message
	config    NtfyConfig
	err       error
	hook      func()
}

func (p *publicadorFake) Publish(_ context.Context, msg delivery.Message) (delivery.SendResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return delivery.SendResult{Attempts: 1}, p.err
	}
	if p.hook != nil {
		p.hook()
	}
	p.mensagens = append(p.mensagens, msg)
	return delivery.SendResult{Status: 200, Bytes: len(msg.Body), Attempts: 1}, nil
}

func (p *publicadorFake) total() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.mensagens)
}

func (p *publicadorFake) todas() []delivery.Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]delivery.Message(nil), p.mensagens...)
}

func novoRunner(t *testing.T, s *store.Store, pub *publicadorFake) (*Runner, *publicadorFake) {
	t.Helper()
	if pub == nil {
		pub = &publicadorFake{}
	}
	return &Runner{
		Store:  s,
		Clock:  platform.NewFakeClock(quando),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Sleep:  func(context.Context, time.Duration) error { return nil },
		NewPublisher: func(cfg NtfyConfig) (Publisher, error) {
			pub.config = cfg
			return pub, nil
		},
	}, pub
}

func criarDocumento(t *testing.T, s *store.Store, titulo string, prioridade int, status store.DocumentStatus) int64 {
	t.Helper()
	ctx := context.Background()
	docID, err := s.Documents().Create(ctx, store.Document{
		Title:        titulo,
		Format:       "md",
		SourceName:   titulo + ".md",
		SourcePath:   "/data/" + titulo + ".md",
		SourceSHA256: fmt.Sprintf("sha-%s-%d", titulo, prioridade),
		Status:       status,
		Priority:     prioridade,
		CreatedAt:    quando,
		UpdatedAt:    quando,
	})
	if err != nil {
		t.Fatalf("criar documento: %v", err)
	}

	lista := make([]blocks.Block, 10)
	for i := range lista {
		texto := fmt.Sprintf("Parágrafo %d do documento %s, com algumas palavras para a leitura.", i, titulo)
		lista[i] = blocks.Block{
			Ord:     i,
			Kind:    blocks.KindParagraph,
			Text:    texto,
			HTML:    "<p>" + texto + "</p>",
			Words:   20,
			Seconds: 60,
			Hash:    fmt.Sprintf("h-%s-%d", titulo, i),
		}
	}
	if err := s.Blocks().Replace(ctx, docID, lista); err != nil {
		t.Fatalf("gravar blocos: %v", err)
	}

	params := plan.Params{
		Mode: plan.ModeMinutesPerDay, Minutes: 1, WPM: 200, Difficulty: 1,
		MinFill: 0.6, MaxOvershoot: 1.4,
	}
	pl, err := plan.Generate(lista, params)
	if err != nil {
		t.Fatalf("gerar plano: %v", err)
	}
	if _, err := s.Plans().Create(ctx, docID, pl, params, quando); err != nil {
		t.Fatalf("persistir plano: %v", err)
	}
	return docID
}

func chunksDoDocumento(t *testing.T, s *store.Store, docID int64) []store.Chunk {
	t.Helper()
	pl, err := s.Plans().Active(context.Background(), docID)
	if err != nil || pl == nil {
		t.Fatalf("plano ativo do documento %d: %+v, %v", docID, pl, err)
	}
	return pl.Chunks
}

func runDaData(t *testing.T, s *store.Store, data string) *store.DeliveryRun {
	t.Helper()
	run, err := s.Runs().GetByDate(context.Background(), data)
	if err != nil {
		t.Fatalf("buscar run %s: %v", data, err)
	}
	return run
}

func TestTickAntesDoHorario(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	criarDocumento(t, s, "Livro", 100, store.StatusReady)
	r, pub := novoRunner(t, s, nil)
	r.Clock = platform.NewFakeClock(time.Date(2026, 10, 26, 6, 59, 0, 0, quando.Location()))

	res, err := r.Tick(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if res.Skipped == "" {
		t.Error("tick deveria informar que nada rodou")
	}
	if len(res.Runs) != 0 || pub.total() != 0 {
		t.Errorf("res = %+v, mensagens = %d", res, pub.total())
	}
	if run := runDaData(t, s, "2026-10-26"); run != nil {
		t.Errorf("run criada antes do horário: %+v", run)
	}
}

func TestTickDiaNaoConfigurado(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	criarDocumento(t, s, "Livro", 100, store.StatusReady)
	r, pub := novoRunner(t, s, nil)
	domingo := time.Date(2026, 11, 1, 8, 0, 0, 0, quando.Location())

	res, err := r.Tick(context.Background(), domingo)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(res.Runs) != 0 || pub.total() != 0 {
		t.Errorf("res = %+v, mensagens = %d", res, pub.total())
	}
	if run := runDaData(t, s, "2026-11-01"); run != nil {
		t.Errorf("run criada em dia não configurado: %+v", run)
	}
}

func TestTickCompletaRun(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	definir(t, s, KeyNtfyPriority, "2")
	docID := criarDocumento(t, s, "Livro", 100, store.StatusReady)
	r, pub := novoRunner(t, s, nil)

	res, err := r.Tick(context.Background(), quando)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(res.Runs) != 1 {
		t.Fatalf("runs = %+v", res.Runs)
	}
	if res.Runs[0].Sent != 1 || res.Runs[0].Failed != 0 {
		t.Errorf("resultado = %+v", res.Runs[0])
	}
	if pub.total() != 1 {
		t.Fatalf("mensagens = %d", pub.total())
	}
	if pub.config.Topic != "leitura" || pub.config.Priority != 2 {
		t.Errorf("config do publisher = %+v", pub.config)
	}
	msg := pub.todas()[0]
	if len(msg.Chunks) != 1 {
		t.Errorf("chunks na mensagem = %v", msg.Chunks)
	}

	run := runDaData(t, s, "2026-10-26")
	if run == nil || run.Status != store.RunSent || run.Late || run.FinishedAt == nil {
		t.Fatalf("run = %+v", run)
	}
	itens, err := s.Runs().ListItems(context.Background(), run.ID)
	if err != nil || len(itens) != 1 || itens[0].DocumentID != docID ||
		itens[0].Status != store.ItemSent || itens[0].Bytes == nil || itens[0].SentAt == nil {
		t.Fatalf("itens = %+v, %v", itens, err)
	}
	chunks := chunksDoDocumento(t, s, docID)
	if chunks[0].Status != store.ChunkSent || chunks[0].SentAt == nil {
		t.Errorf("chunk = %+v", chunks[0])
	}
	if chunks[1].Status != store.ChunkPending {
		t.Errorf("segundo chunk deveria continuar pendente: %+v", chunks[1])
	}

	// Segundo tick no mesmo dia: nada novo.
	res, err = r.Tick(context.Background(), quando.Add(10*time.Minute))
	if err != nil {
		t.Fatalf("segundo tick: %v", err)
	}
	if len(res.Runs) != 0 || pub.total() != 1 {
		t.Errorf("segundo tick criou run: %+v, mensagens = %d", res, pub.total())
	}
}

func TestTickAgrupaDocumentos(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	primeiro := criarDocumento(t, s, "Primeiro", 10, store.StatusReady)
	segundo := criarDocumento(t, s, "Segundo", 20, store.StatusReady)
	r, pub := novoRunner(t, s, nil)

	if _, err := r.Tick(context.Background(), quando); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if pub.total() != 1 {
		t.Fatalf("mensagens = %d, quer 1 agrupada", pub.total())
	}
	if len(pub.todas()[0].Chunks) != 2 {
		t.Errorf("chunks = %v", pub.todas()[0].Chunks)
	}
	run := runDaData(t, s, "2026-10-26")
	itens, _ := s.Runs().ListItems(context.Background(), run.ID)
	if len(itens) != 2 || itens[0].DocumentID != primeiro || itens[1].DocumentID != segundo {
		t.Errorf("itens = %+v", itens)
	}
}

func TestTickCatchupCriaRunPerdida(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	criarDocumento(t, s, "Livro", 100, store.StatusReady)
	r, pub := novoRunner(t, s, nil)

	// Segunda 08:00; a sexta anterior (2026-10-23) não tem run.
	res, err := r.Tick(context.Background(), quando)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(res.Runs) != 2 {
		t.Fatalf("runs = %+v", res.Runs)
	}
	hoje := runDaData(t, s, "2026-10-26")
	sexta := runDaData(t, s, "2026-10-23")
	if hoje == nil || hoje.Late {
		t.Errorf("run de hoje = %+v", hoje)
	}
	if sexta == nil || !sexta.Late || sexta.Status != store.RunSent {
		t.Errorf("run de catch-up = %+v", sexta)
	}
	// Cada run entrega um chunk; a entrega perdida não repete o de hoje.
	if pub.total() != 2 {
		t.Errorf("mensagens = %d, quer 2", pub.total())
	}

	// Outro tick não repete nenhuma das duas.
	res, err = r.Tick(context.Background(), quando.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(res.Runs) != 0 || pub.total() != 2 {
		t.Errorf("tick repetiu runs: %+v, mensagens = %d", res, pub.total())
	}
}

func TestTickCatchupSoUmaVezNaJanela(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	definir(t, s, KeyCatchupDays, "5")
	criarDocumento(t, s, "Livro", 100, store.StatusReady)
	r, pub := novoRunner(t, s, nil)

	// Quarta depois de cinco dias úteis sem runs: só a run de terça (a mais
	// recente perdida) é recuperada, mesmo em ticks seguintes.
	quarta := time.Date(2026, 10, 28, 8, 0, 0, 0, quando.Location())
	res, err := r.Tick(context.Background(), quarta)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(res.Runs) != 2 {
		t.Fatalf("runs = %+v", res.Runs)
	}
	if run := runDaData(t, s, "2026-10-27"); run == nil || !run.Late {
		t.Errorf("run de terça = %+v", run)
	}
	if run := runDaData(t, s, "2026-10-26"); run != nil {
		t.Errorf("catch-up enfileirou a segunda: %+v", run)
	}

	res, err = r.Tick(context.Background(), quarta.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("segundo tick: %v", err)
	}
	if len(res.Runs) != 0 || pub.total() != 2 {
		t.Errorf("segundo tick = %+v, mensagens = %d", res.Runs, pub.total())
	}
	if run := runDaData(t, s, "2026-10-26"); run != nil {
		t.Errorf("catch-up repetiu dias antigos: %+v", run)
	}
}

func TestTickCatchupForaDaJanela(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	definir(t, s, KeyCatchupDays, "1")
	criarDocumento(t, s, "Livro", 100, store.StatusReady)
	r, _ := novoRunner(t, s, nil)

	// Quarta: a terça já tem run; a segunda está fora da janela de 1 dia.
	if _, _, err := s.Runs().Create(context.Background(), "2026-10-27", quando, quando, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Runs().FinishRun(context.Background(),
		runDaData(t, s, "2026-10-27").ID, store.RunSent, "", quando); err != nil {
		t.Fatal(err)
	}

	res, err := r.Tick(context.Background(), time.Date(2026, 10, 28, 8, 0, 0, 0, quando.Location()))
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(res.Runs) != 1 || res.Runs[0].RunDate != "2026-10-28" {
		t.Errorf("runs = %+v", res.Runs)
	}
	if run := runDaData(t, s, "2026-10-26"); run != nil {
		t.Errorf("catch-up fora da janela criou run: %+v", run)
	}
}

func TestTickDesligado(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	definir(t, s, KeyEnabled, "false")
	criarDocumento(t, s, "Livro", 100, store.StatusReady)
	r, pub := novoRunner(t, s, nil)

	res, err := r.Tick(context.Background(), quando)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if res.Skipped == "" || len(res.Runs) != 0 || pub.total() != 0 {
		t.Errorf("res = %+v, mensagens = %d", res, pub.total())
	}
	if run := runDaData(t, s, "2026-10-26"); run != nil {
		t.Errorf("run criada com agendador desligado: %+v", run)
	}
}

func TestTickSemNtfy(t *testing.T) {
	s := abrirStore(t)
	semCatchup(t, s)
	criarDocumento(t, s, "Livro", 100, store.StatusReady)
	r, pub := novoRunner(t, s, nil)

	res, err := r.Tick(context.Background(), quando)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if res.Skipped != "ntfy não configurado" {
		t.Errorf("skipped = %q", res.Skipped)
	}
	if len(res.Runs) != 0 || pub.total() != 0 {
		t.Errorf("res = %+v, mensagens = %d", res, pub.total())
	}
	if run := runDaData(t, s, "2026-10-26"); run != nil {
		t.Errorf("run criada sem ntfy: %+v", run)
	}
}

func TestTickPulaDocumentoPausado(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	pausado := criarDocumento(t, s, "Pausado", 10, store.StatusPaused)
	pronto := criarDocumento(t, s, "Pronto", 20, store.StatusReady)
	r, pub := novoRunner(t, s, nil)

	if _, err := r.Tick(context.Background(), quando); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if pub.total() != 1 {
		t.Fatalf("mensagens = %d", pub.total())
	}
	run := runDaData(t, s, "2026-10-26")
	itens, _ := s.Runs().ListItems(context.Background(), run.ID)
	if len(itens) != 1 || itens[0].DocumentID != pronto {
		t.Errorf("itens = %+v", itens)
	}
	if c := chunksDoDocumento(t, s, pausado)[0]; c.Status != store.ChunkPending {
		t.Errorf("chunk do pausado avançou: %+v", c)
	}
}

func TestTickLimitaDocumentosPorRun(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	definir(t, s, KeyMaxDocsPerRun, "1")
	primeiro := criarDocumento(t, s, "Primeiro", 10, store.StatusReady)
	segundo := criarDocumento(t, s, "Segundo", 20, store.StatusReady)
	r, pub := novoRunner(t, s, nil)

	if _, err := r.Tick(context.Background(), quando); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if pub.total() != 1 {
		t.Fatalf("mensagens = %d", pub.total())
	}
	run := runDaData(t, s, "2026-10-26")
	itens, _ := s.Runs().ListItems(context.Background(), run.ID)
	if len(itens) != 1 || itens[0].DocumentID != primeiro {
		t.Errorf("itens = %+v", itens)
	}
	if c := chunksDoDocumento(t, s, segundo)[0]; c.Status != store.ChunkPending {
		t.Errorf("segundo documento avançou: %+v", c)
	}
}

func TestTickFalhaPublicacaoNaoAvancaChunk(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	docID := criarDocumento(t, s, "Livro", 100, store.StatusReady)
	pub := &publicadorFake{err: errors.New("ntfy: resposta HTTP 401")}
	r, _ := novoRunner(t, s, pub)

	res, err := r.Tick(context.Background(), quando)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(res.Runs) != 1 || res.Runs[0].Failed != 1 || res.Runs[0].Sent != 0 {
		t.Fatalf("res = %+v", res.Runs)
	}
	run := runDaData(t, s, "2026-10-26")
	if run == nil || run.Status != store.RunFailed || run.Error == nil {
		t.Fatalf("run = %+v", run)
	}
	itens, _ := s.Runs().ListItems(context.Background(), run.ID)
	if len(itens) != 1 || itens[0].Status != store.ItemFailed ||
		itens[0].Error == nil || itens[0].Attempts != 1 {
		t.Errorf("itens = %+v", itens)
	}
	if c := chunksDoDocumento(t, s, docID)[0]; c.Status != store.ChunkPending {
		t.Errorf("chunk avançou em falha: %+v", c)
	}
}

func TestTickRetomaRunInterrompida(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	enviado := criarDocumento(t, s, "Já enviado", 100, store.StatusReady)
	pendente := criarDocumento(t, s, "Pendente", 200, store.StatusReady)
	chunksEnviado := chunksDoDocumento(t, s, enviado)
	chunksPendente := chunksDoDocumento(t, s, pendente)
	ctx := context.Background()

	run, _, err := s.Runs().Create(ctx, "2026-10-26", quando, quando, false)
	if err != nil {
		t.Fatal(err)
	}
	primeiro, err := s.Runs().AddItem(ctx, run.ID, enviado, chunksEnviado[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Runs().MarkItemSent(ctx, primeiro.ID, chunksEnviado[0].ID, 100, 1, quando); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Runs().AddItem(ctx, run.ID, pendente, chunksPendente[0].ID); err != nil {
		t.Fatal(err)
	}

	r, pub := novoRunner(t, s, nil)
	res, err := r.Tick(context.Background(), quando.Add(time.Hour))
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(res.Runs) != 1 || res.Runs[0].RunID != run.ID {
		t.Fatalf("res = %+v", res.Runs)
	}
	if pub.total() != 1 {
		t.Fatalf("mensagens = %d, quer só o item pendente", pub.total())
	}
	if got := pub.todas()[0].Chunks; len(got) != 1 || got[0] != chunksPendente[0].ID {
		t.Errorf("chunks reenviados = %v", got)
	}
	if c := chunksDoDocumento(t, s, pendente)[0]; c.Status != store.ChunkSent {
		t.Errorf("chunk pendente = %+v", c)
	}
	fechada := runDaData(t, s, "2026-10-26")
	if fechada == nil || fechada.Status != store.RunSent || fechada.FinishedAt == nil {
		t.Errorf("run = %+v", fechada)
	}
}

func TestTickRetomaPulaChunkJaEnviado(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	docID := criarDocumento(t, s, "Livro", 100, store.StatusReady)
	chunks := chunksDoDocumento(t, s, docID)
	ctx := context.Background()

	run, _, err := s.Runs().Create(ctx, "2026-10-26", quando, quando, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Runs().AddItem(ctx, run.ID, docID, chunks[0].ID); err != nil {
		t.Fatal(err)
	}
	// O chunk foi entregue por outro caminho (run manual) entre a criação do
	// item e a retomada.
	if err := s.Runs().MarkItemSent(ctx, 9999, chunks[0].ID, 1, 1, quando); err == nil {
		// MarkItemSent com item inexistente deve falhar; o chunk é marcado
		// direto para simular a corrida.
		t.Fatal("item inexistente deveria falhar")
	}
	if _, err := s.DB().ExecContext(ctx,
		`UPDATE chunks SET status = 'sent', sent_at = ? WHERE id = ?`,
		quando.UTC().Format(time.RFC3339), chunks[0].ID); err != nil {
		t.Fatal(err)
	}

	r, pub := novoRunner(t, s, nil)
	res, err := r.Tick(context.Background(), quando.Add(time.Hour))
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(res.Runs) != 1 || res.Runs[0].Skipped != 1 || pub.total() != 0 {
		t.Fatalf("res = %+v, mensagens = %d", res.Runs, pub.total())
	}
	itens, _ := s.Runs().ListItems(ctx, run.ID)
	if len(itens) != 1 || itens[0].Status != store.ItemSkipped {
		t.Errorf("itens = %+v", itens)
	}
	if fechada := runDaData(t, s, "2026-10-26"); fechada.Status != store.RunSent {
		t.Errorf("run = %+v", fechada)
	}
}

func TestTickConfiguracaoInvalida(t *testing.T) {
	s := abrirStore(t)
	definir(t, s, KeyTime, "25:00")
	r, _ := novoRunner(t, s, nil)
	if _, err := r.Tick(context.Background(), quando); err == nil {
		t.Fatal("esperava erro de configuração")
	}
}

func TestTickSemStore(t *testing.T) {
	r := &Runner{}
	if _, err := r.Tick(context.Background(), quando); err == nil {
		t.Fatal("esperava erro sem store")
	}
}

func TestRunnerStartStop(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	criarDocumento(t, s, "Livro", 100, store.StatusReady)
	r, pub := novoRunner(t, s, nil)
	r.Interval = 5 * time.Millisecond

	r.Start(context.Background())
	// O primeiro tick é imediato; espera a run terminar (não só publicar, para
	// que Stop não interrompa a finalização).
	limite := time.Now().Add(2 * time.Second)
	for time.Now().Before(limite) {
		run := runDaData(t, s, "2026-10-26")
		if run != nil && run.Status != store.RunRunning {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	r.Stop()

	if pub.total() == 0 {
		t.Fatal("ticker não executou o primeiro tick")
	}
	if run := runDaData(t, s, "2026-10-26"); run == nil || run.Status != store.RunSent {
		t.Errorf("run = %+v", run)
	}

	// Stop é idempotente.
	r.Stop()
}
