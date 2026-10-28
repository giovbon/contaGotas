// Package delivery compõe a mensagem diária e publica no ntfy self-hosted.
package delivery

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/giovani/readingdrip/internal/blocks"
	"github.com/giovani/readingdrip/internal/store"
)

const (
	// DefaultMaxMessageBytes é a margem de segurança sobre os 64 KiB do ntfy
	// (docs/delivery-ntfy.md).
	DefaultMaxMessageBytes = 60_000
	// DefaultPreviewWords é o corte do modo preview.
	DefaultPreviewWords = 400
	// RecapMaxChars limita o recap extrativo.
	RecapMaxChars = 240
	// NextPreviewMaxChars limita a prévia do próximo chunk no rodapé.
	NextPreviewMaxChars = 140
	// MinMessageBytes é o menor limite de corpo aceito na composição. Abaixo
	// disso não cabe cabeçalho, corpo mínimo e link.
	MinMessageBytes = 1_000
)

// BodyMode define quanto do texto vai na mensagem.
type BodyMode string

const (
	BodyFull    BodyMode = "full"
	BodyPreview BodyMode = "preview"
)

// Action é uma ação do ntfy (view ou http).
type Action struct {
	Action string `json:"action"`
	Label  string `json:"label"`
	URL    string `json:"url"`
	Method string `json:"method,omitempty"`
}

// Message é uma mensagem pronta para o ntfy. O corpo é medido em bytes UTF-8.
type Message struct {
	Title   string
	Body    string
	Click   string
	Actions []Action
	// Chunks identifica os trechos incluídos, para log e histórico.
	Chunks []int64
}

// Dose é o material de um documento em uma entrega. Body, Previous e Next
// são os blocos do chunk atual, do anterior e do próximo, já recortados pelo
// chamador a partir de Blocks.List.
type Dose struct {
	Title    string
	Author   string
	Chunk    store.Chunk
	Body     []blocks.Block
	Previous []blocks.Block
	Next     []blocks.Block
	// Percent é o progresso do documento (0..100); negativo omite a métrica.
	Percent  int
	Link     string // leitor
	DoneLink string // marcar como lido
	// BodyMode permite override por documento; vazio usa o global.
	BodyMode BodyMode
}

// Options controla a composição.
type Options struct {
	BodyMode        BodyMode
	MaxMessageBytes int
	PreviewWords    int
}

func (o Options) normalizar() (Options, error) {
	if o.BodyMode == "" {
		o.BodyMode = BodyFull
	}
	switch o.BodyMode {
	case BodyFull, BodyPreview:
	default:
		return Options{}, fmt.Errorf("delivery: modo de corpo inválido: %q", o.BodyMode)
	}
	if o.MaxMessageBytes <= 0 {
		o.MaxMessageBytes = DefaultMaxMessageBytes
	}
	if o.MaxMessageBytes < MinMessageBytes {
		return Options{}, fmt.Errorf("delivery: limite de mensagem muito baixo: %d bytes (mínimo %d)", o.MaxMessageBytes, MinMessageBytes)
	}
	if o.PreviewWords <= 0 {
		o.PreviewWords = DefaultPreviewWords
	}
	return o, nil
}

// Compose monta uma ou mais mensagens. Quando o corpo agrupado passa do
// limite, cada documento vira uma mensagem; um único documento grande demais
// é truncado em fronteira de parágrafo com link para o leitor.
func Compose(doses []Dose, opts Options) ([]Message, error) {
	if len(doses) == 0 {
		return nil, errors.New("delivery: nenhuma dose para compor")
	}
	opts, err := opts.normalizar()
	if err != nil {
		return nil, err
	}

	partes := make([]string, 0, len(doses))
	chunks := make([]int64, 0, len(doses))
	agrupado := len(doses) > 1
	for _, d := range doses {
		if strings.TrimSpace(d.Title) == "" {
			return nil, errors.New("delivery: dose sem título")
		}
		if link := strings.TrimSpace(d.Link); len(link) > opts.MaxMessageBytes-200 {
			return nil, fmt.Errorf(
				"delivery: link do leitor (%d bytes) não cabe no limite de %d bytes",
				len(link), opts.MaxMessageBytes)
		}
		partes = append(partes, composeDose(d, opts, agrupado))
		chunks = append(chunks, d.Chunk.ID)
	}

	corpo := strings.Join(partes, "\n\n---\n\n")
	if len(corpo) <= opts.MaxMessageBytes {
		return []Message{{
			Title:   titulo(doses),
			Body:    corpo,
			Click:   primeiroLink(doses),
			Actions: acoes(doses),
			Chunks:  chunks,
		}}, nil
	}

	// Passou do limite: uma mensagem por documento, truncando se necessário.
	out := make([]Message, 0, len(doses))
	for _, d := range doses {
		texto := composeDose(d, opts, false)
		if len(texto) > opts.MaxMessageBytes {
			texto = truncar(texto, opts.MaxMessageBytes, d.Link)
		}
		out = append(out, Message{
			Title:   tituloUma(d),
			Body:    texto,
			Click:   d.Link,
			Actions: acoes([]Dose{d}),
			Chunks:  []int64{d.Chunk.ID},
		})
	}
	return out, nil
}

func composeDose(d Dose, opts Options, agrupado bool) string {
	modo := opts.BodyMode
	if d.BodyMode != "" {
		modo = d.BodyMode
	}

	titulo := strings.TrimSpace(d.Title)
	autor := strings.TrimSpace(d.Author)

	var b strings.Builder
	if agrupado {
		// Séries no ntfy: cada documento vira uma seção `## 📖 …` (RF-053).
		b.WriteString("## 📖 ")
		b.WriteString(titulo)
		if autor != "" {
			b.WriteString(" · ")
			b.WriteString(autor)
		}
	} else {
		b.WriteString("**")
		b.WriteString(titulo)
		b.WriteString("**")
		if autor != "" {
			b.WriteString(" · ")
			b.WriteString(autor)
		}
	}

	if caminho := caminhoDaSecao(d.Chunk.SectionPath); caminho != "" {
		b.WriteString("\n`")
		b.WriteString(caminho)
		b.WriteString("`")
	}

	b.WriteString("\n`")
	b.WriteString(metricas(d))
	b.WriteString("`\n")

	if recap := Recap(d.Previous); recap != "" {
		b.WriteString("\n> ↩︎ **Onde paramos:** “")
		b.WriteString(recap)
		b.WriteString("”\n")
	}

	if d.Chunk.StartsMidSection {
		if heading := ultimaSecao(d.Chunk.SectionPath); heading != "" {
			b.WriteString("\n## ")
			b.WriteString(heading)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	if modo == BodyPreview {
		b.WriteString(textoPreview(d.Body, opts.PreviewWords))
	} else {
		b.WriteString(renderizar(d.Body))
	}

	b.WriteString("\n\n")
	if d.Chunk.EndsMidSection {
		b.WriteString("⏭️ **Amanhã:** ")
		b.WriteString(proximoHeading(d))
		if previa := previaProximo(d.Next); previa != "" {
			b.WriteString("\n")
			b.WriteString(previa)
		}
	} else {
		b.WriteString("✓ Fim da seção")
	}

	if link := strings.TrimSpace(d.Link); link != "" {
		b.WriteString("\n🔗 [Abrir no leitor](")
		b.WriteString(link)
		b.WriteString(")")
	}
	return strings.TrimSpace(b.String())
}

func titulo(doses []Dose) string {
	if len(doses) == 1 {
		return tituloUma(doses[0])
	}
	return "📚 Leitura de hoje · " + strconv.Itoa(len(doses)) + " doses"
}

func tituloUma(d Dose) string {
	return "📖 " + strings.TrimSpace(d.Title) + " · " + minutos(d.Chunk.Seconds)
}

func primeiroLink(doses []Dose) string {
	for _, d := range doses {
		if l := strings.TrimSpace(d.Link); l != "" {
			return l
		}
	}
	return ""
}

func acoes(doses []Dose) []Action {
	if len(doses) != 1 {
		return nil
	}
	d := doses[0]
	var out []Action
	if l := strings.TrimSpace(d.Link); l != "" {
		out = append(out, Action{Action: "view", Label: "Abrir leitor", URL: l})
	}
	if l := strings.TrimSpace(d.DoneLink); l != "" {
		out = append(out, Action{Action: "http", Label: "Marcar lido", URL: l, Method: "POST"})
	}
	return out
}

func caminhoDaSecao(caminho []string) string {
	partes := make([]string, 0, len(caminho))
	for _, p := range caminho {
		if t := strings.TrimSpace(p); t != "" {
			partes = append(partes, t)
		}
	}
	return strings.Join(partes, " › ")
}

func ultimaSecao(caminho []string) string {
	for i := len(caminho) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(caminho[i]); t != "" {
			return t
		}
	}
	return ""
}

func metricas(d Dose) string {
	palavras := formatarMilhar(d.Chunk.Words) + " palavras"
	partes := []string{palavras, minutos(d.Chunk.Seconds)}
	if d.Percent >= 0 {
		pct := d.Percent
		if pct > 100 {
			pct = 100
		}
		partes = append(partes, strconv.Itoa(pct)+"% do livro")
	}
	return strings.Join(partes, " · ")
}

func minutos(segundos int) string {
	if segundos <= 0 {
		return "~1 min"
	}
	min := (segundos + 59) / 60
	return "~" + strconv.Itoa(min) + " min"
}

func formatarMilhar(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	inicio := len(s) % 3
	if inicio == 0 {
		inicio = 3
	}
	b.WriteString(s[:inicio])
	for i := inicio; i < len(s); i += 3 {
		b.WriteString(".")
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// Recap extrai as duas últimas frases de prosa do chunk anterior, em no
// máximo 240 caracteres. Devolve vazio quando não há prosa.
func Recap(anteriores []blocks.Block) string {
	texto := ""
	for i := len(anteriores) - 1; i >= 0; i-- {
		if prosa(anteriores[i].Kind) && strings.TrimSpace(anteriores[i].Text) != "" {
			texto = strings.TrimSpace(anteriores[i].Text)
			break
		}
	}
	if texto == "" {
		return ""
	}

	frases := blocks.SplitSentences(texto)
	if len(frases) == 0 {
		return ""
	}
	inicio := len(frases) - 2
	if inicio < 0 {
		inicio = 0
	}
	recap := strings.Join(frases[inicio:], " ")
	if utf8.RuneCountInString(recap) > RecapMaxChars {
		recap = cortarPalavra(recap, RecapMaxChars)
	}
	return recap
}

func prosa(k blocks.Kind) bool {
	switch k {
	case blocks.KindParagraph, blocks.KindListItem, blocks.KindQuote:
		return true
	}
	return false
}

// cortarPalavra corta em fronteira de palavra (limite em caracteres) e
// acrescenta reticências.
func cortarPalavra(s string, limite int) string {
	if limite <= 0 {
		return "…"
	}
	if utf8.RuneCountInString(s) <= limite {
		return s
	}
	corte := 0
	contadas := 0
	for corte < len(s) && contadas < limite {
		_, tamanho := utf8.DecodeRuneInString(s[corte:])
		corte += tamanho
		contadas++
	}
	// Se não houver espaço até o limite, corta no limite exato (sem quebrar
	// rune); senão recua até a última fronteira de palavra.
	fim := corte
	for corte > 0 && s[corte-1] != ' ' {
		corte--
	}
	if corte == 0 {
		corte = fim
	}
	s = strings.TrimSpace(s[:corte])
	s = strings.TrimRight(s, ".,;:!?—-")
	return s + "…"
}

func renderizar(lista []blocks.Block) string {
	partes := make([]string, 0, len(lista))
	for _, b := range lista {
		if t := renderBloco(b); t != "" {
			partes = append(partes, t)
		}
	}
	return strings.Join(partes, "\n\n")
}

func renderBloco(b blocks.Block) string {
	texto := strings.TrimSpace(b.Text)
	if b.Kind == blocks.KindCode {
		texto = strings.Trim(b.Text, "\n")
	}
	if texto == "" && b.Kind != blocks.KindHR {
		return ""
	}

	switch b.Kind {
	case blocks.KindH1, blocks.KindH2, blocks.KindH3, blocks.KindH4, blocks.KindH5, blocks.KindH6:
		nivel := b.Level
		if nivel < 1 {
			nivel = 2
		}
		if nivel > 6 {
			nivel = 6
		}
		return strings.Repeat("#", nivel) + " " + texto
	case blocks.KindListItem:
		return "- " + texto
	case blocks.KindCode:
		return "```\n" + texto + "\n```"
	case blocks.KindQuote, blocks.KindFootnote:
		return "> " + strings.ReplaceAll(texto, "\n", "\n> ")
	case blocks.KindHR:
		return "---"
	default:
		return texto
	}
}

func textoPreview(lista []blocks.Block, limitePalavras int) string {
	var (
		partes []string
		total  int
	)
	for _, b := range lista {
		if b.Kind == blocks.KindHR {
			continue
		}
		palavras := strings.Fields(b.Text)
		if len(palavras) == 0 {
			continue
		}
		if total+len(palavras) <= limitePalavras {
			partes = append(partes, strings.TrimSpace(b.Text))
			total += len(palavras)
			continue
		}
		restam := limitePalavras - total
		if restam <= 0 {
			if len(partes) > 0 {
				partes[len(partes)-1] += "…"
			}
			break
		}
		partes = append(partes, strings.Join(palavras[:restam], " ")+"…")
		total = limitePalavras
		break
	}
	return strings.Join(partes, "\n\n")
}

func proximoHeading(d Dose) string {
	for _, b := range d.Next {
		if b.Kind.IsHeading() && strings.TrimSpace(b.Text) != "" {
			return strings.TrimSpace(b.Text)
		}
	}
	if h := ultimaSecao(d.Chunk.SectionPath); h != "" {
		return h + " (continuação)"
	}
	return "continuação"
}

func previaProximo(proximos []blocks.Block) string {
	for _, b := range proximos {
		if !prosa(b.Kind) || strings.TrimSpace(b.Text) == "" {
			continue
		}
		frases := blocks.SplitSentences(strings.TrimSpace(b.Text))
		if len(frases) == 0 {
			return ""
		}
		return cortarPalavra(frases[0], NextPreviewMaxChars)
	}
	return ""
}

// truncar corta em 60% do limite (reservando espaço para o sufixo), em
// fronteira de parágrafo ou palavra, e preserva o link do leitor.
func truncar(texto string, limite int, link string) string {
	sufixo := ""
	if l := strings.TrimSpace(link); l != "" {
		sufixo = "\n\n[…continua no leitor](" + l + ")"
	}
	alvo := limite * 6 / 10
	if max := limite - len(sufixo) - 4; alvo > max {
		alvo = max
	}
	if alvo < 1 {
		return sufixo
	}
	if len(texto) <= limite-len(sufixo) {
		return texto + sufixo
	}

	corte := alvo
	if corte >= len(texto) {
		return texto + sufixo
	}
	for corte > 0 && !utf8.RuneStart(texto[corte]) {
		corte--
	}
	prefixo := texto[:corte]
	if i := strings.LastIndex(prefixo, "\n\n"); i > len(prefixo)/2 {
		prefixo = prefixo[:i]
	} else if i := strings.LastIndex(prefixo, " "); i > 0 {
		prefixo = prefixo[:i]
	}
	prefixo = strings.TrimRight(strings.TrimSpace(prefixo), ".,;:!?—-")
	return prefixo + "…" + sufixo
}
