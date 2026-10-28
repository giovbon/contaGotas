package schedule

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/giovani/readingdrip/internal/blocks"
	"github.com/giovani/readingdrip/internal/plan"
	"github.com/giovani/readingdrip/internal/platform"
	"github.com/giovani/readingdrip/internal/store"
)

func TestTruncarErro(t *testing.T) {
	if got := truncarErro("  curto  "); got != "curto" {
		t.Errorf("curto = %q", got)
	}
	longo := strings.Repeat("a", maxErroRun+20)
	if got := truncarErro(longo); len(got) > maxErroRun+len("…") || !strings.HasSuffix(got, "…") {
		t.Errorf("longo = %d bytes", len(got))
	}
	// Corte no meio de uma rune multibyte não pode quebrar UTF-8.
	multibyte := strings.Repeat("á", maxErroRun)
	if got := truncarErro(multibyte); !strings.HasSuffix(got, "…") || !strings.HasPrefix(got, "á") {
		t.Errorf("multibyte = %q", got[:20])
	}
}

func TestDormir(t *testing.T) {
	r := &Runner{}
	if err := r.dormir(context.Background(), 0); err != nil {
		t.Errorf("zero = %v", err)
	}
	if err := r.dormir(context.Background(), -time.Second); err != nil {
		t.Errorf("negativo = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.dormir(ctx, time.Minute); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelado = %v", err)
	}
	if err := r.dormir(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelado/zero = %v", err)
	}

	if err := r.dormir(context.Background(), time.Millisecond); err != nil {
		t.Errorf("espera = %v", err)
	}
	chamou := false
	r.Sleep = func(_ context.Context, d time.Duration) error {
		chamou = d == time.Second
		return nil
	}
	if err := r.dormir(context.Background(), time.Second); err != nil || !chamou {
		t.Errorf("Sleep injetado = %v, chamou = %v", err, chamou)
	}
}

func TestFecharComErro(t *testing.T) {
	s := abrirStore(t)
	ctx := context.Background()
	r, _ := novoRunner(t, s, nil)

	r.fecharComErro(ctx, nil, errors.New("ignorado"), quando)
	r.fecharComErro(ctx, &store.DeliveryRun{ID: 1}, nil, quando)

	run, _, err := s.Runs().Create(ctx, "2026-10-26", quando, quando, false)
	if err != nil {
		t.Fatal(err)
	}
	docID := criarDocumento(t, s, "Livro", 100, store.StatusReady)
	chunk := chunksDoDocumento(t, s, docID)[0]
	item, err := s.Runs().AddItem(ctx, run.ID, docID, chunk.ID)
	if err != nil {
		t.Fatal(err)
	}
	r.fecharComErro(ctx, run, errors.New("falha grave"), quando)
	fechada := runDaData(t, s, "2026-10-26")
	if fechada.Status != store.RunFailed || fechada.Error == nil || *fechada.Error != "falha grave" {
		t.Errorf("run = %+v", fechada)
	}
	itens, err := s.Runs().ListItems(ctx, run.ID)
	if err != nil || len(itens) != 1 || itens[0].ID != item.ID || itens[0].Status != store.ItemFailed {
		t.Errorf("itens = %+v, %v", itens, err)
	}
	if c, _ := s.Plans().Chunk(ctx, chunk.ID); c.Status != store.ChunkPending {
		t.Errorf("chunk avançou ao fechar run com erro: %+v", c)
	}
	eventos, err := s.Events().List(ctx, 10)
	if err != nil || len(eventos) == 0 || eventos[0].Kind != "delivery" || eventos[0].Level != store.EventError {
		t.Errorf("eventos = %+v, %v", eventos, err)
	}

	// Banco fechado: não dá para listar itens, fechar a run nem registrar o
	// evento; o runner só loga e não entra em pânico.
	if err := s.Close(); err != nil {
		t.Fatalf("fechar banco: %v", err)
	}
	r.fecharComErro(ctx, run, errors.New("sem banco"), quando)
}

func TestFinalizarMarcaPendente(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	docID := criarDocumento(t, s, "Livro", 100, store.StatusReady)
	ctx := context.Background()

	run, _, err := s.Runs().Create(ctx, "2026-10-26", quando, quando, false)
	if err != nil {
		t.Fatal(err)
	}
	chunk := chunksDoDocumento(t, s, docID)[0]
	if _, err := s.Runs().AddItem(ctx, run.ID, docID, chunk.ID); err != nil {
		t.Fatal(err)
	}

	r, _ := novoRunner(t, s, nil)
	res, err := r.finalizar(ctx, run, quando)
	if err != nil {
		t.Fatalf("finalizar: %v", err)
	}
	if res.Failed != 1 || res.Sent != 0 {
		t.Errorf("res = %+v", res)
	}
	itens, _ := s.Runs().ListItems(ctx, run.ID)
	if len(itens) != 1 || itens[0].Status != store.ItemFailed {
		t.Errorf("itens = %+v", itens)
	}
	if fechada := runDaData(t, s, "2026-10-26"); fechada.Status != store.RunFailed {
		t.Errorf("run = %+v", fechada)
	}
}

func TestExecutarSemDocumentos(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	r, pub := novoRunner(t, s, nil)

	res, err := r.Tick(context.Background(), quando)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(res.Runs) != 1 || pub.total() != 0 {
		t.Fatalf("res = %+v, mensagens = %d", res.Runs, pub.total())
	}
	if run := runDaData(t, s, "2026-10-26"); run == nil || run.Status != store.RunSent {
		t.Errorf("run vazia = %+v", run)
	}
}

func TestEnviarDivideMensagens(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	definir(t, s, KeyNtfyMaxBytes, "1000")
	criarDocumentoComTexto(t, s, "Grande 1", 10, store.StatusReady, strings.Repeat("palavra ", 120))
	criarDocumentoComTexto(t, s, "Grande 2", 20, store.StatusReady, strings.Repeat("palavra ", 120))
	r, pub := novoRunner(t, s, nil)

	esperas := 0
	r.Sleep = func(context.Context, time.Duration) error {
		esperas++
		return nil
	}
	if _, err := r.Tick(context.Background(), quando); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if pub.total() != 2 {
		t.Fatalf("mensagens = %d, quer split em 2", pub.total())
	}
	if esperas != 1 {
		t.Errorf("pausa entre mensagens = %d", esperas)
	}
	for _, msg := range pub.todas() {
		if len(msg.Body) > 1000 {
			t.Errorf("mensagem acima do limite: %d bytes", len(msg.Body))
		}
		if len(msg.Chunks) != 1 {
			t.Errorf("mensagem deveria ter um chunk: %v", msg.Chunks)
		}
	}
}

func TestPublisherSemFabrica(t *testing.T) {
	r := &Runner{}
	if _, err := r.publisher(NtfyConfig{}); !errors.Is(err, ErrNtfyNaoConfigurado) {
		t.Errorf("vazio = %v", err)
	}
	if _, err := r.publisher(NtfyConfig{BaseURL: "http://ntfy:8080"}); !errors.Is(err, ErrNtfyNaoConfigurado) {
		t.Errorf("sem topic = %v", err)
	}
	pub, err := r.publisher(NtfyConfig{BaseURL: "http://ntfy:8080", Topic: "leitura"})
	if err != nil || pub == nil {
		t.Errorf("cliente = %v, %v", pub, err)
	}
	if _, err := r.publisher(NtfyConfig{BaseURL: "ftp://ntfy", Topic: "leitura"}); err == nil {
		t.Error("esperava erro de URL inválida")
	}
}

func TestTickEClaro(t *testing.T) {
	s := abrirStore(t)
	r, _ := novoRunner(t, s, nil)
	r.Clock = platform.NewFakeClock(quando)

	definir(t, s, KeyTime, "25:00")
	r.tickEClaro(context.Background()) // erro de configuração: loga e segue

	definir(t, s, KeyTime, "07:00")
	semCatchup(t, s)
	r.tickEClaro(context.Background()) // sem ntfy: debug e segue

	publicarConfigurado(t, s)
	criarDocumento(t, s, "Livro", 100, store.StatusReady)
	r.tickEClaro(context.Background()) // run concluída: info
	if run := runDaData(t, s, "2026-10-26"); run == nil || run.Status != store.RunSent {
		t.Errorf("run = %+v", run)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.tickEClaro(ctx) // contexto cancelado: não loga erro
}

func TestDoseComIntervaloInvalido(t *testing.T) {
	s := abrirStore(t)
	docID := criarDocumento(t, s, "Livro", 100, store.StatusReady)
	chunk := chunksDoDocumento(t, s, docID)[0]
	if _, err := s.DB().ExecContext(context.Background(),
		`UPDATE chunks SET first_block_id = (SELECT MAX(id) FROM blocks WHERE document_id = ?),
		                   last_block_id  = (SELECT MIN(id) FROM blocks WHERE document_id = ?)
		 WHERE id = ?`, docID, docID, chunk.ID); err != nil {
		t.Fatal(err)
	}
	corrompido, err := s.Plans().Chunk(context.Background(), chunk.ID)
	if err != nil || corrompido == nil {
		t.Fatalf("chunk = %+v, %v", corrompido, err)
	}
	doc, _ := s.Documents().Get(context.Background(), docID)
	r, _ := novoRunner(t, s, nil)
	st, err := LoadSettings(context.Background(), s.Settings())
	if err != nil {
		t.Fatalf("configurações: %v", err)
	}
	if _, err := r.dose(context.Background(), st, doc, corrompido); err == nil {
		t.Fatal("esperava erro de intervalo de blocos")
	}
}

func TestRetomadaComDoseInvalida(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	docID := criarDocumento(t, s, "Livro", 100, store.StatusReady)
	ctx := context.Background()
	chunk := chunksDoDocumento(t, s, docID)[0]

	run, _, err := s.Runs().Create(ctx, "2026-10-26", quando, quando, false)
	if err != nil {
		t.Fatal(err)
	}
	item, err := s.Runs().AddItem(ctx, run.ID, docID, chunk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().ExecContext(ctx,
		`UPDATE chunks SET first_block_id = (SELECT MAX(id) FROM blocks WHERE document_id = ?),
		                   last_block_id  = (SELECT MIN(id) FROM blocks WHERE document_id = ?)
		 WHERE id = ?`, docID, docID, chunk.ID); err != nil {
		t.Fatal(err)
	}

	r, _ := novoRunner(t, s, nil)
	res, err := r.Tick(ctx, quando.Add(time.Hour))
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(res.Runs) != 1 {
		t.Fatalf("res = %+v", res)
	}
	itens, _ := s.Runs().ListItems(ctx, run.ID)
	if len(itens) != 1 || itens[0].ID != item.ID || itens[0].Status != store.ItemFailed {
		t.Errorf("itens = %+v", itens)
	}
	if c, _ := s.Plans().Chunk(ctx, chunk.ID); c.Status != store.ChunkPending {
		t.Errorf("chunk = %+v", c)
	}
}

func TestTickComLockOcupado(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	r, _ := novoRunner(t, s, nil)
	r.runMu.Lock()
	defer r.runMu.Unlock()

	res, err := r.Tick(context.Background(), quando)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if res.Skipped != "tick anterior em andamento" {
		t.Errorf("skipped = %q", res.Skipped)
	}
}

func TestStartDuasVezes(t *testing.T) {
	s := abrirStore(t)
	semCatchup(t, s)
	r, _ := novoRunner(t, s, nil)
	r.Interval = time.Hour
	r.Start(nil)
	r.Start(context.Background()) // já iniciado: ignora
	r.Stop()
	r.Stop() // já parado: ignora
}

func TestClockLoggerPadroes(t *testing.T) {
	r := &Runner{}
	if r.clock() == nil {
		t.Error("clock padrão nulo")
	}
	if r.logger() == nil {
		t.Error("logger padrão nulo")
	}
	_ = r.clock().Now()
}

func TestEventoInvalidoNaoQuebra(t *testing.T) {
	s := abrirStore(t)
	r, _ := novoRunner(t, s, nil)
	run, _, err := s.Runs().Create(context.Background(), "2026-10-26", quando, quando, false)
	if err != nil {
		t.Fatal(err)
	}
	// Evento sem data: o store recusa e o runner só registra o aviso.
	r.evento(context.Background(), run, store.EventInfo, "sem data", time.Time{})
	r.evento(context.Background(), nil, store.EventInfo, "sem run", quando)
}

func TestDoseComVizinhosEAutor(t *testing.T) {
	s := abrirStore(t)
	docID := criarDocumento(t, s, "Livro", 100, store.StatusReady)
	ctx := context.Background()
	if _, err := s.DB().ExecContext(ctx,
		`UPDATE documents SET author = 'Autora' WHERE id = ?`, docID); err != nil {
		t.Fatal(err)
	}
	chunks := chunksDoDocumento(t, s, docID)
	if len(chunks) < 2 {
		t.Fatalf("plano com %d chunk(s), quer pelo menos 2", len(chunks))
	}
	doc, err := s.Documents().Get(ctx, docID)
	if err != nil || doc == nil {
		t.Fatalf("documento = %+v, %v", doc, err)
	}

	r, _ := novoRunner(t, s, nil)
	var leitorTTL, lidoTTL time.Duration
	r.Links = func(_ store.Document, _ store.Chunk, ttl, done time.Duration) (string, string) {
		leitorTTL, lidoTTL = ttl, done
		return "https://leitura.exemplo.com/r/1", "https://leitura.exemplo.com/done/1"
	}
	st, err := LoadSettings(ctx, s.Settings())
	if err != nil {
		t.Fatalf("configurações: %v", err)
	}
	dose, err := r.dose(ctx, st, doc, &chunks[1])
	if err != nil {
		t.Fatalf("dose: %v", err)
	}
	if leitorTTL != 180*24*time.Hour || lidoTTL != 30*24*time.Hour {
		t.Errorf("validades = %v, %v", leitorTTL, lidoTTL)
	}
	if dose.Author != "Autora" {
		t.Errorf("autor = %q", dose.Author)
	}
	if len(dose.Previous) == 0 {
		t.Error("dose sem recap do chunk anterior")
	}
	if dose.Link == "" || dose.DoneLink == "" {
		t.Errorf("links = %q, %q", dose.Link, dose.DoneLink)
	}
	if dose.Percent < 0 {
		t.Errorf("percentual = %d", dose.Percent)
	}
}

func TestTickPulaDocumentoComDoseInvalida(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	docID := criarDocumento(t, s, "Livro", 100, store.StatusReady)
	chunk := chunksDoDocumento(t, s, docID)[0]
	if _, err := s.DB().ExecContext(context.Background(),
		`UPDATE chunks SET first_block_id = (SELECT MAX(id) FROM blocks WHERE document_id = ?),
		                   last_block_id  = (SELECT MIN(id) FROM blocks WHERE document_id = ?)
		 WHERE id = ?`, docID, docID, chunk.ID); err != nil {
		t.Fatal(err)
	}

	r, pub := novoRunner(t, s, nil)
	res, err := r.Tick(context.Background(), quando)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if pub.total() != 0 || len(res.Runs) != 1 {
		t.Fatalf("res = %+v, mensagens = %d", res.Runs, pub.total())
	}
	if run := runDaData(t, s, "2026-10-26"); run == nil || run.Status != store.RunSent {
		t.Errorf("run = %+v", run)
	}
	if c, _ := s.Plans().Chunk(context.Background(), chunk.ID); c.Status != store.ChunkPending {
		t.Errorf("chunk = %+v", c)
	}
}

func TestEnviarMarcaSkippedSeChunkJaSaiu(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	docID := criarDocumento(t, s, "Livro", 100, store.StatusReady)
	chunk := chunksDoDocumento(t, s, docID)[0]

	pub := &publicadorFake{hook: func() {
		// Outro caminho entregou o chunk entre a composição e o commit.
		if _, err := s.DB().ExecContext(context.Background(),
			`UPDATE chunks SET status = 'sent', sent_at = ? WHERE id = ?`,
			quando.UTC().Format(time.RFC3339), chunk.ID); err != nil {
			t.Errorf("marcar chunk: %v", err)
		}
	}}
	r, _ := novoRunner(t, s, pub)

	res, err := r.Tick(context.Background(), quando)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(res.Runs) != 1 || res.Runs[0].Skipped != 1 || res.Runs[0].Sent != 0 {
		t.Fatalf("res = %+v", res.Runs)
	}
	itens, _ := s.Runs().ListItems(context.Background(), runDaData(t, s, "2026-10-26").ID)
	if len(itens) != 1 || itens[0].Status != store.ItemSkipped {
		t.Errorf("itens = %+v", itens)
	}
	if run := runDaData(t, s, "2026-10-26"); run.Status != store.RunSent {
		t.Errorf("run = %+v", run)
	}
}

// criarDocumentoComTexto varia o tamanho do parágrafo para forçar split de
// mensagens no teste de envio.
func criarDocumentoComTexto(t *testing.T, s *store.Store, titulo string, prioridade int, status store.DocumentStatus, texto string) int64 {
	t.Helper()
	ctx := context.Background()
	docID, err := s.Documents().Create(ctx, store.Document{
		Title:        titulo,
		Format:       "md",
		SourceName:   titulo + ".md",
		SourcePath:   "/data/" + titulo + ".md",
		SourceSHA256: "sha-" + titulo,
		Status:       status,
		Priority:     prioridade,
		CreatedAt:    quando,
		UpdatedAt:    quando,
	})
	if err != nil {
		t.Fatalf("criar documento: %v", err)
	}
	lista := make([]blocks.Block, 6)
	for i := range lista {
		lista[i] = blocks.Block{
			Ord:     i,
			Kind:    blocks.KindParagraph,
			Text:    texto,
			HTML:    "<p>" + texto + "</p>",
			Words:   120,
			Seconds: 60,
			Hash:    titulo + "-" + string(rune('a'+i)),
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
