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
	"strings"

	_ "modernc.org/sqlite"
)

// Store é o banco aberto, com as migrações aplicadas.
type Store struct {
	db *sql.DB
}

// consulta é o subconjunto de *sql.DB e *sql.Tx usado pelos leitores que
// precisam rodar dentro de uma transação (edição de fronteiras, RF-023).
type consulta interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Options controla a abertura do banco. A zero value mantém o comportamento de
// Open: sem backup pré-migração.
type Options struct {
	// BackupDir, quando não vazio, recebe um backup do banco antes de aplicar
	// migrações pendentes (RF-092). O diretório é criado se preciso.
	BackupDir string
	// OnBackup é chamado com o caminho do backup depois que o arquivo está
	// completo. Serve para log; o callback não deve falhar.
	OnBackup func(path string)
}

// Open abre (ou cria) o banco em path, aplica as migrações pendentes e
// devolve o Store pronto para uso. Equivale a OpenWithOptions sem backup.
func Open(ctx context.Context, path string) (*Store, error) {
	return OpenWithOptions(ctx, path, Options{})
}

// OpenWithOptions abre (ou cria) o banco em path, faz o backup pré-migração
// quando configurado e aplica as migrações pendentes.
func OpenWithOptions(ctx context.Context, path string, opts Options) (*Store, error) {
	if path == "" {
		return nil, errors.New("store: caminho do banco vazio")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("store: criar diretório %q: %w", dir, err)
		}
	}

	// Um arquivo existente e não vazio tem dados a preservar; banco novo não
	// precisa de backup antes da primeira migração.
	existente, err := bancoExistente(path)
	if err != nil {
		return nil, err
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
	if err := s.migrate(ctx, opts, existente); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// bancoExistente informa se path já tem conteúdo. Arquivo vazio (ou ausente)
// é tratado como banco novo.
func bancoExistente(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: verificar banco %q: %w", path, err)
	}
	return info.Size() > 0, nil
}

// Backup grava uma cópia consistente do banco em dest com VACUUM INTO
// (ADR-0007). Se dest já existe, nada é feito: o arquivo só aparece depois de
// completo (gravação em temporário + rename), então um backup existente é um
// backup íntegro de uma tentativa anterior.
func (s *Store) Backup(ctx context.Context, dest string) error {
	if s == nil || s.db == nil {
		return errors.New("store: banco fechado")
	}
	if strings.TrimSpace(dest) == "" {
		return errors.New("store: destino de backup vazio")
	}
	if _, err := os.Stat(dest); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("store: verificar backup %q: %w", dest, err)
	}
	if dir := filepath.Dir(dest); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("store: criar diretório de backup %q: %w", dir, err)
		}
	}

	tmp := dest + ".tmp"
	if err := os.Remove(tmp); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("store: remover backup temporário %q: %w", tmp, err)
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, tmp); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("store: gravar backup %q: %w", dest, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("store: finalizar backup %q: %w", dest, err)
	}
	return nil
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
