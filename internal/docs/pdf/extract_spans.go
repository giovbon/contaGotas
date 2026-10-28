//go:build cgo

package pdf

import (
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/text/unicode/norm"
)

// extrairLinhasHTML converte o HTML estruturado do MuPDF em linhas com
// posição. Cada <p> gerado pelo MuPDF corresponde a uma linha visual e traz
// top/left/line-height no atributo style.
func extrairLinhasHTML(src string, page int) ([]Line, error) {
	root, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return nil, err
	}

	var lines []Line
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "p" {
			if l, ok := linhaDeParagrafo(n, page); ok {
				lines = append(lines, l)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return lines, nil
}

func linhaDeParagrafo(n *html.Node, page int) (Line, bool) {
	style := estilo(n)
	texto, size, bold := textoDaLinha(n)
	texto = normalizarLinha(texto)
	if texto == "" {
		return Line{}, false
	}

	l := Line{
		Page:       page,
		X:          pt(style["left"]),
		Y:          pt(style["top"]),
		LineHeight: pt(style["line-height"]),
		Size:       size,
		Bold:       bold,
		Text:       texto,
	}
	if l.Size == 0 {
		l.Size = l.LineHeight
	}
	if l.LineHeight == 0 {
		l.LineHeight = l.Size
	}
	return l, true
}

// textoDaLinha concatena o texto dos spans e devolve o maior tamanho de fonte
// e se algum span está em negrito.
func textoDaLinha(n *html.Node) (string, float64, bool) {
	var b strings.Builder
	var size float64
	bold := false

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.ElementNode:
			switch n.Data {
			case "br":
				b.WriteString(" ")
			case "b", "strong":
				bold = true
			}
			st := estilo(n)
			if v := pt(st["font-size"]); v > size {
				size = v
			}
			if strings.Contains(strings.ToLower(st["font-weight"]), "bold") {
				bold = true
			}
		case html.TextNode:
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String(), size, bold
}

func estilo(n *html.Node) map[string]string {
	m := make(map[string]string)
	for _, a := range n.Attr {
		if a.Key != "style" {
			continue
		}
		for _, decl := range strings.Split(a.Val, ";") {
			key, val, ok := strings.Cut(decl, ":")
			if !ok {
				continue
			}
			m[strings.TrimSpace(key)] = strings.TrimSpace(val)
		}
	}
	return m
}

func pt(s string) float64 {
	s = strings.TrimSpace(strings.TrimSuffix(s, "pt"))
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func normalizarLinha(s string) string {
	s = norm.NFC.String(s)
	s = strings.ReplaceAll(s, "\u00ad", "")
	s = strings.ReplaceAll(s, "\u00a0", " ")
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

// removerCabecalhoRodape descarta linhas no topo/rodapé que se repetem em mais
// de 40% das páginas, além de números de página isolados.
func removerCabecalhoRodape(paginas [][]Line, larguras, alturas []float64) [][]Line {
	total := len(paginas)
	if total == 0 {
		return paginas
	}

	contagem := make(map[string]int)
	inMargem := func(l Line, page int) bool {
		h := alturas[page]
		if h == 0 {
			h = 842
		}
		w := larguras[page]
		if w == 0 {
			w = 595
		}
		return l.Y < h*0.10 || l.Y > h*0.90 || l.X < w*0.05 || l.X > w*0.95
	}
	for i, pg := range paginas {
		for _, l := range pg {
			if !inMargem(l, i) || len(l.Text) > 100 {
				continue
			}
			contagem[l.Text]++
		}
	}

	repetidos := make(map[string]bool)
	for t, c := range contagem {
		if float64(c)/float64(total) > 0.4 {
			repetidos[t] = true
		}
	}

	out := make([][]Line, len(paginas))
	for i, pg := range paginas {
		limpo := make([]Line, 0, len(pg))
		w := larguras[i]
		if w == 0 {
			w = 595
		}
		h := alturas[i]
		if h == 0 {
			h = 842
		}
		for _, l := range pg {
			// Margens externas contêm marcas d'água, numeração de linha e
			// rodapés; não fazem parte do corpo.
			if l.X < w*0.04 || l.X > w*0.96 || l.Y < h*0.04 || l.Y > h*0.96 {
				continue
			}
			if inMargem(l, i) && (repetidos[l.Text] || ehNumeroDePagina(l.Text)) {
				continue
			}
			limpo = append(limpo, l)
		}
		out[i] = limpo
	}
	return out
}

func ehNumeroDePagina(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 6 {
		return false
	}
	if _, err := strconv.Atoi(s); err == nil {
		return true
	}
	// Números romanos minúsculos/maiúsculos usados em front matter.
	return strings.Trim(strings.ToLower(s), "ivxlcdm") == ""
}
