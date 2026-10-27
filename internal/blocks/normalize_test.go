package blocks

import (
	"strings"
	"testing"
)

func TestSplitSentences(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "duas frases",
			in:   "Ele saiu. Ela ficou.",
			want: []string{"Ele saiu.", "Ela ficou."},
		},
		{
			name: "abreviação sr",
			in:   "O Sr. Silva chegou. Depois saiu.",
			want: []string{"O Sr. Silva chegou.", "Depois saiu."},
		},
		{
			name: "decimal",
			in:   "Custou 3.14 reais. Foi caro.",
			want: []string{"Custou 3.14 reais.", "Foi caro."},
		},
		{
			name: "aspas",
			in:   "Ele disse: “Vamos.” Depois saiu.",
			want: []string{"Ele disse: “Vamos.”", "Depois saiu."},
		},
		{
			name: "reticências",
			in:   "Espera... O que houve?",
			want: []string{"Espera...", "O que houve?"},
		},
		{
			name: "sem pontuação final",
			in:   "Frase sem ponto final",
			want: []string{"Frase sem ponto final"},
		},
		{
			name: "exclamação",
			in:   "Cuidado! Há um degrau.",
			want: []string{"Cuidado!", "Há um degrau."},
		},
		{
			name: "iniciais",
			in:   "J. Silva escreveu. Depois parou.",
			want: []string{"J. Silva escreveu.", "Depois parou."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitSentences(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("frases = %q, want %q", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("frase %d = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSplitParagraph(t *testing.T) {
	text := strings.Repeat("Frase de teste com algumas palavras. ", 20)
	parts := SplitParagraph(text, 20)
	if len(parts) < 2 {
		t.Fatalf("esperava mais de um pedaço, veio %d", len(parts))
	}
	for i, p := range parts {
		if CountWords(p) > 20 {
			t.Errorf("pedaço %d tem %d palavras: %q", i, CountWords(p), p)
		}
	}
}

func TestPrepareSplitsLongParagraphs(t *testing.T) {
	text := strings.Repeat("Frase de teste com algumas palavras. ", 20)
	raw := []Block{{Kind: KindParagraph, Text: text}}
	out := Prepare(raw, PrepareOptions{MaxParagraphWords: 20})
	if len(out) < 2 {
		t.Fatalf("esperava parágrafo dividido, veio %d bloco(s)", len(out))
	}
	for i, b := range out {
		if b.Ord != i {
			t.Errorf("bloco %d com Ord %d", i, b.Ord)
		}
		if b.Words == 0 || b.Seconds == 0 || b.Hash == "" {
			t.Errorf("bloco %d sem métricas: %+v", i, b)
		}
	}
}

func TestPrepareHeadingIDs(t *testing.T) {
	raw := []Block{
		{Kind: KindH1, Level: 1, Text: "A"},
		{Kind: KindParagraph, Text: "um"},
		{Kind: KindH2, Level: 2, Text: "B"},
		{Kind: KindParagraph, Text: "dois"},
		{Kind: KindParagraph, Text: "três"},
		{Kind: KindH1, Level: 1, Text: "C"},
		{Kind: KindParagraph, Text: "quatro"},
	}
	out := Prepare(raw, PrepareOptions{})
	want := []int{0, 0, 2, 2, 2, 5, 5}
	for i := range want {
		if out[i].HeadingID != want[i] {
			t.Errorf("bloco %d HeadingID = %d, want %d", i, out[i].HeadingID, want[i])
		}
	}
}

func TestPrepareSkipsEmpty(t *testing.T) {
	raw := []Block{
		{Kind: KindParagraph, Text: "   "},
		{Kind: KindH1, Level: 1, Text: "Título"},
		{Kind: KindParagraph, Text: "conteúdo"},
	}
	out := Prepare(raw, PrepareOptions{})
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if out[0].Kind != KindH1 {
		t.Fatalf("primeiro bloco = %s", out[0].Kind)
	}
}

func TestHashStability(t *testing.T) {
	a := TextHash(KindParagraph, "texto normalizado")
	b := TextHash(KindParagraph, "texto normalizado")
	c := TextHash(KindH2, "texto normalizado")
	if a != b {
		t.Fatal("hash não é estável")
	}
	if a == c {
		t.Fatal("hash deveria considerar o tipo")
	}
	if len(a) != 16 {
		t.Fatalf("hash com %d caracteres", len(a))
	}
}

func TestNormalizeText(t *testing.T) {
	in := "  Olá\u00a0mundo\u00ad!  \n Nova   linha "
	got := NormalizeText(in)
	if got != "Olá mundo! Nova linha" {
		t.Fatalf("NormalizeText = %q", got)
	}
}
