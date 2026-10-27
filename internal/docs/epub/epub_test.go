package epub

import (
	"archive/zip"
	"bytes"
	"sort"
	"strings"
	"testing"

	"github.com/giovani/readingdrip/internal/blocks"
)

func makeEPUB(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("criar %s: %v", name, err)
		}
		if _, err := w.Write([]byte(files[name])); err != nil {
			t.Fatalf("escrever %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("fechar zip: %v", err)
	}
	return buf.Bytes()
}

const testContainer = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

const testPackage = `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>Livro de Teste</dc:title>
    <dc:creator>Ana Ribeiro</dc:creator>
    <dc:creator>Bruno Lima</dc:creator>
    <dc:language>pt-BR</dc:language>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="cover" href="cover.xhtml" media-type="application/xhtml+xml"/>
    <item id="ch1" href="ch1.xhtml" media-type="application/xhtml+xml"/>
    <item id="ch2" href="ch2.xhtml" media-type="application/xhtml+xml"/>
    <item id="style" href="style.css" media-type="text/css"/>
  </manifest>
  <spine>
    <itemref idref="nav" linear="no"/>
    <itemref idref="cover" linear="no"/>
    <itemref idref="ch1"/>
    <itemref idref="ch2"/>
  </spine>
</package>`

const testChapter1 = `<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
  <title>Capítulo 1</title>
  <style>p { color: red }</style>
  <script>alert("xss")</script>
</head>
<body>
  <h1>Primeiro capítulo</h1>
  <p>Texto com <strong>ênfase</strong> e <a href="https://example.com" onclick="evil()">link externo</a>.</p>
  <p><a href="javascript:alert(1)">perigo</a> e <a href="mailto:a@b.c">email</a>.</p>
  <h2>Seção A</h2>
  <ul>
    <li>Primeiro item</li>
    <li>Segundo <em>item</em></li>
  </ul>
  <pre><code class="language-go">func main() {
    println("oi")
}</code></pre>
  <blockquote><p>Uma citação.</p></blockquote>
  <aside epub:type="footnote"><p>Nota de rodapé.</p></aside>
  <p><img alt="Diagrama do fluxo" src="img/diagram.png"></p>
</body>
</html>`

const testChapter2 = `<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<body>
  <h1>Segundo capítulo</h1>
  <p>Texto final.</p>
  <table>
    <caption>Comparação</caption>
    <tr><th>Estratégia</th><th>Risco</th></tr>
    <tr><td>Ler até cansar</td><td>irregular</td></tr>
  </table>
  <figure><img alt="Fluxo do sistema" src="img/fluxo.png"><figcaption>Fluxo</figcaption></figure>
  <hr>
  <p>Depois.</p>
</body>
</html>`

func testBook(t *testing.T) []byte {
	t.Helper()
	return makeEPUB(t, map[string]string{
		"META-INF/container.xml": testContainer,
		"OEBPS/content.opf":      testPackage,
		"OEBPS/nav.xhtml":        `<html><body><h1>Sumário</h1></body></html>`,
		"OEBPS/cover.xhtml":      `<html><body><h1>Capa</h1></body></html>`,
		"OEBPS/ch1.xhtml":        testChapter1,
		"OEBPS/ch2.xhtml":        testChapter2,
	})
}

func TestExtract(t *testing.T) {
	res, err := Extract(testBook(t), "fallback")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if res.Title != "Livro de Teste" {
		t.Fatalf("title = %q", res.Title)
	}
	if res.Author != "Ana Ribeiro, Bruno Lima" {
		t.Fatalf("author = %q", res.Author)
	}
	if res.Language != "pt-BR" {
		t.Fatalf("language = %q", res.Language)
	}

	var kinds []blocks.Kind
	for _, b := range res.Blocks {
		kinds = append(kinds, b.Kind)
		if b.Kind != blocks.KindHR && strings.TrimSpace(b.Text) == "" {
			t.Errorf("bloco %s sem texto", b.Kind)
		}
	}
	want := []blocks.Kind{
		blocks.KindH1, blocks.KindParagraph, blocks.KindParagraph, blocks.KindH2,
		blocks.KindListItem, blocks.KindListItem, blocks.KindCode, blocks.KindQuote,
		blocks.KindFootnote, blocks.KindFigure, blocks.KindH1, blocks.KindParagraph,
		blocks.KindTable, blocks.KindFigure, blocks.KindHR, blocks.KindParagraph,
	}
	if len(kinds) != len(want) {
		t.Fatalf("kinds = %v, want %d blocos", kinds, len(want))
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("kinds[%d] = %s, want %s (todos: %v)", i, kinds[i], want[i], kinds)
		}
	}

	allText := make([]string, len(res.Blocks))
	for i, b := range res.Blocks {
		allText[i] = b.Text
	}
	joined := strings.Join(allText, "\n")
	if strings.Contains(joined, "Sumário") || strings.Contains(joined, "Capa") {
		t.Fatalf("nav/capa não deveriam entrar no conteúdo: %q", joined)
	}
	if !strings.Contains(joined, "Nota de rodapé.") {
		t.Fatalf("nota de rodapé ausente: %q", joined)
	}

	var code, codeHTML, table, figure, quote, footnote string
	var paraHTML, linkHTML string
	for _, b := range res.Blocks {
		switch b.Kind {
		case blocks.KindCode:
			code = b.Text
			codeHTML = b.HTML
		case blocks.KindTable:
			table = b.HTML
		case blocks.KindFigure:
			figure = b.HTML
		case blocks.KindQuote:
			quote = b.HTML
		case blocks.KindFootnote:
			footnote = b.HTML
		case blocks.KindParagraph:
			if strings.Contains(b.Text, "link externo") {
				paraHTML = b.HTML
			}
			if strings.Contains(b.Text, "perigo") {
				linkHTML = b.HTML
			}
		}
	}

	if !strings.Contains(code, "func main") || !strings.Contains(code, "\n") {
		t.Fatalf("código = %q", code)
	}
	if !strings.Contains(codeHTML, `class="language-go"`) {
		t.Fatalf("código sem linguagem: %q", codeHTML)
	}
	if !strings.Contains(table, "<table>") || !strings.Contains(table, "<th>Estratégia</th>") || !strings.Contains(table, "<td>irregular</td>") {
		t.Fatalf("tabela = %q", table)
	}
	if !strings.Contains(figure, `alt="Fluxo do sistema"`) || !strings.Contains(figure, "<figcaption>Fluxo</figcaption>") {
		t.Fatalf("figura = %q", figure)
	}
	if !strings.Contains(quote, "<p>Uma citação.</p>") {
		t.Fatalf("citação = %q", quote)
	}
	if !strings.Contains(footnote, "Nota de rodapé") {
		t.Fatalf("footnote = %q", footnote)
	}

	if !strings.Contains(paraHTML, `<strong>ênfase</strong>`) {
		t.Fatalf("HTML do parágrafo = %q", paraHTML)
	}
	if !strings.Contains(paraHTML, `rel="noopener noreferrer"`) {
		t.Fatalf("link sem rel: %q", paraHTML)
	}
	if strings.Contains(paraHTML, "onclick") {
		t.Fatalf("onclick sobreviveu: %q", paraHTML)
	}
	if strings.Contains(linkHTML, "javascript:") {
		t.Fatalf("javascript: sobreviveu: %q", linkHTML)
	}
	if !strings.Contains(linkHTML, `href="mailto:a@b.c"`) {
		t.Fatalf("mailto removido: %q", linkHTML)
	}
	for _, b := range res.Blocks {
		if strings.Contains(strings.ToLower(b.HTML), "<script") || strings.Contains(strings.ToLower(b.HTML), "<style") {
			t.Fatalf("script/style no HTML: %q", b.HTML)
		}
	}
}

func TestFallbackTitle(t *testing.T) {
	opf := `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:language>pt</dc:language></metadata>
  <manifest><item id="c" href="c.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine><itemref idref="c"/></spine>
</package>`
	data := makeEPUB(t, map[string]string{
		"META-INF/container.xml": testContainer,
		"OEBPS/content.opf":      opf,
		"OEBPS/c.xhtml":          `<html><body><p>Sem título.</p></body></html>`,
	})
	res, err := Extract(data, "arquivo")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.Title != "arquivo" {
		t.Fatalf("title = %q", res.Title)
	}
	if len(res.Blocks) != 1 || res.Blocks[0].Text != "Sem título." {
		t.Fatalf("blocks = %#v", res.Blocks)
	}
}

func TestErrors(t *testing.T) {
	valid := testBook(t)

	if _, err := Extract(nil, "x"); err == nil {
		t.Fatal("esperava erro para EPUB vazio")
	}
	if _, err := Extract([]byte("não é zip"), "x"); err == nil {
		t.Fatal("esperava erro para zip inválido")
	}
	if _, err := Extract(makeEPUB(t, map[string]string{"x": "y"}), "x"); err == nil {
		t.Fatal("esperava erro sem container.xml")
	}

	noRoot := makeEPUB(t, map[string]string{
		"META-INF/container.xml": `<container><rootfiles></rootfiles></container>`,
	})
	if _, err := Extract(noRoot, "x"); err == nil {
		t.Fatal("esperava erro sem rootfile")
	}

	noSpine := makeEPUB(t, map[string]string{
		"META-INF/container.xml": testContainer,
		"OEBPS/content.opf": `<?xml version="1.0"?><package>
  <manifest><item id="c" href="c.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine></spine></package>`,
		"OEBPS/c.xhtml": `<html><body><p>x</p></body></html>`,
	})
	if _, err := Extract(noSpine, "x"); err == nil {
		t.Fatal("esperava erro para spine vazio")
	}

	emptyBlocks := makeEPUB(t, map[string]string{
		"META-INF/container.xml": testContainer,
		"OEBPS/content.opf": `<?xml version="1.0"?><package>
  <manifest><item id="c" href="c.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine><itemref idref="c"/></spine></package>`,
		"OEBPS/c.xhtml": `<html><body><script>x</script></body></html>`,
	})
	if _, err := Extract(emptyBlocks, "x"); err == nil {
		t.Fatal("esperava erro sem blocos")
	}

	escape := makeEPUB(t, map[string]string{
		"META-INF/container.xml": testContainer,
		"OEBPS/content.opf": `<?xml version="1.0"?><package>
  <manifest><item id="c" href="../../evil.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine><itemref idref="c"/></spine></package>`,
	})
	if _, err := Extract(escape, "x"); err == nil {
		t.Fatal("esperava erro para href fora do pacote")
	}

	if _, err := Extract(valid, "x"); err != nil {
		t.Fatalf("fixture válida quebrou: %v", err)
	}
}

func TestResolveHref(t *testing.T) {
	cases := []struct {
		opf, href, want string
		wantErr         bool
	}{
		{"OEBPS/content.opf", "ch1.xhtml", "OEBPS/ch1.xhtml", false},
		{"OEBPS/content.opf", "ch1.xhtml#frag", "OEBPS/ch1.xhtml", false},
		{"OEBPS/content.opf", "Chapter%201.xhtml", "OEBPS/Chapter 1.xhtml", false},
		{"content.opf", "../evil.xhtml", "", true},
		{"OEBPS/content.opf", "../Text/ch1.xhtml", "Text/ch1.xhtml", false},
		{"OEBPS/content.opf", "/abs.xhtml", "", true},
		{"OEBPS/content.opf", "https://example.com/a.xhtml", "", true},
		{"OEBPS/content.opf", "", "", false},
	}
	for _, tc := range cases {
		got, err := resolveHref(tc.opf, tc.href)
		if tc.wantErr {
			if err == nil {
				t.Errorf("resolveHref(%q) = %q, esperava erro", tc.href, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("resolveHref(%q): %v", tc.href, err)
			continue
		}
		if got != tc.want {
			t.Errorf("resolveHref(%q) = %q, want %q", tc.href, got, tc.want)
		}
	}
}

func TestSafeHref(t *testing.T) {
	allowed := []string{"https://example.com", "http://x", "mailto:a@b.c", "tel:+55", "#id", "/path", "rel/file"}
	for _, href := range allowed {
		if got := safeHref(href); got != href {
			t.Errorf("safeHref(%q) = %q, want allow", href, got)
		}
	}
	blocked := []string{"javascript:alert(1)", "JaVaScRiPt:alert(1)", "data:text/html,x", "vbscript:x", "file:///etc/passwd", "http://x/\x00"}
	for _, href := range blocked {
		if got := safeHref(href); got != "" {
			t.Errorf("safeHref(%q) = %q, want block", href, got)
		}
	}
}

func TestAllLinearNoStillReads(t *testing.T) {
	opf := `<?xml version="1.0"?><package>
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>X</dc:title></metadata>
  <manifest><item id="c" href="c.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine><itemref idref="c" linear="no"/></spine></package>`
	data := makeEPUB(t, map[string]string{
		"META-INF/container.xml": testContainer,
		"OEBPS/content.opf":      opf,
		"OEBPS/c.xhtml":          `<html><body><h1>Único</h1></body></html>`,
	})
	res, err := Extract(data, "x")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(res.Blocks) != 1 || res.Blocks[0].Text != "Único" {
		t.Fatalf("blocks = %#v", res.Blocks)
	}
}
