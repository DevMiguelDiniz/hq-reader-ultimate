package reader

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DevMiguelDiniz/hq-reader-ultimate/internal/archive"
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

func TestServiceSetAndGetCurrentPage(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{
		"page1.jpg": tinyJPEG,
		"page2.jpg": tinyJPEG,
		"page3.jpg": tinyJPEG,
	})
	s := NewService()
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close(res.BookID)

	if err := s.SetCurrentPage(res.BookID, 2); err != nil {
		t.Fatalf("SetCurrentPage: %v", err)
	}
	got, err := s.CurrentPage(res.BookID)
	if err != nil {
		t.Fatalf("CurrentPage: %v", err)
	}
	if got != 2 {
		t.Errorf("CurrentPage = %d, want 2", got)
	}
}

func TestServiceCurrentPageDefaultsToZero(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG})
	s := NewService()
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close(res.BookID)

	got, err := s.CurrentPage(res.BookID)
	if err != nil {
		t.Fatalf("CurrentPage: %v", err)
	}
	if got != 0 {
		t.Errorf("CurrentPage antes de qualquer SetCurrentPage = %d, want 0", got)
	}
}

func TestServiceCurrentPageUnknownBook(t *testing.T) {
	s := NewService()
	_, err := s.CurrentPage("nao-existe")
	if !errors.Is(err, ErrBookNotFound) {
		t.Errorf("err = %v, want ErrBookNotFound", err)
	}
}

func TestServiceSetCurrentPageUnknownBook(t *testing.T) {
	s := NewService()
	err := s.SetCurrentPage("nao-existe", 0)
	if !errors.Is(err, ErrBookNotFound) {
		t.Errorf("err = %v, want ErrBookNotFound", err)
	}
}

func TestServiceSetCurrentPageOutOfRange(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG})
	s := NewService()
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close(res.BookID)

	err = s.SetCurrentPage(res.BookID, 5)
	if !errors.Is(err, archive.ErrCorrupted) {
		t.Errorf("err = %v, want archive.ErrCorrupted", err)
	}
}

func TestServiceOpenCreatesPrefetchCoordinator(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG, "page2.jpg": tinyJPEG})
	s := NewService()
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close(res.BookID)

	s.mu.RLock()
	entry := s.books[res.BookID]
	s.mu.RUnlock()
	if entry.prefetch == nil {
		t.Fatal("entry.prefetch é nil apos Open()")
	}
}

func TestServiceSetCurrentPageTriggersPrefetch(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{
		"page0.jpg": tinyJPEG, "page1.jpg": tinyJPEG, "page2.jpg": tinyJPEG,
	})
	s := NewService()
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close(res.BookID)

	if err := s.SetCurrentPage(res.BookID, 1); err != nil {
		t.Fatalf("SetCurrentPage: %v", err)
	}

	s.mu.RLock()
	entry := s.books[res.BookID]
	s.mu.RUnlock()

	waitForSlot(t, entry.prefetch, 1)
}

func TestServicePageReaderUsesPrefetchedBytes(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{
		"page0.jpg": tinyJPEG, "page1.jpg": tinyJPEG, "page2.jpg": tinyJPEG,
	})
	s := NewService()
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close(res.BookID)

	if err := s.SetCurrentPage(res.BookID, 1); err != nil {
		t.Fatalf("SetCurrentPage: %v", err)
	}

	s.mu.RLock()
	entry := s.books[res.BookID]
	s.mu.RUnlock()
	waitForSlot(t, entry.prefetch, 1)

	rc, err := s.PageReader(res.BookID, 1)
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

// TestServiceCloseAfterSetCurrentPageReturnsNil é a regressao do Finding 2
// no nivel que importa de verdade: SetCurrentPage dispara prefetches em
// background e, em seguida, Close() e chamado quase imediatamente — antes
// de qualquer fetch ter chance de terminar sozinho. Isso exercita
// prefetchCoordinator.Cancel() com fetches genuinamente em andamento.
//
// Com fixtures sinteticas em CBZ, archive.Close() nao tem o modo de falha
// especifico do Windows com CBR/RAR (RemoveAll falhando por handle aberto
// num diretorio temporario) — mas esse teste ainda garante que o Cancel()
// baseado em WaitGroup nao trava, nao deadlocka e nao faz Close() retornar
// erro, que e o mecanismo do qual o fix do CBR depende.
func TestServiceCloseAfterSetCurrentPageReturnsNil(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{
		"page0.jpg": tinyJPEG, "page1.jpg": tinyJPEG, "page2.jpg": tinyJPEG,
		"page3.jpg": tinyJPEG, "page4.jpg": tinyJPEG, "page5.jpg": tinyJPEG,
	})
	s := NewService()
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := s.SetCurrentPage(res.BookID, 3); err != nil {
		t.Fatalf("SetCurrentPage: %v", err)
	}

	// Fecha imediatamente, sem esperar os fetches disparados por
	// SetCurrentPage terminarem — e exatamente a corrida que o Finding 2
	// descreve.
	if err := s.Close(res.BookID); err != nil {
		t.Fatalf("Close: %v, want nil", err)
	}

	if _, err := s.PageInfo(res.BookID, 0); !errors.Is(err, ErrBookNotFound) {
		t.Errorf("PageInfo apos Close: err = %v, want ErrBookNotFound", err)
	}
}

func TestServicePageReaderFallsBackWhenNotPrefetched(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page0.jpg": tinyJPEG})
	s := NewService()
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close(res.BookID)

	// Nenhum SetCurrentPage foi chamado, entao o cache esta vazio — o
	// caminho de leitura direta do archive (cache-through) deve funcionar
	// normalmente.
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
