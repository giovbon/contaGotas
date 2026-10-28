//go:build cgo

package pdf

import "testing"

func TestDetectarColunas(t *testing.T) {
	lines := []Line{
		{X: 72, Y: 100, Text: "a"},
		{X: 72, Y: 120, Text: "b"},
		{X: 90, Y: 140, Text: "recuo"},
		{X: 320, Y: 100, Text: "c"},
		{X: 320, Y: 120, Text: "d"},
	}
	cols := detectarColunas(lines)
	if len(cols) != 2 {
		t.Fatalf("colunas = %d, quer 2", len(cols))
	}
	if cols[0].XMin != 72 || cols[1].XMin != 320 {
		t.Fatalf("origens = %v, %v; quer 72 e 320", cols[0].XMin, cols[1].XMin)
	}
	if len(cols[0].Lines) != 3 {
		t.Fatalf("linhas da coluna 1 = %d, quer 3 (recuo absorvido)", len(cols[0].Lines))
	}

	ordenadas := ordenarLinhas(lines)
	if len(ordenadas) != 5 {
		t.Fatalf("linhas ordenadas = %d, quer 5", len(ordenadas))
	}
	if ordenadas[0].Text != "a" || ordenadas[2].Text != "recuo" || ordenadas[3].Text != "c" {
		t.Fatalf("ordem inesperada: %+v", ordenadas)
	}
}

func TestColunaUnicaAgrupamento(t *testing.T) {
	lines := []Line{
		{X: 72, Y: 100},
		{X: 73, Y: 120},
		{X: 74, Y: 140},
	}
	cols := detectarColunas(lines)
	if len(cols) != 1 {
		t.Fatalf("colunas = %d, quer 1", len(cols))
	}
}

func TestOrdenarCapa(t *testing.T) {
	// Folha de rosto: título centralizado grande, autores, e duas colunas de
	// corpo abaixo. O título deve vir primeiro e as colunas não se misturam.
	lines := []Line{
		{X: 150, Y: 90, Size: 18, Bold: true, Text: "Titulo"},
		{X: 180, Y: 120, Size: 12, Text: "Autor Um"},
		{X: 300, Y: 120, Size: 12, Text: "Autor Dois"},
		{X: 72, Y: 200, Size: 10, Text: "esquerda 1"},
		{X: 72, Y: 214, Size: 10, Text: "esquerda 2"},
		{X: 320, Y: 200, Size: 10, Text: "direita 1"},
		{X: 320, Y: 214, Size: 10, Text: "direita 2"},
	}
	got := ordenarLinhas(lines)
	var textos []string
	for _, l := range got {
		textos = append(textos, l.Text)
	}
	want := []string{"Titulo", "Autor Um Autor Dois", "esquerda 1", "esquerda 2", "direita 1", "direita 2"}
	if len(textos) != len(want) {
		t.Fatalf("ordem = %q, quer %q", textos, want)
	}
	for i := range want {
		if textos[i] != want[i] {
			t.Fatalf("ordem = %q, quer %q", textos, want)
		}
	}
}

func TestRanksDeTitulos(t *testing.T) {
	lines := []Line{
		{X: 100, Y: 90, Size: 24, Bold: true, Page: 0, Text: "Titulo do Documento"},
		{X: 100, Y: 200, Size: 14, Bold: true, Text: "1 Secao"},
		{X: 100, Y: 300, Size: 12, Bold: true, Text: "1.1 Subsecao"},
		{X: 100, Y: 400, Size: 30, Page: 1, Text: "rotulo de figura"},
	}
	med := 10.0
	maxTitulo := tamanhoDoTitulo(lines, med)
	if maxTitulo != 24 {
		t.Fatalf("tamanho do título = %.1f, quer 24", maxTitulo)
	}
	ranks := ranksDeTitulos(lines, med, maxTitulo)
	if ranks[24] != 1 {
		t.Errorf("título = nível %d, quer 1", ranks[24])
	}
	if ranks[14] != 2 {
		t.Errorf("seção = nível %d, quer 2", ranks[14])
	}
	if ranks[12] != 3 {
		t.Errorf("subseção = nível %d, quer 3", ranks[12])
	}
	if _, ok := ranks[30]; ok {
		t.Errorf("rótulo de figura não deveria ter nível de título")
	}
}
