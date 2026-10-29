package delivery

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/giovani/readingdrip/internal/blocks"
	"github.com/giovani/readingdrip/internal/store"
)

func doseBase() Dose {
	return Dose{
		Title:  "Dom Casmurro",
		Author: "Machado de Assis",
		Chunk: store.Chunk{
			ID:          42,
			Seq:         4,
			SectionPath: []string{"Cap. 32 — A carta", "32.4 Primeira desconfiança"},
			Words:       1842,
			Seconds:     540,
		},
		Body: []blocks.Block{
			{Kind: blocks.KindH3, Level: 3, Text: "32.4 — Primeira desconfiança"},
			{Kind: blocks.KindParagraph, Text: "Capitu olhou para a janela."},
		},
		Percent: 21,
		Link:    "https://leitura.exemplo.com/r/42?t=abc",
	}
}

func TestComposeMensagemUnica(t *testing.T) {
	d := doseBase()
	d.DoneLink = "https://leitura.exemplo.com/api/actions/chunks/42/done?t=done"
	msgs, err := Compose([]Dose{d}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("mensagens = %d", len(msgs))
	}
	m := msgs[0]
	if m.Title != "📖 Dom Casmurro · ~9 min" {
		t.Errorf("título = %q", m.Title)
	}
	for _, trecho := range []string{
		"**Dom Casmurro** · Machado de Assis",
		"`Cap. 32 — A carta › 32.4 Primeira desconfiança`",
		"`1.842 palavras · ~9 min · 21% do livro`",
		"### 32.4 — Primeira desconfiança",
		"Capitu olhou para a janela.",
		"✓ Fim da seção",
		"🔗 [Abrir no leitor](https://leitura.exemplo.com/r/42?t=abc)",
	} {
		if !strings.Contains(m.Body, trecho) {
			t.Errorf("corpo sem %q\n%s", trecho, m.Body)
		}
	}
	if m.Click != d.Link {
		t.Errorf("click = %q", m.Click)
	}
	if len(m.Actions) != 2 {
		t.Fatalf("actions = %d", len(m.Actions))
	}
	if m.Actions[0].Action != "view" || m.Actions[1].Action != "http" || m.Actions[1].Method != "POST" {
		t.Errorf("actions = %+v", m.Actions)
	}
	if len(m.Chunks) != 1 || m.Chunks[0] != 42 {
		t.Errorf("chunks = %v", m.Chunks)
	}
}

func TestComposeRecap(t *testing.T) {
	d := doseBase()
	d.Previous = []blocks.Block{{
		Kind: blocks.KindParagraph,
		Text: "Primeira frase do trecho. Segunda frase importante. Terceira frase encerra.",
	}}
	msgs, err := Compose([]Dose{d}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	corpo := msgs[0].Body
	if !strings.Contains(corpo, "> ↩︎ **Onde paramos:** “Segunda frase importante. Terceira frase encerra.”") {
		t.Errorf("recap ausente ou errado:\n%s", corpo)
	}
	if strings.Contains(corpo, "Primeira frase do trecho") {
		t.Error("recap incluiu frase além das duas últimas")
	}
}

func TestComposeRecapIgnoraCodigo(t *testing.T) {
	d := doseBase()
	d.Previous = []blocks.Block{
		{Kind: blocks.KindParagraph, Text: "Prosa anterior. Continua aqui."},
		{Kind: blocks.KindCode, Text: "func main() {}"},
		{Kind: blocks.KindTable, Text: "a | b"},
	}
	msgs, _ := Compose([]Dose{d}, Options{})
	if !strings.Contains(msgs[0].Body, "Prosa anterior. Continua aqui.") {
		t.Errorf("não caiu para a prosa anterior:\n%s", msgs[0].Body)
	}
}

func TestComposeSemRecapNoPrimeiroChunk(t *testing.T) {
	msgs, _ := Compose([]Dose{doseBase()}, Options{})
	if strings.Contains(msgs[0].Body, "Onde paramos") {
		t.Error("primeiro chunk não deveria ter recap")
	}
}

func TestComposeRecapDesligado(t *testing.T) {
	d := doseBase()
	d.Previous = []blocks.Block{{
		Kind: blocks.KindParagraph,
		Text: "Primeira frase do trecho. Segunda frase importante.",
	}}
	msgs, err := Compose([]Dose{d}, Options{RecapDisabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(msgs[0].Body, "Onde paramos") {
		t.Errorf("recap deveria estar desligado:\n%s", msgs[0].Body)
	}
}

func TestComposeMeioDeSecao(t *testing.T) {
	d := doseBase()
	d.Chunk.StartsMidSection = true
	d.Chunk.EndsMidSection = true
	d.Chunk.SectionPath = []string{"Cap. 32"}
	d.Next = []blocks.Block{
		{Kind: blocks.KindH2, Level: 2, Text: "32.5 — O jantar"},
		{Kind: blocks.KindParagraph, Text: "No dia seguinte, tudo mudou. E continuou."},
	}
	msgs, _ := Compose([]Dose{d}, Options{})
	corpo := msgs[0].Body
	for _, trecho := range []string{"## Cap. 32", "⏭️ **Amanhã:** 32.5 — O jantar", "No dia seguinte, tudo mudou."} {
		if !strings.Contains(corpo, trecho) {
			t.Errorf("corpo sem %q\n%s", trecho, corpo)
		}
	}
	if strings.Contains(corpo, "Fim da seção") {
		t.Error("rodapé de fim de seção indevido")
	}
}

func TestComposePreview(t *testing.T) {
	d := doseBase()
	d.Body = nil
	for i := 0; i < 30; i++ {
		d.Body = append(d.Body, blocks.Block{Kind: blocks.KindParagraph, Text: "palavra" + string(rune('a'+i))})
	}
	msgs, err := Compose([]Dose{d}, Options{BodyMode: BodyPreview, PreviewWords: 5})
	if err != nil {
		t.Fatal(err)
	}
	corpo := msgs[0].Body
	if !strings.Contains(corpo, "palavraa") || !strings.Contains(corpo, "palavrae") {
		t.Errorf("preview não incluiu as primeiras palavras:\n%s", corpo)
	}
	if strings.Contains(corpo, "palavraf") {
		t.Errorf("preview passou do limite:\n%s", corpo)
	}
	if !strings.Contains(corpo, "…") {
		t.Error("preview sem reticências")
	}
}

func TestComposeVariosDocumentos(t *testing.T) {
	a := doseBase()
	b := doseBase()
	b.Title = "O Hobbit"
	b.Author = "Tolkien"
	b.Link = ""
	b.Body = []blocks.Block{{Kind: blocks.KindParagraph, Text: "Em uma toca no chão."}}

	msgs, err := Compose([]Dose{a, b}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("mensagens = %d", len(msgs))
	}
	m := msgs[0]
	if m.Title != "📚 Leitura de hoje · 2 doses" {
		t.Errorf("título = %q", m.Title)
	}
	if !strings.Contains(m.Body, "\n\n---\n\n") {
		t.Error("separador ausente")
	}
	if !strings.Contains(m.Body, "## 📖 Dom Casmurro · Machado de Assis") {
		t.Errorf("seção do primeiro documento ausente ou sem heading:\n%s", m.Body)
	}
	if !strings.Contains(m.Body, "## 📖 O Hobbit · Tolkien") {
		t.Errorf("seção do segundo documento ausente ou sem heading:\n%s", m.Body)
	}
	if strings.Contains(m.Body, "**Dom Casmurro**") {
		t.Error("mensagem agrupada repetiu o título em negrito")
	}
	if m.Click != a.Link {
		t.Errorf("click = %q, quer o primeiro link", m.Click)
	}
	if len(m.Actions) != 0 {
		t.Errorf("ações agrupadas = %+v", m.Actions)
	}
	if len(m.Chunks) != 2 {
		t.Errorf("chunks = %v", m.Chunks)
	}
}

func TestComposeDivideAcimaDoLimite(t *testing.T) {
	a := doseBase()
	b := doseBase()
	b.Title = "Outro"
	corpo := strings.Repeat("palavra ", 80)
	a.Body = []blocks.Block{{Kind: blocks.KindParagraph, Text: corpo}}
	b.Body = []blocks.Block{{Kind: blocks.KindParagraph, Text: corpo}}

	msgs, err := Compose([]Dose{a, b}, Options{MaxMessageBytes: 1200})
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("mensagens = %d, quer 2", len(msgs))
	}
	for i, m := range msgs {
		if len(m.Body) > 1200 {
			t.Errorf("mensagem %d = %d bytes", i, len(m.Body))
		}
		if len(m.Chunks) != 1 {
			t.Errorf("mensagem %d chunks = %v", i, m.Chunks)
		}
		if len(m.Actions) != 1 || m.Actions[0].Action != "view" {
			t.Errorf("mensagem %d actions = %+v", i, m.Actions)
		}
		if m.Click != m.Actions[0].URL {
			t.Errorf("mensagem %d click = %q", i, m.Click)
		}
	}
	if msgs[0].Title != "📖 Dom Casmurro · ~9 min" || msgs[1].Title != "📖 Outro · ~9 min" {
		t.Errorf("títulos = %q / %q", msgs[0].Title, msgs[1].Title)
	}
}

func TestComposeTruncaComLink(t *testing.T) {
	d := doseBase()
	var partes []string
	for i := 0; i < 80; i++ {
		partes = append(partes, strings.Repeat("conteúdo ", 20))
	}
	d.Body = []blocks.Block{{Kind: blocks.KindParagraph, Text: strings.Join(partes, "\n\n")}}

	msgs, err := Compose([]Dose{d}, Options{MaxMessageBytes: 1200})
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("mensagens = %d", len(msgs))
	}
	if len(msgs[0].Body) > 1200 {
		t.Errorf("corpo = %d bytes", len(msgs[0].Body))
	}
	if limite := 1200*6/10 + 3 + len(d.Link) + 30; len(msgs[0].Body) > limite {
		t.Errorf("corpo = %d bytes, esperava corte em 60%% (%d)", len(msgs[0].Body), limite)
	}
	if !strings.Contains(msgs[0].Body, "…") || !strings.Contains(msgs[0].Body, "[…continua no leitor]("+d.Link+")") {
		t.Errorf("truncamento sem reticências/link:\n%s", msgs[0].Body)
	}
}

func TestComposePercentual(t *testing.T) {
	d := doseBase()
	d.Percent = -1
	msgs, err := Compose([]Dose{d}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(msgs[0].Body, "% do livro") {
		t.Error("percentual negativo deveria omitir a métrica")
	}

	d.Percent = 130
	msgs, err = Compose([]Dose{d}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msgs[0].Body, "100% do livro") {
		t.Errorf("percentual não limitado a 100:\n%s", msgs[0].Body)
	}
}

func TestComposeErros(t *testing.T) {
	if _, err := Compose(nil, Options{}); err == nil {
		t.Error("esperava erro sem doses")
	}
	_, err := Compose([]Dose{doseBase()}, Options{BodyMode: "resumo"})
	if err == nil || !strings.Contains(err.Error(), "modo de corpo") {
		t.Errorf("erro = %v", err)
	}
	_, err = Compose([]Dose{doseBase()}, Options{MaxMessageBytes: MinMessageBytes - 1})
	if err == nil || !strings.Contains(err.Error(), "limite de mensagem") {
		t.Errorf("erro para limite baixo = %v", err)
	}
	d := doseBase()
	d.Title = ""
	if _, err := Compose([]Dose{d}, Options{}); err == nil {
		t.Error("esperava erro sem título")
	}
	d = doseBase()
	d.Link = "https://leitura.exemplo.com/" + strings.Repeat("t", MinMessageBytes)
	if _, err := Compose([]Dose{d}, Options{MaxMessageBytes: MinMessageBytes}); err == nil {
		t.Error("esperava erro para link maior que o limite")
	}
}

func TestComposeTruncamentoUTF8(t *testing.T) {
	d := doseBase()
	var partes []string
	for i := 0; i < 120; i++ {
		partes = append(partes, strings.Repeat("ação coração ", 8))
	}
	d.Body = []blocks.Block{{Kind: blocks.KindParagraph, Text: strings.Join(partes, "\n\n")}}

	msgs, err := Compose([]Dose{d}, Options{MaxMessageBytes: 1300})
	if err != nil {
		t.Fatal(err)
	}
	corpo := msgs[0].Body
	if len(corpo) > 1300 {
		t.Errorf("corpo = %d bytes", len(corpo))
	}
	if !utf8.ValidString(corpo) {
		t.Error("truncamento quebrou um rune")
	}
	if !strings.Contains(corpo, "ç") {
		t.Error("conteúdo UTF-8 perdido")
	}
	if !strings.Contains(corpo, "[…continua no leitor]("+d.Link+")") {
		t.Error("link do leitor ausente no truncamento")
	}
}

func TestRecapLimiteEmCaracteres(t *testing.T) {
	longa := strings.Repeat("coração ", 25) // 201 caracteres, 226 bytes
	texto := strings.TrimSpace(longa) + ". " + strings.TrimSpace(longa) + "."
	recap := Recap([]blocks.Block{{Kind: blocks.KindParagraph, Text: texto}})
	if recap == "" {
		t.Fatal("recap vazio")
	}
	if n := utf8.RuneCountInString(recap); n > RecapMaxChars+1 {
		t.Errorf("recap = %d caracteres, quer <= %d", n, RecapMaxChars+1)
	}
	if !utf8.ValidString(recap) {
		t.Error("recap quebrou um rune")
	}
	if !strings.HasSuffix(recap, "…") {
		t.Errorf("recap sem reticências: %q", recap)
	}
}

func TestRecapDuasFrases(t *testing.T) {
	texto := "Frase antiga. Penúltima frase. Última frase."
	if got := Recap([]blocks.Block{{Kind: blocks.KindParagraph, Text: texto}}); got != "Penúltima frase. Última frase." {
		t.Errorf("recap = %q", got)
	}
	if got := Recap(nil); got != "" {
		t.Errorf("recap vazio = %q", got)
	}
	if got := Recap([]blocks.Block{{Kind: blocks.KindCode, Text: "x := 1"}}); got != "" {
		t.Errorf("recap de código = %q", got)
	}
}

func TestComposeBodyModeOverride(t *testing.T) {
	d := doseBase()
	d.Body = nil
	for i := 0; i < 500; i++ {
		d.Body = append(d.Body, blocks.Block{Kind: blocks.KindParagraph, Text: "palavra"})
	}
	d.BodyMode = BodyPreview

	msgs, err := Compose([]Dose{d}, Options{BodyMode: BodyFull, PreviewWords: 10})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(strings.Fields(msgs[0].Body)); n > 200 {
		t.Errorf("override não aplicado: %d palavras", n)
	}
}
