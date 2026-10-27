package plan_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/giovani/readingdrip/internal/blocks"
	"github.com/giovani/readingdrip/internal/plan"
)

func tinyDoc() []blocks.Block {
	return blocks.Prepare([]blocks.Block{
		{Kind: blocks.KindParagraph, Text: "Um texto curto para uma única dose."},
	}, blocks.PrepareOptions{Estimator: blocks.Estimator{WPM: 200}})
}

func TestSingleChunk(t *testing.T) {
	pl, err := plan.Generate(tinyDoc(), plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(pl.Chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(pl.Chunks))
	}
	c := pl.Chunks[0]
	if c.FirstBlock != 0 || c.LastBlock != 0 {
		t.Fatalf("intervalo = [%d..%d]", c.FirstBlock, c.LastBlock)
	}
	if c.StartsMidSection || c.EndsMidSection {
		t.Fatal("documento de um bloco não deveria ser marcado como meio de seção")
	}
}

func TestModeTargets(t *testing.T) {
	bs := loadPrepared(t, "headings")

	a, err := plan.Generate(bs, plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 2})
	if err != nil {
		t.Fatal(err)
	}
	if a.TargetSeconds != 120 {
		t.Fatalf("modo A: target = %d, want 120", a.TargetSeconds)
	}

	b, err := plan.Generate(bs, plan.Params{Mode: plan.ModeFinishByDays, Days: 5})
	if err != nil {
		t.Fatal(err)
	}
	if want := a.TotalSeconds / 5; b.TargetSeconds != want {
		t.Fatalf("modo B: target = %d, want %d", b.TargetSeconds, want)
	}
}

func TestCoverage(t *testing.T) {
	bs := loadPrepared(t, "headings")
	pl, err := plan.Generate(bs, plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 2})
	if err != nil {
		t.Fatal(err)
	}

	if pl.Chunks[0].FirstBlock != 0 {
		t.Fatalf("primeiro chunk começa em %d", pl.Chunks[0].FirstBlock)
	}
	for i := 1; i < len(pl.Chunks); i++ {
		if pl.Chunks[i].FirstBlock != pl.Chunks[i-1].LastBlock+1 {
			t.Fatalf("gap entre chunks %d e %d", i-1, i)
		}
	}
	if last := pl.Chunks[len(pl.Chunks)-1].LastBlock; last != len(bs)-1 {
		t.Fatalf("último bloco = %d, want %d", last, len(bs)-1)
	}
}

func TestNoHeadingEndsChunk(t *testing.T) {
	bs := loadPrepared(t, "headings")
	pl, err := plan.Generate(bs, plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range pl.Chunks {
		if bs[c.LastBlock].Kind.IsHeading() {
			t.Fatalf("chunk %d termina no heading %d", c.Seq, c.LastBlock)
		}
	}
}

func TestOversized(t *testing.T) {
	bs := loadPrepared(t, "oversized")
	pl, err := plan.Generate(bs, plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 2})
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, c := range pl.Chunks {
		if c.Oversized {
			found = true
			if c.Seconds <= pl.TargetSeconds {
				t.Fatalf("chunk oversized com %ds e alvo %ds", c.Seconds, pl.TargetSeconds)
			}
		}
	}
	if !found {
		t.Fatal("esperava um chunk oversized para o bloco de código de 200 linhas")
	}
}

func TestDeterminism(t *testing.T) {
	bs := loadPrepared(t, "headings")
	params := plan.Params{Mode: plan.ModeFinishByDays, Days: 7}

	var first []byte
	for i := 0; i < 100; i++ {
		pl, err := plan.Generate(bs, params)
		if err != nil {
			t.Fatal(err)
		}
		got, err := json.Marshal(pl)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = got
			continue
		}
		if string(got) != string(first) {
			t.Fatalf("iteração %d produziu plano diferente", i)
		}
	}
}

func TestValidateRejectsGap(t *testing.T) {
	bs := loadPrepared(t, "headings")
	params := plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 2}
	pl, err := plan.Generate(bs, params)
	if err != nil {
		t.Fatal(err)
	}
	if len(pl.Chunks) < 2 {
		t.Skip("fixture gerou menos de dois chunks")
	}

	pl.Chunks[1].FirstBlock++
	if err := plan.Validate(bs, pl, params); err == nil {
		t.Fatal("esperava erro de cobertura")
	}
}

func TestTargetErrors(t *testing.T) {
	bs := tinyDoc()
	if _, err := plan.Generate(bs, plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 0}); err == nil {
		t.Fatal("esperava erro para minutos = 0")
	}
	if _, err := plan.Generate(bs, plan.Params{Mode: plan.ModeFinishByDays, Days: 0}); err == nil {
		t.Fatal("esperava erro para dias = 0")
	}
}

func TestEmptyDocument(t *testing.T) {
	if _, err := plan.Generate(nil, plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 10}); err == nil {
		t.Fatal("esperava erro para documento vazio")
	}
}

func TestLongParagraphIsSplitBeforePlanning(t *testing.T) {
	bs := loadPrepared(t, "long-para")
	if len(bs) < 2 {
		t.Fatalf("parágrafo longo deveria ter sido dividido, veio %d bloco(s)", len(bs))
	}
	for _, b := range bs {
		if b.Words > 300 {
			t.Fatalf("bloco com %d palavras não foi dividido", b.Words)
		}
	}
	pl, err := plan.Generate(bs, plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 2})
	if err != nil {
		t.Fatal(err)
	}
	// A divisão interna não pode deixar o documento com um único chunk gigante.
	if len(pl.Chunks) < 2 && pl.TotalSeconds > 180 {
		t.Fatalf("esperava mais de um chunk, veio %d para %ds", len(pl.Chunks), pl.TotalSeconds)
	}
}

func TestSectionPathContinuity(t *testing.T) {
	bs := loadPrepared(t, "headings")
	pl, err := plan.Generate(bs, plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range pl.Chunks {
		if len(c.SectionPath) == 0 && !strings.Contains(strings.ToLower(bs[c.FirstBlock].Text), "arte") {
			t.Fatalf("chunk %d sem breadcrumb", c.Seq)
		}
	}
}
