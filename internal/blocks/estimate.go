package blocks

import (
	"math"
	"strings"
)

// Estimator calcula palavras e segundos de um bloco.
//
// Fórmula (ver docs/reading-time.md):
//
//	base   = palavras * 60 / wpm
//	seconds = round(base * difficulty + extras)
//
// Código é contado por linha, não por palavra.
type Estimator struct {
	WPM        int
	Difficulty float64
}

func (e Estimator) normalized() Estimator {
	if e.WPM <= 0 {
		e.WPM = 200
	}
	if e.Difficulty <= 0 {
		e.Difficulty = 1
	}
	return e
}

// Estimate devolve palavras (ou linhas, para código) e segundos.
func (e Estimator) Estimate(kind Kind, text, html string) (words, seconds int) {
	e = e.normalized()
	words = CountWords(text)

	if kind == KindHR {
		return 0, 0
	}

	var base float64
	switch kind {
	case KindCode:
		words = countLines(text)
		base = 0
	case KindFigure:
		base = 0
	default:
		base = float64(words) * 60 / float64(e.WPM)
	}

	extra := 0.0
	switch kind {
	case KindH1, KindH2, KindH3, KindH4, KindH5, KindH6:
		extra = 2
	case KindListItem:
		extra = 0.5
	case KindQuote:
		extra = 1
	case KindCode:
		extra = 2 + 1.5*float64(countLines(text))
	case KindTable:
		rows := countTableRows(html)
		if rows > 1 {
			extra = 3 + float64(rows-1)
		} else {
			extra = 3
		}
	case KindFigure:
		extra = 12
	case KindFootnote:
		extra = 1
	}

	total := base*e.Difficulty + extra
	seconds = int(math.Round(total))
	if seconds < 1 && kind != KindHR {
		seconds = 1
	}
	return words, seconds
}

func countLines(text string) int {
	t := strings.TrimRight(text, "\n")
	if t == "" {
		return 0
	}
	return strings.Count(t, "\n") + 1
}

func countTableRows(html string) int {
	return strings.Count(strings.ToLower(html), "<tr")
}
