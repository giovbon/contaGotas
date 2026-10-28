//go:build !cgo

package pdf

import "testing"

func TestExtractStub(t *testing.T) {
	if _, err := Extract(nil, 0, "x"); err == nil {
		t.Fatal("esperado erro sem CGO")
	}
}
