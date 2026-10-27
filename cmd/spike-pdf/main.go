//go:build pdfspike

package main

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"

	fitz "github.com/gen2brain/go-fitz"
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
	for i := 0; i < doc.NumPage() && i < 3; i++ {
		text, err := doc.Text(i)
		if err != nil {
			panic(err)
		}
		html, err := doc.HTML(i)
		if err != nil {
			fmt.Printf("page %d text-len=%d html-err=%v\n", i, len(text), err)
		} else {
			fmt.Printf("page %d text-len=%d html-len=%d\n", i, len(text), len(html))
			// Analisar fontes
			re := regexp.MustCompile(`font-size:\s*([0-9.]+)pt`)
			matches := re.FindAllStringSubmatch(html, -1)
			if len(matches) > 0 {
				var sizes []float64
				for _, match := range matches {
					if v, err := strconv.ParseFloat(match[1], 64); err == nil {
						sizes = append(sizes, v)
					}
				}
				if len(sizes) > 0 {
					sort.Float64s(sizes)
					fmt.Printf("  font-sizes: min=%.1f med=%.1f max=%.1f count=%d\n", sizes[0], sizes[len(sizes)/2], sizes[len(sizes)-1], len(sizes))
				}
			}
			// Verificar links/imagens
			if regexp.MustCompile(`<img`).MatchString(html) {
				fmt.Printf("  has-images: yes\n")
			}
			if regexp.MustCompile(`<a `).MatchString(html) {
				fmt.Printf("  has-links: yes\n")
			}
		}
	}

	runtime.GC()
	runtime.ReadMemStats(&m)
	fmt.Printf("end: alloc=%d MiB sys=%d MiB\n", m.Alloc/1024/1024, m.Sys/1024/1024)
}
