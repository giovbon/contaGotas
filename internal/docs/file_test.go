package docs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovani/readingdrip/internal/docs"
)

func TestExtractFileMarkdown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "livro.md")
	conteudo := "# Título do Livro\n\nPrimeiro parágrafo.\n\n## Capítulo\n\nSegundo parágrafo.\n"
	if err := os.WriteFile(path, []byte(conteudo), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := docs.ExtractFile(context.Background(), path, "fallback")
	if err != nil {
		t.Fatalf("extrair: %v", err)
	}
	if res.Title != "Título do Livro" {
		t.Errorf("título = %q", res.Title)
	}
	if len(res.Blocks) != 4 {
		t.Errorf("blocos = %d, quer 4", len(res.Blocks))
	}
	if res.Author != "" || res.Language != "" {
		t.Errorf("metadados inesperados: %+v", res)
	}
}

func TestExtractFileEPUB(t *testing.T) {
	res, err := docs.ExtractFile(context.Background(), "../../testdata/epub/livro.epub", "fallback")
	if err != nil {
		t.Fatalf("extrair: %v", err)
	}
	if res.Title != "Leitura em Camadas" {
		t.Errorf("título = %q", res.Title)
	}
	if res.Author != "Ana Ribeiro" {
		t.Errorf("autor = %q", res.Author)
	}
	if res.Language != "pt-BR" {
		t.Errorf("idioma = %q", res.Language)
	}
	if len(res.Blocks) == 0 {
		t.Error("nenhum bloco extraído")
	}
}

func TestExtractFileFormatoDesconhecido(t *testing.T) {
	_, err := docs.ExtractFile(context.Background(), "livro.docx", "x")
	if err == nil || !strings.Contains(err.Error(), "não suportado") {
		t.Fatalf("erro = %v", err)
	}
}

func TestExtractFileInexistente(t *testing.T) {
	_, err := docs.ExtractFile(context.Background(), filepath.Join(t.TempDir(), "sumiu.md"), "x")
	if err == nil {
		t.Fatal("esperado erro para arquivo inexistente")
	}
}

func TestExtractFileContextoCancelado(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := docs.ExtractFile(ctx, "../../testdata/md/plain.md", "x")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("erro = %v, quer context.Canceled", err)
	}
}
