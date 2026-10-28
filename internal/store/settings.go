package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Settings guarda pares chave/valor da configuração.
type Settings struct {
	db *sql.DB
}

// Settings devolve o repositório de configurações.
func (s *Store) Settings() *Settings { return &Settings{db: s.db} }

// Get devolve o valor e se a chave existe.
func (r *Settings) Get(ctx context.Context, chave string) (string, bool, error) {
	var valor string
	err := r.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, chave).Scan(&valor)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: ler configuração %q: %w", chave, err)
	}
	return valor, true, nil
}

// Set grava (ou substitui) um valor.
func (r *Settings) Set(ctx context.Context, chave, valor string, agora time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		chave, valor, formatarData(agora))
	if err != nil {
		return fmt.Errorf("store: gravar configuração %q: %w", chave, err)
	}
	return nil
}

// SetMany grava vários pares na mesma transação: ou todos entram, ou nada
// muda. Chave com valor vazio é gravada como vazia (equivale a "não
// configurada" para quem lê).
func (r *Settings) SetMany(ctx context.Context, valores map[string]string, agora time.Time) error {
	if len(valores) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: iniciar gravação de configurações: %w", err)
	}
	defer tx.Rollback()

	for chave, valor := range valores {
		if strings.TrimSpace(chave) == "" {
			return errors.New("store: configuração sem chave")
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			chave, valor, formatarData(agora)); err != nil {
			return fmt.Errorf("store: gravar configuração %q: %w", chave, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: confirmar configurações: %w", err)
	}
	return nil
}

// All devolve todas as configurações em um mapa.
func (r *Settings) All(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, fmt.Errorf("store: listar configurações: %w", err)
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("store: ler configuração: %w", err)
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: listar configurações: %w", err)
	}
	return out, nil
}
