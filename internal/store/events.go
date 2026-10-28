package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Níveis de evento aceitos pelo schema.
const (
	EventDebug = "debug"
	EventInfo  = "info"
	EventWarn  = "warn"
	EventError = "error"
)

// Event é uma linha de events.
type Event struct {
	ID         int64
	At         time.Time
	Level      string
	Kind       string
	DocumentID *int64
	Message    string
}

// Events agrupa as operações de events.
type Events struct {
	db *sql.DB
}

// Events devolve o repositório de eventos.
func (s *Store) Events() *Events { return &Events{db: s.db} }

// Add registra um evento.
func (r *Events) Add(ctx context.Context, e Event) error {
	if e.At.IsZero() {
		return fmt.Errorf("store: evento sem data")
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO events (at, level, kind, document_id, message) VALUES (?, ?, ?, ?, ?)`,
		formatarData(e.At), e.Level, e.Kind, e.DocumentID, e.Message)
	if err != nil {
		return fmt.Errorf("store: registrar evento %q: %w", e.Kind, err)
	}
	return nil
}

// List devolve os eventos mais recentes, até limit.
func (r *Events) List(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, at, level, kind, document_id, message FROM events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: listar eventos: %w", err)
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var (
			e     Event
			at    string
			docID sql.NullInt64
		)
		if err := rows.Scan(&e.ID, &at, &e.Level, &e.Kind, &docID, &e.Message); err != nil {
			return nil, fmt.Errorf("store: ler evento: %w", err)
		}
		if e.At, err = parsearData(at); err != nil {
			return nil, err
		}
		if docID.Valid {
			v := docID.Int64
			e.DocumentID = &v
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: listar eventos: %w", err)
	}
	return out, nil
}
