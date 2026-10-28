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

func TestWarningsDePacing(t *testing.T) {
	casos := []struct {
		nome   string
		params plan.Params
		target int
		chunks int
		quer   []string
	}{
		{
			nome:   "modo A nunca avisa",
			params: plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1},
			target: 60, chunks: 1,
		},
		{
			nome:   "alvo no limite inferior não avisa",
			params: plan.Params{Mode: plan.ModeFinishByDays, Days: 10},
			target: 180, chunks: 10,
		},
		{
			nome:   "alvo no limite superior não avisa",
			params: plan.Params{Mode: plan.ModeFinishByDays, Days: 2},
			target: 3600, chunks: 2,
		},
		{
			nome:   "alvo baixo fragmenta",
			params: plan.Params{Mode: plan.ModeFinishByDays, Days: 30},
			target: 40, chunks: 30,
			quer: []string{"Alvo de 40s", "30 dias", "fragmentada"},
		},
		{
			nome:   "alvo alto passa de uma hora",
			params: plan.Params{Mode: plan.ModeFinishByDays, Days: 1},
			target: 3601, chunks: 1,
			quer: []string{"Alvo de 60m01s", "1 dia", "mais de 1 h"},
		},
		{
			nome:   "sobram dias sem conteúdo",
			params: plan.Params{Mode: plan.ModeFinishByDays, Days: 30},
			target: 300, chunks: 4,
			quer: []string{"Sobram 26 dias sem conteúdo", "4 dias"},
		},
		{
			nome:   "sobra um único dia",
			params: plan.Params{Mode: plan.ModeFinishByDays, Days: 5},
			target: 300, chunks: 4,
			quer: []string{"Sobra 1 dia sem conteúdo", "4 dias"},
		},
		{
			nome:   "dias iguais aos chunks não sobram",
			params: plan.Params{Mode: plan.ModeFinishByDays, Days: 4},
			target: 300, chunks: 4,
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			avisos := caso.params.Warnings(caso.target, caso.chunks)
			if len(caso.quer) == 0 {
				if len(avisos) != 0 {
					t.Fatalf("avisos = %v, quer nenhum", avisos)
				}
				return
			}
			texto := strings.Join(avisos, "\n")
			for _, trecho := range caso.quer {
				if !strings.Contains(texto, trecho) {
					t.Errorf("avisos = %v, quer conter %q", avisos, trecho)
				}
			}
		})
	}
}

func TestFormatIncluiAvisos(t *testing.T) {
	bs := loadPrepared(t, "headings")
	params := plan.Params{Mode: plan.ModeFinishByDays, Days: 5}
	pl, err := plan.Generate(bs, params)
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	if err := plan.Format(&buf, "headings.md", bs, pl, params); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "# aviso: Alvo de ") {
		t.Errorf("formato sem aviso de pacing:\n%s", buf.String())
	}
}
