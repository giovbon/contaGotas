//go:build pdfspike

// Command spike-pdf investiga a saída do go-fitz para um PDF real.
//
// Uso, a partir da raiz do repositório:
//
//	go run -tags pdfspike ./cmd/spike-pdf arquivo.pdf
//
// Ele imprime páginas, limites, amostras de posição/tamanho de fonte e o
// consumo de memória no início e no fim.
package main

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	fitz "github.com/gen2brain/go-fitz"
)

var (
	reTop  = regexp.MustCompile(`top:([0-9.]+)pt`)
	reLeft = regexp.MustCompile(`left:([0-9.]+)pt`)
	reSize = regexp.MustCompile(`font-size:([0-9.]+)pt`)
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: spike-pdf <file.pdf>")
		os.Exit(2)
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("start: alloc=%d MiB sys=%d MiB\n", m.Alloc/1024/1024, m.Sys/1024/1024)

	doc, err := fitz.New(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer doc.Close()

	fmt.Printf("pages: %d\n", doc.NumPage())
	if meta := doc.Metadata(); meta != nil {
		fmt.Printf("title=%q author=%q\n", strings.Trim(meta["title"], "\x00"), strings.Trim(meta["author"], "\x00"))
	}

	for i := 0; i < doc.NumPage() && i < 5; i++ {
		text, err := doc.Text(i)
		if err != nil {
			fmt.Printf("page %d: text error: %v\n", i, err)
			continue
		}
		bounds, _ := doc.Bound(i)
		html, htmlErr := doc.HTML(i, false)
		fmt.Printf("page %d: chars=%d bounds=%v html-len=%d", i, len([]rune(text)), bounds, len(html))
		if htmlErr != nil {
			fmt.Printf(" html-error=%v\n", htmlErr)
			continue
		}
		fmt.Println()

		sizes := extrair(reSize, html)
		if len(sizes) > 0 {
			sort.Float64s(sizes)
			fmt.Printf("  font-sizes: min=%.1f med=%.1f max=%.1f count=%d\n",
				sizes[0], sizes[len(sizes)/2], sizes[len(sizes)-1], len(sizes))
		}
		lefts := extrair(reLeft, html)
		if len(lefts) > 0 {
			sort.Float64s(lefts)
			fmt.Printf("  left: min=%.1f med=%.1f max=%.1f\n",
				lefts[0], lefts[len(lefts)/2], lefts[len(lefts)-1])
		}
		tops := extrair(reTop, html)
		if len(tops) > 0 {
			fmt.Printf("  top: first=%.1f last=%.1f\n", tops[0], tops[len(tops)-1])
		}
	}

	runtime.GC()
	runtime.ReadMemStats(&m)
	fmt.Printf("end: alloc=%d MiB sys=%d MiB\n", m.Alloc/1024/1024, m.Sys/1024/1024)
}

func extrair(re *regexp.Regexp, html string) []float64 {
	var vals []float64
	for _, m := range re.FindAllStringSubmatch(html, -1) {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			vals = append(vals, v)
		}
	}
	return vals
}
