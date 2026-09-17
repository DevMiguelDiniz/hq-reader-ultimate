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

// TestOpenExtensionIsSeparatorAware confirma que a detecção de extensão usa
// filepath.Ext (ciente de separador de path) em vez de procurar o último
// "." na string inteira — um bug anterior extraía ".5\book" como "extensão"
// de um caminho Windows como "C:\comics\v1.5\book" (ponto num nome de
// diretório, sem extensão real no componente final).
func TestOpenExtensionIsSeparatorAware(t *testing.T) {
	_, err := Open(`C:\comics\v1.5\book`)
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("err = %v, want ErrUnsupportedFormat (ponto está num diretório, não numa extensão real)", err)
	}
}
