//go:build cgo

package pdf

import (
	"regexp"
	"sort"
	"strings"

	"github.com/gen2brain/go-fitz"
)

// extractPageWithSpans tenta extrair usando informações de posição quando disponível.
// go-fitz não expõe spans diretamente em todas as versões, então usamos heurísticas.
func extractPageWithSpans(doc *fitz.Document, pageNum int, fontSizeMap map[float64]int) ([]blocks.Block, error) {
	// Por enquanto usar HTML/Text com heurísticas
	return extractPage(doc, pageNum)
}

// analisarFontes coleta tamanhos de fonte para detectar headings.
func analisarFontes(html string) (float64, float64) {
	re := regexp.MustCompile(`font-size:\s*([0-9.]+)pt`)
	matches := re.FindAllStringSubmatch(html, -1)
	if len(matches) == 0 {
		return 12, 12
	}
	var sizes []float64
	for _, m := range matches {
		var s float64
		// parse simples
		for i := 0; i < len(m[1]); i++ {
			if m[1][i] >= '0' && m[1][i] <= '9' || m[1][i] == '.' {
				continue
			}
		}
		// usar strconv
	}
	// fallback
	if len(sizes) == 0 {
		return 12, 12
	}
	sort.Float64s(sizes)
	med := sizes[len(sizes)/2]
	return med, sizes[len(sizes)-1]
}
