package delivery

import (
	"strings"
	"testing"

	"github.com/giovani/readingdrip/internal/blocks"
)

func BenchmarkComposeMessage5Docs(b *testing.B) {
	doses := make([]Dose, 5)
	for i := range doses {
		d := doseBase()
		d.Title = "Documento " + strings.Repeat("x", i+1)
		d.Chunk.ID = int64(i + 1)
		d.Chunk.Words = 1800
		d.Chunk.Seconds = 540
		d.Percent = 10 * (i + 1)
		paragrafo := strings.Repeat("Uma frase de prosa com tamanho realista para o benchmark. ", 40)
		d.Body = []blocks.Block{
			{Kind: blocks.KindH3, Level: 3, Text: "Seção do trecho"},
			{Kind: blocks.KindParagraph, Text: paragrafo},
			{Kind: blocks.KindParagraph, Text: paragrafo},
		}
		d.Previous = []blocks.Block{
			{Kind: blocks.KindParagraph, Text: paragrafo},
		}
		d.Next = []blocks.Block{
			{Kind: blocks.KindH2, Level: 2, Text: "Próxima seção"},
			{Kind: blocks.KindParagraph, Text: paragrafo},
		}
		doses[i] = d
	}
	opts := Options{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msgs, err := Compose(doses, opts)
		if err != nil {
			b.Fatal(err)
		}
		if len(msgs) != 1 {
			b.Fatalf("mensagens = %d", len(msgs))
		}
	}
}
