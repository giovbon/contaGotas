package library_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/giovani/readingdrip/internal/ingest"
	"github.com/giovani/readingdrip/internal/library"
	"github.com/giovani/readingdrip/internal/store"
)

var agora = time.Date(2026, 10, 27, 15, 0, 0, 0, time.FixedZone("BRT", -3*3600))

type filaFalsa struct {
	mu  sync.Mutex
	ids []int64
	err error
}

func (f *filaFalsa) Enqueue(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.ids = append(f.ids, id)
	return nil
}

func abrirImportador(t *testing.T) (*store.Store, *library.Importer, *filaFalsa, string) {
	t.Helper()
	root := t.TempDir()
	s, err := store.Open(context.Background(), filepath.Join(root, "app.db"))
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("fechar banco: %v", err)
		}
	})
	fila := &filaFalsa{}
	imp := &library.Importer{
		Store: s,
		Root:  filepath.Join(root, "data"),
		Queue: fila,
		Now:   func() time.Time { return agora },
	}
	return s, imp, fila, imp.Root
}

func entradasDir(t *testing.T, dir string) []string {
	t.Helper()
	lista, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var nomes []string
	for _, e := range lista {
		nomes = append(nomes, e.Name())
	}
	return nomes
}

func TestImporterMarkdown(t *testing.T) {
	_, imp, fila, root := abrirImportador(t)
	conteudo := "# Meu Livro\n\nPrimeiro parágrafo.\n"

	doc, err := imp.Import(context.Background(), library.Entrada{
		Name:   "Meu Livro.md",
		Reader: strings.NewReader(conteudo),
	})
	if err != nil {
		t.Fatalf("importar: %v", err)
	}
	if doc.Title != "Meu Livro" || doc.Format != "md" || doc.Status != store.StatusProcessing {
		t.Errorf("documento = %+v", doc)
	}
	if doc.SourceName != "Meu Livro.md" {
		t.Errorf("source_name = %q", doc.SourceName)
	}

	sum := sha256.Sum256([]byte(conteudo))
	if doc.SourceSHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("sha256 = %q", doc.SourceSHA256)
	}

	esperado := filepath.Join(root, "documents", fmt.Sprint(doc.ID), "source.md")
	if doc.SourcePath != esperado {
		t.Errorf("source_path = %q, quer %q", doc.SourcePath, esperado)
	}
	lido, err := os.ReadFile(esperado)
	if err != nil {
		t.Fatalf("ler original: %v", err)
	}
	if string(lido) != conteudo {
		t.Errorf("original alterado: %q", lido)
	}

	fila.mu.Lock()
	defer fila.mu.Unlock()
	if len(fila.ids) != 1 || fila.ids[0] != doc.ID {
		t.Errorf("fila = %v, quer [%d]", fila.ids, doc.ID)
	}
}

func TestImporterDuplicado(t *testing.T) {
	s, imp, fila, root := abrirImportador(t)
	ctx := context.Background()
	primeiro, err := imp.Import(ctx, library.Entrada{Name: "a.md", Reader: strings.NewReader("# Igual\n\nMesmo conteúdo.\n")})
	if err != nil {
		t.Fatal(err)
	}

	_, err = imp.Import(ctx, library.Entrada{Name: "b.md", Reader: strings.NewReader("# Igual\n\nMesmo conteúdo.\n")})
	var dup *library.DuplicateError
	if !errors.As(err, &dup) || !errors.Is(err, library.ErrDuplicate) {
		t.Fatalf("erro = %v, quer DuplicateError", err)
	}
	if dup.Existing == nil || dup.Existing.ID != primeiro.ID {
		t.Errorf("duplicado aponta para %+v, quer id %d", dup.Existing, primeiro.ID)
	}

	docs, err := s.Documents().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Errorf("documentos = %d, quer 1", len(docs))
	}
	if nomes := entradasDir(t, filepath.Join(root, "documents")); len(nomes) != 1 {
		t.Errorf("entradas em documents = %v, quer só o primeiro documento", nomes)
	}
	fila.mu.Lock()
	defer fila.mu.Unlock()
	if len(fila.ids) != 1 {
		t.Errorf("fila = %v, quer só o primeiro", fila.ids)
	}
}

func TestImporterAcimaDoLimite(t *testing.T) {
	s, imp, fila, root := abrirImportador(t)
	imp.MaxBytes = 16
	conteudo := strings.Repeat("a", 17)

	_, err := imp.Import(context.Background(), library.Entrada{Name: "grande.md", Reader: strings.NewReader(conteudo)})
	var grande *library.TooLargeError
	if !errors.As(err, &grande) || !errors.Is(err, library.ErrTooLarge) {
		t.Fatalf("erro = %v, quer TooLargeError", err)
	}
	if grande.Limit != 16 || grande.Size != 17 {
		t.Errorf("limite/tamanho = %d/%d", grande.Limit, grande.Size)
	}

	docs, err := s.Documents().List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 0 {
		t.Errorf("documentos = %d, quer 0", len(docs))
	}
	if nomes := entradasDir(t, filepath.Join(root, "documents")); len(nomes) != 0 {
		t.Errorf("arquivos deixados para trás: %v", nomes)
	}
	fila.mu.Lock()
	defer fila.mu.Unlock()
	if len(fila.ids) != 0 {
		t.Errorf("fila = %v, quer vazia", fila.ids)
	}
}

func TestImporterRejeicoes(t *testing.T) {
	ctx := context.Background()

	t.Run("formato", func(t *testing.T) {
		_, imp, _, _ := abrirImportador(t)
		_, err := imp.Import(ctx, library.Entrada{Name: "nota.txt", Reader: strings.NewReader("x")})
		if !errors.Is(err, library.ErrUnsupportedFormat) {
			t.Fatalf("erro = %v, quer ErrUnsupportedFormat", err)
		}
	})

	t.Run("vazio", func(t *testing.T) {
		_, imp, _, _ := abrirImportador(t)
		_, err := imp.Import(ctx, library.Entrada{Name: "vazio.md", Reader: strings.NewReader("")})
		if !errors.Is(err, library.ErrEmptyFile) {
			t.Fatalf("erro = %v, quer ErrEmptyFile", err)
		}
	})

	t.Run("sem leitor", func(t *testing.T) {
		_, imp, _, _ := abrirImportador(t)
		if _, err := imp.Import(ctx, library.Entrada{Name: "x.md"}); err == nil {
			t.Fatal("esperado erro sem conteúdo")
		}
	})
}

func TestImporterSanitizaNome(t *testing.T) {
	_, imp, _, root := abrirImportador(t)

	doc, err := imp.Import(context.Background(), library.Entrada{
		Name:   "../../fora/evil.md",
		Reader: strings.NewReader("# Evil\n\nConteúdo.\n"),
	})
	if err != nil {
		t.Fatalf("importar: %v", err)
	}
	if doc.SourceName != "evil.md" || doc.Title != "evil" {
		t.Errorf("nome/título = %q/%q", doc.SourceName, doc.Title)
	}
	if !strings.HasPrefix(doc.SourcePath, filepath.Join(root, "documents")) {
		t.Errorf("caminho escapou da raiz: %q", doc.SourcePath)
	}
	if _, err := os.Stat(filepath.Join(root, "fora", "evil.md")); !os.IsNotExist(err) {
		t.Errorf("arquivo criado fora da raiz: %v", err)
	}
}

func TestImporterFalhaNaFilaDesfazTudo(t *testing.T) {
	s, imp, fila, root := abrirImportador(t)
	fila.err = errors.New("fila cheia")

	_, err := imp.Import(context.Background(), library.Entrada{
		Name:   "livro.md",
		Reader: strings.NewReader("# Livro\n\nConteúdo.\n"),
	})
	if err == nil || !strings.Contains(err.Error(), "fila cheia") {
		t.Fatalf("erro = %v", err)
	}

	docs, err := s.Documents().List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 0 {
		t.Errorf("documentos = %d, quer 0", len(docs))
	}
	if nomes := entradasDir(t, filepath.Join(root, "documents")); len(nomes) != 0 {
		t.Errorf("arquivos deixados para trás: %v", nomes)
	}
}

func TestImporterComFilaRealProcessa(t *testing.T) {
	s, imp, _, _ := abrirImportador(t)

	svc := &ingest.Service{Store: s, Now: func() time.Time { return agora }}
	fila := ingest.NewQueue(svc, 4)
	pronto := make(chan int64, 1)
	fila.OnResult = func(id int64, res *ingest.Result, err error) {
		if err != nil {
			t.Errorf("processar documento %d: %v", id, err)
			return
		}
		pronto <- id
	}
	if err := fila.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer fila.Close()
	imp.Queue = fila

	doc, err := imp.Import(context.Background(), library.Entrada{
		Name:   "assincrono.md",
		Reader: strings.NewReader("# Assíncrono\n\nUm parágrafo.\n\n## Seção\n\nOutro parágrafo.\n"),
	})
	if err != nil {
		t.Fatalf("importar: %v", err)
	}
	if doc.Status != store.StatusProcessing {
		t.Errorf("status na resposta = %q, quer processing", doc.Status)
	}

	select {
	case id := <-pronto:
		if id != doc.ID {
			t.Errorf("processado = %d, quer %d", id, doc.ID)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("worker não processou o documento")
	}

	final, err := s.Documents().Get(context.Background(), doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != store.StatusReady {
		t.Errorf("status final = %q, quer ready (erro: %v)", final.Status, final.Error)
	}
	ativo, err := s.Plans().Active(context.Background(), doc.ID)
	if err != nil || ativo == nil {
		t.Fatalf("plano ativo: %v, %v", ativo, err)
	}
	if len(ativo.Chunks) == 0 {
		t.Error("plano sem chunks")
	}
}

func TestCleanupTemp(t *testing.T) {
	_, imp, _, root := abrirImportador(t)
	docsDir := filepath.Join(root, "documents")
	if err := os.MkdirAll(docsDir, 0o750); err != nil {
		t.Fatal(err)
	}
	temp := filepath.Join(docsDir, ".upload-interrompido")
	if err := os.WriteFile(temp, []byte("resto"), 0o640); err != nil {
		t.Fatal(err)
	}
	manter := filepath.Join(docsDir, "manter.txt")
	if err := os.WriteFile(manter, []byte("ok"), 0o640); err != nil {
		t.Fatal(err)
	}

	if err := imp.CleanupTemp(); err != nil {
		t.Fatalf("limpar temporários: %v", err)
	}
	if _, err := os.Stat(temp); !os.IsNotExist(err) {
		t.Errorf("temporário ainda existe: %v", err)
	}
	if _, err := os.Stat(manter); err != nil {
		t.Errorf("arquivo comum removido: %v", err)
	}
}

func TestRemoveOriginal(t *testing.T) {
	_, imp, _, root := abrirImportador(t)
	doc, err := imp.Import(context.Background(), library.Entrada{
		Name:   "Remover.md",
		Reader: strings.NewReader("# Remover\n\nConteúdo.\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "documents", strconv.FormatInt(doc.ID, 10))
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("diretório do documento não existe: %v", err)
	}

	if err := imp.RemoveOriginal(doc.ID); err != nil {
		t.Fatalf("remover: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("diretório ainda existe: %v", err)
	}
	// Repetir é inofensivo: o diretório já não existe.
	if err := imp.RemoveOriginal(doc.ID); err != nil {
		t.Errorf("segunda remoção = %v", err)
	}
	if err := imp.RemoveOriginal(0); err == nil {
		t.Error("esperado erro para id inválido")
	}
	if err := (&library.Importer{}).RemoveOriginal(1); err == nil {
		t.Error("esperado erro sem diretório de dados")
	}
}
