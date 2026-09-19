package reader

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

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

func TestServiceOpenAndPageReader(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{
		"page1.jpg": tinyJPEG,
		"page2.jpg": tinyJPEG,
	})

	s := NewService()
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close(res.BookID)

	if res.BookID == "" {
		t.Fatal("BookID vazio")
	}
	if res.PageCount != 2 {
		t.Fatalf("PageCount = %d, want 2", res.PageCount)
	}

	info, err := s.PageInfo(res.BookID, 0)
	if err != nil {
		t.Fatalf("PageInfo: %v", err)
	}
	if info.ContentType != "image/jpeg" {
		t.Errorf("ContentType = %q", info.ContentType)
	}

	rc, err := s.PageReader(res.BookID, 0)
	if err != nil {
		t.Fatalf("PageReader: %v", err)
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(data) != len(tinyJPEG) {
		t.Errorf("len(data) = %d, want %d", len(data), len(tinyJPEG))
	}
}

func TestServiceUnknownBookID(t *testing.T) {
	s := NewService()
	_, err := s.PageInfo("nao-existe", 0)
	if !errors.Is(err, ErrBookNotFound) {
		t.Errorf("err = %v, want ErrBookNotFound", err)
	}
}

func TestServiceOpenNonexistentFile(t *testing.T) {
	s := NewService()
	_, err := s.Open(filepath.Join(t.TempDir(), "nao-existe.cbz"))
	if err == nil {
		t.Fatal("esperava erro ao abrir arquivo inexistente")
	}
}

func TestServiceTwoOpensAreDifferentBookIDs(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG})
	s := NewService()
	res1, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open 1: %v", err)
	}
	defer s.Close(res1.BookID)

	res2, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open 2: %v", err)
	}
	defer s.Close(res2.BookID)

	if res1.BookID == res2.BookID {
		t.Error("dois Open() geraram o mesmo bookID")
	}
}

func TestServiceClose(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG})
	s := NewService()
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s.Close(res.BookID); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := s.PageInfo(res.BookID, 0); !errors.Is(err, ErrBookNotFound) {
		t.Errorf("PageInfo apos Close: err = %v, want ErrBookNotFound", err)
	}
}

func TestServiceCurrent(t *testing.T) {
	s := NewService()
	if _, err := s.Current(); !errors.Is(err, ErrBookNotFound) {
		t.Errorf("Current() antes de qualquer Open: err = %v, want ErrBookNotFound", err)
	}

	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG, "page2.jpg": tinyJPEG})
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close(res.BookID)

	cur, err := s.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if cur.BookID != res.BookID || cur.PageCount != res.PageCount {
		t.Errorf("Current() = %+v, want %+v", cur, res)
	}
}
