package plan_test

import (
	"strings"
	"testing"

	"github.com/giovani/readingdrip/internal/blocks"
	"github.com/giovani/readingdrip/internal/plan"
)

// blocosComSecoes monta um documento curto com dois capítulos e segundos
// conhecidos, para conferir a regeneração sem depender de estimativa.
func blocosComSecoes() []blocks.Block {
	return []blocks.Block{
		{Ord: 0, Kind: blocks.KindH1, Level: 1, Text: "Capítulo 1", Words: 2, Seconds: 5, Hash: "h0"},
		{Ord: 1, Kind: blocks.KindParagraph, Text: "Primeiro parágrafo.", Words: 2, Seconds: 50, Hash: "h1"},
		{Ord: 2, Kind: blocks.KindParagraph, Text: "Segundo parágrafo.", Words: 2, Seconds: 50, Hash: "h2"},
		{Ord: 3, Kind: blocks.KindH2, Level: 2, Text: "Seção 1.1", Words: 2, Seconds: 5, Hash: "h3"},
		{Ord: 4, Kind: blocks.KindParagraph, Text: "Terceiro parágrafo.", Words: 2, Seconds: 50, Hash: "h4"},
		{Ord: 5, Kind: blocks.KindParagraph, Text: "Quarto parágrafo.", Words: 2, Seconds: 50, Hash: "h5"},
	}
}

func TestGenerateFromCobreApenasORestante(t *testing.T) {
	bs := blocosComSecoes()
	params := plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1}

	pl, err := plan.GenerateFrom(bs, params, 2)
	if err != nil {
		t.Fatalf("gerar a partir do bloco 2: %v", err)
	}
	if pl.TotalSeconds != 155 {
		t.Errorf("total de segundos = %d, quer 155 (blocos 2..5)", pl.TotalSeconds)
	}
	if len(pl.Chunks) == 0 {
		t.Fatal("plano sem chunks")
	}
	if first := pl.Chunks[0]; first.FirstBlock != 2 {
		t.Errorf("primeiro chunk começa no bloco %d, quer 2", first.FirstBlock)
	}
	if last := pl.Chunks[len(pl.Chunks)-1]; last.LastBlock != len(bs)-1 {
		t.Errorf("último chunk termina no bloco %d, quer %d", last.LastBlock, len(bs)-1)
	}
	for i, c := range pl.Chunks {
		if c.Seq != i {
			t.Errorf("chunk %d com seq %d", i, c.Seq)
		}
		if c.FirstHash != bs[c.FirstBlock].Hash || c.LastHash != bs[c.LastBlock].Hash {
			t.Errorf("chunk %d com hashes divergentes", i)
		}
		if i > 0 && c.FirstBlock != pl.Chunks[i-1].LastBlock+1 {
			t.Errorf("chunk %d não é contíguo ao anterior", i)
		}
	}
	if err := plan.ValidateFrom(bs, pl, params, 2); err != nil {
		t.Errorf("plano inválido: %v", err)
	}

	// O primeiro bloco do restante é um parágrafo do Capítulo 1: o breadcrumb
	// precisa vir do documento inteiro, não só do pedaço replanejado.
	if got := strings.Join(pl.Chunks[0].SectionPath, " › "); got != "Capítulo 1" {
		t.Errorf("section_path = %q, quer %q", got, "Capítulo 1")
	}
	if !pl.Chunks[0].StartsMidSection {
		t.Error("primeiro chunk começa no meio da seção e deveria sinalizar")
	}
}

func TestGenerateFromFinishByDaysUsaTempoRestante(t *testing.T) {
	bs := blocosComSecoes()
	params := plan.Params{Mode: plan.ModeFinishByDays, Days: 5}

	pl, err := plan.GenerateFrom(bs, params, 2)
	if err != nil {
		t.Fatalf("gerar: %v", err)
	}
	if pl.TargetSeconds != 155/5 {
		t.Errorf("alvo = %d, quer %d (155 s restantes ÷ 5 dias)", pl.TargetSeconds, 155/5)
	}
}

func TestGenerateFromInicioInvalido(t *testing.T) {
	bs := blocosComSecoes()
	for _, inicio := range []int{-1, len(bs)} {
		if _, err := plan.GenerateFrom(bs, plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1}, inicio); err == nil {
			t.Errorf("início %d deveria falhar", inicio)
		}
	}
}

func TestGenerateFromZeroEquivaleAoPlanoCompleto(t *testing.T) {
	bs := blocosComSecoes()
	params := plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1}

	completo, err := plan.Generate(bs, params)
	if err != nil {
		t.Fatal(err)
	}
	restante, err := plan.GenerateFrom(bs, params, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(restante.Chunks) != len(completo.Chunks) {
		t.Fatalf("chunks = %d, quer %d", len(restante.Chunks), len(completo.Chunks))
	}
	for i := range completo.Chunks {
		a, b := completo.Chunks[i], restante.Chunks[i]
		if a.FirstBlock != b.FirstBlock || a.LastBlock != b.LastBlock ||
			a.SectionPath[0] != b.SectionPath[0] || a.StartsMidSection != b.StartsMidSection {
			t.Errorf("chunk %d difere: %+v vs %+v", i, a, b)
		}
	}
}
