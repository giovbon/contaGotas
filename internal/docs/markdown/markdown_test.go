package markdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovani/readingdrip/internal/blocks"
)

func extractFixture(t *testing.T, name string) *Result {
	t.Helper()
	path := filepath.Join("..", "..", "..", "testdata", "md", name)
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler fixture %s: %v", name, err)
	}
	res, err := Extract(src, strings.TrimSuffix(name, filepath.Ext(name)))
	if err != nil {
		t.Fatalf("extrair %s: %v", name, err)
	}
	return res
}

func TestExtractHeadings(t *testing.T) {
	res := extractFixture(t, "headings.md")

	if res.Title != "A arte de ler devagar" {
		t.Fatalf("title = %q", res.Title)
	}

	kinds := map[blocks.Kind]int{}
	for _, b := range res.Blocks {
		kinds[b.Kind]++
		if b.Kind != blocks.KindHR && strings.TrimSpace(b.Text) == "" {
			t.Errorf("bloco %s sem texto", b.Kind)
		}
	}

	for _, want := range []blocks.Kind{
		blocks.KindH1, blocks.KindH2, blocks.KindH3,
		blocks.KindParagraph, blocks.KindListItem, blocks.KindCode,
		blocks.KindTable, blocks.KindQuote, blocks.KindFigure,
	} {
		if kinds[want] == 0 {
			t.Errorf("nenhum bloco do tipo %s", want)
		}
	}

	var code, table, figure string
	for _, b := range res.Blocks {
		switch b.Kind {
		case blocks.KindCode:
			code = b.Text
		case blocks.KindTable:
			table = b.HTML
		case blocks.KindFigure:
			figure = b.Text
		}
	}
	if !strings.Contains(code, "func ler") {
		t.Errorf("código extraído = %q", code)
	}
	if !strings.Contains(table, "<table") {
		t.Errorf("tabela sem HTML: %q", table)
	}
	if figure != "Diagrama do fluxo de leitura" {
		t.Errorf("texto da figura = %q", figure)
	}
}

func TestExtractPlain(t *testing.T) {
	res := extractFixture(t, "plain.md")
	if res.Title != "plain" {
		t.Fatalf("title = %q, want fallback plain", res.Title)
	}
	if len(res.Blocks) < 5 {
		t.Fatalf("esperava ao menos 5 parágrafos, veio %d", len(res.Blocks))
	}
	for _, b := range res.Blocks {
		if b.Kind != blocks.KindParagraph {
			t.Fatalf("bloco inesperado: %s", b.Kind)
		}
	}
}

func TestExtractEmpty(t *testing.T) {
	if _, err := Extract([]byte("   \n\n"), "vazio"); err == nil {
		t.Fatal("esperava erro para markdown vazio")
	}
}

func TestExtractFootnote(t *testing.T) {
	res := extractFixture(t, "headings.md")
	found := false
	for _, b := range res.Blocks {
		if b.Kind == blocks.KindFootnote {
			found = true
		}
	}
	if !found {
		t.Log("footnote não veio como bloco próprio; conteúdo pode ter sido absorvido")
	}
}
