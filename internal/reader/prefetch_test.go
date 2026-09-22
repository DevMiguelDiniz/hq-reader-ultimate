package reader

import (
	"fmt"
	"testing"
	"time"

	"github.com/DevMiguelDiniz/hq-reader-ultimate/internal/archive"
)

func openTestArchiveWithPages(t *testing.T, n int) archive.Archive {
	t.Helper()
	files := make(map[string][]byte, n)
	for i := 0; i < n; i++ {
		files[fmt.Sprintf("page%02d.jpg", i)] = tinyJPEG
	}
	path := writeTestZip(t, files)
	a, err := archive.Open(path)
	if err != nil {
		t.Fatalf("archive.Open: %v", err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

func waitForSlot(t *testing.T, p *prefetchCoordinator, index int) []byte {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if data, ok := p.Get(index); ok {
			return data
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("pagina %d nunca apareceu no cache dentro do prazo", index)
	return nil
}

func TestPrefetchCoordinatorFetchesWindow(t *testing.T) {
	a := openTestArchiveWithPages(t, 10)
	p := newPrefetchCoordinator(a)

	p.SetCurrent(5)

	for _, idx := range []int{3, 4, 5, 6, 7} {
		data := waitForSlot(t, p, idx)
		if len(data) != len(tinyJPEG) {
			t.Errorf("pagina %d: len(data) = %d, want %d", idx, len(data), len(tinyJPEG))
		}
	}
}

func TestPrefetchCoordinatorClipsAtBookBoundaries(t *testing.T) {
	a := openTestArchiveWithPages(t, 3)
	p := newPrefetchCoordinator(a)

	p.SetCurrent(0)

	for _, idx := range []int{0, 1, 2} {
		waitForSlot(t, p, idx)
	}

	if _, ok := p.Get(-1); ok {
		t.Error("Get(-1) deveria retornar false, indice invalido")
	}
	if _, ok := p.Get(3); ok {
		t.Error("Get(3) deveria retornar false, fora dos limites do livro (3 paginas: 0,1,2)")
	}
}

func TestPrefetchCoordinatorEvictsOutOfWindowSlots(t *testing.T) {
	a := openTestArchiveWithPages(t, 20)
	p := newPrefetchCoordinator(a)

	p.SetCurrent(0)
	waitForSlot(t, p, 0)

	p.SetCurrent(15)

	if _, ok := p.Get(0); ok {
		t.Error("pagina 0 deveria ter sido removida do cache apos SetCurrent(15) — janela nao inclui mais o indice 0")
	}
}

func TestPrefetchCoordinatorGetMissingReturnsFalse(t *testing.T) {
	a := openTestArchiveWithPages(t, 5)
	p := newPrefetchCoordinator(a)

	if _, ok := p.Get(2); ok {
		t.Error("Get(2) antes de qualquer SetCurrent deveria retornar false")
	}
}

func TestPrefetchCoordinatorCancel(t *testing.T) {
	a := openTestArchiveWithPages(t, 10)
	p := newPrefetchCoordinator(a)

	p.SetCurrent(5)
	waitForSlot(t, p, 5)

	p.Cancel()

	p.mu.Lock()
	pending := len(p.cancel)
	p.mu.Unlock()
	if pending != 0 {
		t.Errorf("apos Cancel(), %d fetches ainda registrados como em andamento, want 0", pending)
	}
}
