package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migracao é um arquivo SQL versionado pelo prefixo numérico do nome
// (0001_init.sql → versão 1).
type migracao struct {
	versao int
	nome   string
	sql    string
}

// migrate aplica, em ordem e uma vez, as migrações embutidas. Cada migração
// roda em transação junto com o registro em schema_migrations.
func (s *Store) migrate(ctx context.Context) error {
	if err := s.criarTabelaMigracoes(ctx); err != nil {
		return err
	}

	pendentes, err := migracoesPendentes(ctx, s.db)
	if err != nil {
		return err
	}

	for _, m := range pendentes {
		if err := s.aplicarMigracao(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) criarTabelaMigracoes(ctx context.Context) error {
	const q = `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	)`
	if _, err := s.db.ExecContext(ctx, q); err != nil {
		return fmt.Errorf("store: criar schema_migrations: %w", err)
	}
	return nil
}

func migracoesPendentes(ctx context.Context, db *sql.DB) ([]migracao, error) {
	todas, err := lerMigracoes()
	if err != nil {
		return nil, err
	}

	aplicadas := make(map[int]bool)
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("store: ler versões aplicadas: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("store: ler versão aplicada: %w", err)
		}
		aplicadas[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ler versões aplicadas: %w", err)
	}

	var pendentes []migracao
	for _, m := range todas {
		if !aplicadas[m.versao] {
			pendentes = append(pendentes, m)
		}
	}
	return pendentes, nil
}

// lerMigracoes carrega os arquivos embutidos ordenados por versão.
func lerMigracoes() ([]migracao, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("store: ler migrações: %w", err)
	}

	var out []migracao
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		versao, err := versaoDoArquivo(e.Name())
		if err != nil {
			return nil, err
		}
		conteudo, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("store: ler migração %q: %w", e.Name(), err)
		}
		out = append(out, migracao{versao: versao, nome: e.Name(), sql: string(conteudo)})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].versao < out[j].versao })
	for i := 1; i < len(out); i++ {
		if out[i].versao == out[i-1].versao {
			return nil, fmt.Errorf("store: versão de migração duplicada %d", out[i].versao)
		}
	}
	return out, nil
}

func versaoDoArquivo(nome string) (int, error) {
	prefixo, _, ok := strings.Cut(nome, "_")
	if !ok {
		return 0, fmt.Errorf("store: migração %q sem prefixo de versão", nome)
	}
	v, err := strconv.Atoi(prefixo)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("store: versão inválida em %q", nome)
	}
	return v, nil
}

func (s *Store) aplicarMigracao(ctx context.Context, m migracao) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: iniciar migração %s: %w", m.nome, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return fmt.Errorf("store: aplicar migração %s: %w", m.nome, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, m.versao); err != nil {
		return fmt.Errorf("store: registrar migração %s: %w", m.nome, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: confirmar migração %s: %w", m.nome, err)
	}
	return nil
}
