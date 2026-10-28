//go:build cgo

package pdf

import "testing"

func TestCandidatoTitulo(t *testing.T) {
	const med = 10.0

	casos := []struct {
		nome  string
		linha Line
		quer  bool
	}{
		{"título grande e longo", Line{Size: 17.2, Text: "Attention Is All You Need"}, true},
		{"seção curta", Line{Size: 12, Text: "2 Background"}, true},
		{"seção longa em negrito", Line{Size: 12, Bold: true, Text: "Measuring and Preventing Memorization Of Benchmarks In Practice"}, true},
		{"aviso longo com fonte de título", Line{Size: 12, Text: "Provided proper attribution is provided, Google hereby grants permission to"}, false},
		{"rótulo de tabela", Line{Size: 12, Text: "0.3 300K 4.33"}, false},
		{"corpo", Line{Size: 10, Text: "texto normal do parágrafo"}, false},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := candidatoTitulo(c.linha, med); got != c.quer {
				t.Fatalf("candidatoTitulo(%q) = %v, quer %v", c.linha.Text, got, c.quer)
			}
		})
	}
}
