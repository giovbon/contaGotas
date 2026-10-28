//go:build cgo

package pdf

import (
	"math"
	"sort"
	"strings"
)

// Line é uma linha visual extraída de uma página, com a posição que o
// MuPDF reporta no HTML estruturado.
type Line struct {
	Page       int
	Column     int
	X          float64
	Y          float64
	LineHeight float64
	Size       float64
	Bold       bool
	Text       string
}

// Coluna representa um agrupamento de linhas com a mesma origem horizontal.
type Coluna struct {
	XMin  float64
	XMax  float64
	Lines []Line
}

const (
	// columnTolerance é a distância máxima entre origens para considerar que
	// duas linhas começam na mesma coluna.
	columnTolerance = 12.0
	// columnGutter é o vão mínimo entre grupos para tratá-los como colunas
	// distintas. Valores menores separam recuos de primeira linha.
	columnGutter = 36.0
)

// detectarColunas agrupa linhas por origem horizontal. Recuos pequenos são
// absorvidos pela coluna mais próxima; vãos maiores que o gutter separam
// colunas de leitura.
func detectarColunas(lines []Line) []Coluna {
	if len(lines) == 0 {
		return nil
	}

	sorted := append([]Line(nil), lines...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].X == sorted[j].X {
			return sorted[i].Y < sorted[j].Y
		}
		return sorted[i].X < sorted[j].X
	})

	var cols []Coluna
	for _, l := range sorted {
		if len(cols) == 0 || l.X-cols[len(cols)-1].XMax > columnTolerance {
			cols = append(cols, Coluna{XMin: l.X, XMax: l.X, Lines: []Line{l}})
			continue
		}
		c := &cols[len(cols)-1]
		c.Lines = append(c.Lines, l)
		if l.X < c.XMin {
			c.XMin = l.X
		}
		if l.X > c.XMax {
			c.XMax = l.X
		}
	}

	// Absorve grupos separados só por recuo (gap < gutter).
	merged := make([]Coluna, 0, len(cols))
	for _, c := range cols {
		if len(merged) == 0 {
			merged = append(merged, c)
			continue
		}
		last := &merged[len(merged)-1]
		if c.XMin-last.XMax < columnGutter {
			last.Lines = append(last.Lines, c.Lines...)
			if c.XMin < last.XMin {
				last.XMin = c.XMin
			}
			if c.XMax > last.XMax {
				last.XMax = c.XMax
			}
			continue
		}
		merged = append(merged, c)
	}

	// Grupos residuais (títulos centralizados, uma linha isolada) não formam
	// coluna própria: vão para a coluna mais próxima.
	minLines := len(lines) / 10
	if minLines < 2 {
		minLines = 2
	}

	// Três ou mais grupos fortes indicam layout irregular (capa, revista,
	// figuras largas). Nesse caso, a ordem por y é mais fiel do que a leitura
	// coluna a coluna.
	fortes := 0
	for _, c := range merged {
		if len(c.Lines) >= minLines {
			fortes++
		}
	}
	if fortes >= 3 {
		all := append([]Line(nil), lines...)
		xmin, xmax := all[0].X, all[0].X
		for _, l := range all {
			if l.X < xmin {
				xmin = l.X
			}
			if l.X > xmax {
				xmax = l.X
			}
		}
		return []Coluna{{XMin: xmin, XMax: xmax, Lines: all}}
	}

	var keep, drop []Coluna
	for _, c := range merged {
		if len(c.Lines) >= minLines || len(merged) == 1 {
			keep = append(keep, c)
		} else {
			drop = append(drop, c)
		}
	}
	if len(keep) == 0 {
		keep = merged
		drop = nil
	}
	for _, c := range drop {
		best := 0
		for i := 1; i < len(keep); i++ {
			if distancia(c.XMin, keep[i].XMin) < distancia(c.XMin, keep[best].XMin) {
				best = i
			}
		}
		keep[best].Lines = append(keep[best].Lines, c.Lines...)
		if c.XMin < keep[best].XMin {
			keep[best].XMin = c.XMin
		}
		if c.XMax > keep[best].XMax {
			keep[best].XMax = c.XMax
		}
	}

	sort.SliceStable(keep, func(i, j int) bool { return keep[i].XMin < keep[j].XMin })
	return keep
}

// ordenarLinhas devolve as linhas em ordem de leitura: coluna a coluna, e
// dentro de cada coluna de cima para baixo.
func ordenarLinhas(lines []Line) []Line {
	if temTituloGrande(lines) {
		return ordenarCapa(lines)
	}
	cols := detectarColunas(lines)
	out := make([]Line, 0, len(lines))
	for i := range cols {
		sort.SliceStable(cols[i].Lines, func(a, b int) bool {
			if cols[i].Lines[a].Y == cols[i].Lines[b].Y {
				return cols[i].Lines[a].X < cols[i].Lines[b].X
			}
			return cols[i].Lines[a].Y < cols[i].Lines[b].Y
		})
		for _, l := range cols[i].Lines {
			l.Column = i
			out = append(out, l)
		}
	}
	return fundirFragmentos(out)
}

// temTituloGrande informa se a página tem uma linha de título (fonte bem maior
// que o corpo). Páginas assim costumam ser folha de rosto/capa com layout
// irregular, onde a ordem por y é mais fiel do que a leitura coluna a coluna.
func temTituloGrande(lines []Line) bool {
	if len(lines) < 5 {
		return false
	}
	maxSize := 0.0
	for _, l := range lines {
		if l.Size > maxSize {
			maxSize = l.Size
		}
	}
	corpo := tamanhoDoCorpo(lines)
	if corpo <= 0 {
		return false
	}
	return maxSize >= corpo*1.4
}

// ordenarCapa trata páginas com título grande. O bloco acima do primeiro
// parágrafo (título, autores, resumo destacado) é lido de cima para baixo;
// o restante segue a leitura por colunas, para não intercalar figuras e
// legendas com o corpo do texto.
func ordenarCapa(lines []Line) []Line {
	corpo := tamanhoDoCorpo(lines)
	corte := math.Inf(1)
	for _, l := range lines {
		if l.Size > 0 && l.Size <= corpo*1.1 && l.Y < corte {
			corte = l.Y
		}
	}
	if math.IsInf(corte, 1) {
		return ordenarPorY(lines)
	}

	var cabeca, resto []Line
	for _, l := range lines {
		if l.Y < corte {
			cabeca = append(cabeca, l)
		} else {
			resto = append(resto, l)
		}
	}

	out := ordenarPorY(cabeca)
	cols := detectarColunas(resto)
	for i := range cols {
		sort.SliceStable(cols[i].Lines, func(a, b int) bool {
			if cols[i].Lines[a].Y == cols[i].Lines[b].Y {
				return cols[i].Lines[a].X < cols[i].Lines[b].X
			}
			return cols[i].Lines[a].Y < cols[i].Lines[b].Y
		})
		for _, l := range cols[i].Lines {
			l.Column = i + 1
			out = append(out, l)
		}
	}
	return fundirFragmentos(out)
}

// ordenarPorY ordena as linhas de cima para baixo.
func ordenarPorY(lines []Line) []Line {
	out := append([]Line(nil), lines...)
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Y == out[b].Y {
			return out[a].X < out[b].X
		}
		return out[a].Y < out[b].Y
	})
	for i := range out {
		out[i].Column = 0
	}
	return fundirFragmentos(out)
}

// tamanhoDoCorpo devolve a mediana dos tamanhos de fonte da página.
func tamanhoDoCorpo(lines []Line) float64 {
	sizes := make([]float64, 0, len(lines))
	for _, l := range lines {
		if l.Size > 0 {
			sizes = append(sizes, l.Size)
		}
	}
	return mediana(sizes)
}

// fundirFragmentos junta pedaços da mesma linha visual que o MuPDF emitiu em
// <p> separados (por exemplo, "3.1" e o título logo ao lado).
func fundirFragmentos(lines []Line) []Line {
	out := make([]Line, 0, len(lines))
	for _, l := range lines {
		if n := len(out); n > 0 {
			last := &out[n-1]
			if l.Page == last.Page && l.Column == last.Column &&
				math.Abs(l.Y-last.Y) <= 1.0 && l.X >= last.X && l.X-last.X <= 120 {
				last.Text = strings.TrimSpace(last.Text + " " + l.Text)
				if l.Size > last.Size {
					last.Size = l.Size
				}
				if l.LineHeight > last.LineHeight {
					last.LineHeight = l.LineHeight
				}
				last.Bold = last.Bold || l.Bold
				continue
			}
		}
		out = append(out, l)
	}
	return out
}

func distancia(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}

func mediana(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	cpy := make([]float64, len(xs))
	copy(cpy, xs)
	sort.Float64s(cpy)
	if len(cpy)%2 == 1 {
		return cpy[len(cpy)/2]
	}
	return (cpy[len(cpy)/2-1] + cpy[len(cpy)/2]) / 2
}
