// Command readingdrip expõe a CLI do núcleo de segmentação.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/giovani/readingdrip/internal/blocks"
	"github.com/giovani/readingdrip/internal/docs"
	"github.com/giovani/readingdrip/internal/docs/epub"
	"github.com/giovani/readingdrip/internal/docs/markdown"
	"github.com/giovani/readingdrip/internal/docs/pdf"
	"github.com/giovani/readingdrip/internal/plan"
)

const version = "0.1.0-dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "plan":
		os.Exit(runPlan(os.Args[2:]))
	case "version", "-v", "--version":
		fmt.Printf("readingdrip %s\n", version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "comando desconhecido: %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `readingdrip — particionador de leitura

Uso:
  readingdrip plan --input <arquivo.md|.epub|.pdf> [flags]
  readingdrip version

Flags de plan:
  --input <arquivo>       arquivo de entrada (obrigatório)
  --minutes <n>           minutos por dia de leitura (padrão 10)
  --days <n>              terminar em n dias de leitura (sobrepõe --minutes)
  --wpm <n>               palavras por minuto (padrão 200)
  --difficulty <f>        fator de dificuldade (padrão 1.0)
  --min-fill <f>          fração mínima do alvo (padrão 0.6)
  --max-overshoot <f>     fração máxima do alvo (padrão 1.4)
  --max-para-words <n>    divide parágrafos acima de n palavras (padrão 300)
  --json                  imprime o plano em JSON

Exemplos:
  readingdrip plan --input livro.md --minutes 10
  readingdrip plan --input livro.epub --days 20 --json
  readingdrip plan --input livro.pdf --minutes 10
`)
}

func runPlan(args []string) int {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	input := fs.String("input", "", "arquivo de entrada")
	minutes := fs.Int("minutes", 10, "minutos por dia")
	days := fs.Int("days", 0, "terminar em n dias de leitura")
	wpm := fs.Int("wpm", 200, "palavras por minuto")
	difficulty := fs.Float64("difficulty", 1.0, "fator de dificuldade")
	minFill := fs.Float64("min-fill", 0.6, "fração mínima do alvo")
	maxOvershoot := fs.Float64("max-overshoot", 1.4, "fração máxima do alvo")
	maxParaWords := fs.Int("max-para-words", 300, "máximo de palavras por parágrafo")
	asJSON := fs.Bool("json", false, "saída em JSON")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *input == "" {
		fmt.Fprintln(os.Stderr, "erro: --input é obrigatório")
		return 2
	}

	format, ok := docs.Detect(*input)
	if !ok {
		fmt.Fprintf(os.Stderr, "erro: formato não suportado: %s\n", filepath.Ext(*input))
		return 2
	}
	src, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro ao ler %s: %v\n", *input, err)
		return 1
	}

	fallback := strings.TrimSuffix(filepath.Base(*input), filepath.Ext(*input))
	var raw []blocks.Block
	switch format {
	case docs.FormatMarkdown:
		res, err := markdown.Extract(src, fallback)
		if err != nil {
			fmt.Fprintf(os.Stderr, "erro na extração: %v\n", err)
			return 1
		}
		raw = res.Blocks
	case docs.FormatEPUB:
		res, err := epub.Extract(src, fallback)
		if err != nil {
			fmt.Fprintf(os.Stderr, "erro na extração: %v\n", err)
			return 1
		}
		raw = res.Blocks
	case docs.FormatPDF:
		f, err := os.Open(*input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "erro ao ler %s: %v\n", *input, err)
			return 1
		}
		fi, err := f.Stat()
		if err != nil {
			f.Close()
			fmt.Fprintf(os.Stderr, "erro ao ler %s: %v\n", *input, err)
			return 1
		}
		res, err := pdf.Extract(f, fi.Size(), fallback)
		f.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "erro na extração: %v\n", err)
			return 1
		}
		raw = res.Blocks
	default:
		fmt.Fprintf(os.Stderr, "erro: formato não suportado: %s\n", format)
		return 2
	}

	prepared := blocks.Prepare(raw, blocks.PrepareOptions{
		Estimator:         blocks.Estimator{WPM: *wpm, Difficulty: *difficulty},
		MaxParagraphWords: *maxParaWords,
	})

	params := plan.Params{
		Mode:         plan.ModeMinutesPerDay,
		Minutes:      *minutes,
		WPM:          *wpm,
		Difficulty:   *difficulty,
		MinFill:      *minFill,
		MaxOvershoot: *maxOvershoot,
	}
	if *days > 0 {
		params.Mode = plan.ModeFinishByDays
		params.Days = *days
		params.Minutes = 0
	}

	pl, err := plan.Generate(prepared, params)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro ao gerar plano: %v\n", err)
		return 1
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(pl); err != nil {
			fmt.Fprintf(os.Stderr, "erro ao escrever JSON: %v\n", err)
			return 1
		}
		return 0
	}

	if err := plan.Format(os.Stdout, *input, prepared, pl, params); err != nil {
		fmt.Fprintf(os.Stderr, "erro ao formatar plano: %v\n", err)
		return 1
	}
	return 0
}
