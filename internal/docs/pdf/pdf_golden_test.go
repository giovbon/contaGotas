//go:build cgo

package pdf

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/giovani/readingdrip/internal/blocks"
	"github.com/giovani/readingdrip/internal/plan"
)

var update = flag.Bool("update", false, "atualiza golden files")

func TestPDFGolden(t *testing.T) {
	cases := []struct {
		name string
		days int
	}{
		{"coluna-unica", 2},
		{"duas-colunas", 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := extrairFixture(t, tc.name)
			prepared := blocks.Prepare(res.Blocks, blocks.PrepareOptions{
				Estimator:         blocks.Estimator{WPM: 200, Difficulty: 1.0},
				MaxParagraphWords: 300,
			})
			params := plan.Params{
				Mode:         plan.ModeFinishByDays,
				Days:         tc.days,
				WPM:          200,
				Difficulty:   1.0,
				MinFill:      0.6,
				MaxOvershoot: 1.4,
			}
			pl, err := plan.Generate(prepared, params)
			if err != nil {
				t.Fatalf("gerar plano: %v", err)
			}

			var buf bytes.Buffer
			doc := "testdata/pdf/" + tc.name + ".pdf"
			if err := plan.Format(&buf, doc, prepared, pl, params); err != nil {
				t.Fatalf("formatar plano: %v", err)
			}

			golden := filepath.Join("..", "..", "..", "testdata", "golden", "pdf_"+tc.name+".plan.txt")
			if *update {
				if err := os.WriteFile(golden, buf.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}

			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("golden ausente (rode com -update): %v", err)
			}
			if !bytes.Equal(buf.Bytes(), want) {
				t.Fatalf("golden divergente:\n--- obtido ---\n%s\n--- esperado ---\n%s", buf.String(), string(want))
			}
		})
	}
}
