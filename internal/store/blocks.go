package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/giovani/readingdrip/internal/blocks"
)

// Blocks agrupa as operações de blocks.
type Blocks struct {
	db *sql.DB
}

// Blocks devolve o repositório de blocos.
func (s *Store) Blocks() *Blocks { return &Blocks{db: s.db} }

// Replace troca todos os blocos de um documento em uma transação. Os
// heading_id são regravados na segunda passada, quando os ids já existem.
func (r *Blocks) Replace(ctx context.Context, documentID int64, lista []blocks.Block) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: iniciar troca de blocos: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM blocks WHERE document_id = ?`, documentID); err != nil {
		return fmt.Errorf("store: limpar blocos do documento %d: %w", documentID, err)
	}

	const inserir = `INSERT INTO blocks
		(document_id, ord, kind, level, heading_id, text, html, words, seconds, hash)
		VALUES (?, ?, ?, ?, NULL, ?, ?, ?, ?, ?)`

	ids := make([]int64, len(lista))
	for i, b := range lista {
		res, err := tx.ExecContext(ctx, inserir,
			documentID, b.Ord, string(b.Kind), b.Level, b.Text, b.HTML, b.Words, b.Seconds, b.Hash)
		if err != nil {
			return fmt.Errorf("store: inserir bloco %d do documento %d: %w", b.Ord, documentID, err)
		}
		if ids[i], err = res.LastInsertId(); err != nil {
			return fmt.Errorf("store: id do bloco %d do documento %d: %w", b.Ord, documentID, err)
		}
	}

	for i, b := range lista {
		if b.HeadingID < 0 || b.HeadingID >= len(lista) {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE blocks SET heading_id = ? WHERE id = ?`, ids[b.HeadingID], ids[i]); err != nil {
			return fmt.Errorf("store: ligar bloco %d ao heading %d: %w", b.Ord, b.HeadingID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: confirmar blocos do documento %d: %w", documentID, err)
	}
	return nil
}

// List devolve os blocos do documento em ordem, com heading_id convertido de
// id do banco para índice do bloco.
func (r *Blocks) List(ctx context.Context, documentID int64) ([]blocks.Block, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, ord, kind, level, heading_id, text, html, words, seconds, hash
		FROM blocks WHERE document_id = ? ORDER BY ord`, documentID)
	if err != nil {
		return nil, fmt.Errorf("store: listar blocos do documento %d: %w", documentID, err)
	}
	defer rows.Close()

	type registro struct {
		bloco     blocks.Block
		headingDB int64
		temHead   bool
	}
	var regs []registro
	indicePorID := make(map[int64]int)

	for rows.Next() {
		var (
			id        int64
			headingID sql.NullInt64
			kind      string
			reg       registro
		)
		if err := rows.Scan(&id, &reg.bloco.Ord, &kind, &reg.bloco.Level, &headingID,
			&reg.bloco.Text, &reg.bloco.HTML, &reg.bloco.Words, &reg.bloco.Seconds, &reg.bloco.Hash); err != nil {
			return nil, fmt.Errorf("store: ler bloco do documento %d: %w", documentID, err)
		}
		reg.bloco.Kind = blocks.Kind(kind)
		reg.bloco.HeadingID = -1
		reg.headingDB = headingID.Int64
		reg.temHead = headingID.Valid
		indicePorID[id] = reg.bloco.Ord
		regs = append(regs, reg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: listar blocos do documento %d: %w", documentID, err)
	}

	out := make([]blocks.Block, len(regs))
	for i, reg := range regs {
		if reg.temHead {
			if idx, ok := indicePorID[reg.headingDB]; ok {
				reg.bloco.HeadingID = idx
			}
		}
		out[i] = reg.bloco
	}
	return out, nil
}

// Between devolve os blocos cujos ids do banco estão no intervalo fechado
// [firstID, lastID], na ordem de `ord`. Os ids vêm de um chunk do plano ativo,
// então o intervalo é contíguo por construção (DB-02).
func (r *Blocks) Between(ctx context.Context, documentID, firstID, lastID int64) ([]blocks.Block, error) {
	ids, err := r.IDs(ctx, documentID)
	if err != nil {
		return nil, err
	}
	inicio, fim := -1, -1
	for i, id := range ids {
		if id == firstID {
			inicio = i
		}
		if id == lastID {
			fim = i
		}
	}
	if inicio < 0 || fim < inicio {
		return nil, fmt.Errorf("store: intervalo de blocos %d..%d inválido no documento %d",
			firstID, lastID, documentID)
	}

	lista, err := r.List(ctx, documentID)
	if err != nil {
		return nil, err
	}
	if fim >= len(lista) {
		return nil, fmt.Errorf("store: intervalo de blocos %d..%d fora do documento %d",
			firstID, lastID, documentID)
	}
	out := make([]blocks.Block, fim-inicio+1)
	copy(out, lista[inicio:fim+1])
	return out, nil
}

// Section devolve a seção que contém o bloco cujo heading é headingIndex (o
// campo Block.HeadingID): do heading até o próximo heading de nível igual ou
// superior, incluindo subseções. headingIndex < 0 devolve os blocos anteriores
// ao primeiro heading, que formam a seção de abertura.
func (r *Blocks) Section(ctx context.Context, documentID int64, headingIndex int) ([]blocks.Block, error) {
	lista, err := r.List(ctx, documentID)
	if err != nil {
		return nil, err
	}
	if len(lista) == 0 {
		return nil, nil
	}
	if headingIndex < 0 {
		out := make([]blocks.Block, 0, 8)
		for _, b := range lista {
			if b.HeadingID >= 0 {
				break
			}
			out = append(out, b)
		}
		return out, nil
	}
	if headingIndex >= len(lista) {
		return nil, fmt.Errorf("store: heading %d fora do documento %d", headingIndex, documentID)
	}
	if !lista[headingIndex].Kind.IsHeading() {
		return nil, fmt.Errorf("store: bloco %d do documento %d não é heading", headingIndex, documentID)
	}

	level := lista[headingIndex].Level
	fim := len(lista)
	for i := headingIndex + 1; i < len(lista); i++ {
		if lista[i].Kind.IsHeading() && lista[i].Level <= level {
			fim = i
			break
		}
	}
	return lista[headingIndex:fim], nil
}

// Count devolve quantos blocos o documento tem.
func (r *Blocks) Count(ctx context.Context, documentID int64) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM blocks WHERE document_id = ?`, documentID).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: contar blocos do documento %d: %w", documentID, err)
	}
	return n, nil
}

// IDs devolve os ids do banco na ordem de `ord`, para o repositório de planos
// ligar os chunks aos blocos.
func (r *Blocks) IDs(ctx context.Context, documentID int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id FROM blocks WHERE document_id = ? ORDER BY ord`, documentID)
	if err != nil {
		return nil, fmt.Errorf("store: ids dos blocos do documento %d: %w", documentID, err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: ler id de bloco do documento %d: %w", documentID, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ids dos blocos do documento %d: %w", documentID, err)
	}
	return ids, nil
}
