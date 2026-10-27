//go:build !cgo

package pdf

import (
	"fmt"
	"io"

	"github.com/giovani/readingdrip/internal/blocks"
)

// Result é o resultado da extração de PDF.
type Result struct {
	Title    string
	Author   string
	Language string
	Blocks   []blocks.Block
}

// Extract não está disponível sem CGO.
func Extract(_ io.ReaderAt, _ int64, _ string) (*Result, error) {
	return nil, fmt.Errorf("pdf: extração requer CGO (build tag cgo)")
}
