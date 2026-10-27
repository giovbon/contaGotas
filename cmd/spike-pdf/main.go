//go:build pdfspike

package main

import (
	"fmt"
	"os"
	"runtime"

	fitz "github.com/gen2brain/go-fitz"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: spike-pdf <file.pdf>")
		os.Exit(2)
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("start: alloc=%d MiB\n", m.Alloc/1024/1024)

	doc, err := fitz.New(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer doc.Close()

	fmt.Println("pages:", doc.NumPage())
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
		}
	}

	runtime.GC()
	runtime.ReadMemStats(&m)
	fmt.Printf("end: alloc=%d MiB sys=%d MiB\n", m.Alloc/1024/1024, m.Sys/1024/1024)
}
