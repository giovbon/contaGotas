//go:build cgo

package pdf

import (
	"os"
	"testing"
)

func TestExtractMissingFile(t *testing.T) {
	f, err := os.Open("nonexistent.pdf")
	if err != nil {
		// expected
		return
	}
	defer f.Close()
	fi, _ := f.Stat()
	_, err = Extract(f, fi.Size(), "test")
	if err == nil {
		t.Fatal("expected error")
	}
}
