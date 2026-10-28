// Package store concentra o acesso ao SQLite: abertura, migrações e
// repositórios. Nenhum SQL vive fora deste pacote (ADR-0007).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store é o banco aberto, com as migrações aplicadas.
type Store struct {
	db *sql.DB
}

// Open abre (ou cria) o banco em path, aplica as migrações pendentes e
// devolve o Store pronto para uso.
func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("store: caminho do banco vazio")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("store: criar diretório %q: %w", dir, err)
		}
	}

	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("store: abrir %q: %w", path, err)
	}
	// WAL permite leitura durante escrita; quatro conexões bastam para a UI e
	// para o worker de extração.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(0)

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// dsn fixa os PRAGMAs de cada conexão. foreign_keys e busy_timeout são por
// conexão; journal_mode fica gravado no arquivo.
func dsn(path string) string {
	return path +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(ON)" +
		"&_pragma=busy_timeout(5000)"
}

// Close fecha o banco.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Ping verifica a conexão.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("store: ping: %w", err)
	}
	return nil
}

// DB expõe o handle para os repositórios do próprio pacote e para testes.
// Handlers não devem usá-lo.
func (s *Store) DB() *sql.DB { return s.db }
