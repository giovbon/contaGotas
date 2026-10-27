package plan_test

import (
	"strings"
	"testing"

	"github.com/giovani/readingdrip/internal/blocks"
	"github.com/giovani/readingdrip/internal/plan"
)

func BenchmarkPlan10kBlocks(b *testing.B) {
	raw := make([]blocks.Block, 0, 10_000)
	for i := 0; i < 10_000; i++ {
		if i%100 == 0 {
			raw = append(raw, blocks.Block{
				Kind:  blocks.KindH2,
				Level: 2,
				Text:  "Seção de benchmark",
			})
			continue
		}
		raw = append(raw, blocks.Block{
			Kind: blocks.KindParagraph,
			Text: strings.Repeat("palavra ", 30),
		})
	}

	prepared := blocks.Prepare(raw, blocks.PrepareOptions{
		Estimator:         blocks.Estimator{WPM: 200},
		MaxParagraphWords: 300,
	})
	params := plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 10}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := plan.Generate(prepared, params); err != nil {
			b.Fatal(err)
		}
	}
}
