//go:build cgo

package pdf

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/giovani/readingdrip/internal/blocks"
)

func extrairFixture(t *testing.T, name string) *Result {
	t.Helper()
	path := "../../../testdata/pdf/" + name + ".pdf"
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("abrir fixture %s: %v", path, err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	res, err := Extract(f, fi.Size(), name)
	if err != nil {
		t.Fatalf("extrair %s: %v", name, err)
	}
	return res
}

func TestColunaUnica(t *testing.T) {
	res := extrairFixture(t, "coluna-unica")

	if res.Title != "A arte de ler devagar" {
		t.Errorf("título = %q, quer %q", res.Title, "A arte de ler devagar")
	}

	var kinds []blocks.Kind
	var texto strings.Builder
	for _, b := range res.Blocks {
		kinds = append(kinds, b.Kind)
		texto.WriteString(b.Text)
		texto.WriteString("\n")
	}

	want := []blocks.Kind{
		blocks.KindH1, blocks.KindParagraph, blocks.KindParagraph, blocks.KindH2,
		blocks.KindParagraph, blocks.KindH2, blocks.KindParagraph, blocks.KindParagraph,
		blocks.KindH2, blocks.KindParagraph, blocks.KindH2, blocks.KindParagraph,
		blocks.KindH2, blocks.KindParagraph,
	}
	if len(kinds) != len(want) {
		t.Fatalf("tipos = %v, quer %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("bloco %d: tipo = %s, quer %s", i, kinds[i], want[i])
		}
	}

	// Cabeçalho e número de página repetidos não entram no conteúdo.
	if strings.Contains(texto.String(), "Leitura em Doses - Manual") {
		t.Error("cabeçalho repetido não foi removido")
	}
	// Hifenização remontada.
	if !strings.Contains(texto.String(), "como leitura, precisam") {
		t.Errorf("hifenização não remontada:\n%s", texto.String())
	}
	// Parágrafos contínuos foram unidos em um único bloco.
	if !strings.Contains(texto.String(), "manter o habito por meses.") {
		t.Error("parágrafo da primeira página não foi unido")
	}
}

func TestDuasColunasOrdem(t *testing.T) {
	res := extrairFixture(t, "duas-colunas")

	var texto strings.Builder
	for _, b := range res.Blocks {
		texto.WriteString(b.Text)
		texto.WriteString("\n")
	}
	doc := texto.String()

	esperado := []string{
		"A primeira coluna comeca aqui e segue ate o fim da pagina, sem invadir o espaco reservado a coluna vizinha.",
		"Este paragrafo pertence somente a coluna da esquerda e termina aqui.",
		"A segunda coluna continua o texto com informacoes complementares sobre o mesmo assunto, em paralelo.",
		"Ja este paragrafo pertence apenas a coluna da direita e termina aqui.",
	}
	anterior := -1
	for _, frase := range esperado {
		i := strings.Index(doc, frase)
		if i < 0 {
			t.Fatalf("frase ausente: %q\n---\n%s", frase, doc)
		}
		if i <= anterior {
			t.Fatalf("ordem de leitura errada em %q\n---\n%s", frase, doc)
		}
		anterior = i
	}
}

func TestHeadingsDezPaginas(t *testing.T) {
	res := extrairFixture(t, "headings-10pg")

	var h1, h2, outros int
	for _, b := range res.Blocks {
		switch b.Kind {
		case blocks.KindH1:
			h1++
		case blocks.KindH2:
			h2++
		default:
			if b.Kind.IsHeading() {
				outros++
			}
		}
	}
	if h1 != 10 || h2 != 10 || outros != 0 {
		t.Fatalf("headings h1=%d h2=%d outros=%d, quer 10/10/0", h1, h2, outros)
	}
	if len(res.Blocks) != 40 {
		t.Fatalf("blocos = %d, quer 40 (10 páginas × 2 títulos + 2 parágrafos)", len(res.Blocks))
	}
}

func TestEscaneado(t *testing.T) {
	path := "../../../testdata/pdf/escaneado.pdf"
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	fi, _ := f.Stat()
	_, err = Extract(f, fi.Size(), "escaneado")
	if err == nil {
		t.Fatal("esperado erro para PDF sem camada de texto")
	}
	if !strings.Contains(err.Error(), "escaneado") {
		t.Fatalf("erro = %v, quer menção a PDF escaneado", err)
	}
}

func TestExtractInvalido(t *testing.T) {
	if _, err := Extract(bytes.NewReader(nil), 0, "x"); err == nil {
		t.Fatal("esperado erro para arquivo vazio")
	}
	src := []byte("%PDF-1.4 inválido")
	if _, err := Extract(bytes.NewReader(src), int64(len(src)), "x"); err == nil {
		t.Fatal("esperado erro para PDF inválido")
	}
}
