package blocks

import (
	"strings"
	"testing"
)

func TestEstimate(t *testing.T) {
	prose2000 := strings.Repeat("palavra ", 2000)
	tableHTML := `<table><tr><th>a</th></tr><tr><td>b</td></tr><tr><td>c</td></tr><tr><td>d</td></tr><tr><td>e</td></tr></table>`

	tests := []struct {
		name     string
		kind     Kind
		text     string
		html     string
		wpm      int
		diff     float64
		wantWord int
		wantSec  int
	}{
		{"prosa 2000 palavras", KindParagraph, prose2000, "", 200, 1, 2000, 600},
		{"prosa com dificuldade", KindParagraph, prose2000, "", 200, 1.2, 2000, 720},
		{"heading", KindH1, "Título", "", 200, 1, 1, 2},
		{"lista", KindListItem, "Um item de lista", "", 200, 1, 4, 2},
		{"código", KindCode, "a\nb\nc\nd\ne\nf\ng\nh\ni\nj", "", 200, 1, 10, 17},
		{"tabela", KindTable, "a b c d e f g h i j", tableHTML, 200, 1, 10, 10},
		{"figura", KindFigure, "Diagrama do fluxo", "", 200, 1, 3, 12},
		{"nota", KindFootnote, "Uma nota", "", 200, 1, 2, 2},
		{"separador", KindHR, "", "<hr>", 200, 1, 0, 0},
		{"uma palavra", KindParagraph, "oi", "", 200, 1, 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			est := Estimator{WPM: tt.wpm, Difficulty: tt.diff}
			words, seconds := est.Estimate(tt.kind, tt.text, tt.html)
			if words != tt.wantWord {
				t.Fatalf("words = %d, want %d", words, tt.wantWord)
			}
			if seconds != tt.wantSec {
				t.Fatalf("seconds = %d, want %d", seconds, tt.wantSec)
			}
		})
	}
}

func TestEstimateDefaults(t *testing.T) {
	est := Estimator{}
	words, seconds := est.Estimate(KindParagraph, strings.Repeat("x ", 200), "")
	if words != 200 || seconds != 60 {
		t.Fatalf("words=%d seconds=%d, want 200/60", words, seconds)
	}
}

func TestCountLines(t *testing.T) {
	cases := map[string]int{
		"":            0,
		"a":           1,
		"a\nb":        2,
		"a\nb\n":      2,
		"a\n\nb":      3,
		"a\r\nb\r\nc": 3,
	}
	for in, want := range cases {
		if got := countLines(strings.ReplaceAll(in, "\r\n", "\n")); got != want {
			t.Errorf("countLines(%q) = %d, want %d", in, got, want)
		}
	}
}
