package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupCopiaDados(t *testing.T) {
	ctx := context.Background()
	s := abrirTeste(t)
	agora := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	if err := s.Settings().Set(ctx, "backup.chave", "valor", agora); err != nil {
		t.Fatalf("gravar configuração: %v", err)
	}

	destino := filepath.Join(t.TempDir(), "sub", "copia.db")
	if err := s.Backup(ctx, destino); err != nil {
		t.Fatalf("backup: %v", err)
	}
	if _, err := os.Stat(destino); err != nil {
		t.Fatalf("backup não existe: %v", err)
	}

	db, err := sql.Open("sqlite", destino)
	if err != nil {
		t.Fatalf("abrir backup: %v", err)
	}
	defer db.Close()

	var valor string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, "backup.chave").Scan(&valor); err != nil {
		t.Fatalf("ler configuração do backup: %v", err)
	}
	if valor != "valor" {
		t.Errorf("valor no backup = %q, quer %q", valor, "valor")
	}
}

func TestBackupNaoSobrescreveExistente(t *testing.T) {
	ctx := context.Background()
	s := abrirTeste(t)
	destino := filepath.Join(t.TempDir(), "copia.db")
	if err := os.WriteFile(destino, []byte("anterior"), 0o644); err != nil {
		t.Fatalf("gravar backup anterior: %v", err)
	}

	if err := s.Backup(ctx, destino); err != nil {
		t.Fatalf("backup: %v", err)
	}
	conteudo, err := os.ReadFile(destino)
	if err != nil {
		t.Fatalf("ler backup: %v", err)
	}
	if string(conteudo) != "anterior" {
		t.Errorf("backup existente foi sobrescrito: %q", conteudo)
	}
}

func TestBackupDestinoVazio(t *testing.T) {
	s := abrirTeste(t)
	if err := s.Backup(context.Background(), "  "); err == nil {
		t.Fatal("Backup com destino vazio devolveu nil")
	}
}

func TestBackupAntesDeMigracao(t *testing.T) {
	ctx := context.Background()
	s := abrirTeste(t)
	agora := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	if err := s.Settings().Set(ctx, "antes.migracao", "sim", agora); err != nil {
		t.Fatalf("gravar configuração: %v", err)
	}

	dir := t.TempDir()
	var avisado string
	opts := Options{BackupDir: dir, OnBackup: func(caminho string) { avisado = caminho }}
	pendente := migracao{
		versao: 99,
		nome:   "0099_teste.sql",
		sql:    `CREATE TABLE teste_migracao (id INTEGER PRIMARY KEY)`,
	}
	if err := s.migrar(ctx, opts, true, []migracao{pendente}); err != nil {
		t.Fatalf("migrar: %v", err)
	}

	destino := filepath.Join(dir, "pre-migrate-99.db")
	if avisado != destino {
		t.Errorf("OnBackup recebeu %q, quer %q", avisado, destino)
	}
	if _, err := os.Stat(destino); err != nil {
		t.Fatalf("backup pré-migração não existe: %v", err)
	}

	var n int
	if err := s.DB().QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'teste_migracao'`,
	).Scan(&n); err != nil {
		t.Fatalf("consultar tabela migrada: %v", err)
	}
	if n != 1 {
		t.Errorf("migração não foi aplicada: tabela teste_migracao ausente")
	}

	// O backup é o estado anterior: tem a configuração e não tem a tabela nova.
	db, err := sql.Open("sqlite", destino)
	if err != nil {
		t.Fatalf("abrir backup: %v", err)
	}
	defer db.Close()

	var valor string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, "antes.migracao").Scan(&valor); err != nil {
		t.Fatalf("ler configuração do backup: %v", err)
	}
	if valor != "sim" {
		t.Errorf("valor no backup = %q, quer %q", valor, "sim")
	}
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'teste_migracao'`,
	).Scan(&n); err != nil {
		t.Fatalf("consultar tabela no backup: %v", err)
	}
	if n != 0 {
		t.Error("backup já contém a tabela criada pela migração")
	}
}

func TestOpenNovoNaoGravaBackup(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backup")
	s, err := OpenWithOptions(ctx, filepath.Join(dir, "app.db"), Options{BackupDir: backupDir})
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("fechar banco: %v", err)
		}
	})

	if _, err := os.Stat(backupDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("diretório de backup criado para banco novo: %v", err)
	}
}
