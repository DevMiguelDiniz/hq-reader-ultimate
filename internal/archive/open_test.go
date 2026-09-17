package archive

import (
	"errors"
	"testing"
)

func TestOpenUnsupportedFormat(t *testing.T) {
	_, err := Open("comic.pdf")
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("err = %v, want ErrUnsupportedFormat", err)
	}
}

func TestOpenDispatchesCBZ(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG})
	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	if len(a.Pages()) != 1 {
		t.Errorf("got %d pages, want 1", len(a.Pages()))
	}
}

func TestOpenDispatchesCBR(t *testing.T) {
	path := realCBRPath(t)
	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	if len(a.Pages()) == 0 {
		t.Error("esperava ao menos uma página")
	}
}
