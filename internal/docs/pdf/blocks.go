//go:build cgo

package pdf

import (
	"fmt"
	stdhtml "html"
	"math"
	"regexp"
	"strings"
	"unicode"

	"github.com/giovani/readingdrip/internal/blocks"
)

// montarBlocos transforma linhas ordenadas em blocos, juntando linhas em
// parágrafos e detectando títulos por estilo.
func montarBlocos(lines []Line) []blocks.Block {
	if len(lines) == 0 {
		return nil
	}

	medSize := medianaTamanho(lines)
	medDelta := medianaDelta(lines)
	tituloMax := tamanhoDoTitulo(lines, medSize)
	ranks := ranksDeTitulos(lines, medSize, tituloMax)

	var out []blocks.Block
	var cur *paragrafo

	flush := func() {
		if cur == nil {
			return
		}
		texto := strings.TrimSpace(cur.text.String())
		if texto != "" {
			out = append(out, blocoPDF(cur.heading, cur.level, texto))
		}
		cur = nil
	}

	var prev *Line
	for i := range lines {
		l := lines[i]
		titulo := candidatoTitulo(l, medSize)
		if titulo && tituloMax > 0 && l.Size > tituloMax*1.05 {
			// Rótulos de figura podem usar fontes enormes; não são títulos.
			titulo = false
		}
		nivel := 0
		if titulo {
			nivel = nivelDoTitulo(l, ranks)
		}

		if cur == nil || deveQuebrar(prev, &l, cur, medSize, medDelta) {
			flush()
			cur = &paragrafo{heading: titulo, level: nivel}
			cur.text.WriteString(l.Text)
		} else {
			acrescentarTexto(&cur.text, l.Text)
		}
		cp := l
		prev = &cp
	}
	flush()

	return out
}

type paragrafo struct {
	heading bool
	level   int
	text    strings.Builder
}

func blocoPDF(heading bool, level int, texto string) blocks.Block {
	if heading {
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		kind := headingKindPDF(level)
		return blocks.Block{
			Kind:  kind,
			Level: level,
			Text:  texto,
			HTML:  fmt.Sprintf("<h%d>%s</h%d>", level, stdhtml.EscapeString(texto), level),
		}
	}
	return blocks.Block{
		Kind: blocks.KindParagraph,
		Text: texto,
		HTML: "<p>" + stdhtml.EscapeString(texto) + "</p>",
	}
}

func deveQuebrar(prev *Line, l *Line, cur *paragrafo, medSize, medDelta float64) bool {
	if prev == nil {
		return true
	}

	// Título e corpo nunca ficam no mesmo bloco.
	tituloCur := candidatoTitulo(*l, medSize)
	if cur.heading != tituloCur {
		return true
	}
	// Mudança de tamanho de fonte no corpo indica novo bloco.
	if !cur.heading && math.Abs(l.Size-prev.Size) > 0.5 {
		return true
	}
	// Fim de página: só continua se a frase anterior ficou em aberto.
	if l.Page != prev.Page {
		return terminaFrase(prev.Text)
	}
	// Mudança de coluna dentro da mesma página: o fluxo da coluna terminou.
	if l.Column != prev.Column {
		return true
	}
	// Vão vertical maior que 1,4× a entrelinha mediana.
	if medDelta > 0 && l.Y-prev.Y > medDelta*1.4 {
		return true
	}
	if l.Y <= prev.Y {
		return true
	}
	// Recuo de primeira linha depois de pontuação final.
	if terminaFrase(prev.Text) && comecaMaiuscula(l.Text) && l.X > prev.X+6 {
		return true
	}
	return false
}

func acrescentarTexto(b *strings.Builder, proxima string) {
	atual := b.String()
	if strings.HasSuffix(atual, "-") && len(proxima) > 0 && unicode.IsLower(rune(proxima[0])) {
		b.Reset()
		b.WriteString(strings.TrimSuffix(atual, "-"))
		b.WriteString(proxima)
		return
	}
	if !strings.HasSuffix(atual, " ") {
		b.WriteString(" ")
	}
	b.WriteString(proxima)
}

func terminaFrase(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	last := []rune(s)
	switch last[len(last)-1] {
	case '.', '!', '?', ':', ';', '…':
		return true
	}
	return false
}

func comecaMaiuscula(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		return unicode.IsUpper(r)
	}
	return false
}

func medianaTamanho(lines []Line) float64 {
	var sizes []float64
	for _, l := range lines {
		if l.Size > 0 {
			sizes = append(sizes, l.Size)
		}
	}
	return mediana(sizes)
}

// medianaDelta calcula a entrelinha mediana entre linhas consecutivas da
// mesma coluna e página.
func medianaDelta(lines []Line) float64 {
	var deltas []float64
	var prev *Line
	for i := range lines {
		l := lines[i]
		if prev != nil && l.Page == prev.Page && l.Column == prev.Column {
			if d := l.Y - prev.Y; d > 0 {
				deltas = append(deltas, d)
			}
		}
		cp := l
		prev = &cp
	}
	if len(deltas) == 0 {
		if len(lines) > 0 {
			return lines[0].LineHeight
		}
		return 0
	}
	return mediana(deltas)
}

// blocosDeTexto é o caminho de fallback quando o HTML estruturado não está
// disponível: separa parágrafos por linha em branco e detecta títulos apenas
// por numeração.
func blocosDeTexto(text string) []blocks.Block {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	paras := regexp.MustCompile(`\n\s*\n`).Split(text, -1)
	var out []blocks.Block
	for _, p := range paras {
		p = desfazerHifenizacao(p)
		p = normalizarLinha(p)
		if p == "" {
			continue
		}
		if pareceNumerado(p) && len([]rune(p)) < 80 {
			nivel := nivelPorNumeracao(p)
			if nivel < 1 {
				nivel = 1
			}
			out = append(out, blocoPDF(true, nivel, p))
			continue
		}
		out = append(out, blocoPDF(false, 0, p))
	}
	return out
}

func desfazerHifenizacao(s string) string {
	re := regexp.MustCompile(`(\p{Ll})-\s*\n\s*(\p{Ll})`)
	return re.ReplaceAllString(s, "$1$2")
}
