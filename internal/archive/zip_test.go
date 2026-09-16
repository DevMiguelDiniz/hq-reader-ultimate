package archive

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// tinyJPEG é o conteúdo mínimo usado nos testes — só os bytes importam,
// nada decodifica a imagem de verdade nesta camada.
var tinyJPEG = []byte{0xFF, 0xD8, 0xFF, 0xD9}

func writeTestZip(t *testing.T, files map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cbz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return path
}

func TestZipArchive(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{
		"chapter1/page2.jpg": tinyJPEG,
		"chapter1/page1.jpg": tinyJPEG,
		"Thumbs.db":          []byte("lixo"),
		"ComicInfo.xml":      []byte(`<ComicInfo><Title>Teste</Title></ComicInfo>`),
	})

	a, err := newZipArchive(path)
	if err != nil {
		t.Fatalf("newZipArchive: %v", err)
	}
	defer a.Close()

	pages := a.Pages()
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	if pages[0].Name != "chapter1/page1.jpg" || pages[1].Name != "chapter1/page2.jpg" {
		t.Errorf("ordem errada: %+v", pages)
	}

	rc, err := a.Open(0)
	if err != nil {
		t.Fatalf("Open(0): %v", err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(data, tinyJPEG) {
		t.Errorf("bytes da página 0 não batem com o esperado")
	}

	info, err := a.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if info == nil || info.Title != "Teste" {
		t.Errorf("Metadata = %+v", info)
	}
}

func TestZipArchiveCorrupted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupted.cbz")
	if err := os.WriteFile(path, []byte("isso nao e um zip"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := newZipArchive(path)
	if !errors.Is(err, ErrCorrupted) {
		t.Errorf("err = %v, want ErrCorrupted", err)
	}
}

func TestZipArchiveNoMetadata(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG})
	a, err := newZipArchive(path)
	if err != nil {
		t.Fatalf("newZipArchive: %v", err)
	}
	defer a.Close()

	info, err := a.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if info != nil {
		t.Errorf("Metadata = %+v, want nil (sem ComicInfo.xml)", info)
	}
}
