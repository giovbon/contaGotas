package plan_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/giovani/readingdrip/internal/blocks"
	"github.com/giovani/readingdrip/internal/docs/epub"
	"github.com/giovani/readingdrip/internal/docs/markdown"
	"github.com/giovani/readingdrip/internal/plan"
)

var update = flag.Bool("update", false, "atualiza golden files")

func loadPrepared(t *testing.T, name string) []blocks.Block {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "md", name+".md")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler fixture %s: %v", name, err)
	}
	res, err := markdown.Extract(src, name)
	if err != nil {
		t.Fatalf("extrair %s: %v", name, err)
	}
	return prepare(t, res.Blocks)
}

func loadPreparedEPUB(t *testing.T, name string) []blocks.Block {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "epub", name+".epub")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler fixture %s: %v", name, err)
	}
	res, err := epub.Extract(src, name)
	if err != nil {
		t.Fatalf("extrair %s: %v", name, err)
	}
	return prepare(t, res.Blocks)
}

func prepare(t *testing.T, raw []blocks.Block) []blocks.Block {
	t.Helper()
	return blocks.Prepare(raw, blocks.PrepareOptions{
		Estimator:         blocks.Estimator{WPM: 200},
		MaxParagraphWords: 300,
	})
}

func checkGolden(t *testing.T, name, docName string, bs []blocks.Block, params plan.Params) {
	t.Helper()

	pl, err := plan.Generate(bs, params)
	if err != nil {
		t.Fatalf("gerar plano: %v", err)
	}

	var buf bytes.Buffer
	if err := plan.Format(&buf, docName, bs, pl, params); err != nil {
		t.Fatalf("formatar: %v", err)
	}

	goldenPath := filepath.Join("..", "..", "testdata", "golden", name+".plan.txt")
	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("golden ausente (rode com -update): %v", err)
	}
	// O checkout com core.autocrlf converte os goldens para CRLF no Windows;
	// a comparação é de conteúdo, não de fim de linha.
	got := bytes.ReplaceAll(buf.Bytes(), []byte("\r\n"), []byte("\n"))
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(got, want) {
		t.Fatalf("golden divergente:\n--- obtido ---\n%s\n--- esperado ---\n%s", got, want)
	}
}

func TestGoldenPlan(t *testing.T) {
	cases := []struct {
		name   string
		doc    string
		params plan.Params
	}{
		{"headings", "headings", plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 2}},
		{"plain", "plain", plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 2}},
		{"long-para", "long-para", plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 2}},
		{"oversized", "oversized", plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 2}},
		{"headings-days", "headings", plan.Params{Mode: plan.ModeFinishByDays, Days: 5}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bs := loadPrepared(t, tc.doc)
			checkGolden(t, tc.name, "testdata/md/"+tc.doc+".md", bs, tc.params)
		})
	}
}

func TestGoldenEPUB(t *testing.T) {
	bs := loadPreparedEPUB(t, "livro")
	checkGolden(t, "epub", "testdata/epub/livro.epub", bs, plan.Params{
		Mode:    plan.ModeMinutesPerDay,
		Minutes: 1,
	})
}
