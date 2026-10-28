package schedule

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/giovani/readingdrip/internal/delivery"
	"github.com/giovani/readingdrip/internal/store"
)

// Erros do envio manual e do envio de teste. A UI traduz cada um em uma
// mensagem; nenhum deles expõe corpo de leitura nem token.
var (
	// ErrEnvioEmAndamento indica que um tick ou outro envio manual está em
	// andamento; nada é enviado e o usuário pode tentar de novo.
	ErrEnvioEmAndamento = errors.New("schedule: entrega em andamento")
	// ErrSemTrechos indica documento sem chunk pendente no plano ativo.
	ErrSemTrechos = errors.New("schedule: documento sem trechos pendentes")
	// ErrJaEnviadoHoje indica que a run do dia já entregou (ou fechou) o
	// documento; o próximo trecho sai na próxima run.
	ErrJaEnviadoHoje = errors.New("schedule: a run de hoje já entregou este documento")
	// ErrDocumentoNaoEncontrado indica id inexistente.
	ErrDocumentoNaoEncontrado = errors.New("schedule: documento não encontrado")
	// ErrDocumentoNaoPronto indica documento fora de `ready` (processing,
	// pausado, concluído…).
	ErrDocumentoNaoPronto = errors.New("schedule: documento não está pronto para leitura")
)

// corpoTeste é a mensagem de teste do ntfy (docs/delivery-ntfy.md).
const corpoTeste = "✅ **Teste do Reading Drip**\nSe você está lendo isto, o ntfy está configurado."

// SendNowResult descreve o envio manual de um documento.
type SendNowResult struct {
	RunID      int64
	RunDate    string
	DocumentID int64
	ChunkID    int64
	Sent       bool
	Bytes      int
	Attempts   int
	Latency    time.Duration
}

// SendNow entrega agora o próximo chunk pendente de um documento, fora do
// horário da agenda. A entrega usa a run do dia (DB-04): se ela ainda não
// existe, é criada com `late=0`; se já foi fechada, nada é reenviado.
//
// O envio é síncrono e usa uma única tentativa — quem clica vê o resultado e,
// em caso de falha, pode tentar de novo. Uma run criada aqui e que falhou
// antes de qualquer envio é removida, para não consumir a run programada do
// dia. A run de um tick anterior (`running`) é reaproveitada sem ser fechada:
// o próximo tick a retoma e finaliza.
func (r *Runner) SendNow(ctx context.Context, documentID int64) (SendNowResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if r == nil || r.Store == nil {
		return SendNowResult{}, errors.New("schedule: runner sem store")
	}
	if documentID <= 0 {
		return SendNowResult{}, ErrDocumentoNaoEncontrado
	}
	if !r.runMu.TryLock() {
		return SendNowResult{}, ErrEnvioEmAndamento
	}
	defer r.runMu.Unlock()

	st, err := LoadSettings(ctx, r.Store.Settings())
	if err != nil {
		return SendNowResult{}, err
	}
	pub, err := r.publisherOnce(st.Ntfy)
	if err != nil {
		return SendNowResult{}, err
	}

	doc, err := r.Store.Documents().Get(ctx, documentID)
	if err != nil {
		return SendNowResult{}, err
	}
	if doc == nil {
		return SendNowResult{}, ErrDocumentoNaoEncontrado
	}
	if doc.Status != store.StatusReady {
		return SendNowResult{}, ErrDocumentoNaoPronto
	}

	// Só cria a run do dia depois de saber que há algo para enviar: assim um
	// clique sem trechos pendentes não consome a run programada.
	chunk, err := r.Store.Plans().NextPending(ctx, documentID)
	if err != nil {
		return SendNowResult{}, err
	}
	if chunk == nil {
		return SendNowResult{}, ErrSemTrechos
	}

	now := r.clock().Now()
	local := now.In(st.Schedule.Location)
	run, criada, err := r.Store.Runs().Create(ctx, local.Format("2006-01-02"), now, now, false)
	if err != nil {
		return SendNowResult{}, err
	}
	if run == nil {
		return SendNowResult{}, errors.New("schedule: run do dia indisponível")
	}

	res := SendNowResult{RunID: run.ID, RunDate: run.RunDate, DocumentID: documentID}
	if !criada && run.Status != store.RunRunning {
		return res, ErrJaEnviadoHoje
	}

	item, err := r.Store.Runs().ItemByDocument(ctx, run.ID, documentID)
	if err != nil {
		r.descartarRun(ctx, run, criada, err, now)
		return res, err
	}
	if item != nil {
		if item.Status == store.ItemSent || item.Status == store.ItemSkipped {
			return res, ErrJaEnviadoHoje
		}
		// Item `failed` de uma tentativa anterior: reenvia o mesmo trecho, que
		// continua pendente porque falha não avança o cursor (RF-064).
		if item.ChunkID != chunk.ID {
			deItem, err := r.Store.Plans().Chunk(ctx, item.ChunkID)
			if err != nil {
				r.descartarRun(ctx, run, criada, err, now)
				return res, err
			}
			if deItem == nil || deItem.Status != store.ChunkPending {
				return res, ErrJaEnviadoHoje
			}
			chunk = deItem
		}
	} else {
		item, err = r.Store.Runs().AddItem(ctx, run.ID, documentID, chunk.ID)
		if err != nil {
			r.descartarRun(ctx, run, criada, err, now)
			return res, err
		}
	}

	dose, err := r.dose(ctx, st, doc, chunk)
	if err != nil {
		r.falharItem(ctx, run, item, criada, err, 0, 0, now)
		return res, err
	}
	msgs, err := delivery.Compose([]delivery.Dose{dose}, delivery.Options{
		BodyMode:        st.Ntfy.BodyMode,
		MaxMessageBytes: st.Ntfy.MaxMessageBytes,
	})
	if err != nil {
		r.falharItem(ctx, run, item, criada, err, 0, 0, now)
		return res, err
	}
	if len(msgs) != 1 {
		err := fmt.Errorf("schedule: envio manual gerou %d mensagens", len(msgs))
		r.falharItem(ctx, run, item, criada, err, 0, 0, now)
		return res, err
	}

	msg := msgs[0]
	envio, err := pub.Publish(ctx, msg)
	if err != nil {
		r.falharItem(ctx, run, item, criada, err, envio.Attempts, len(msg.Body), now)
		return res, err
	}

	if err := r.Store.Runs().MarkItemSent(ctx, item.ID, chunk.ID, len(msg.Body), envio.Attempts, now); err != nil {
		if errors.Is(err, store.ErrChunkNotPending) {
			return res, ErrJaEnviadoHoje
		}
		r.descartarRun(ctx, run, criada, err, now)
		return res, err
	}

	res.ChunkID = chunk.ID
	res.Sent = true
	res.Bytes = len(msg.Body)
	res.Attempts = envio.Attempts
	res.Latency = envio.Latency

	r.eventoKind(ctx, run, store.EventInfo, "delivery.manual",
		fmt.Sprintf("envio manual: documento %d, trecho %d", doc.ID, chunk.ID), now)
	r.logger().Info("envio manual publicado",
		"run", run.ID, "documento", doc.ID, "chunk", chunk.ID,
		"bytes", len(msg.Body), "tentativas", envio.Attempts,
		"latencia_ms", envio.Latency.Milliseconds())

	if criada {
		if _, err := r.finalizar(ctx, run, now); err != nil {
			return res, err
		}
	}
	return res, nil
}

// SendTest publica a mensagem de teste com a configuração salva. Usa uma única
// tentativa: o resultado é imediato e o usuário decide se repete.
func (r *Runner) SendTest(ctx context.Context) (delivery.SendResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if r == nil || r.Store == nil {
		return delivery.SendResult{}, errors.New("schedule: runner sem store")
	}
	st, err := LoadSettings(ctx, r.Store.Settings())
	if err != nil {
		return delivery.SendResult{}, err
	}
	pub, err := r.publisherOnce(st.Ntfy)
	if err != nil {
		return delivery.SendResult{}, err
	}
	msg := delivery.Message{Title: "✅ Teste do Reading Drip", Body: corpoTeste}
	envio, err := pub.Publish(ctx, msg)
	if err != nil {
		r.logger().Warn("envio de teste falhou",
			"tentativas", envio.Attempts, "erro", err)
		return envio, err
	}
	r.logger().Info("envio de teste publicado",
		"bytes", len(msg.Body), "tentativas", envio.Attempts,
		"latencia_ms", envio.Latency.Milliseconds())
	return envio, nil
}

// falharItem registra a falha e, quando a run nasceu neste envio manual, a
// descarta: sem envio, o dia continua livre para a run programada e para uma
// nova tentativa do usuário.
func (r *Runner) falharItem(ctx context.Context, run *store.DeliveryRun, item *store.DeliveryItem,
	criada bool, causa error, tentativas, bytes int, now time.Time) {

	if run == nil || causa == nil {
		return
	}
	if item != nil {
		if err := r.Store.Runs().MarkItemFailed(ctx, item.ID, tentativas, bytes, causa.Error()); err != nil {
			r.logger().Warn("marcar item do envio manual como falho", "run", run.ID, "erro", err)
		}
	}
	r.descartarRun(ctx, run, criada, causa, now)
}

// descartarRun fecha e remove uma run criada por este envio manual que não
// chegou a enviar nada. Runs de ticks anteriores são preservadas para a
// retomada.
func (r *Runner) descartarRun(ctx context.Context, run *store.DeliveryRun, criada bool, causa error, now time.Time) {
	if run == nil || causa == nil {
		return
	}
	motivo := truncarErro(causa.Error())
	r.eventoKind(ctx, run, store.EventWarn, "delivery.manual",
		"envio manual falhou: "+motivo, now)
	if !criada {
		return
	}
	if err := r.Store.Runs().Delete(ctx, run.ID); err != nil {
		r.logger().Warn("descartar run do envio manual", "run", run.ID, "erro", err)
	}
}
