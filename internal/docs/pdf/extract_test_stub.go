//go:build !cgo

package pdf

import (
	"io"
	"testing"

	"github.com/giovani/readingdrip/internal/blocks"
)

func TestExtractStub(t *testing.T) {
	_, err := Extract(nil, 0, "x")
	if err == nil {
		t.Fatal("expected error without cgo")
	}
}
