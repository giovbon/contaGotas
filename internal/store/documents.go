package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DocumentStatus é o estado de processamento de um documento.
type DocumentStatus string

const (
	StatusProcessing DocumentStatus = "processing"
	StatusReady      DocumentStatus = "ready"
	StatusPaused     DocumentStatus = "paused"
	StatusCompleted  DocumentStatus = "completed"
	StatusFailed     DocumentStatus = "failed"
	StatusNeedsOCR   DocumentStatus = "needs_ocr"
)

// Document é a linha de documents. Campos opcionais usam ponteiro para
// distinguir ausência de valor vazio.
type Document struct {
	ID            int64
	Title         string
	Author        *string
	Language      string
	Format        string
	SourceName    string
	SourcePath    string
	SourceSHA256  string
	CoverPath     *string
	Status        DocumentStatus
	PacingMode    string
	PacingMinutes int
	PacingDays    *int
	WPM           int
	Difficulty    float64
	Priority      int
	TotalWords    int
	TotalSeconds  int
	Error         *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Documents agrupa as operações de documents.
type Documents struct {
	db *sql.DB
}

// Documents devolve o repositório de documentos.
func (s *Store) Documents() *Documents { return &Documents{db: s.db} }

const colunasDocumento = `id, title, author, language, format, source_name,
	source_path, source_sha256, cover_path, status, pacing_mode, pacing_minutes,
	pacing_days, wpm, difficulty, priority, total_words, total_seconds, error,
	created_at, updated_at`

// Create insere um documento e devolve o id gerado.
func (r *Documents) Create(ctx context.Context, d Document) (int64, error) {
	if d.Title == "" {
		return 0, errors.New("store: documento sem título")
	}
	if d.CreatedAt.IsZero() || d.UpdatedAt.IsZero() {
		return 0, errors.New("store: documento sem created_at/updated_at")
	}
	const q = `INSERT INTO documents
		(title, author, language, format, source_name, source_path, source_sha256,
		 cover_path, status, pacing_mode, pacing_minutes, pacing_days, wpm,
		 difficulty, priority, total_words, total_seconds, error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	res, err := r.db.ExecContext(ctx, q,
		d.Title, d.Author, defaultString(d.Language, "pt-BR"), d.Format,
		d.SourceName, d.SourcePath, d.SourceSHA256, d.CoverPath,
		defaultStatus(d.Status), defaultString(d.PacingMode, "minutes_per_day"),
		defaultInt(d.PacingMinutes, 10), d.PacingDays, defaultInt(d.WPM, 200),
		defaultFloat(d.Difficulty, 1.0), defaultInt(d.Priority, 100),
		d.TotalWords, d.TotalSeconds, d.Error, formatarData(d.CreatedAt), formatarData(d.UpdatedAt),
	)
	if err != nil {
		return 0, fmt.Errorf("store: criar documento %q: %w", d.Title, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: id do documento %q: %w", d.Title, err)
	}
	return id, nil
}

// Get busca um documento pelo id. Devolve nil sem erro quando não existe.
func (r *Documents) Get(ctx context.Context, id int64) (*Document, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+colunasDocumento+` FROM documents WHERE id = ?`, id)
	d, err := lerDocumento(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: buscar documento %d: %w", id, err)
	}
	return d, nil
}

// BySHA256 busca pelo hash do arquivo original (deduplicação).
func (r *Documents) BySHA256(ctx context.Context, sha string) (*Document, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+colunasDocumento+` FROM documents WHERE source_sha256 = ?`, sha)
	d, err := lerDocumento(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: buscar documento por sha256: %w", err)
	}
	return d, nil
}

// List devolve todos os documentos na ordem de prioridade.
func (r *Documents) List(ctx context.Context) ([]Document, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+colunasDocumento+` FROM documents ORDER BY priority, id`)
	if err != nil {
		return nil, fmt.Errorf("store: listar documentos: %w", err)
	}
	defer rows.Close()

	var out []Document
	for rows.Next() {
		d, err := lerDocumento(rows)
		if err != nil {
			return nil, fmt.Errorf("store: ler documento: %w", err)
		}
		out = append(out, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: listar documentos: %w", err)
	}
	return out, nil
}

// UpdateStatus muda o status e atualiza updated_at.
func (r *Documents) UpdateStatus(ctx context.Context, id int64, status DocumentStatus, agora time.Time) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE documents SET status = ?, updated_at = ? WHERE id = ?`,
		string(status), formatarData(agora), id)
	if err != nil {
		return fmt.Errorf("store: atualizar status do documento %d: %w", id, err)
	}
	return exigirLinhaAfetada(res, "status do documento")
}

// UpdateTotais grava as contagens calculadas pela extração.
func (r *Documents) UpdateTotais(ctx context.Context, id int64, palavras, segundos int, agora time.Time) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE documents SET total_words = ?, total_seconds = ?, updated_at = ? WHERE id = ?`,
		palavras, segundos, formatarData(agora), id)
	if err != nil {
		return fmt.Errorf("store: atualizar totais do documento %d: %w", id, err)
	}
	return exigirLinhaAfetada(res, "totais do documento")
}

// UpdateSource grava o nome e o caminho definitivos do arquivo original,
// depois que o upload termina de mover o temporário para o destino.
func (r *Documents) UpdateSource(ctx context.Context, id int64, nome, caminho string, agora time.Time) error {
	if nome == "" || caminho == "" {
		return errors.New("store: origem do documento sem nome/caminho")
	}
	if agora.IsZero() {
		return errors.New("store: origem do documento sem data")
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE documents SET source_name = ?, source_path = ?, updated_at = ? WHERE id = ?`,
		nome, caminho, formatarData(agora), id)
	if err != nil {
		return fmt.Errorf("store: atualizar origem do documento %d: %w", id, err)
	}
	return exigirLinhaAfetada(res, "origem do documento")
}

// Delete remove o documento e os dados derivados (itens de entrega, chunks,
// planos e blocos) em uma transação. O chamador remove os arquivos em disco.
func (r *Documents) Delete(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: iniciar exclusão do documento %d: %w", id, err)
	}
	defer tx.Rollback()

	for _, q := range []string{
		`DELETE FROM delivery_items WHERE document_id = ?`,
		`DELETE FROM chunks WHERE document_id = ?`,
		`DELETE FROM plans WHERE document_id = ?`,
		`DELETE FROM blocks WHERE document_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return fmt.Errorf("store: excluir dados derivados do documento %d: %w", id, err)
		}
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM documents WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: excluir documento %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: excluir documento %d: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("store: excluir documento %d: %w", id, ErrDocumentNotFound)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: confirmar exclusão do documento %d: %w", id, err)
	}
	return nil
}

// ErrDocumentNotFound indica que o documento não existe.
var ErrDocumentNotFound = errors.New("store: documento não encontrado")

// Extraction é o resultado da extração que fica gravado no documento.
// Título, autor e idioma vazios preservam o valor atual (a extração não apaga
// metadados editados pelo usuário). Status é obrigatório e Error vazio limpa
// o erro anterior.
type Extraction struct {
	Title        string
	Author       string
	Language     string
	TotalWords   int
	TotalSeconds int
	Status       DocumentStatus
	Error        string
}

// SaveExtraction grava metadados, totais e status em uma única instrução.
func (r *Documents) SaveExtraction(ctx context.Context, id int64, e Extraction, agora time.Time) error {
	if !e.Status.valid() {
		return fmt.Errorf("store: status de documento inválido: %q", e.Status)
	}
	if agora.IsZero() {
		return errors.New("store: extração sem data")
	}

	res, err := r.db.ExecContext(ctx, `
		UPDATE documents SET
			title         = COALESCE(NULLIF(?, ''), title),
			author        = COALESCE(NULLIF(?, ''), author),
			language      = COALESCE(NULLIF(?, ''), language),
			total_words   = ?,
			total_seconds = ?,
			status        = ?,
			error         = NULLIF(?, ''),
			updated_at    = ?
		WHERE id = ?`,
		e.Title, e.Author, e.Language, e.TotalWords, e.TotalSeconds,
		string(e.Status), e.Error, formatarData(agora), id)
	if err != nil {
		return fmt.Errorf("store: gravar extração do documento %d: %w", id, err)
	}
	return exigirLinhaAfetada(res, "extração do documento")
}

func (s DocumentStatus) valid() bool {
	switch s {
	case StatusProcessing, StatusReady, StatusPaused, StatusCompleted, StatusFailed, StatusNeedsOCR:
		return true
	}
	return false
}

// linha é o que QueryRow e Rows têm em comum.
type linha interface {
	Scan(dest ...any) error
}

func lerDocumento(l linha) (*Document, error) {
	var d Document
	var author, cover, erro sql.NullString
	var dias sql.NullInt64
	var criado, atualizado string

	err := l.Scan(
		&d.ID, &d.Title, &author, &d.Language, &d.Format, &d.SourceName,
		&d.SourcePath, &d.SourceSHA256, &cover, &d.Status, &d.PacingMode,
		&d.PacingMinutes, &dias, &d.WPM, &d.Difficulty, &d.Priority,
		&d.TotalWords, &d.TotalSeconds, &erro, &criado, &atualizado,
	)
	if err != nil {
		return nil, err
	}

	d.Author = textoOpcional(author)
	d.CoverPath = textoOpcional(cover)
	d.Error = textoOpcional(erro)
	if dias.Valid {
		v := int(dias.Int64)
		d.PacingDays = &v
	}
	d.CreatedAt, err = parsearData(criado)
	if err != nil {
		return nil, err
	}
	d.UpdatedAt, err = parsearData(atualizado)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func textoOpcional(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	v := s.String
	return &v
}

func formatarData(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func parsearData(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("store: data inválida %q: %w", s, err)
	}
	return t, nil
}

func exigirLinhaAfetada(res sql.Result, oque string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: %s: %w", oque, err)
	}
	if n == 0 {
		return fmt.Errorf("store: %s: nenhuma linha afetada", oque)
	}
	return nil
}

func defaultString(v, padrao string) string {
	if v == "" {
		return padrao
	}
	return v
}

func defaultStatus(v DocumentStatus) DocumentStatus {
	if v == "" {
		return StatusProcessing
	}
	return v
}

func defaultInt(v, padrao int) int {
	if v == 0 {
		return padrao
	}
	return v
}

func defaultFloat(v, padrao float64) float64 {
	if v == 0 {
		return padrao
	}
	return v
}
