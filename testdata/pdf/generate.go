// Command generate cria as fixtures PDF usadas nos testes de extracao.
//
// Os arquivos sao montados a mao (sem dependencia externa), com fontes
// base-14 do PDF, para que o resultado seja deterministico e o conteudo
// textual seja conhecido.
//
// Uso, a partir da raiz do repositorio:
//
//	go run testdata/pdf/generate.go
package main

import (
	"fmt"
	"os"
	"strings"
)

// page descreve uma pagina: lista de linhas posicionadas.
type page struct {
	lines []line
}

type line struct {
	x, y float64
	size float64
	bold bool
	text string
}

func main() {
	files := map[string][]page{
		"coluna-unica.pdf":  colunaUnica(),
		"duas-colunas.pdf":  duasColunas(),
		"escaneado.pdf":     escaneado(),
		"headings-10pg.pdf": headingsDezPaginas(),
	}
	for name, pages := range files {
		path := "testdata/pdf/" + name
		if err := os.WriteFile(path, buildPDF(pages), 0o644); err != nil {
			panic(err)
		}
		fmt.Println(path)
	}
}

func colunaUnica() []page {
	const (
		left  = 72.0
		right = 523.0
	)
	header := func(n int) []line {
		return []line{
			{x: left, y: 800, size: 9, text: "Leitura em Doses - Manual"},
			{x: right - 20, y: 40, size: 9, text: fmt.Sprintf("%d", n)},
		}
	}

	p1 := page{lines: append(header(1),
		line{x: left, y: 740, size: 24, text: "A arte de ler devagar"},
		line{x: left, y: 700, size: 12, text: "Este manual descreve uma forma simples de manter a leitura"},
		line{x: left, y: 684, size: 12, text: "diaria, mesmo em semanas ocupadas. A ideia central e dividir"},
		line{x: left, y: 668, size: 12, text: "o texto em doses curtas e previsiveis."},
		line{x: left, y: 640, size: 12, text: "Cada dose deve caber em uma pausa do dia. O objetivo nao e"},
		line{x: left, y: 624, size: 12, text: "terminar rapido, mas manter o habito por meses."},
		line{x: left, y: 580, size: 18, text: "1. Como comecar"},
		line{x: left, y: 552, size: 12, text: "Escolha um documento, defina quantos minutos por dia voce"},
		line{x: left, y: 536, size: 12, text: "tem disponiveis e deixe o planejador calcular o restante."},
	)}

	p2 := page{lines: append(header(2),
		line{x: left, y: 740, size: 18, text: "1.1 Um exemplo concreto"},
		line{x: left, y: 712, size: 12, text: "Com dez minutos por dia e um ritmo de duzentas palavras por"},
		line{x: left, y: 696, size: 12, text: "minuto, cada dose cobre cerca de duas mil palavras. Um livro"},
		line{x: left, y: 680, size: 12, text: "de sessenta mil palavras rende aproximadamente trinta dias."},
		line{x: left, y: 652, size: 12, text: "O plano e recalculado por inteiro quando os parametros mudam,"},
		line{x: left, y: 636, size: 12, text: "o que mantem o resultado previsivel."},
		line{x: left, y: 592, size: 18, text: "1.2 Hifenizacao"},
		line{x: left, y: 564, size: 12, text: "Palavras quebradas no fim da linha, como leitu-"},
		line{x: left, y: 548, size: 12, text: "ra, precisam ser remontadas antes da segmentacao."},
	)}

	p3 := page{lines: append(header(3),
		line{x: left, y: 740, size: 18, text: "2. Conclusao"},
		line{x: left, y: 712, size: 12, text: "A consistencia vence a intensidade. Leia um pouco todos os"},
		line{x: left, y: 696, size: 12, text: "dias e o progresso aparece sem esforco extraordinario."},
		line{x: left, y: 660, size: 12, bold: true, text: "RESUMO"},
		line{x: left, y: 636, size: 12, text: "Leia um pouco todos os dias e o progresso aparece."},
	)}

	return []page{p1, p2, p3}
}

func duasColunas() []page {
	const (
		left  = 72.0
		right = 320.0
	)
	header := func(n int) []line {
		return []line{
			{x: left, y: 800, size: 9, text: "Revista de Leitura - Edicao 1"},
			{x: 515, y: 40, size: 9, text: fmt.Sprintf("%d", n)},
		}
	}

	// O fluxo de conteudo e escrito linha a linha (esquerda, direita) para
	// garantir que a ordem de leitura nao dependa da ordem do fluxo.
	var lines []line
	lines = append(lines,
		line{x: left, y: 740, size: 20, text: "Colunas e ordem de leitura"},
		line{x: left, y: 712, size: 11, text: "A primeira coluna comeca aqui e segue"},
		line{x: right, y: 712, size: 11, text: "A segunda coluna continua o texto com"},
		line{x: left, y: 697, size: 11, text: "ate o fim da pagina, sem invadir o"},
		line{x: right, y: 697, size: 11, text: "informacoes complementares sobre o"},
		line{x: left, y: 682, size: 11, text: "espaco reservado a coluna vizinha."},
		line{x: right, y: 682, size: 11, text: "mesmo assunto, em paralelo."},
		line{x: left, y: 655, size: 11, text: "Este paragrafo pertence somente a"},
		line{x: right, y: 655, size: 11, text: "Ja este paragrafo pertence apenas a"},
		line{x: left, y: 640, size: 11, text: "coluna da esquerda e termina aqui."},
		line{x: right, y: 640, size: 11, text: "coluna da direita e termina aqui."},
	)
	p1 := page{lines: append(header(1), lines...)}
	return []page{p1}
}

func escaneado() []page {
	return []page{{}}
}

// headingsDezPaginas gera dez páginas com dois títulos cada, para medir a
// detecção por estilo (critério do SPIKE-01).
func headingsDezPaginas() []page {
	pages := make([]page, 0, 10)
	for n := 1; n <= 10; n++ {
		lines := []line{
			{x: 72, y: 800, size: 9, text: "Manual de Teste"},
			{x: 515, y: 40, size: 9, text: fmt.Sprintf("%d", n)},
			{x: 72, y: 740, size: 18, text: fmt.Sprintf("Capitulo %d", n)},
			{x: 72, y: 712, size: 11, text: "Este paragrafo de apoio existe apenas para dar corpo a pagina."},
			{x: 72, y: 697, size: 11, text: "Ele nao deve ser confundido com um titulo pelo extrator."},
			{x: 72, y: 655, size: 14, text: fmt.Sprintf("%d.1 Detalhe", n)},
			{x: 72, y: 627, size: 11, text: "Mais um paragrafo de corpo, com duas linhas de extensao."},
			{x: 72, y: 612, size: 11, text: "O conteudo se repete de forma deliberadamente previsivel."},
		}
		pages = append(pages, page{lines: lines})
	}
	return pages
}

// buildPDF monta um PDF 1.4 minimo com uma fonte Helvetica.
func buildPDF(pages []page) []byte {
	var objs []string
	add := func(body string) int {
		objs = append(objs, body)
		return len(objs)
	}

	fontID := add("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	fontBoldID := add("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")

	// Reserva o catalogo e a arvore de paginas.
	catalogID := add("")
	pagesID := add("")

	var kids []string
	for _, p := range pages {
		content := contentStream(p.lines)
		contentID := add(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))
		pageID := add(fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 %d 0 R /F2 %d 0 R >> >> /Contents %d 0 R >>",
			pagesID, fontID, fontBoldID, contentID))
		kids = append(kids, fmt.Sprintf("%d 0 R", pageID))
	}

	objs[catalogID-1] = fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pagesID)
	objs[pagesID-1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pages))

	var buf strings.Builder
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, body := range objs {
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(objs)+1)
	buf.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, catalogID, xref)
	return []byte(buf.String())
}

func contentStream(lines []line) string {
	var b strings.Builder
	for _, l := range lines {
		if l.text == "" {
			continue
		}
		font := "/F1"
		if l.bold {
			font = "/F2"
		}
		fmt.Fprintf(&b, "BT %s %.1f Tf %.1f %.1f Td (%s) Tj ET\n", font, l.size, l.x, l.y, escapePDF(l.text))
	}
	// Pagina sem texto (escaneado): apenas um retangulo preenchido.
	if b.Len() == 0 {
		b.WriteString("0.9 g 40 40 515 762 re f\n")
	}
	return b.String()
}

func escapePDF(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "(", `\(`)
	s = strings.ReplaceAll(s, ")", `\)`)
	return s
}
