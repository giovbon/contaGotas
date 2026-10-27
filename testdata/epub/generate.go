// Command generate cria a fixture EPUB usada nos testes.
//
// Uso, a partir da raiz do repositório:
//
//	go run testdata/epub/generate.go
package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"sort"
	"time"
)

func main() {
	files := map[string]string{
		"META-INF/container.xml": container,
		"OEBPS/content.opf":      opf,
		"OEBPS/nav.xhtml":        navigation,
		"OEBPS/ch1.xhtml":        ch1,
		"OEBPS/ch2.xhtml":        ch2,
		"OEBPS/ch3.xhtml":        ch3,
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range names {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.Modified = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
		w, err := zw.CreateHeader(h)
		if err != nil {
			panic(err)
		}
		if _, err := w.Write([]byte(files[name])); err != nil {
			panic(err)
		}
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}

	if err := os.WriteFile("testdata/epub/livro.epub", buf.Bytes(), 0o644); err != nil {
		panic(err)
	}
	fmt.Println("testdata/epub/livro.epub")
}

const container = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

const opf = `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>Leitura em Camadas</dc:title>
    <dc:creator>Ana Ribeiro</dc:creator>
    <dc:language>pt-BR</dc:language>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="ch1" href="ch1.xhtml" media-type="application/xhtml+xml"/>
    <item id="ch2" href="ch2.xhtml" media-type="application/xhtml+xml"/>
    <item id="ch3" href="ch3.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="nav" linear="no"/>
    <itemref idref="ch1"/>
    <itemref idref="ch2"/>
    <itemref idref="ch3"/>
  </spine>
</package>`

const navigation = `<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<body>
  <nav epub:type="toc">
    <h1>Sumário</h1>
    <ol>
      <li><a href="ch1.xhtml">A primeira camada</a></li>
      <li><a href="ch2.xhtml">A segunda camada</a></li>
      <li><a href="ch3.xhtml">A terceira camada</a></li>
    </ol>
  </nav>
</body>
</html>`

const ch1 = `<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>A primeira camada</title></head>
<body>
  <h1>A primeira camada</h1>
  <p>Ler em camadas é aceitar que um texto não se entrega de uma vez. Na primeira passagem, o leitor reconhece o terreno: o tom, o vocabulário, a promessa do autor. Nas passagens seguintes, volta aos mesmos parágrafos com perguntas diferentes e encontra o que antes passou despercebido.</p>
  <p>Esse retorno não é desperdício. É o que transforma informação em memória. Uma dose diária bem desenhada protege esse ciclo: começa em uma fronteira reconhecível, termina em outra e deixa uma pergunta aberta que pertence ao leitor.</p>
  <h2>Ritmo e atenção</h2>
  <p>O ritmo não é apenas velocidade. É a relação entre o esforço e o descanso, entre a densidade de uma ideia e o espaço para assimilá-la. Quando o texto exige mais, a dose precisa encolher; quando flui, pode crescer sem cansar.</p>
  <ul>
    <li>Escolha um horário protegido, mesmo que curto.</li>
    <li>Termine sempre em um ponto que você consiga nomear.</li>
    <li>Anote uma pergunta para o dia seguinte.</li>
  </ul>
  <blockquote><p>Uma boa interrupção não fecha o livro: deixa uma porta entreaberta.</p></blockquote>
</body>
</html>`

const ch2 = `<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>A segunda camada</title></head>
<body>
  <h1>A segunda camada</h1>
  <p>Na segunda camada, o leitor já não precisa reconstruir o mapa. Pode prestar atenção nas conexões: como uma ideia antecipa outra, como um exemplo contradiz a tese, como o autor volta a uma imagem do primeiro capítulo.</p>
  <p>É aqui que a releitura deixa de ser repetição. O mesmo parágrafo ilumina uma passagem distante; uma nota de rodapé muda o sentido de uma seção inteira. O texto ganha espessura sem que uma palavra nova tenha sido acrescentada.</p>
  <h2>Continuidade</h2>
  <p>A continuidade é feita de pequenos sinais. Repetir o caminho hierárquico, recuperar as últimas frases e anunciar o que vem a seguir reduz o custo de retomada. O leitor não precisa lembrar de tudo; precisa saber onde está.</p>
  <pre><code class="language-go">func retomar(ultimo Trecho) Dose {
    contexto := ultimo.Resumo(3)
    return Dose{
        Abertura: contexto,
        Caminho:  ultimo.Seccao,
    }
}</code></pre>
  <table>
    <caption>Camadas de leitura</caption>
    <tr><th>Camada</th><th>Pergunta</th></tr>
    <tr><td>Primeira</td><td>Do que trata?</td></tr>
    <tr><td>Segunda</td><td>Como se conecta?</td></tr>
    <tr><td>Terceira</td><td>O que mudou em mim?</td></tr>
  </table>
  <figure><img alt="Três camadas concêntricas" src="img/camadas.png"><figcaption>Camadas concêntricas de leitura</figcaption></figure>
</body>
</html>`

const ch3 = `<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>A terceira camada</title></head>
<body>
  <h1>A terceira camada</h1>
  <p>A terceira camada é a mais lenta e a mais difícil de ensinar. Ela não depende de técnicas de retomada, mas de tempo vivido: o leitor relaciona o texto com outras leituras, com o próprio trabalho e com perguntas que ainda não sabia formular.</p>
  <p>Não há atalho para essa camada. O que existe é espaço protegido, repetição espaçada e uma disposição para tolerar a ambiguidade sem correr para a conclusão. O texto continua trabalhando quando o livro já está fechado.</p>
  <h2>O fim</h2>
  <p>Terminar um livro é menos um evento e mais uma dissolução. As ideias continuam trabalhando depois da última página, em conversas, decisões e novas leituras. Um bom sistema de doses não compete com esse trabalho: ele apenas mantém o texto vivo até que ele encontre o leitor.</p>
  <aside epub:type="footnote"><p>Uma nota sobre notas: elas também merecem uma dose própria.</p></aside>
</body>
</html>`
