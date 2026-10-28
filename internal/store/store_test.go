package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func abrirTeste(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("fechar banco: %v", err)
		}
	})
	return s
}

func TestOpenAplicaMigracoes(t *testing.T) {
	s := abrirTeste(t)

	tabelas := []string{
		"documents", "blocks", "plans", "chunks",
		"delivery_runs", "delivery_items", "settings", "events",
	}
	for _, tabela := range tabelas {
		var n int
		err := s.DB().QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tabela,
		).Scan(&n)
		if err != nil {
			t.Fatalf("consultar tabela %q: %v", tabela, err)
		}
		if n != 1 {
			t.Errorf("tabela %q ausente", tabela)
		}
	}

	var versao int
	if err := s.DB().QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&versao); err != nil {
		t.Fatal(err)
	}
	if versao != 1 {
		t.Errorf("versão = %d, quer 1", versao)
	}

	var aplicada string
	if err := s.DB().QueryRow(`SELECT applied_at FROM schema_migrations WHERE version = 1`).Scan(&aplicada); err != nil {
		t.Fatal(err)
	}
	if aplicada == "" {
		t.Error("applied_at vazio")
	}
}

func TestOpenCriaDiretorio(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "app.db")
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("abrir banco em diretório novo: %v", err)
	}
	defer s.Close()
	if err := s.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestOpenCaminhoVazio(t *testing.T) {
	if _, err := Open(context.Background(), ""); err == nil {
		t.Fatal("esperado erro para caminho vazio")
	}
}

func TestMigracoesIdempotentes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")

	primeiro, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	var antes string
	if err := primeiro.DB().QueryRow(`SELECT applied_at FROM schema_migrations WHERE version = 1`).Scan(&antes); err != nil {
		t.Fatal(err)
	}
	if err := primeiro.Close(); err != nil {
		t.Fatal(err)
	}

	segundo, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("reabrir banco: %v", err)
	}
	defer segundo.Close()

	var total int
	if err := segundo.DB().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("migrações registradas = %d, quer 1", total)
	}
	var depois string
	if err := segundo.DB().QueryRow(`SELECT applied_at FROM schema_migrations WHERE version = 1`).Scan(&depois); err != nil {
		t.Fatal(err)
	}
	if depois != antes {
		t.Errorf("migração reaplicada: applied_at %q -> %q", antes, depois)
	}
}

func TestPragmas(t *testing.T) {
	s := abrirTeste(t)

	var modo string
	if err := s.DB().QueryRow(`PRAGMA journal_mode`).Scan(&modo); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(modo, "wal") {
		t.Errorf("journal_mode = %q, quer wal", modo)
	}
	var fk, busy int
	if err := s.DB().QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, quer 1", fk)
	}
	if err := s.DB().QueryRow(`PRAGMA busy_timeout`).Scan(&busy); err != nil {
		t.Fatal(err)
	}
	if busy != 5000 {
		t.Errorf("busy_timeout = %d, quer 5000", busy)
	}
}

func TestChaveEstrangeiraAtiva(t *testing.T) {
	s := abrirTeste(t)

	_, err := s.DB().Exec(
		`INSERT INTO blocks (document_id, ord, kind, text, hash) VALUES (999, 0, 'p', 'x', 'h')`)
	if err == nil {
		t.Fatal("esperado erro de chave estrangeira para documento inexistente")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "foreign key") {
		t.Fatalf("erro = %v, quer violação de chave estrangeira", err)
	}
}

func TestInvariantesDoSchema(t *testing.T) {
	s := abrirTeste(t)
	ctx := context.Background()

	// DB-07: source_sha256 única.
	if _, err := s.DB().ExecContext(ctx, `INSERT INTO documents
		(title, format, source_name, source_path, source_sha256, created_at, updated_at)
		VALUES ('A', 'md', 'a.md', '/a.md', 'sha', '2026-10-27T00:00:00Z', '2026-10-27T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().ExecContext(ctx, `INSERT INTO documents
		(title, format, source_name, source_path, source_sha256, created_at, updated_at)
		VALUES ('B', 'md', 'b.md', '/b.md', 'sha', '2026-10-27T00:00:00Z', '2026-10-27T00:00:00Z')`); err == nil {
		t.Error("DB-07: source_sha256 duplicado foi aceito")
	}

	// DB-03: um único plano ativo por documento.
	if _, err := s.DB().ExecContext(ctx, `INSERT INTO plans
		(document_id, version, status, pacing_mode, target_seconds, params_json, created_at)
		VALUES (1, 1, 'active', 'minutes_per_day', 600, '{}', '2026-10-27T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().ExecContext(ctx, `INSERT INTO plans
		(document_id, version, status, pacing_mode, target_seconds, params_json, created_at)
		VALUES (1, 2, 'active', 'minutes_per_day', 600, '{}', '2026-10-27T00:00:00Z')`); err == nil {
		t.Error("DB-03: segundo plano ativo foi aceito")
	}
	// Plano superseded pode coexistir.
	if _, err := s.DB().ExecContext(ctx, `INSERT INTO plans
		(document_id, version, status, pacing_mode, target_seconds, params_json, created_at)
		VALUES (1, 3, 'superseded', 'minutes_per_day', 600, '{}', '2026-10-27T00:00:00Z')`); err != nil {
		t.Errorf("plano superseded recusado: %v", err)
	}
}
