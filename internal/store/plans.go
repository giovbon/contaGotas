package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/giovani/readingdrip/internal/plan"
)

// PlanStatus é o estado de um plano.
type PlanStatus string

const (
	PlanActive     PlanStatus = "active"
	PlanSuperseded PlanStatus = "superseded"
	PlanStale      PlanStatus = "stale"
)

// ChunkStatus é o estado de entrega de um chunk.
type ChunkStatus string

const (
	ChunkPending ChunkStatus = "pending"
	ChunkSent    ChunkStatus = "sent"
	ChunkSkipped ChunkStatus = "skipped"
)

// Chunk é uma linha de chunks com os campos que a entrega e a UI usam.
type Chunk struct {
	ID               int64
	PlanID           int64
	DocumentID       int64
	Seq              int
	FirstBlockID     int64
	LastBlockID      int64
	FirstHash        string
	LastHash         string
	SectionPath      []string
	StartsMidSection bool
	EndsMidSection   bool
	Oversized        bool
	Words            int
	Seconds          int
	TargetSeconds    int
	Status           ChunkStatus
	SentAt           *time.Time
	DoneAt           *time.Time
}

// Plan é uma linha de plans com os chunks ordenados por seq.
type Plan struct {
	ID            int64
	DocumentID    int64
	Version       int
	Status        PlanStatus
	PacingMode    string
	TargetSeconds int
	HorizonDays   *int
	Params        plan.Params
	CreatedAt     time.Time
	Chunks        []Chunk
}

// Plans agrupa as operações de plans e chunks.
type Plans struct {
	db *sql.DB
}

// Plans devolve o repositório de planos.
func (s *Store) Plans() *Plans { return &Plans{db: s.db} }

// paramsJSON é o formato gravado em plans.params_json (ver data-model.md).
type paramsJSON struct {
	MinFill          float64 `json:"min_fill"`
	MaxOvershoot     float64 `json:"max_overshoot"`
	WPM              int     `json:"wpm"`
	Difficulty       float64 `json:"difficulty"`
	Mode             string  `json:"mode"`
	HorizonDays      int     `json:"horizon_days,omitempty"`
	TargetSeconds    int     `json:"target_seconds"`
	SectionAlignment bool    `json:"section_alignment"`
}

// Create persiste um plano como ativo, substituindo o anterior em uma
// transação. Os chunks usam os ids reais dos blocos do documento.
func (r *Plans) Create(ctx context.Context, documentID int64, pl *plan.Plan, params plan.Params, agora time.Time) (*Plan, error) {
	return r.criar(ctx, documentID, 0, pl, params, agora, false)
}

// Replan persiste uma nova versão que cobre de startBlock (índice do bloco no
// documento) até o fim, e atualiza o ritmo do documento na mesma transação
// (RF-024). As chunks já entregues ficam no plano anterior, que passa a
// `superseded`; nada é reenviado.
func (r *Plans) Replan(ctx context.Context, documentID int64, startBlock int, pl *plan.Plan, params plan.Params, agora time.Time) (*Plan, error) {
	return r.criar(ctx, documentID, startBlock, pl, params, agora, true)
}

func (r *Plans) criar(ctx context.Context, documentID int64, startBlock int, pl *plan.Plan, params plan.Params, agora time.Time, atualizarRitmo bool) (*Plan, error) {
	if pl == nil || len(pl.Chunks) == 0 {
		return nil, errors.New("store: plano vazio")
	}
	if agora.IsZero() {
		return nil, errors.New("store: plano sem created_at")
	}
	if startBlock < 0 {
		return nil, fmt.Errorf("store: início do plano negativo (%d)", startBlock)
	}
	if err := validarCobertura(pl, startBlock); err != nil {
		return nil, err
	}

	blockIDs, err := (&Blocks{db: r.db}).IDs(ctx, documentID)
	if err != nil {
		return nil, err
	}
	if len(blockIDs) == 0 {
		return nil, fmt.Errorf("store: documento %d não tem blocos", documentID)
	}
	if startBlock >= len(blockIDs) {
		return nil, fmt.Errorf("store: início %d fora dos %d blocos do documento %d",
			startBlock, len(blockIDs), documentID)
	}
	if err := validarLimites(pl, startBlock, len(blockIDs)); err != nil {
		return nil, err
	}
	if atualizarRitmo {
		if err := validarRitmo(params); err != nil {
			return nil, err
		}
	}

	paramsJSON, err := json.Marshal(paramsJSON{
		MinFill:          params.MinFill,
		MaxOvershoot:     params.MaxOvershoot,
		WPM:              params.WPM,
		Difficulty:       params.Difficulty,
		Mode:             string(pl.Mode),
		HorizonDays:      params.Days,
		TargetSeconds:    pl.TargetSeconds,
		SectionAlignment: params.SectionAlignment,
	})
	if err != nil {
		return nil, fmt.Errorf("store: serializar parâmetros do plano: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: iniciar plano: %w", err)
	}
	defer tx.Rollback()

	if atualizarRitmo {
		if err := atualizarPacing(ctx, tx, documentID, params, agora); err != nil {
			return nil, err
		}
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE plans SET status = 'superseded' WHERE document_id = ? AND status = 'active'`,
		documentID); err != nil {
		return nil, fmt.Errorf("store: substituir plano ativo do documento %d: %w", documentID, err)
	}

	var versao int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) + 1 FROM plans WHERE document_id = ?`,
		documentID).Scan(&versao); err != nil {
		return nil, fmt.Errorf("store: próxima versão do plano: %w", err)
	}

	res, err := tx.ExecContext(ctx, `INSERT INTO plans
		(document_id, version, status, pacing_mode, target_seconds, horizon_days, params_json, created_at)
		VALUES (?, ?, 'active', ?, ?, ?, ?, ?)`,
		documentID, versao, string(pl.Mode), pl.TargetSeconds, nullableInt(params.Days), string(paramsJSON), formatarData(agora))
	if err != nil {
		return nil, fmt.Errorf("store: inserir plano do documento %d: %w", documentID, err)
	}
	planID, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("store: id do plano: %w", err)
	}

	const inserirChunk = `INSERT INTO chunks
		(plan_id, document_id, seq, first_block_id, last_block_id, first_hash, last_hash,
		 section_path, starts_mid_section, ends_mid_section, oversized, words, seconds,
		 target_seconds, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending')`

	for _, c := range pl.Chunks {
		sectionPath, err := json.Marshal(c.SectionPath)
		if err != nil {
			return nil, fmt.Errorf("store: serializar seção do chunk %d: %w", c.Seq, err)
		}
		if _, err := tx.ExecContext(ctx, inserirChunk,
			planID, documentID, c.Seq,
			blockIDs[c.FirstBlock], blockIDs[c.LastBlock],
			c.FirstHash, c.LastHash, string(sectionPath),
			boolInt(c.StartsMidSection), boolInt(c.EndsMidSection), boolInt(c.Oversized),
			c.Words, c.Seconds, c.TargetSeconds); err != nil {
			return nil, fmt.Errorf("store: inserir chunk %d: %w", c.Seq, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: confirmar plano do documento %d: %w", documentID, err)
	}

	return &Plan{
		ID:            planID,
		DocumentID:    documentID,
		Version:       versao,
		Status:        PlanActive,
		PacingMode:    string(pl.Mode),
		TargetSeconds: pl.TargetSeconds,
		HorizonDays:   nullableIntPtr(params.Days),
		Params:        params,
		CreatedAt:     agora,
	}, nil
}

// atualizarPacing grava o novo ritmo do documento dentro da transação do
// replanejamento. Em finish_by_days os minutos anteriores são preservados; em
// minutes_per_day, pacing_days volta a NULL.
func atualizarPacing(ctx context.Context, tx *sql.Tx, documentID int64, params plan.Params, agora time.Time) error {
	var (
		q    string
		args []any
	)
	if params.Mode == plan.ModeMinutesPerDay {
		q = `UPDATE documents SET pacing_mode = ?, pacing_minutes = ?,
			pacing_days = NULL, updated_at = ? WHERE id = ?`
		args = []any{string(params.Mode), params.Minutes, formatarData(agora), documentID}
	} else {
		q = `UPDATE documents SET pacing_mode = ?, pacing_days = ?,
			updated_at = ? WHERE id = ?`
		args = []any{string(params.Mode), params.Days, formatarData(agora), documentID}
	}
	res, err := tx.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("store: atualizar ritmo do documento %d: %w", documentID, err)
	}
	return exigirLinhaAfetada(res, "ritmo do documento")
}

// validarRitmo confere o modo e o valor de pacing antes de gravar no documento.
func validarRitmo(p plan.Params) error {
	switch p.Mode {
	case plan.ModeMinutesPerDay:
		if p.Minutes <= 0 {
			return fmt.Errorf("store: minutes_per_day exige minutos > 0")
		}
	case plan.ModeFinishByDays:
		if p.Days <= 0 {
			return fmt.Errorf("store: finish_by_days exige dias > 0")
		}
	default:
		return fmt.Errorf("store: modo de ritmo desconhecido %q", p.Mode)
	}
	return nil
}

// Active devolve o plano ativo do documento com os chunks, ou nil.
func (r *Plans) Active(ctx context.Context, documentID int64) (*Plan, error) {
	p, err := r.buscarPlan(ctx, `WHERE document_id = ? AND status = 'active'`, documentID)
	if err != nil || p == nil {
		return p, err
	}
	if p.Chunks, err = r.chunksDoPlan(ctx, p.ID); err != nil {
		return nil, err
	}
	return p, nil
}

// Get devolve um plano pelo id, com chunks.
func (r *Plans) Get(ctx context.Context, id int64) (*Plan, error) {
	p, err := r.buscarPlan(ctx, `WHERE id = ?`, id)
	if err != nil || p == nil {
		return p, err
	}
	if p.Chunks, err = r.chunksDoPlan(ctx, p.ID); err != nil {
		return nil, err
	}
	return p, nil
}

// Versions lista os planos do documento, do mais recente para o mais antigo,
// sem carregar os chunks.
func (r *Plans) Versions(ctx context.Context, documentID int64) ([]Plan, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, document_id, version, status, pacing_mode, target_seconds, horizon_days, params_json, created_at
		FROM plans WHERE document_id = ? ORDER BY version DESC`, documentID)
	if err != nil {
		return nil, fmt.Errorf("store: listar planos do documento %d: %w", documentID, err)
	}
	defer rows.Close()

	var out []Plan
	for rows.Next() {
		p, err := lerPlan(rows)
		if err != nil {
			return nil, fmt.Errorf("store: ler plano: %w", err)
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: listar planos do documento %d: %w", documentID, err)
	}
	return out, nil
}

func (r *Plans) buscarPlan(ctx context.Context, filtro string, arg any) (*Plan, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, document_id, version, status, pacing_mode, target_seconds, horizon_days, params_json, created_at
		FROM plans `+filtro+` ORDER BY version DESC LIMIT 1`, arg)
	p, err := lerPlan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: buscar plano: %w", err)
	}
	return p, nil
}

func lerPlan(l linha) (*Plan, error) {
	var (
		p         Plan
		horizonte sql.NullInt64
		paramsRaw string
		criado    string
	)
	if err := l.Scan(&p.ID, &p.DocumentID, &p.Version, &p.Status, &p.PacingMode,
		&p.TargetSeconds, &horizonte, &paramsRaw, &criado); err != nil {
		return nil, err
	}
	if horizonte.Valid {
		v := int(horizonte.Int64)
		p.HorizonDays = &v
	}
	var pj paramsJSON
	if err := json.Unmarshal([]byte(paramsRaw), &pj); err != nil {
		return nil, fmt.Errorf("store: params_json inválido no plano %d: %w", p.ID, err)
	}
	p.Params = plan.Params{
		Mode:             plan.Mode(pj.Mode),
		Minutes:          minutesDoParams(pj, p.TargetSeconds),
		Days:             pj.HorizonDays,
		MinFill:          pj.MinFill,
		MaxOvershoot:     pj.MaxOvershoot,
		WPM:              pj.WPM,
		Difficulty:       pj.Difficulty,
		SectionAlignment: pj.SectionAlignment,
	}
	var err error
	if p.CreatedAt, err = parsearData(criado); err != nil {
		return nil, err
	}
	return &p, nil
}

// minutesDoParams reconstrói o alvo em minutos do modo minutes_per_day.
func minutesDoParams(pj paramsJSON, targetSeconds int) int {
	if pj.Mode == string(plan.ModeMinutesPerDay) {
		return targetSeconds / 60
	}
	return 0
}

func (r *Plans) chunksDoPlan(ctx context.Context, planID int64) ([]Chunk, error) {
	return listarChunksDoPlano(ctx, r.db, planID)
}

func listarChunksDoPlano(ctx context.Context, q consulta, planID int64) ([]Chunk, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, plan_id, document_id, seq, first_block_id, last_block_id, first_hash, last_hash,
		       section_path, starts_mid_section, ends_mid_section, oversized, words, seconds,
		       target_seconds, status, sent_at, done_at
		FROM chunks WHERE plan_id = ? ORDER BY seq`, planID)
	if err != nil {
		return nil, fmt.Errorf("store: listar chunks do plano %d: %w", planID, err)
	}
	defer rows.Close()

	var out []Chunk
	for rows.Next() {
		c, err := lerChunk(rows)
		if err != nil {
			return nil, fmt.Errorf("store: ler chunk do plano %d: %w", planID, err)
		}
		out = append(out, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: listar chunks do plano %d: %w", planID, err)
	}
	return out, nil
}

// NextPending devolve o próximo chunk pendente do plano ativo (cursor).
func (r *Plans) NextPending(ctx context.Context, documentID int64) (*Chunk, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT c.id, c.plan_id, c.document_id, c.seq, c.first_block_id, c.last_block_id,
		       c.first_hash, c.last_hash, c.section_path, c.starts_mid_section,
		       c.ends_mid_section, c.oversized, c.words, c.seconds, c.target_seconds,
		       c.status, c.sent_at, c.done_at
		FROM chunks c
		JOIN plans p ON p.id = c.plan_id AND p.status = 'active'
		WHERE c.document_id = ? AND c.status = 'pending'
		ORDER BY c.seq LIMIT 1`, documentID)
	c, err := lerChunk(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: próximo chunk do documento %d: %w", documentID, err)
	}
	return c, nil
}

// Chunk devolve um chunk pelo id. Devolve nil sem erro quando não existe.
func (r *Plans) Chunk(ctx context.Context, chunkID int64) (*Chunk, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT c.id, c.plan_id, c.document_id, c.seq, c.first_block_id, c.last_block_id,
		       c.first_hash, c.last_hash, c.section_path, c.starts_mid_section,
		       c.ends_mid_section, c.oversized, c.words, c.seconds, c.target_seconds,
		       c.status, c.sent_at, c.done_at
		FROM chunks c WHERE c.id = ?`, chunkID)
	c, err := lerChunk(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: buscar chunk %d: %w", chunkID, err)
	}
	return c, nil
}

// PreviousChunk devolve o chunk anterior do mesmo plano, ou nil no primeiro.
func (r *Plans) PreviousChunk(ctx context.Context, c *Chunk) (*Chunk, error) {
	return r.chunkVizinho(ctx, c, "<", "DESC")
}

// NextChunk devolve o chunk seguinte do mesmo plano, ou nil no último.
func (r *Plans) NextChunk(ctx context.Context, c *Chunk) (*Chunk, error) {
	return r.chunkVizinho(ctx, c, ">", "ASC")
}

func (r *Plans) chunkVizinho(ctx context.Context, c *Chunk, op, ordem string) (*Chunk, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT c.id, c.plan_id, c.document_id, c.seq, c.first_block_id, c.last_block_id,
		       c.first_hash, c.last_hash, c.section_path, c.starts_mid_section,
		       c.ends_mid_section, c.oversized, c.words, c.seconds, c.target_seconds,
		       c.status, c.sent_at, c.done_at
		FROM chunks c
		WHERE c.plan_id = ? AND c.seq `+op+` ?
		ORDER BY c.seq `+ordem+` LIMIT 1`, c.PlanID, c.Seq)
	vizinho, err := lerChunk(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: chunk vizinho do chunk %d: %w", c.ID, err)
	}
	return vizinho, nil
}

func lerChunk(l linha) (*Chunk, error) {
	var (
		c          Chunk
		sectionRaw string
		starts     int
		ends       int
		oversized  int
		sentAt     sql.NullString
		doneAt     sql.NullString
	)
	if err := l.Scan(&c.ID, &c.PlanID, &c.DocumentID, &c.Seq, &c.FirstBlockID, &c.LastBlockID,
		&c.FirstHash, &c.LastHash, &sectionRaw, &starts, &ends, &oversized, &c.Words,
		&c.Seconds, &c.TargetSeconds, &c.Status, &sentAt, &doneAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(sectionRaw), &c.SectionPath); err != nil {
		return nil, fmt.Errorf("store: section_path inválido no chunk %d: %w", c.ID, err)
	}
	c.StartsMidSection = starts != 0
	c.EndsMidSection = ends != 0
	c.Oversized = oversized != 0
	var err error
	if c.SentAt, err = dataOpcional(sentAt); err != nil {
		return nil, err
	}
	if c.DoneAt, err = dataOpcional(doneAt); err != nil {
		return nil, err
	}
	return &c, nil
}

// MarkSent marca o chunk como enviado. Chunk já enviado não é reescrito
// (DB-08).
func (r *Plans) MarkSent(ctx context.Context, chunkID int64, quando time.Time) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE chunks SET status = 'sent', sent_at = ?
		WHERE id = ? AND status = 'pending'`, formatarData(quando), chunkID)
	if err != nil {
		return fmt.Errorf("store: marcar chunk %d como enviado: %w", chunkID, err)
	}
	return exigirLinhaAfetada(res, "chunk enviado")
}

// MarkSkipped marca o chunk como pulado.
func (r *Plans) MarkSkipped(ctx context.Context, chunkID int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE chunks SET status = 'skipped' WHERE id = ? AND status = 'pending'`, chunkID)
	if err != nil {
		return fmt.Errorf("store: pular chunk %d: %w", chunkID, err)
	}
	return exigirLinhaAfetada(res, "chunk pulado")
}

// MarkDone registra a leitura do chunk pelo usuário.
func (r *Plans) MarkDone(ctx context.Context, chunkID int64, quando time.Time) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE chunks SET done_at = ? WHERE id = ?`, formatarData(quando), chunkID)
	if err != nil {
		return fmt.Errorf("store: marcar chunk %d como lido: %w", chunkID, err)
	}
	return exigirLinhaAfetada(res, "chunk lido")
}

// Progress é o resumo de andamento de um plano.
type Progress struct {
	Total    int
	Enviados int
	Pulados  int
	Pct      float64
}

// Progress informa quantos chunks do plano já foram enviados.
func (r *Plans) Progress(ctx context.Context, planID int64) (Progress, error) {
	var p Progress
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		       COALESCE(SUM(status = 'sent'), 0),
		       COALESCE(SUM(status = 'skipped'), 0)
		FROM chunks WHERE plan_id = ?`, planID).
		Scan(&p.Total, &p.Enviados, &p.Pulados)
	if err != nil {
		return Progress{}, fmt.Errorf("store: progresso do plano %d: %w", planID, err)
	}
	if p.Total > 0 {
		p.Pct = float64(p.Enviados) * 100 / float64(p.Total)
	}
	return p, nil
}

// ErrDeliveryHistory indica que o documento já tem entrega registrada (chunk
// enviado/pulado ou item de run) e por isso não pode ser reextraído sem perder
// histórico.
var ErrDeliveryHistory = errors.New("store: documento já tem histórico de entrega")

// HasDeliveryHistory informa se o documento tem chunk enviado/pulado ou item
// de run. Nesse caso os blocos não podem ser substituídos sem perder histórico.
func (r *Plans) HasDeliveryHistory(ctx context.Context, documentID int64) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `
		SELECT (SELECT COUNT(*) FROM chunks
		        WHERE document_id = ? AND status <> 'pending')
		     + (SELECT COUNT(*) FROM delivery_items WHERE document_id = ?)`,
		documentID, documentID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("store: verificar histórico de entrega do documento %d: %w", documentID, err)
	}
	return n > 0, nil
}

// DiscardUndelivered apaga planos e chunks de um documento que ainda não teve
// entrega, liberando os blocos para a reextração de um reprocessamento. Se
// houver qualquer histórico de entrega devolve ErrDeliveryHistory sem alterar
// nada.
func (r *Plans) DiscardUndelivered(ctx context.Context, documentID int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: iniciar descarte de planos do documento %d: %w", documentID, err)
	}
	defer tx.Rollback()

	var enviados, itens int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM chunks WHERE document_id = ? AND status <> 'pending'`,
		documentID).Scan(&enviados); err != nil {
		return fmt.Errorf("store: contar chunks entregues do documento %d: %w", documentID, err)
	}
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM delivery_items WHERE document_id = ?`,
		documentID).Scan(&itens); err != nil {
		return fmt.Errorf("store: contar entregas do documento %d: %w", documentID, err)
	}
	if enviados+itens > 0 {
		return fmt.Errorf("%w (documento %d)", ErrDeliveryHistory, documentID)
	}

	for _, q := range []string{
		`DELETE FROM chunks WHERE document_id = ?`,
		`DELETE FROM plans WHERE document_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, documentID); err != nil {
			return fmt.Errorf("store: descartar planos do documento %d: %w", documentID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: confirmar descarte de planos do documento %d: %w", documentID, err)
	}
	return nil
}

// validarCobertura confere que os chunks começam em início e são contíguos,
// sem gap nem overlap (DB-02/DB-09), antes de tocar no banco. O primeiro chunk
// precisa começar em `inicio` e o último precisa fechar o documento.
func validarCobertura(pl *plan.Plan, inicio int) error {
	for i, c := range pl.Chunks {
		if c.Seq != i {
			return fmt.Errorf("store: chunk %d com seq %d", i, c.Seq)
		}
		esperado := inicio
		if i > 0 {
			esperado = pl.Chunks[i-1].LastBlock + 1
		}
		if c.FirstBlock != esperado {
			return fmt.Errorf("store: chunk %d começa no bloco %d, esperado %d", i, c.FirstBlock, esperado)
		}
		if c.LastBlock < c.FirstBlock {
			return fmt.Errorf("store: chunk %d termina antes de começar", i)
		}
	}
	return nil
}

// validarLimites confere que os índices de bloco existem no documento. Os
// índices dos chunks são absolutos no documento (ver GenerateFrom).
func validarLimites(pl *plan.Plan, inicio, total int) error {
	limite := total
	for _, c := range pl.Chunks {
		if c.FirstBlock < inicio || c.LastBlock >= limite || c.FirstBlock > c.LastBlock {
			return fmt.Errorf("store: chunk %d referencia blocos %d..%d de %d (início %d)",
				c.Seq, c.FirstBlock, c.LastBlock, limite, inicio)
		}
	}
	if ultimo := pl.Chunks[len(pl.Chunks)-1]; ultimo.LastBlock != limite-1 {
		return fmt.Errorf("store: cobertura incompleta: termina no bloco %d, esperado %d",
			ultimo.LastBlock, limite-1)
	}
	return nil
}

func nullableInt(v int) any {
	if v <= 0 {
		return nil
	}
	return v
}

func nullableIntPtr(v int) *int {
	if v <= 0 {
		return nil
	}
	return &v
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func dataOpcional(s sql.NullString) (*time.Time, error) {
	if !s.Valid || s.String == "" {
		return nil, nil
	}
	t, err := parsearData(s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
