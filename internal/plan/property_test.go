package plan_test

import (
	"encoding/json"
	"math/rand"
	"strings"
	"testing"

	"github.com/giovani/readingdrip/internal/blocks"
	"github.com/giovani/readingdrip/internal/plan"
)

func kindForLevel(level int) blocks.Kind {
	switch level {
	case 2:
		return blocks.KindH2
	case 3:
		return blocks.KindH3
	default:
		return blocks.KindH1
	}
}

func randomRaw(r *rand.Rand) []blocks.Block {
	n := 1 + r.Intn(40)
	raw := make([]blocks.Block, 0, n)

	for i := 0; i < n; i++ {
		switch roll := r.Intn(12); {
		case roll < 3:
			level := 1 + r.Intn(3)
			raw = append(raw, blocks.Block{
				Kind:  kindForLevel(level),
				Level: level,
				Text:  "Seção " + strings.Repeat("tema ", 1+r.Intn(4)),
			})
		case roll < 9:
			words := 5 + r.Intn(200)
			raw = append(raw, blocks.Block{
				Kind: blocks.KindParagraph,
				Text: strings.Repeat("palavra ", words),
			})
		case roll < 11:
			lines := 1 + r.Intn(60)
			raw = append(raw, blocks.Block{
				Kind: blocks.KindCode,
				Text: strings.Repeat("linha();\n", lines),
			})
		default:
			raw = append(raw, blocks.Block{
				Kind: blocks.KindTable,
				Text: "a b c d",
				HTML: "<table><tr><th>a</th></tr><tr><td>b</td></tr></table>",
			})
		}
	}
	return raw
}

func TestProperties(t *testing.T) {
	r := rand.New(rand.NewSource(42))

	for i := 0; i < 300; i++ {
		raw := randomRaw(r)
		bs := blocks.Prepare(raw, blocks.PrepareOptions{
			Estimator:         blocks.Estimator{WPM: 200},
			MaxParagraphWords: 300,
		})
		if len(bs) == 0 {
			continue
		}

		params := plan.Params{Mode: plan.ModeMinutesPerDay, Minutes: 1 + r.Intn(8)}
		if i%3 == 0 {
			params = plan.Params{Mode: plan.ModeFinishByDays, Days: 2 + r.Intn(9)}
		}

		pl, err := plan.Generate(bs, params)
		if err != nil {
			t.Fatalf("iteração %d: %v", i, err)
		}

		if pl.Chunks[0].FirstBlock != 0 {
			t.Fatalf("iteração %d: primeiro bloco = %d", i, pl.Chunks[0].FirstBlock)
		}
		total := 0
		for j, c := range pl.Chunks {
			if j > 0 && c.FirstBlock != pl.Chunks[j-1].LastBlock+1 {
				t.Fatalf("iteração %d: gap no chunk %d", i, j)
			}
			if bs[c.LastBlock].Kind.IsHeading() && c.LastBlock != len(bs)-1 {
				t.Fatalf("iteração %d: chunk %d termina em heading", i, j)
			}
			for b := c.FirstBlock; b <= c.LastBlock; b++ {
				total++
			}
		}
		if got := pl.Chunks[len(pl.Chunks)-1].LastBlock + 1; got != len(bs) {
			t.Fatalf("iteração %d: cobertura %d de %d", i, got, len(bs))
		}
		if total != len(bs) {
			t.Fatalf("iteração %d: contagem %d != %d", i, total, len(bs))
		}

		if i%10 == 0 {
			again, err := plan.Generate(bs, params)
			if err != nil {
				t.Fatal(err)
			}
			a, _ := json.Marshal(pl)
			b, _ := json.Marshal(again)
			if string(a) != string(b) {
				t.Fatalf("iteração %d: plano não determinístico", i)
			}
		}
	}
}
