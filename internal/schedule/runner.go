package schedule

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/giovani/readingdrip/internal/blocks"
	"github.com/giovani/readingdrip/internal/delivery"
	"github.com/giovani/readingdrip/internal/platform"
	"github.com/giovani/readingdrip/internal/store"
)

const (
	// DefaultInterval é o período do ticker (docs/scheduling.md).
	DefaultInterval = 60 * time.Second
	// pausaSplit separa mensagens de uma mesma run dividida por documento.
	pausaSplit = time.Second
	// maxErroRun limita o texto de erro gravado na run.
	maxErroRun = 500
)

// ErrNtfyNaoConfigurado indica que agenda existe, mas falta base URL ou topic.
var ErrNtfyNaoConfigurado = errors.New("schedule: ntfy não configurado")

// Publisher é o que o agendador usa para enviar. *delivery.Client implementa.
type Publisher interface {
	Publish(ctx context.Context, msg delivery.Message) (delivery.SendResult, error)
}

// LinkBuilder monta os links assinados do leitor e de “marcar lido” para uma
// dose. Recebe as validades configuradas (reader e done); nil deixa as
// mensagens sem link.
type LinkBuilder func(document store.Document, chunk store.Chunk, readerTTL, doneTTL time.Duration) (reader, done string)

// RunResult resume o que aconteceu em uma run.
type RunResult struct {
	RunID   int64
	RunDate string
	Late    bool
	Sent    int
	Failed  int
	Skipped int
}

// TickResult informa o que o tick fez. Skipped explica quando nada rodou.
type TickResult struct {
	Runs    []RunResult
	Skipped string
}

// Runner executa ticks. É seguro chamar Tick concorrentemente: só um tick
// roda por vez.
type Runner struct {
	Store        *store.Store
	Clock        platform.Clock
	Logger       *slog.Logger
	Links        LinkBuilder
	Interval     time.Duration
	NewPublisher func(NtfyConfig) (Publisher, error)
	Sleep        func(context.Context, time.Duration) error
	HTTPClient   *http.Client

	lifeMu  sync.Mutex
	runMu   sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	started bool
}

// Start dispara o ticker em uma goroutine. O primeiro tick roda imediatamente
// (catch-up). Stop encerra e espera a run corrente.
func (r *Runner) Start(parent context.Context) {
	if r == nil {
		return
	}
	r.lifeMu.Lock()
	if r.started {
		r.lifeMu.Unlock()
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	intervalo := r.Interval
	if intervalo <= 0 {
		intervalo = DefaultInterval
	}
	r.cancel = cancel
	done := make(chan struct{})
	r.done = done
	r.started = true
	r.lifeMu.Unlock()

	go func() {
		defer close(done)
		r.tickEClaro(ctx)
		ticker := time.NewTicker(intervalo)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.tickEClaro(ctx)
			}
		}
	}()
}

// Stop cancela o ticker e espera a run corrente terminar. Uma run interrompida
// fica `running` e é retomada no próximo Start.
func (r *Runner) Stop() {
	if r == nil {
		return
	}
	r.lifeMu.Lock()
	if !r.started {
		r.lifeMu.Unlock()
		return
	}
	cancel, done := r.cancel, r.done
	r.started = false
	r.lifeMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (r *Runner) tickEClaro(ctx context.Context) {
	resultado, err := r.Tick(ctx, r.clock().Now())
	if err != nil {
		if ctx.Err() == nil {
			r.logger().Warn("tick do agendador", "erro", err)
		}
		return
	}
	if resultado.Skipped != "" {
		r.logger().Debug("tick sem entrega", "motivo", resultado.Skipped)
		return
	}
	for _, run := range resultado.Runs {
		r.logger().Info("run concluída",
			"run", run.RunID, "data", run.RunDate, "late", run.Late,
			"enviados", run.Sent, "falhas", run.Failed, "pulados", run.Skipped)
	}
}

// Tick decide se é hora de entregar e executa as runs devidas. now zero usa o
// relógio do Runner.
func (r *Runner) Tick(ctx context.Context, now time.Time) (TickResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if r == nil || r.Store == nil {
		return TickResult{}, errors.New("schedule: runner sem store")
	}
	if now.IsZero() {
		now = r.clock().Now()
	}
	if !r.runMu.TryLock() {
		return TickResult{Skipped: "tick anterior em andamento"}, nil
	}
	defer r.runMu.Unlock()

	st, err := LoadSettings(ctx, r.Store.Settings())
	if err != nil {
		return TickResult{}, err
	}
	if !st.Schedule.Enabled {
		return TickResult{Skipped: "agendador desligado"}, nil
	}

	pub, err := r.publisher(st.Ntfy)
	if errors.Is(err, ErrNtfyNaoConfigurado) {
		return TickResult{Skipped: "ntfy não configurado"}, nil
	}
	if err != nil {
		return TickResult{}, err
	}

	// Uma run deixada em andamento por crash/shutdown é retomada antes de
	// qualquer run nova (idempotência por item).
	if pendente, err := r.Store.Runs().Running(ctx); err != nil {
		return TickResult{}, err
	} else if pendente != nil {
		resultado, err := r.resumeRun(ctx, st, pub, pendente, now)
		return TickResult{Runs: []RunResult{resultado}}, err
	}

	local := now.In(st.Schedule.Location)
	var resultado TickResult

	// Run do dia, se for dia de leitura e o horário já passou.
	if st.Schedule.isReadingDay(local.Weekday()) && !local.Before(st.Schedule.scheduledFor(local)) {
		run, criada, err := r.Store.Runs().Create(ctx,
			local.Format("2006-01-02"), st.Schedule.scheduledFor(local), now, false)
		if err != nil {
			return resultado, err
		}
		if criada {
			rr, err := r.executar(ctx, st, pub, run, now)
			resultado.Runs = append(resultado.Runs, rr)
			if err != nil {
				return resultado, err
			}
		}
	}

	// Catch-up: a run perdida mais recente dentro da janela (em dias de
	// leitura), independente da run de hoje.
	if dia, ok, err := r.diaPerdido(ctx, st.Schedule, local); err != nil {
		return resultado, err
	} else if ok {
		run, criada, err := r.Store.Runs().Create(ctx,
			dia.Format("2006-01-02"), st.Schedule.scheduledFor(dia), now, true)
		if err != nil {
			return resultado, err
		}
		if criada {
			rr, err := r.executar(ctx, st, pub, run, now)
			resultado.Runs = append(resultado.Runs, rr)
			if err != nil {
				return resultado, err
			}
		}
	}

	if len(resultado.Runs) == 0 {
		resultado.Skipped = "fora do dia ou do horário"
	}
	return resultado, nil
}

// diaPerdido devolve o dia de leitura mais recente, dentro da janela, que não
// tem run. A janela conta dias de leitura, não dias corridos.
func (r *Runner) diaPerdido(ctx context.Context, cfg Config, local time.Time) (time.Time, bool, error) {
	if cfg.CatchupDays <= 0 {
		return time.Time{}, false, nil
	}
	encontrados := 0
	dia := local
	for i := 0; i < cfg.CatchupDays*7+7 && encontrados < cfg.CatchupDays; i++ {
		dia = dia.AddDate(0, 0, -1)
		if !cfg.isReadingDay(dia.Weekday()) {
			continue
		}
		encontrados++
		if local.Before(cfg.scheduledFor(dia)) {
			continue
		}
		existente, err := r.Store.Runs().GetByDate(ctx, dia.Format("2006-01-02"))
		if err != nil {
			return time.Time{}, false, err
		}
		if existente == nil {
			return dia, true, nil
		}
		if existente.Late {
			// Já houve um catch-up dentro da janela: não enfileira os dias
			// mais antigos, um por tick.
			return time.Time{}, false, nil
		}
	}
	return time.Time{}, false, nil
}

// executar seleciona documentos prontos, cria os itens da run e publica.
func (r *Runner) executar(ctx context.Context, st Settings, pub Publisher, run *store.DeliveryRun, now time.Time) (RunResult, error) {
	docs, err := r.Store.Documents().List(ctx)
	if err != nil {
		r.fecharComErro(ctx, run, err, now)
		return RunResult{RunID: run.ID, RunDate: run.RunDate, Late: run.Late}, err
	}

	doses := make([]delivery.Dose, 0, st.Schedule.MaxDocsPerRun)
	itens := make(map[int64]store.DeliveryItem, st.Schedule.MaxDocsPerRun)
	for i := range docs {
		if len(doses) >= st.Schedule.MaxDocsPerRun {
			break
		}
		doc := &docs[i]
		if doc.Status != store.StatusReady {
			continue
		}
		chunk, err := r.Store.Plans().NextPending(ctx, doc.ID)
		if err != nil {
			r.fecharComErro(ctx, run, err, now)
			return RunResult{RunID: run.ID, RunDate: run.RunDate, Late: run.Late}, err
		}
		if chunk == nil {
			continue
		}
		dose, err := r.dose(ctx, st, doc, chunk)
		if err != nil {
			r.logger().Warn("montar dose", "documento", doc.ID, "chunk", chunk.ID, "erro", err)
			continue
		}
		item, err := r.Store.Runs().AddItem(ctx, run.ID, doc.ID, chunk.ID)
		if err != nil {
			r.fecharComErro(ctx, run, err, now)
			return RunResult{RunID: run.ID, RunDate: run.RunDate, Late: run.Late}, err
		}
		itens[chunk.ID] = *item
		doses = append(doses, dose)
	}

	if len(doses) == 0 {
		if err := r.Store.Runs().FinishRun(ctx, run.ID, store.RunSent, "", now); err != nil {
			return RunResult{RunID: run.ID, RunDate: run.RunDate, Late: run.Late}, err
		}
		r.evento(ctx, run, store.EventInfo, "run sem documentos elegíveis", now)
		return RunResult{RunID: run.ID, RunDate: run.RunDate, Late: run.Late}, nil
	}

	res, err := r.enviar(ctx, st, pub, run, doses, itens, now)
	if err != nil {
		if ctx.Err() != nil {
			// Run fica `running` para ser retomada no próximo start.
			return res, err
		}
		r.fecharComErro(ctx, run, err, now)
		return res, err
	}
	return r.finalizar(ctx, run, now)
}

// resumeRun reenvia apenas itens pendentes de uma run interrompida.
func (r *Runner) resumeRun(ctx context.Context, st Settings, pub Publisher, run *store.DeliveryRun, now time.Time) (RunResult, error) {
	itens, err := r.Store.Runs().ListItems(ctx, run.ID)
	if err != nil {
		return RunResult{RunID: run.ID, RunDate: run.RunDate, Late: run.Late}, err
	}

	doses := make([]delivery.Dose, 0, len(itens))
	porChunk := make(map[int64]store.DeliveryItem, len(itens))
	for _, item := range itens {
		if item.Status != store.ItemPending {
			continue
		}
		chunk, err := r.Store.Plans().Chunk(ctx, item.ChunkID)
		if err != nil {
			return RunResult{RunID: run.ID, RunDate: run.RunDate, Late: run.Late}, err
		}
		if chunk == nil || chunk.Status != store.ChunkPending {
			if err := r.Store.Runs().MarkItemSkipped(ctx, item.ID, "chunk não está pendente"); err != nil {
				return RunResult{RunID: run.ID, RunDate: run.RunDate, Late: run.Late}, err
			}
			continue
		}
		doc, err := r.Store.Documents().Get(ctx, item.DocumentID)
		if err != nil {
			return RunResult{RunID: run.ID, RunDate: run.RunDate, Late: run.Late}, err
		}
		if doc == nil {
			if err := r.Store.Runs().MarkItemSkipped(ctx, item.ID, "documento removido"); err != nil {
				return RunResult{RunID: run.ID, RunDate: run.RunDate, Late: run.Late}, err
			}
			continue
		}
		dose, err := r.dose(ctx, st, doc, chunk)
		if err != nil {
			if err := r.Store.Runs().MarkItemFailed(ctx, item.ID, 0, 0, err.Error()); err != nil {
				return RunResult{RunID: run.ID, RunDate: run.RunDate, Late: run.Late}, err
			}
			continue
		}
		porChunk[chunk.ID] = item
		doses = append(doses, dose)
	}

	if len(doses) > 0 {
		res, err := r.enviar(ctx, st, pub, run, doses, porChunk, now)
		if err != nil {
			if ctx.Err() != nil {
				return res, err
			}
			r.fecharComErro(ctx, run, err, now)
			return res, err
		}
	}
	return r.finalizar(ctx, run, now)
}

// enviar compõe as doses e publica cada mensagem. Erros de publicação viram
// item `failed`; erros de store/contexto são devolvidos.
func (r *Runner) enviar(ctx context.Context, st Settings, pub Publisher, run *store.DeliveryRun,
	doses []delivery.Dose, itens map[int64]store.DeliveryItem, now time.Time) (RunResult, error) {

	msgs, err := delivery.Compose(doses, delivery.Options{
		BodyMode:        st.Ntfy.BodyMode,
		MaxMessageBytes: st.Ntfy.MaxMessageBytes,
		RecapDisabled:   !st.Plan.Recap,
	})
	if err != nil {
		return RunResult{RunID: run.ID, RunDate: run.RunDate, Late: run.Late}, err
	}

	res := RunResult{RunID: run.ID, RunDate: run.RunDate, Late: run.Late}
	var falhas []string
	for i, msg := range msgs {
		if i > 0 {
			if err := r.dormir(ctx, pausaSplit); err != nil {
				return res, err
			}
		}
		if err := ctx.Err(); err != nil {
			return res, err
		}

		envio, err := pub.Publish(ctx, msg)
		if err != nil {
			if ctx.Err() != nil {
				return res, ctx.Err()
			}
			res.Failed += len(msg.Chunks)
			falhas = append(falhas, err.Error())
			r.logger().Warn("envio falhou",
				"run", run.ID, "bytes", len(msg.Body), "tentativas", envio.Attempts, "erro", err)
			for _, chunkID := range msg.Chunks {
				item, ok := itens[chunkID]
				if !ok {
					continue
				}
				if err := r.Store.Runs().MarkItemFailed(ctx, item.ID,
					envio.Attempts, len(msg.Body), err.Error()); err != nil {
					return res, err
				}
			}
			continue
		}

		for _, chunkID := range msg.Chunks {
			item, ok := itens[chunkID]
			if !ok {
				continue
			}
			if err := r.Store.Runs().MarkItemSent(ctx, item.ID, chunkID,
				len(msg.Body), envio.Attempts, now); err != nil {
				if errors.Is(err, store.ErrChunkNotPending) {
					if err := r.Store.Runs().MarkItemSkipped(ctx, item.ID,
						"chunk já não estava pendente"); err != nil {
						return res, err
					}
					res.Skipped++
					continue
				}
				return res, err
			}
			res.Sent++
		}

		r.logger().Info("mensagem publicada",
			"run", run.ID, "chunks", len(msg.Chunks), "bytes", len(msg.Body),
			"tentativas", envio.Attempts, "latencia_ms", envio.Latency.Milliseconds())
	}
	return res, nil
}

// finalizar recalcula o estado da run a partir dos itens persistidos e a fecha.
func (r *Runner) finalizar(ctx context.Context, run *store.DeliveryRun, now time.Time) (RunResult, error) {
	itens, err := r.Store.Runs().ListItems(ctx, run.ID)
	if err != nil {
		return RunResult{RunID: run.ID, RunDate: run.RunDate, Late: run.Late}, err
	}

	res := RunResult{RunID: run.ID, RunDate: run.RunDate, Late: run.Late}
	var falhas []string
	for _, item := range itens {
		switch item.Status {
		case store.ItemSent:
			res.Sent++
		case store.ItemFailed:
			res.Failed++
			if item.Error != nil && *item.Error != "" && len(falhas) < 3 {
				falhas = append(falhas, *item.Error)
			}
		case store.ItemSkipped:
			res.Skipped++
		case store.ItemPending:
			// Itens pendentes ao final (por exemplo, falha inesperada) não
			// podem ficar órfãos: fecham como falha e o chunk continua pending.
			if err := r.Store.Runs().MarkItemFailed(ctx, item.ID, item.Attempts, 0, "não processado"); err != nil {
				return res, err
			}
			res.Failed++
		}
	}

	status := store.RunSent
	switch {
	case res.Failed == 0:
		status = store.RunSent
	case res.Sent == 0 && res.Skipped == 0:
		status = store.RunFailed
	default:
		status = store.RunPartial
	}
	mensagem := truncarErro(strings.Join(falhas, "; "))
	if err := r.Store.Runs().FinishRun(ctx, run.ID, status, mensagem, now); err != nil {
		return res, err
	}

	nivel := store.EventInfo
	if status != store.RunSent {
		nivel = store.EventWarn
	}
	texto := fmt.Sprintf("run %s: %d enviados, %d falhas, %d pulados",
		run.RunDate, res.Sent, res.Failed, res.Skipped)
	if run.Late {
		texto += " (catch-up)"
	}
	r.evento(ctx, run, nivel, texto, now)
	return res, nil
}

func (r *Runner) fecharComErro(ctx context.Context, run *store.DeliveryRun, causa error, now time.Time) {
	if run == nil || causa == nil {
		return
	}
	motivo := truncarErro(causa.Error())

	// Itens criados antes do erro não podem ficar pendentes numa run fechada:
	// o chunk continua pending e entra na próxima run.
	if itens, err := r.Store.Runs().ListItems(ctx, run.ID); err != nil {
		r.logger().Warn("listar itens ao fechar run com erro", "run", run.ID, "erro", err)
	} else {
		for _, item := range itens {
			if item.Status != store.ItemPending {
				continue
			}
			if err := r.Store.Runs().MarkItemFailed(ctx, item.ID, item.Attempts, 0, motivo); err != nil {
				r.logger().Warn("marcar item pendente como falho", "run", run.ID, "item", item.ID, "erro", err)
			}
		}
	}

	if err := r.Store.Runs().FinishRun(ctx, run.ID, store.RunFailed, motivo, now); err != nil {
		r.logger().Error("fechar run com erro", "run", run.ID, "erro", err)
	}
	r.evento(ctx, run, store.EventError, "run "+run.RunDate+": "+motivo, now)
}

func (r *Runner) evento(ctx context.Context, run *store.DeliveryRun, nivel, mensagem string, now time.Time) {
	r.eventoKind(ctx, run, nivel, "delivery", mensagem, now)
}

func (r *Runner) eventoKind(ctx context.Context, run *store.DeliveryRun, nivel, kind, mensagem string, now time.Time) {
	if run == nil {
		return
	}
	err := r.Store.Events().Add(ctx, store.Event{
		At:      now,
		Level:   nivel,
		Kind:    kind,
		Message: mensagem,
	})
	if err != nil {
		r.logger().Warn("registrar evento de entrega", "run", run.ID, "erro", err)
	}
}

// dose monta o material do chunk: corpo, vizinhos para recap/rodapé, avanço e
// links.
func (r *Runner) dose(ctx context.Context, st Settings, doc *store.Document, chunk *store.Chunk) (delivery.Dose, error) {
	body, err := r.Store.Blocks().Between(ctx, doc.ID, chunk.FirstBlockID, chunk.LastBlockID)
	if err != nil {
		return delivery.Dose{}, err
	}
	var anterior, proximo []blocks.Block
	if prev, err := r.Store.Plans().PreviousChunk(ctx, chunk); err != nil {
		return delivery.Dose{}, err
	} else if prev != nil {
		if anterior, err = r.Store.Blocks().Between(ctx, doc.ID, prev.FirstBlockID, prev.LastBlockID); err != nil {
			return delivery.Dose{}, err
		}
	}
	if next, err := r.Store.Plans().NextChunk(ctx, chunk); err != nil {
		return delivery.Dose{}, err
	} else if next != nil {
		if proximo, err = r.Store.Blocks().Between(ctx, doc.ID, next.FirstBlockID, next.LastBlockID); err != nil {
			return delivery.Dose{}, err
		}
	}
	prog, err := r.Store.Plans().Progress(ctx, chunk.PlanID)
	if err != nil {
		return delivery.Dose{}, err
	}

	d := delivery.Dose{
		Title:    doc.Title,
		Chunk:    *chunk,
		Body:     body,
		Previous: anterior,
		Next:     proximo,
		Percent:  int(math.Round(prog.Pct)),
	}
	if doc.Author != nil {
		d.Author = *doc.Author
	}
	if r.Links != nil {
		leitorTTL, lidoTTL := st.LinkTTLs()
		d.Link, d.DoneLink = r.Links(*doc, *chunk, leitorTTL, lidoTTL)
	}
	return d, nil
}

func (r *Runner) publisher(cfg NtfyConfig) (Publisher, error) {
	return r.publisherCom(cfg, 0)
}

// publisherOnce usa uma única tentativa: envio manual e envio de teste não
// devem segurar a request por minutos em caso de falha.
func (r *Runner) publisherOnce(cfg NtfyConfig) (Publisher, error) {
	return r.publisherCom(cfg, 1)
}

// publisherCom monta o cliente; tentativas 0 usa o padrão do cliente (com
// retry e backoff).
func (r *Runner) publisherCom(cfg NtfyConfig, tentativas int) (Publisher, error) {
	if cfg.BaseURL == "" || cfg.Topic == "" {
		return nil, ErrNtfyNaoConfigurado
	}
	if r.NewPublisher != nil {
		return r.NewPublisher(cfg)
	}
	return delivery.NewClient(delivery.ClientConfig{
		BaseURL:         cfg.BaseURL,
		Topic:           cfg.Topic,
		Token:           cfg.Token,
		Username:        cfg.Username,
		Password:        cfg.Password,
		Priority:        cfg.Priority,
		MaxAttempts:     tentativas,
		MaxMessageBytes: cfg.MaxMessageBytes,
		HTTPClient:      r.HTTPClient,
	})
}

func (r *Runner) clock() platform.Clock {
	if r.Clock != nil {
		return r.Clock
	}
	return platform.SystemClock{}
}

func (r *Runner) logger() *slog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}

func (r *Runner) dormir(ctx context.Context, d time.Duration) error {
	if r.Sleep != nil {
		return r.Sleep(ctx, d)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func truncarErro(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxErroRun {
		return s
	}
	corte := maxErroRun
	for corte > 0 && !utf8.RuneStart(s[corte]) {
		corte--
	}
	return s[:corte] + "…"
}
