//go:build cgo

package pdf

import (
	"math"
	"sort"
)

// Coluna representa uma coluna detectada na página.
type Coluna struct {
	XMin float64
	XMax float64
	Lines []LineSpan
}

// LineSpan representa uma linha com posição.
type LineSpan struct {
	Y    float64
	X    float64
	Text string
	Size float64
	Bold bool
}

// detectarColunas agrupa linhas em colunas por clustering de x.
func detectarColunas(lines []LineSpan) []Coluna {
	if len(lines) == 0 {
		return nil
	}
	if len(lines) == 1 {
		return []Coluna{{XMin: lines[0].X, XMax: lines[0].X, Lines: lines}}
	}

	// Coletar posições x iniciais
	xs := make([]float64, 0, len(lines))
	for _, l := range lines {
		xs = append(xs, l.X)
	}
	sort.Float64s(xs)

	// Encontrar gaps significativos
	var gaps []float64
	for i := 1; i < len(xs); i++ {
		if xs[i]-xs[i-1] > 5 {
			gaps = append(gaps, xs[i]-xs[i-1])
		}
	}
	if len(gaps) == 0 {
		return []Coluna{{XMin: xs[0], XMax: xs[len(xs)-1], Lines: lines}}
	}

	// Threshold: gap acima da mediana * 1.5 indica separação de coluna
	sort.Float64s(gaps)
	med := gaps[len(gaps)/2]
	threshold := med * 1.5
	if threshold < 20 {
		threshold = 20
	}

	// Agrupar
	var colunas []Coluna
	var current []LineSpan
	var curX float64
	for i, l := range lines {
		if i == 0 {
			current = []LineSpan{l}
			curX = l.X
			continue
		}
		if l.X-curX > threshold && len(current) > 0 {
			// Fechar coluna atual
			if len(current) > 0 {
				colunas = append(colunas, colunaFromLines(current))
			}
			current = []LineSpan{l}
			curX = l.X
		} else {
			current = append(current, l)
			if l.X < curX {
				curX = l.X
			}
		}
	}
	if len(current) > 0 {
		colunas = append(colunas, colunaFromLines(current))
	}

	if len(colunas) <= 1 {
		return []Coluna{{XMin: xs[0], XMax: xs[len(xs)-1], Lines: lines}}
	}
	return colunas
}

func colunaFromLines(lines []LineSpan) Coluna {
	if len(lines) == 0 {
		return Coluna{}
	}
	xmin, xmax := lines[0].X, lines[0].X
	for _, l := range lines {
		if l.X < xmin {
			xmin = l.X
		}
		if l.X > xmax {
			xmax = l.X
		}
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].Y < lines[j].Y })
	return Coluna{XMin: xmin, XMax: xmax, Lines: lines}
}

// ordenarPorColunaY ordena linhas por coluna e depois por y.
func ordenarPorColunaY(colunas []Coluna) []LineSpan {
	var out []LineSpan
	for _, c := range colunas {
		sort.Slice(c.Lines, func(i, j int) bool { return c.Lines[i].Y < c.Lines[j].Y })
		out = append(out, c.Lines...)
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

func media(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

func desvioPadrao(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	m := media(xs)
	var sum float64
	for _, x := range xs {
		d := x - m
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(xs)))
}
