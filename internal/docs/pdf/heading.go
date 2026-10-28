//go:build cgo

package pdf

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/giovani/readingdrip/internal/blocks"
)

var (
	// A numeração precisa ser seguida de uma palavra com maiúscula: evita
	// linhas de tabela como "0.3 300K 4.33".
	reSecao    = regexp.MustCompile(`^\d+(?:\.\d+)*\.?\s+\p{Lu}`)
	reCapitulo = regexp.MustCompile(`(?i)^(cap[ií]tulo|parte|anexo|ap[eê]ndice)\b`)
	reSoNumero = regexp.MustCompile(`^\d+(?:\.\d+)*\.?$`)
)

// candidatoTitulo decide se a linha parece um título por estilo (fonte maior
// que o corpo) ou por negrito + numeração/linha curta.
func candidatoTitulo(l Line, medSize float64) bool {
	texto := strings.TrimSpace(l.Text)
	if texto == "" || len([]rune(texto)) > 120 {
		return false
	}
	if !predominaTexto(texto) {
		return false
	}
	if medSize > 0 && l.Size >= medSize*1.15 {
		// Fonte um pouco maior só basta para título se a linha for curta ou
		// estiver em negrito; blocos de aviso/licença usam a mesma fonte de
		// um título, mas são longos e não são negrito.
		if l.Size >= medSize*1.4 || l.Bold || len([]rune(texto)) <= 60 {
			return true
		}
	}
	if l.Bold && len([]rune(texto)) < 60 {
		return pareceNumerado(texto) || isAllCapsOrTitle(texto)
	}
	return false
}

// predominaTexto rejeita rótulos de tabela/figura feitos de números e
// símbolos (ex.: "0.3 300K 4.33"), que não são títulos.
func predominaTexto(s string) bool {
	letras, visiveis := 0, 0
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		visiveis++
		if unicode.IsLetter(r) {
			letras++
		}
	}
	return letras >= 3 && letras*2 >= visiveis
}

// ranksDeTitulos mapeia cada tamanho de fonte de título (arredondado) para um
// nível, do maior para o menor. Tamanhos maiores que o título do documento
// (maxSize) são rótulos de figura e não entram na hierarquia. Tamanhos muito
// próximos são agrupados no mesmo nível.
func ranksDeTitulos(lines []Line, medSize, maxSize float64) map[float64]int {
	seen := make(map[float64]bool)
	var sizes []float64
	for _, l := range lines {
		if !candidatoTitulo(l, medSize) || l.Size < medSize*1.15 {
			continue
		}
		if maxSize > 0 && l.Size > maxSize*1.05 {
			continue
		}
		s := math.Round(l.Size*10) / 10
		if !seen[s] {
			seen[s] = true
			sizes = append(sizes, s)
		}
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(sizes)))

	ranks := make(map[float64]int, len(sizes))
	nivel := 1
	representante := 0.0
	for i, s := range sizes {
		if i > 0 && s < representante*0.92 {
			nivel++
			representante = s
		}
		if i == 0 {
			representante = s
		}
		ranks[s] = nivel
	}
	return ranks
}

// tamanhoDoTitulo devolve o maior tamanho de fonte de título na primeira
// página, que é a referência do título do documento.
func tamanhoDoTitulo(lines []Line, medSize float64) float64 {
	max := 0.0
	for _, l := range lines {
		if l.Page != 0 || l.Size < medSize*1.15 {
			continue
		}
		if l.Size > max {
			max = l.Size
		}
	}
	return max
}

func nivelDoTitulo(l Line, ranks map[float64]int) int {
	s := math.Round(l.Size*10) / 10
	if lv, ok := ranks[s]; ok && l.Size > 0 {
		if lv > 6 {
			lv = 6
		}
		return lv
	}
	if lv := nivelPorNumeracao(l.Text); lv > 0 {
		return lv
	}
	return 2
}

func pareceNumerado(s string) bool {
	s = strings.TrimSpace(s)
	return reSecao.MatchString(s) || reCapitulo.MatchString(s)
}

func nivelPorNumeracao(s string) int {
	s = strings.TrimSpace(s)
	if !reSecao.MatchString(s) && !reCapitulo.MatchString(s) {
		return 0
	}
	partes := strings.Fields(s)[0]
	partes = strings.TrimSuffix(partes, ".")
	if partes != "" && reSoNumero.MatchString(partes) {
		n := len(strings.Split(partes, "."))
		if n > 6 {
			n = 6
		}
		if n < 1 {
			n = 1
		}
		return n
	}
	return 1
}

func isAllCapsOrTitle(s string) bool {
	words := strings.Fields(s)
	if len(words) == 0 {
		return false
	}
	caps := 0
	for _, w := range words {
		if len([]rune(w)) > 1 && strings.ToUpper(w) == w {
			caps++
		}
	}
	return float64(caps)/float64(len(words)) > 0.5
}

func headingKindPDF(level int) blocks.Kind {
	switch level {
	case 2:
		return blocks.KindH2
	case 3:
		return blocks.KindH3
	case 4:
		return blocks.KindH4
	case 5:
		return blocks.KindH5
	case 6:
		return blocks.KindH6
	default:
		return blocks.KindH1
	}
}
