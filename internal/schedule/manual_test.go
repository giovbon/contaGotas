package schedule

import (
	"context"
	"errors"
	"testing"

	"github.com/giovani/readingdrip/internal/store"
)

func TestSendNowCriaRunDoDia(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	docID := criarDocumento(t, s, "Envio manual", 10, store.StatusReady)
	r, pub := novoRunner(t, s, nil)
	ctx := context.Background()

	res, err := r.SendNow(ctx, docID)
	if err != nil {
		t.Fatalf("enviar agora: %v", err)
	}
	if !res.Sent || res.ChunkID == 0 || res.RunID == 0 || res.Bytes == 0 {
		t.Fatalf("resultado = %+v", res)
	}
	if res.RunDate != "2026-10-26" {
		t.Errorf("run_date = %q", res.RunDate)
	}
	if pub.total() != 1 {
		t.Fatalf("mensagens = %d, quer 1", pub.total())
	}

	run := runDaData(t, s, res.RunDate)
	if run == nil {
		t.Fatal("run do dia não foi criada")
	}
	if run.Status != store.RunSent || run.Late || run.FinishedAt == nil {
		t.Errorf("run = %+v", run)
	}
	itens, err := s.Runs().ListItems(ctx, run.ID)
	if err != nil || len(itens) != 1 {
		t.Fatalf("itens = %+v, %v", itens, err)
	}
	if itens[0].Status != store.ItemSent || itens[0].SentAt == nil {
		t.Errorf("item = %+v", itens[0])
	}
	chunk, err := s.Plans().Chunk(ctx, res.ChunkID)
	if err != nil || chunk == nil || chunk.Status != store.ChunkSent {
		t.Errorf("chunk = %+v, %v", chunk, err)
	}

	eventos, err := s.Events().List(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !temEvento(eventos, "delivery.manual") {
		t.Errorf("evento delivery.manual ausente: %+v", eventos)
	}

	// A run do dia já existe: um tick depois do horário não entrega de novo.
	if _, err := r.Tick(ctx, quando.Add(2*60*60*1e9)); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if pub.total() != 1 {
		t.Errorf("tick reenviou: mensagens = %d", pub.total())
	}
}

func TestSendNowRunFechadaNaoReenvia(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	docID := criarDocumento(t, s, "Duas vezes", 10, store.StatusReady)
	r, pub := novoRunner(t, s, nil)
	ctx := context.Background()

	if _, err := r.SendNow(ctx, docID); err != nil {
		t.Fatalf("primeiro envio: %v", err)
	}
	if _, err := r.SendNow(ctx, docID); !errors.Is(err, ErrJaEnviadoHoje) {
		t.Fatalf("segundo envio = %v, quer ErrJaEnviadoHoje", err)
	}
	if pub.total() != 1 {
		t.Errorf("mensagens = %d, quer 1", pub.total())
	}
}

func TestSendNowSemTrechosNaoCriaRun(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	docID := criarDocumento(t, s, "Sem trechos", 10, store.StatusReady)
	r, _ := novoRunner(t, s, nil)
	ctx := context.Background()

	for _, c := range chunksDoDocumento(t, s, docID) {
		if err := s.Plans().MarkSent(ctx, c.ID, quando); err != nil {
			t.Fatalf("marcar chunk: %v", err)
		}
	}

	if _, err := r.SendNow(ctx, docID); !errors.Is(err, ErrSemTrechos) {
		t.Fatalf("erro = %v, quer ErrSemTrechos", err)
	}
	if run := runDaData(t, s, "2026-10-26"); run != nil {
		t.Errorf("run não deveria ser criada: %+v", run)
	}
}

func TestSendNowFalhaRemoveRunCriada(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	docID := criarDocumento(t, s, "Falha manual", 10, store.StatusReady)
	r, pub := novoRunner(t, s, nil)
	pub.err = errors.New("ntfy: resposta HTTP 401")
	ctx := context.Background()

	if _, err := r.SendNow(ctx, docID); err == nil {
		t.Fatal("esperado erro de publicação")
	}
	if run := runDaData(t, s, "2026-10-26"); run != nil {
		t.Errorf("run que falhou antes de qualquer envio deve ser descartada: %+v", run)
	}
	chunk, err := s.Plans().NextPending(ctx, docID)
	if err != nil || chunk == nil {
		t.Fatalf("cursor deveria continuar no primeiro chunk: %+v, %v", chunk, err)
	}
	if emAndamento, err := s.Runs().Running(ctx); err != nil || emAndamento != nil {
		t.Errorf("run em andamento = %+v, %v", emAndamento, err)
	}

	eventos, err := s.Events().List(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !temEvento(eventos, "delivery.manual") {
		t.Errorf("falha deveria registrar delivery.manual: %+v", eventos)
	}

	// Sem resíduo: a próxima tentativa (com o ntfy de volta) funciona.
	pub.err = nil
	if _, err := r.SendNow(ctx, docID); err != nil {
		t.Fatalf("nova tentativa: %v", err)
	}
	if run := runDaData(t, s, "2026-10-26"); run == nil || run.Status != store.RunSent {
		t.Errorf("run depois da nova tentativa = %+v", run)
	}
}

func TestSendNowSemNtfy(t *testing.T) {
	s := abrirStore(t)
	docID := criarDocumento(t, s, "Sem ntfy", 10, store.StatusReady)
	r, _ := novoRunner(t, s, nil)

	if _, err := r.SendNow(context.Background(), docID); !errors.Is(err, ErrNtfyNaoConfigurado) {
		t.Fatalf("erro = %v, quer ErrNtfyNaoConfigurado", err)
	}
	if run := runDaData(t, s, "2026-10-26"); run != nil {
		t.Errorf("sem ntfy não cria run: %+v", run)
	}
}

func TestSendNowDocumentosInvalidos(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	r, _ := novoRunner(t, s, nil)
	ctx := context.Background()

	if _, err := r.SendNow(ctx, 9999); !errors.Is(err, ErrDocumentoNaoEncontrado) {
		t.Errorf("id inexistente = %v", err)
	}
	if _, err := r.SendNow(ctx, 0); !errors.Is(err, ErrDocumentoNaoEncontrado) {
		t.Errorf("id zero = %v", err)
	}

	processando := criarDocumento(t, s, "Processando", 10, store.StatusProcessing)
	if _, err := r.SendNow(ctx, processando); !errors.Is(err, ErrDocumentoNaoPronto) {
		t.Errorf("documento em processing = %v", err)
	}

	var semStore *Runner
	if _, err := semStore.SendNow(ctx, 1); err == nil {
		t.Error("runner sem store deveria falhar")
	}
}

func TestSendNowReaproveitaItemFalho(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	docID := criarDocumento(t, s, "Item falho", 10, store.StatusReady)
	ctx := context.Background()

	run, _, err := s.Runs().Create(ctx, "2026-10-26", quando, quando, false)
	if err != nil {
		t.Fatal(err)
	}
	chunk, err := s.Plans().NextPending(ctx, docID)
	if err != nil || chunk == nil {
		t.Fatalf("cursor: %+v, %v", chunk, err)
	}
	item, err := s.Runs().AddItem(ctx, run.ID, docID, chunk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Runs().MarkItemFailed(ctx, item.ID, 3, 0, "timeout"); err != nil {
		t.Fatal(err)
	}

	r, pub := novoRunner(t, s, nil)
	res, err := r.SendNow(ctx, docID)
	if err != nil {
		t.Fatalf("reenviar item falho: %v", err)
	}
	if !res.Sent || res.ChunkID != chunk.ID {
		t.Errorf("resultado = %+v", res)
	}
	if pub.total() != 1 {
		t.Errorf("mensagens = %d", pub.total())
	}
	itens, err := s.Runs().ListItems(ctx, run.ID)
	if err != nil || len(itens) != 1 {
		t.Fatalf("itens = %+v, %v", itens, err)
	}
	if itens[0].Status != store.ItemSent || itens[0].ID != item.ID {
		t.Errorf("item = %+v", itens[0])
	}
	// A run já existia (criada fora do envio manual): quem finaliza é o tick.
	if emAndamento, err := s.Runs().Running(ctx); err != nil || emAndamento == nil {
		t.Errorf("run deveria seguir running para a retomada: %+v, %v", emAndamento, err)
	}
}

func TestSendNowComLockOcupado(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	docID := criarDocumento(t, s, "Concorrente", 10, store.StatusReady)
	r, pub := novoRunner(t, s, nil)

	r.runMu.Lock()
	defer r.runMu.Unlock()

	if _, err := r.SendNow(context.Background(), docID); !errors.Is(err, ErrEnvioEmAndamento) {
		t.Fatalf("erro = %v, quer ErrEnvioEmAndamento", err)
	}
	if pub.total() != 0 {
		t.Errorf("nada deveria ser enviado: %d", pub.total())
	}
}

func TestSendTest(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	r, pub := novoRunner(t, s, nil)

	if _, err := r.SendTest(context.Background()); err != nil {
		t.Fatalf("enviar teste: %v", err)
	}
	if pub.total() != 1 {
		t.Fatalf("mensagens = %d, quer 1", pub.total())
	}
	msg := pub.todas()[0]
	if msg.Title == "" || msg.Body == "" {
		t.Errorf("mensagem = %+v", msg)
	}
	if len(msg.Chunks) != 0 {
		t.Errorf("teste não deveria ter chunks: %v", msg.Chunks)
	}

	pub.err = errors.New("ntfy: resposta HTTP 500")
	if _, err := r.SendTest(context.Background()); err == nil {
		t.Error("esperado erro de publicação no teste")
	}

	var semStore *Runner
	if _, err := semStore.SendTest(context.Background()); err == nil {
		t.Error("runner sem store deveria falhar")
	}
}

func TestSendTestSemNtfy(t *testing.T) {
	s := abrirStore(t)
	r, _ := novoRunner(t, s, nil)
	if _, err := r.SendTest(context.Background()); !errors.Is(err, ErrNtfyNaoConfigurado) {
		t.Fatalf("erro = %v, quer ErrNtfyNaoConfigurado", err)
	}
}

func temEvento(eventos []store.Event, kind string) bool {
	for _, e := range eventos {
		if e.Kind == kind {
			return true
		}
	}
	return false
}

func TestSendNowItemPuladoNaoReenvia(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	docID := criarDocumento(t, s, "Item pulado", 10, store.StatusReady)
	ctx := context.Background()

	run, _, err := s.Runs().Create(ctx, "2026-10-26", quando, quando, false)
	if err != nil {
		t.Fatal(err)
	}
	chunk, err := s.Plans().NextPending(ctx, docID)
	if err != nil || chunk == nil {
		t.Fatalf("cursor: %+v, %v", chunk, err)
	}
	item, err := s.Runs().AddItem(ctx, run.ID, docID, chunk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Runs().MarkItemSkipped(ctx, item.ID, "chunk já saiu"); err != nil {
		t.Fatal(err)
	}

	r, pub := novoRunner(t, s, nil)
	if _, err := r.SendNow(ctx, docID); !errors.Is(err, ErrJaEnviadoHoje) {
		t.Fatalf("erro = %v, quer ErrJaEnviadoHoje", err)
	}
	if pub.total() != 0 {
		t.Errorf("nada deveria ser enviado: %d", pub.total())
	}
}

func TestSendNowItemComChunkNaoPendente(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	docID := criarDocumento(t, s, "Item antigo", 10, store.StatusReady)
	ctx := context.Background()

	run, _, err := s.Runs().Create(ctx, "2026-10-26", quando, quando, false)
	if err != nil {
		t.Fatal(err)
	}
	chunks := chunksDoDocumento(t, s, docID)
	if len(chunks) < 2 {
		t.Fatalf("documento precisa de 2+ chunks, tem %d", len(chunks))
	}
	item, err := s.Runs().AddItem(ctx, run.ID, docID, chunks[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Plans().MarkSent(ctx, chunks[0].ID, quando); err != nil {
		t.Fatal(err)
	}

	r, pub := novoRunner(t, s, nil)
	if _, err := r.SendNow(ctx, docID); !errors.Is(err, ErrJaEnviadoHoje) {
		t.Fatalf("erro = %v, quer ErrJaEnviadoHoje", err)
	}
	if pub.total() != 0 {
		t.Errorf("nada deveria ser enviado: %d", pub.total())
	}
	if itens, _ := s.Runs().ListItems(ctx, run.ID); len(itens) != 1 || itens[0].ID != item.ID {
		t.Errorf("itens = %+v", itens)
	}
}

func TestSendNowChunkSaiuDuranteOEnvio(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	docID := criarDocumento(t, s, "Corrida", 10, store.StatusReady)
	ctx := context.Background()

	r, pub := novoRunner(t, s, nil)
	// Simula outra entrega marcando o chunk entre o compose e a gravação.
	pub.hook = func() {
		chunk, err := s.Plans().NextPending(ctx, docID)
		if err != nil || chunk == nil {
			t.Errorf("cursor no hook: %+v, %v", chunk, err)
			return
		}
		if err := s.Plans().MarkSent(ctx, chunk.ID, quando); err != nil {
			t.Errorf("marcar chunk no hook: %v", err)
		}
	}

	if _, err := r.SendNow(ctx, docID); !errors.Is(err, ErrJaEnviadoHoje) {
		t.Fatalf("erro = %v, quer ErrJaEnviadoHoje", err)
	}
}

func TestSendNowDoseInvalidaDescartaRun(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	docID := criarDocumento(t, s, "Dose inválida", 10, store.StatusReady)
	ctx := context.Background()

	// Aponta o primeiro chunk para blocos de outro documento (FK válida,
	// intervalo inválido para Blocks.Between), simulando um plano corrompido.
	chunk, err := s.Plans().NextPending(ctx, docID)
	if err != nil || chunk == nil {
		t.Fatalf("cursor: %+v, %v", chunk, err)
	}
	outro := criarDocumento(t, s, "Outro documento", 20, store.StatusReady)
	ids, err := s.Blocks().IDs(ctx, outro)
	if err != nil || len(ids) < 2 {
		t.Fatalf("blocos do outro documento: %v, %v", ids, err)
	}
	if _, err := s.DB().ExecContext(ctx,
		`UPDATE chunks SET first_block_id = ?, last_block_id = ? WHERE id = ?`,
		ids[0], ids[len(ids)-1], chunk.ID); err != nil {
		t.Fatalf("corromper chunk: %v", err)
	}

	r, pub := novoRunner(t, s, nil)
	if _, err := r.SendNow(ctx, docID); err == nil {
		t.Fatal("esperado erro ao montar a dose")
	}
	if pub.total() != 0 {
		t.Errorf("nada deveria ser publicado: %d", pub.total())
	}
	if run := runDaData(t, s, "2026-10-26"); run != nil {
		t.Errorf("run da dose inválida deveria ser descartada: %+v", run)
	}
	// O cursor continua no mesmo chunk, para nova tentativa.
	pendente, err := s.Plans().NextPending(ctx, docID)
	if err != nil || pendente == nil || pendente.ID != chunk.ID {
		t.Errorf("cursor = %+v, %v", pendente, err)
	}
}

func TestSendTestFabricaComErro(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	r, _ := novoRunner(t, s, nil)
	r.NewPublisher = func(NtfyConfig) (Publisher, error) {
		return nil, errors.New("fábrica indisponível")
	}
	if _, err := r.SendTest(context.Background()); err == nil {
		t.Fatal("esperado erro da fábrica de publisher")
	}
}

func TestSendNowFalhaMantemRunDeTick(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	docID := criarDocumento(t, s, "Run do tick", 10, store.StatusReady)
	ctx := context.Background()

	run, _, err := s.Runs().Create(ctx, "2026-10-26", quando, quando, false)
	if err != nil {
		t.Fatal(err)
	}
	chunk, err := s.Plans().NextPending(ctx, docID)
	if err != nil || chunk == nil {
		t.Fatalf("cursor: %+v, %v", chunk, err)
	}
	item, err := s.Runs().AddItem(ctx, run.ID, docID, chunk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Runs().MarkItemFailed(ctx, item.ID, 1, 0, "401"); err != nil {
		t.Fatal(err)
	}

	r, pub := novoRunner(t, s, nil)
	pub.err = errors.New("ntfy: resposta HTTP 500")
	if _, err := r.SendNow(ctx, docID); err == nil {
		t.Fatal("esperado erro de publicação")
	}

	// A run veio de um tick anterior: não é descartada nem fechada.
	if atual := runDaData(t, s, "2026-10-26"); atual == nil || atual.Status != store.RunRunning {
		t.Errorf("run = %+v, quer running", atual)
	}
	itens, err := s.Runs().ListItems(ctx, run.ID)
	if err != nil || len(itens) != 1 || itens[0].Status != store.ItemFailed {
		t.Errorf("itens = %+v, %v", itens, err)
	}
}

func TestSendNowConfiguracaoInvalida(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	definir(t, s, KeyTime, "25:00")
	docID := criarDocumento(t, s, "Config inválida", 10, store.StatusReady)
	r, _ := novoRunner(t, s, nil)

	if _, err := r.SendNow(context.Background(), docID); err == nil {
		t.Error("esperado erro de configuração")
	}
	if _, err := r.SendTest(context.Background()); err == nil {
		t.Error("esperado erro de configuração no teste")
	}
	if run := runDaData(t, s, "2026-10-26"); run != nil {
		t.Errorf("não deveria criar run: %+v", run)
	}
}

func TestSendNowContextoNulo(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	docID := criarDocumento(t, s, "Contexto nulo", 10, store.StatusReady)
	r, _ := novoRunner(t, s, nil)

	if _, err := r.SendNow(nil, docID); err != nil {
		t.Fatalf("contexto nulo: %v", err)
	}
	if _, err := r.SendTest(nil); err != nil {
		t.Fatalf("teste com contexto nulo: %v", err)
	}
}

func TestSendNowFalhaAoFinalizarRun(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	docID := criarDocumento(t, s, "Finalização", 10, store.StatusReady)
	ctx := context.Background()

	r, pub := novoRunner(t, s, nil)
	// Simula outro caminho fechando a run entre a publicação e o fim.
	pub.hook = func() {
		run, err := s.Runs().GetByDate(ctx, "2026-10-26")
		if err != nil || run == nil {
			t.Errorf("run no hook: %+v, %v", run, err)
			return
		}
		if err := s.Runs().FinishRun(ctx, run.ID, store.RunSent, "", quando); err != nil {
			t.Errorf("fechar run no hook: %v", err)
		}
	}

	if _, err := r.SendNow(ctx, docID); err == nil {
		t.Fatal("esperado erro ao finalizar run já fechada")
	}
	if run := runDaData(t, s, "2026-10-26"); run == nil || run.Status != store.RunSent {
		t.Errorf("run = %+v", run)
	}
}

func TestSendNowTituloVazioDescartaRun(t *testing.T) {
	s := abrirStore(t)
	publicarConfigurado(t, s)
	semCatchup(t, s)
	docID := criarDocumento(t, s, "Sem título", 10, store.StatusReady)
	ctx := context.Background()

	if _, err := s.DB().ExecContext(ctx, `UPDATE documents SET title = '' WHERE id = ?`, docID); err != nil {
		t.Fatalf("limpar título: %v", err)
	}

	r, pub := novoRunner(t, s, nil)
	if _, err := r.SendNow(ctx, docID); err == nil {
		t.Fatal("esperado erro de composição")
	}
	if pub.total() != 0 {
		t.Errorf("nada deveria ser publicado: %d", pub.total())
	}
	if run := runDaData(t, s, "2026-10-26"); run != nil {
		t.Errorf("run deveria ser descartada: %+v", run)
	}
}
