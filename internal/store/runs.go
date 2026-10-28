package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RunStatus é o estado de uma run de entrega.
type RunStatus string

const (
	RunRunning RunStatus = "running"
	RunSent    RunStatus = "sent"
	RunPartial RunStatus = "partial"
	RunFailed  RunStatus = "failed"
)

// ItemStatus é o estado de um documento dentro da run.
type ItemStatus string

const (
	ItemPending ItemStatus = "pending"
	ItemSent    ItemStatus = "sent"
	ItemFailed  ItemStatus = "failed"
	ItemSkipped ItemStatus = "skipped"
)

// DeliveryRun é uma linha de delivery_runs.
type DeliveryRun struct {
	ID           int64
	RunDate      string
	ScheduledFor time.Time
	StartedAt    time.Time
	FinishedAt   *time.Time
	Status       RunStatus
	Late         bool
	Error        *string
}

// DeliveryItem é uma linha de delivery_items.
type DeliveryItem struct {
	ID         int64
	RunID      int64
	DocumentID int64
	ChunkID    int64
	Status     ItemStatus
	Attempts   int
	Bytes      *int
	Error      *string
	SentAt     *time.Time
}

// Runs agrupa as operações de delivery_runs e delivery_items.
type Runs struct {
	db *sql.DB
}

// Runs devolve o repositório de runs.
func (s *Store) Runs() *Runs { return &Runs{db: s.db} }

const colunasRun = `id, run_date, scheduled_for, started_at, finished_at, status, late, error`

// Create insere uma run para runDate. Devolve a run e se ela foi criada; uma
// run existente para a mesma data é devolvida com criada=false (DB-04).
func (r *Runs) Create(ctx context.Context, runDate string, scheduledFor, startedAt time.Time, late bool) (*DeliveryRun, bool, error) {
	if _, err := time.Parse("2006-01-02", runDate); err != nil {
		return nil, false, fmt.Errorf("store: run_date inválida %q: %w", runDate, err)
	}
	if scheduledFor.IsZero() || startedAt.IsZero() {
		return nil, false, errors.New("store: run sem scheduled_for/started_at")
	}

	res, err := r.db.ExecContext(ctx, `
		INSERT INTO delivery_runs (run_date, scheduled_for, started_at, status, late)
		VALUES (?, ?, ?, 'running', ?)
		ON CONFLICT (run_date) DO NOTHING`,
		runDate, formatarData(scheduledFor), formatarData(startedAt), boolInt(late))
	if err != nil {
		return nil, false, fmt.Errorf("store: criar run %s: %w", runDate, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, false, fmt.Errorf("store: criar run %s: %w", runDate, err)
	}
	run, err := r.GetByDate(ctx, runDate)
	if err != nil {
		return nil, false, err
	}
	return run, n == 1, nil
}

// GetByDate busca a run da data local. Devolve nil sem erro quando não existe.
func (r *Runs) GetByDate(ctx context.Context, runDate string) (*DeliveryRun, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+colunasRun+` FROM delivery_runs WHERE run_date = ?`, runDate)
	run, err := lerRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: buscar run %s: %w", runDate, err)
	}
	return run, nil
}

// Running devolve a run mais recente ainda em andamento, para retomada após
// restart. Devolve nil sem erro quando não há nenhuma.
func (r *Runs) Running(ctx context.Context) (*DeliveryRun, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+colunasRun+` FROM delivery_runs WHERE status = 'running' ORDER BY id DESC LIMIT 1`)
	run, err := lerRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: buscar run em andamento: %w", err)
	}
	return run, nil
}

func lerRun(l linha) (*DeliveryRun, error) {
	var (
		run    DeliveryRun
		sched  string
		inicio string
		fim    sql.NullString
		status string
		late   int
		errMsg sql.NullString
	)
	if err := l.Scan(&run.ID, &run.RunDate, &sched, &inicio, &fim, &status, &late, &errMsg); err != nil {
		return nil, err
	}
	var err error
	if run.ScheduledFor, err = parsearData(sched); err != nil {
		return nil, err
	}
	if run.StartedAt, err = parsearData(inicio); err != nil {
		return nil, err
	}
	if fim.Valid {
		t, err := parsearData(fim.String)
		if err != nil {
			return nil, err
		}
		run.FinishedAt = &t
	}
	run.Status = RunStatus(status)
	run.Late = late != 0
	if errMsg.Valid {
		run.Error = &errMsg.String
	}
	return &run, nil
}

// List devolve as runs mais recentes, da mais nova para a mais antiga. Com
// documentID > 0, só entram as runs que tiveram item daquele documento. O
// limite padrão é 30 e o máximo 200, para a página de histórico não crescer
// sem controle.
func (r *Runs) List(ctx context.Context, limit int, documentID int64) ([]DeliveryRun, error) {
	if limit <= 0 {
		limit = 30
	}
	if limit > 200 {
		limit = 200
	}
	if documentID < 0 {
		documentID = 0
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT r.id, r.run_date, r.scheduled_for, r.started_at, r.finished_at,
		       r.status, r.late, r.error
		FROM delivery_runs r
		WHERE ? = 0 OR EXISTS (
			SELECT 1 FROM delivery_items i
			WHERE i.run_id = r.id AND i.document_id = ?
		)
		ORDER BY r.run_date DESC, r.id DESC
		LIMIT ?`, documentID, documentID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: listar runs: %w", err)
	}
	defer rows.Close()

	var out []DeliveryRun
	for rows.Next() {
		run, err := lerRun(rows)
		if err != nil {
			return nil, fmt.Errorf("store: ler run: %w", err)
		}
		out = append(out, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: listar runs: %w", err)
	}
	return out, nil
}

// Delete remove uma run e, em cascata, seus itens. Serve para desfazer uma run
// criada por um envio manual que falhou antes de qualquer publicação; não é
// usada para apagar histórico já entregue.
func (r *Runs) Delete(ctx context.Context, runID int64) error {
	if runID <= 0 {
		return fmt.Errorf("store: run inválida para remoção: %d", runID)
	}
	if _, err := r.db.ExecContext(ctx, `DELETE FROM delivery_runs WHERE id = ? AND status = 'running'`, runID); err != nil {
		return fmt.Errorf("store: remover run %d: %w", runID, err)
	}
	return nil
}

// AddItem cria o item de um documento na run. A UNIQUE (run_id, document_id)
// impede duas doses do mesmo documento na mesma run (DB-05).
func (r *Runs) AddItem(ctx context.Context, runID, documentID, chunkID int64) (*DeliveryItem, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO delivery_items (run_id, document_id, chunk_id, status)
		VALUES (?, ?, ?, 'pending')`, runID, documentID, chunkID)
	if err != nil {
		return nil, fmt.Errorf("store: criar item da run %d: %w", runID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("store: id do item da run %d: %w", runID, err)
	}
	return &DeliveryItem{
		ID:         id,
		RunID:      runID,
		DocumentID: documentID,
		ChunkID:    chunkID,
		Status:     ItemPending,
	}, nil
}

const colunasItem = `id, run_id, document_id, chunk_id, status, attempts, bytes, error, sent_at`

// ListItems devolve os itens da run na ordem de criação.
func (r *Runs) ListItems(ctx context.Context, runID int64) ([]DeliveryItem, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+colunasItem+` FROM delivery_items WHERE run_id = ? ORDER BY id`, runID)
	if err != nil {
		return nil, fmt.Errorf("store: listar itens da run %d: %w", runID, err)
	}
	defer rows.Close()

	var out []DeliveryItem
	for rows.Next() {
		item, err := lerItem(rows)
		if err != nil {
			return nil, fmt.Errorf("store: ler item da run %d: %w", runID, err)
		}
		out = append(out, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: listar itens da run %d: %w", runID, err)
	}
	return out, nil
}

// ItemByDocument devolve o item de um documento na run. Devolve nil sem erro
// quando o documento não tem item naquela run (DB-05).
func (r *Runs) ItemByDocument(ctx context.Context, runID, documentID int64) (*DeliveryItem, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+colunasItem+` FROM delivery_items WHERE run_id = ? AND document_id = ?`,
		runID, documentID)
	item, err := lerItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: buscar item do documento %d na run %d: %w", documentID, runID, err)
	}
	return item, nil
}

func lerItem(l linha) (*DeliveryItem, error) {
	var (
		item   DeliveryItem
		status string
		bytes  sql.NullInt64
		errMsg sql.NullString
		sentAt sql.NullString
	)
	if err := l.Scan(&item.ID, &item.RunID, &item.DocumentID, &item.ChunkID,
		&status, &item.Attempts, &bytes, &errMsg, &sentAt); err != nil {
		return nil, err
	}
	item.Status = ItemStatus(status)
	if bytes.Valid {
		v := int(bytes.Int64)
		item.Bytes = &v
	}
	if errMsg.Valid {
		item.Error = &errMsg.String
	}
	if sentAt.Valid {
		t, err := parsearData(sentAt.String)
		if err != nil {
			return nil, err
		}
		item.SentAt = &t
	}
	return &item, nil
}

// MarkItemSent grava item e chunk como enviados na mesma transação. O chunk
// precisa estar pending (DB-08); se outro caminho já o marcou, devolve erro e
// nada muda.
func (r *Runs) MarkItemSent(ctx context.Context, itemID, chunkID int64, bytes, attempts int, quando time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: iniciar envio do item %d: %w", itemID, err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE delivery_items
		SET status = 'sent', attempts = ?, bytes = ?, error = NULL, sent_at = ?
		WHERE id = ? AND status IN ('pending', 'failed')`,
		attempts, bytes, formatarData(quando), itemID)
	if err != nil {
		return fmt.Errorf("store: marcar item %d como enviado: %w", itemID, err)
	}
	if err := exigirLinhaAfetada(res, "item enviado"); err != nil {
		return err
	}

	res, err = tx.ExecContext(ctx, `
		UPDATE chunks SET status = 'sent', sent_at = ?
		WHERE id = ? AND status = 'pending'`, formatarData(quando), chunkID)
	if err != nil {
		return fmt.Errorf("store: marcar chunk %d como enviado: %w", chunkID, err)
	}
	if err := exigirLinhaAfetada(res, "chunk enviado"); err != nil {
		return errors.Join(err, ErrChunkNotPending)
	}
	return tx.Commit()
}

// ErrChunkNotPending indica que o chunk deixou de estar pending antes da
// gravação do envio (corrida com outra run ou envio manual).
var ErrChunkNotPending = errors.New("store: chunk não está pendente")

// MarkItemFailed registra a falha sem avançar o cursor: o chunk continua
// pending e entra na próxima run.
func (r *Runs) MarkItemFailed(ctx context.Context, itemID int64, attempts, bytes int, errMsg string) error {
	msg := errMsg
	res, err := r.db.ExecContext(ctx, `
		UPDATE delivery_items
		SET status = 'failed', attempts = ?, bytes = ?, error = ?
		WHERE id = ? AND status IN ('pending', 'failed')`,
		attempts, bytes, msg, itemID)
	if err != nil {
		return fmt.Errorf("store: marcar item %d como falho: %w", itemID, err)
	}
	return exigirLinhaAfetada(res, "item falho")
}

// MarkItemSkipped marca o item como pulado (por exemplo, chunk já entregue
// quando a run foi retomada).
func (r *Runs) MarkItemSkipped(ctx context.Context, itemID int64, motivo string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE delivery_items SET status = 'skipped', error = ?
		WHERE id = ? AND status = 'pending'`, motivo, itemID)
	if err != nil {
		return fmt.Errorf("store: pular item %d: %w", itemID, err)
	}
	return exigirLinhaAfetada(res, "item pulado")
}

// FinishRun fecha a run com o status final e, opcionalmente, uma mensagem.
func (r *Runs) FinishRun(ctx context.Context, runID int64, status RunStatus, errMsg string, quando time.Time) error {
	switch status {
	case RunSent, RunPartial, RunFailed:
	default:
		return fmt.Errorf("store: status final inválido para a run %d: %q", runID, status)
	}
	var msg any
	if errMsg != "" {
		msg = errMsg
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE delivery_runs
		SET status = ?, finished_at = ?, error = ?
		WHERE id = ? AND status = 'running'`,
		string(status), formatarData(quando), msg, runID)
	if err != nil {
		return fmt.Errorf("store: fechar run %d: %w", runID, err)
	}
	return exigirLinhaAfetada(res, "run fechada")
}
