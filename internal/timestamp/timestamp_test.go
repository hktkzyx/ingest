package timestamp

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestExtract_UnsupportedExtensionReturnsErrUnsupported(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "x.txt")
	_, err := Extract(tmp)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported for .txt, got %v", err)
	}
}
