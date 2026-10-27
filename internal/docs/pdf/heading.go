//go:build cgo

package pdf

import (
	"regexp"
	"strings"
)

// detectHeading detecta se uma linha é heading por estilo/texto.
func detectHeading(text string, fontSize float64, medianSize float64, bold bool) (bool, int) {
	text = strings.TrimSpace(text)
	if text == "" {
		return false, 0
	}
	if len(text) > 120 {
		return false, 0
	}

	// Padrões numéricos
	if regexp.MustCompile(`^(\d+(\.\d+)*\.?\s+|[Cc]ap[íi]tulo\s+\d+|[Pp]arte\s+\d+|[Aa]nexo\s+[A-Z])`).MatchString(text) {
		level := 1
		if regexp.MustCompile(`^\d+\.\d+\.`).MatchString(text) {
			level = 3
		} else if regexp.MustCompile(`^\d+\.\d+`).MatchString(text) {
			level = 2
		} else if regexp.MustCompile(`^\d+\.`).MatchString(text) {
			level = 1
		}
		return true, level
	}

	// Por tamanho de fonte
	if medianSize > 0 && fontSize >= medianSize*1.15 {
		level := 1
		if fontSize >= medianSize*1.4 {
			level = 1
		} else if fontSize >= medianSize*1.25 {
			level = 2
		} else {
			level = 3
		}
		return true, level
	}

	// Negrito + linha curta
	if bold && len(text) < 60 {
		// Maiúsculas ou título
		if isAllCapsOrTitle(text) {
			return true, 2
		}
	}

	return false, 0
}

func isAllCapsOrTitle(s string) bool {
	words := strings.Fields(s)
	if len(words) == 0 {
		return false
	}
	caps := 0
	for _, w := range words {
		if len(w) == 0 {
			continue
		}
		if strings.ToUpper(w) == w && len(w) > 1 {
			caps++
		}
	}
	return float64(caps)/float64(len(words)) > 0.5
}
