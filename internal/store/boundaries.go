package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/giovani/readingdrip/internal/blocks"
	"github.com/giovani/readingdrip/internal/plan"
)

// BoundaryAction é a operação de edição de fronteira (RF-023).
type BoundaryAction string

const (
	BoundaryMerge BoundaryAction = "merge"
	BoundarySplit BoundaryAction = "split"
	BoundaryMove  BoundaryAction = "move"
)

// MoveDirection diz de qual vizinho o chunk editado recebe blocos.
type MoveDirection string

const (
	MoveNext MoveDirection = "next" // recebe do início do próximo
	MovePrev MoveDirection = "prev" // recebe do fim do anterior
)

// BoundaryEdit é o pedido de edição de fronteira sobre um chunk pendente.
type BoundaryEdit struct {
	Action    BoundaryAction
	Block     int           // split: índice absoluto do primeiro bloco do novo trecho
	Direction MoveDirection // move: next (padrão) ou prev
	Blocks    int           // move: quantidade (padrão 1)
}

// BoundaryResult resume a edição aplicada ao plano ativo.
type BoundaryResult struct {
	Plan   *Plan
	Action BoundaryAction
}

var (
	// ErrBoundaryChunkNotFound indica um chunk inexistente.
	ErrBoundaryChunkNotFound = errors.New("store: chunk não encontrado")
	// ErrBoundaryPlanNotActive indica que o chunk não pertence ao plano ativo.
	ErrBoundaryPlanNotActive = errors.New("store: plano não está ativo")
	// ErrBoundaryChunkNotPending indica chunk já enviado/pulado, que não pode
	// ser reescrito (DB-08).
	ErrBoundaryChunkNotPending = errors.New("store: chunk não está pendente")
	// ErrBoundaryInvalid indica uma operação que quebraria as invariantes do
	// plano (intervalo, heading, vizinho, cobertura).
	ErrBoundaryInvalid = errors.New("store: fronteira inválida")
)

// EditBoundary muta o plano ativo no lugar: mescla um chunk com o próximo,
// divide um chunk antes de um bloco ou move blocos entre vizinhos pendentes.
//
// A operação lê chunk, plano, blocos e vizinhos dentro da mesma transação,
// recalcula as pontas e métricas e revalida o plano inteiro com
// plan.ValidateFrom antes do commit. Chunks enviados/pulados nunca têm conteúdo
// reescrito; no máximo o `seq` é renumerado quando a edição desloca a ordem
// (DB-08, I-01…I-05).
func (r *Plans) EditBoundary(ctx context.Context, chunkID int64, edit BoundaryEdit) (*BoundaryResult, error) {
	if chunkID <= 0 {
		return nil, fmt.Errorf("%w: id %d", ErrBoundaryChunkNotFound, chunkID)
	}
	switch edit.Action {
	case BoundaryMerge, BoundarySplit, BoundaryMove:
	default:
		return nil, fmt.Errorf("%w: ação %q", ErrBoundaryInvalid, edit.Action)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: iniciar edição de fronteira: %w", err)
	}
	defer tx.Rollback()

	alvo, err := chunkEm(ctx, tx, chunkID)
	if err != nil {
		return nil, err
	}
	if alvo == nil {
		return nil, fmt.Errorf("%w: %d", ErrBoundaryChunkNotFound, chunkID)
	}
	plano, err := planoEm(ctx, tx, alvo.PlanID)
	if err != nil {
		return nil, err
	}
	if plano == nil || plano.Status != PlanActive {
		return nil, fmt.Errorf("%w: plano %d", ErrBoundaryPlanNotActive, alvo.PlanID)
	}

	chunks, err := listarChunksDoPlano(ctx, tx, plano.ID)
	if err != nil {
		return nil, err
	}
	ids, err := idsDeBlocos(ctx, tx, plano.DocumentID)
	if err != nil {
		return nil, err
	}
	bs, err := listarBlocos(ctx, tx, plano.DocumentID)
	if err != nil {
		return nil, err
	}
	if len(bs) == 0 || len(ids) != len(bs) {
		return nil, fmt.Errorf("%w: documento %d sem blocos consistentes",
			ErrBoundaryInvalid, plano.DocumentID)
	}

	indice := make(map[int64]int, len(ids))
	for i, id := range ids {
		indice[id] = i
	}
	pos := -1
	for i := range chunks {
		if chunks[i].ID == chunkID {
			pos = i
			break
		}
	}
	if pos < 0 {
		return nil, fmt.Errorf("%w: %d", ErrBoundaryChunkNotFound, chunkID)
	}
	if chunks[pos].Status != ChunkPending {
		return nil, fmt.Errorf("%w: chunk %d está %q",
			ErrBoundaryChunkNotPending, chunkID, chunks[pos].Status)
	}

	maxSegundos := maxDoPlano(plano)
	recalculados := make(map[int64]bool, 3)

	primeiro, ultimo, err := intervaloDoChunk(chunks[pos], indice, len(bs))
	if err != nil {
		return nil, err
	}

	var resultado []Chunk
	switch edit.Action {
	case BoundaryMerge:
		resultado, err = mesclarChunks(chunks, pos, primeiro, ultimo, indice, ids, bs, plano.TargetSeconds, maxSegundos, recalculados)
	case BoundarySplit:
		resultado, err = dividirChunk(chunks, pos, primeiro, ultimo, edit.Block, indice, ids, bs, plano.TargetSeconds, maxSegundos, recalculados)
	case BoundaryMove:
		resultado, err = moverBlocos(chunks, pos, primeiro, ultimo, edit, indice, ids, bs, plano.TargetSeconds, maxSegundos, recalculados)
	}
	if err != nil {
		return nil, err
	}

	for i := range resultado {
		resultado[i].Seq = i
	}
	if err := validarPlanoEditado(bs, plano, resultado, indice); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBoundaryInvalid, err)
	}
	if err := persistirEdicao(ctx, tx, plano, chunks, resultado, recalculados); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: confirmar edição do plano %d: %w", plano.ID, err)
	}

	atualizado, err := r.Active(ctx, plano.DocumentID)
	if err != nil {
		return nil, err
	}
	return &BoundaryResult{Plan: atualizado, Action: edit.Action}, nil
}

// mesclarChunks junta o chunk em pos com o próximo. O primeiro mantém o id; o
// vizinho é removido. Os dois precisam estar pendentes.
func mesclarChunks(chunks []Chunk, pos, primeiro, ultimo int, indice map[int64]int,
	ids []int64, bs []blocks.Block, alvo, maxSegundos int, recalculados map[int64]bool) ([]Chunk, error) {
	if pos+1 >= len(chunks) {
		return nil, fmt.Errorf("%w: não há próximo chunk", ErrBoundaryInvalid)
	}
	proximo := chunks[pos+1]
	if proximo.Status != ChunkPending {
		return nil, fmt.Errorf("%w: próximo chunk %d está %q",
			ErrBoundaryChunkNotPending, proximo.ID, proximo.Status)
	}
	_, fim, err := intervaloDoChunk(proximo, indice, len(bs))
	if err != nil {
		return nil, err
	}

	novo := chunks[pos]
	recalcularChunk(&novo, primeiro, fim, ids, bs, alvo, maxSegundos)
	recalculados[novo.ID] = true

	resultado := make([]Chunk, 0, len(chunks)-1)
	resultado = append(resultado, chunks[:pos]...)
	resultado = append(resultado, novo)
	resultado = append(resultado, chunks[pos+2:]...)
	return resultado, nil
}

// dividirChunk cria um segundo chunk começando no bloco `bloco` (absoluto). O
// primeiro pedaço mantém o id; o segundo é novo e nasce pendente.
func dividirChunk(chunks []Chunk, pos, primeiro, ultimo, bloco int, indice map[int64]int,
	ids []int64, bs []blocks.Block, alvo, maxSegundos int, recalculados map[int64]bool) ([]Chunk, error) {
	if bloco <= primeiro || bloco > ultimo {
		return nil, fmt.Errorf("%w: bloco %d fora do chunk [%d..%d]",
			ErrBoundaryInvalid, bloco, primeiro, ultimo)
	}
	if bs[bloco-1].Kind.IsHeading() {
		return nil, fmt.Errorf("%w: o primeiro trecho terminaria no título do bloco %d",
			ErrBoundaryInvalid, bloco-1)
	}

	primeiraParte := chunks[pos]
	recalcularChunk(&primeiraParte, primeiro, bloco-1, ids, bs, alvo, maxSegundos)
	recalculados[primeiraParte.ID] = true

	segundaParte := chunks[pos]
	segundaParte.ID = 0
	segundaParte.Status = ChunkPending
	segundaParte.SentAt = nil
	segundaParte.DoneAt = nil
	recalcularChunk(&segundaParte, bloco, ultimo, ids, bs, alvo, maxSegundos)

	resultado := make([]Chunk, 0, len(chunks)+1)
	resultado = append(resultado, chunks[:pos]...)
	resultado = append(resultado, primeiraParte, segundaParte)
	resultado = append(resultado, chunks[pos+1:]...)
	return resultado, nil
}

// moverBlocos desloca a fronteira entre o chunk em pos e um vizinho pendente.
// Em `next`, o chunk cresce recebendo blocos do início do próximo; em `prev`,
// recebe do fim do anterior. O vizinho nunca fica vazio.
func moverBlocos(chunks []Chunk, pos, primeiro, ultimo int, edit BoundaryEdit,
	indice map[int64]int, ids []int64, bs []blocks.Block, alvo, maxSegundos int,
	recalculados map[int64]bool) ([]Chunk, error) {
	direcao := edit.Direction
	if direcao == "" {
		direcao = MoveNext
	}
	quantidade := edit.Blocks
	if quantidade == 0 {
		quantidade = 1
	}
	if quantidade < 1 {
		return nil, fmt.Errorf("%w: quantidade de blocos inválida (%d)",
			ErrBoundaryInvalid, quantidade)
	}

	resultado := make([]Chunk, len(chunks))
	copy(resultado, chunks)

	switch direcao {
	case MoveNext:
		if pos+1 >= len(resultado) {
			return nil, fmt.Errorf("%w: não há próximo chunk", ErrBoundaryInvalid)
		}
		vizinho := resultado[pos+1]
		if vizinho.Status != ChunkPending {
			return nil, fmt.Errorf("%w: próximo chunk %d está %q",
				ErrBoundaryChunkNotPending, vizinho.ID, vizinho.Status)
		}
		vizPrimeiro, vizUltimo, err := intervaloDoChunk(vizinho, indice, len(bs))
		if err != nil {
			return nil, err
		}
		if quantidade >= vizUltimo-vizPrimeiro+1 {
			return nil, fmt.Errorf("%w: o próximo chunk ficaria vazio", ErrBoundaryInvalid)
		}
		novoFim := vizPrimeiro + quantidade - 1
		if bs[novoFim].Kind.IsHeading() {
			return nil, fmt.Errorf("%w: o chunk terminaria no título do bloco %d",
				ErrBoundaryInvalid, novoFim)
		}
		recalcularChunk(&resultado[pos], primeiro, novoFim, ids, bs, alvo, maxSegundos)
		recalcularChunk(&resultado[pos+1], novoFim+1, vizUltimo, ids, bs, alvo, maxSegundos)
		recalculados[resultado[pos].ID] = true
		recalculados[resultado[pos+1].ID] = true

	case MovePrev:
		if pos == 0 {
			return nil, fmt.Errorf("%w: não há chunk anterior", ErrBoundaryInvalid)
		}
		vizinho := resultado[pos-1]
		if vizinho.Status != ChunkPending {
			return nil, fmt.Errorf("%w: chunk anterior %d está %q",
				ErrBoundaryChunkNotPending, vizinho.ID, vizinho.Status)
		}
		vizPrimeiro, vizUltimo, err := intervaloDoChunk(vizinho, indice, len(bs))
		if err != nil {
			return nil, err
		}
		if quantidade >= vizUltimo-vizPrimeiro+1 {
			return nil, fmt.Errorf("%w: o chunk anterior ficaria vazio", ErrBoundaryInvalid)
		}
		novoInicio := primeiro - quantidade
		if bs[novoInicio-1].Kind.IsHeading() {
			return nil, fmt.Errorf("%w: o chunk anterior terminaria no título do bloco %d",
				ErrBoundaryInvalid, novoInicio-1)
		}
		recalcularChunk(&resultado[pos-1], vizPrimeiro, novoInicio-1, ids, bs, alvo, maxSegundos)
		recalcularChunk(&resultado[pos], novoInicio, ultimo, ids, bs, alvo, maxSegundos)
		recalculados[resultado[pos-1].ID] = true
		recalculados[resultado[pos].ID] = true

	default:
		return nil, fmt.Errorf("%w: direção %q", ErrBoundaryInvalid, direcao)
	}
	return resultado, nil
}

// recalcularChunk reescreve pontas, hashes, seção, flags e métricas a partir
// dos blocos reais; `oversized` acompanha o máximo do plano.
func recalcularChunk(c *Chunk, primeiro, ultimo int, ids []int64,
	bs []blocks.Block, alvo, maxSegundos int) {
	c.FirstBlockID = ids[primeiro]
	c.LastBlockID = ids[ultimo]
	c.FirstHash = bs[primeiro].Hash
	c.LastHash = bs[ultimo].Hash
	c.SectionPath = blocks.SectionPath(bs, primeiro)
	c.StartsMidSection = primeiro != 0 && !bs[primeiro].Kind.IsHeading()
	c.EndsMidSection = ultimo+1 < len(bs) && !bs[ultimo+1].Kind.IsHeading()
	c.TargetSeconds = alvo
	words, seconds := 0, 0
	for i := primeiro; i <= ultimo; i++ {
		words += bs[i].Words
		seconds += bs[i].Seconds
	}
	c.Words, c.Seconds = words, seconds
	c.Oversized = seconds > maxSegundos
}

// intervaloDoChunk traduz as pontas persistidas (ids de bloco) para índices
// absolutos no documento.
func intervaloDoChunk(c Chunk, indice map[int64]int, total int) (int, int, error) {
	primeiro, ok := indice[c.FirstBlockID]
	if !ok {
		return 0, 0, fmt.Errorf("%w: bloco %d do chunk %d não existe",
			ErrBoundaryInvalid, c.FirstBlockID, c.ID)
	}
	ultimo, ok := indice[c.LastBlockID]
	if !ok {
		return 0, 0, fmt.Errorf("%w: bloco %d do chunk %d não existe",
			ErrBoundaryInvalid, c.LastBlockID, c.ID)
	}
	if primeiro > ultimo || ultimo >= total {
		return 0, 0, fmt.Errorf("%w: intervalo [%d..%d] do chunk %d",
			ErrBoundaryInvalid, primeiro, ultimo, c.ID)
	}
	return primeiro, ultimo, nil
}

// validarPlanoEditado roda as invariantes do segmentador sobre o plano inteiro,
// inclusive os chunks já enviados/pulados e a cobertura parcial de um plano
// regenerado a partir do cursor.
func validarPlanoEditado(bs []blocks.Block, plano *Plan, chunks []Chunk, indice map[int64]int) error {
	if len(chunks) == 0 {
		return fmt.Errorf("%w: plano sem chunks", ErrBoundaryInvalid)
	}
	pp := &plan.Plan{
		Mode:          plan.Mode(plano.PacingMode),
		TargetSeconds: plano.TargetSeconds,
	}
	for _, c := range chunks {
		primeiro, ultimo, err := intervaloDoChunk(c, indice, len(bs))
		if err != nil {
			return err
		}
		pp.Chunks = append(pp.Chunks, plan.Chunk{
			Seq:              c.Seq,
			FirstBlock:       primeiro,
			LastBlock:        ultimo,
			FirstHash:        c.FirstHash,
			LastHash:         c.LastHash,
			SectionPath:      c.SectionPath,
			StartsMidSection: c.StartsMidSection,
			EndsMidSection:   c.EndsMidSection,
			Oversized:        c.Oversized,
			Words:            c.Words,
			Seconds:          c.Seconds,
			TargetSeconds:    c.TargetSeconds,
		})
		pp.TotalSeconds += c.Seconds
		pp.TotalWords += c.Words
	}
	inicio, _, err := intervaloDoChunk(chunks[0], indice, len(bs))
	if err != nil {
		return err
	}
	return plan.ValidateFrom(bs, pp, plano.Params, inicio)
}

// persistirEdicao grava o resultado na transação. O seq é deslocado antes para
// não colidir com UNIQUE(plan_id, seq) durante a renumeração. Chunks intocados
// só têm o seq atualizado; os recalculados têm as fronteiras e métricas; os
// novos são inseridos como pendentes.
func persistirEdicao(ctx context.Context, tx *sql.Tx, plano *Plan, antes, depois []Chunk,
	recalculados map[int64]bool) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE chunks SET seq = seq + 1000000 WHERE plan_id = ?`, plano.ID); err != nil {
		return fmt.Errorf("store: deslocar seq do plano %d: %w", plano.ID, err)
	}

	restantes := make(map[int64]bool, len(depois))
	for _, c := range depois {
		if c.ID != 0 {
			restantes[c.ID] = true
		}
	}
	for _, c := range antes {
		if restantes[c.ID] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM chunks WHERE id = ? AND plan_id = ?`,
			c.ID, plano.ID); err != nil {
			return fmt.Errorf("store: remover chunk %d: %w", c.ID, err)
		}
	}

	const atualizar = `UPDATE chunks SET seq = ?, first_block_id = ?, last_block_id = ?,
		first_hash = ?, last_hash = ?, section_path = ?, starts_mid_section = ?,
		ends_mid_section = ?, oversized = ?, words = ?, seconds = ?, target_seconds = ?
		WHERE id = ? AND plan_id = ?`

	const inserir = `INSERT INTO chunks
		(plan_id, document_id, seq, first_block_id, last_block_id, first_hash, last_hash,
		 section_path, starts_mid_section, ends_mid_section, oversized, words, seconds,
		 target_seconds, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending')`

	for _, c := range depois {
		sectionPath, err := json.Marshal(c.SectionPath)
		if err != nil {
			return fmt.Errorf("store: serializar seção do chunk %d: %w", c.ID, err)
		}
		if c.ID == 0 {
			if _, err := tx.ExecContext(ctx, inserir,
				plano.ID, plano.DocumentID, c.Seq, c.FirstBlockID, c.LastBlockID,
				c.FirstHash, c.LastHash, string(sectionPath),
				boolInt(c.StartsMidSection), boolInt(c.EndsMidSection), boolInt(c.Oversized),
				c.Words, c.Seconds, c.TargetSeconds); err != nil {
				return fmt.Errorf("store: inserir chunk novo no plano %d: %w", plano.ID, err)
			}
			continue
		}
		if !recalculados[c.ID] {
			if _, err := tx.ExecContext(ctx,
				`UPDATE chunks SET seq = ? WHERE id = ? AND plan_id = ?`,
				c.Seq, c.ID, plano.ID); err != nil {
				return fmt.Errorf("store: renumerar chunk %d: %w", c.ID, err)
			}
			continue
		}
		res, err := tx.ExecContext(ctx, atualizar,
			c.Seq, c.FirstBlockID, c.LastBlockID, c.FirstHash, c.LastHash,
			string(sectionPath), boolInt(c.StartsMidSection), boolInt(c.EndsMidSection),
			boolInt(c.Oversized), c.Words, c.Seconds, c.TargetSeconds, c.ID, plano.ID)
		if err != nil {
			return fmt.Errorf("store: atualizar chunk %d: %w", c.ID, err)
		}
		if err := exigirLinhaAfetada(res, "chunk editado"); err != nil {
			return err
		}
	}
	return nil
}

// maxDoPlano aplica max_overshoot ao alvo persistido. Mesma regra de
// plan.Params.bounds, sem depender do total de segundos do documento.
func maxDoPlano(plano *Plan) int {
	overshoot := plano.Params.MaxOvershoot
	if overshoot < 1 {
		overshoot = 1.4
	}
	maximo := int(float64(plano.TargetSeconds) * overshoot)
	if maximo < plano.TargetSeconds {
		maximo = plano.TargetSeconds
	}
	return maximo
}

// chunkEm busca um chunk dentro da transação.
func chunkEm(ctx context.Context, q consulta, chunkID int64) (*Chunk, error) {
	row := q.QueryRowContext(ctx, `
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

// planoEm busca um plano dentro da transação.
func planoEm(ctx context.Context, q consulta, planID int64) (*Plan, error) {
	row := q.QueryRowContext(ctx, `
		SELECT id, document_id, version, status, pacing_mode, target_seconds, horizon_days,
		       params_json, created_at
		FROM plans WHERE id = ?`, planID)
	p, err := lerPlan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: buscar plano %d: %w", planID, err)
	}
	return p, nil
}
