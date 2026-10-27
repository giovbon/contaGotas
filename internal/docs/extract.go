// Package docs define utilidades comuns de extração de documentos.
package docs

import (
	"path/filepath"
	"strings"
)

// Format identifica o formato de origem.
type Format string

const (
	FormatMarkdown Format = "md"
	FormatEPUB     Format = "epub"
	FormatPDF      Format = "pdf"
)

// Detect infere o formato pela extensão do arquivo.
func Detect(name string) (Format, bool) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown":
		return FormatMarkdown, true
	case ".epub":
		return FormatEPUB, true
	case ".pdf":
		return FormatPDF, true
	}
	return "", false
}
