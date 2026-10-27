package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func errorsIs(err, target error) bool { return errors.Is(err, target) }

func TestUpsertBooksAndFingerprints(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustUpsertLibs(t, s, mkRoot("r", "Raiz"), mkRoot("r2", "Outra"))
	a, b, c := mkBook("a", "r", "a.cbz"), mkBook("b", "r", "b.cbz"), mkBook("c", "r", "c.cbz")
	a.Size, a.MTime = 11, 111
	b.Size, b.MTime = 22, 222
	c.Size, c.MTime = 33, 333
	other := mkBook("z", "r2", "z.cbz")
	mustUpsertBooks(t, s, a, b, c, other)

	fp, err := s.Fingerprints(ctx, "r")
	if err != nil {
		t.Fatal(err)
	}
	if len(fp) != 3 {
		t.Fatalf("Fingerprints len = %d, want 3 (only root r): %v", len(fp), fp)
	}
	for id, want := range map[string]Fingerprint{"a": {11, 111}, "b": {22, 222}, "c": {33, 333}} {
		if fp[id] != want {
			t.Errorf("Fingerprints[%s] = %+v, want %+v", id, fp[id], want)
		}
	}

	// Reupsert do mesmo ID atualiza campos e não duplica; added_at é preservado.
	a2 := a
	a2.Size, a2.MTime, a2.Title, a2.PageCount = 99, 999, "Novo titulo", 42
	a2.Series, a2.Number = "Serie", "7"
	a2.Broken, a2.BrokenReason = true, "corrupted"
	a2.AddedAt = 123456
	mustUpsertBooks(t, s, a2)
	if n := countRows(t, s, "books"); n != 4 {
		t.Errorf("books = %d, want 4 (no duplicates)", n)
	}
	got, ok, err := s.GetBook(ctx, "a")
	if err != nil || !ok {
		t.Fatalf("GetBook: ok=%v err=%v", ok, err)
	}
	want := a2
	want.AddedAt = a.AddedAt
	if got.Book != want {
		t.Errorf("book after reupsert:\n got %+v\nwant %+v", got.Book, want)
	}
	if got.Progress != nil {
		t.Errorf("Progress = %+v, want nil", got.Progress)
	}
	fp, _ = s.Fingerprints(ctx, "r")
	if fp["a"] != (Fingerprint{99, 999}) {
		t.Errorf("Fingerprints[a] = %+v", fp["a"])
	}
}

func TestUpsertBooksClearsMissingAndRecordsLastSeen(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustUpsertLibs(t, s, mkRoot("r", "Raiz"))
	b := mkBook("a", "r", "a.cbz")
	b.LastSeen = 10
	mustUpsertBooks(t, s, b)
	if _, err := s.db.Exec("UPDATE books SET missing=1"); err != nil {
		t.Fatal(err)
	}
	b.LastSeen = 20
	mustUpsertBooks(t, s, b)
	got, _, _ := s.GetBook(ctx, "a")
	if got.Missing || got.LastSeen != 20 {
		t.Errorf("Missing=%v LastSeen=%d, want false/20", got.Missing, got.LastSeen)
	}
}

func TestUpsertBooksIsAtomic(t *testing.T) {
	s := newTestStore(t)
	mustUpsertLibs(t, s, mkRoot("r", "Raiz"))
	err := s.UpsertBooks(context.Background(), []Book{
		mkBook("ok", "r", "ok.cbz"),
		mkBook("bad", "ghost-library", "bad.cbz"), // viola a FK
	})
	if err == nil {
		t.Fatal("expected FK error")
	}
	if n := countRows(t, s, "books"); n != 0 {
		t.Errorf("books = %d, want 0 (rolled back)", n)
	}
}

func TestMarkMissing(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustUpsertLibs(t, s, mkRoot("r", "Raiz"), mkLib("f", "r", "r", "F"), mkRoot("o", "Outra"))
	a, b := mkBook("a", "r", "a.cbz"), mkBook("b", "f", "b.cbz") // b numa sub-biblioteca da raiz
	a.LastSeen, b.LastSeen = 100, 50
	stranger := mkBook("x", "o", "x.cbz")
	stranger.LastSeen = 1 // velho, mas de outra raiz
	mustUpsertBooks(t, s, a, b, stranger)

	n, err := s.MarkMissing(ctx, "r", 100)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("MarkMissing = %d, want 1", n)
	}
	missing := func(id string) bool {
		g, ok, err := s.GetBook(ctx, id)
		if err != nil || !ok {
			t.Fatalf("GetBook(%s): ok=%v err=%v", id, ok, err)
		}
		return g.Missing
	}
	if missing("a") || !missing("b") || missing("x") {
		t.Errorf("missing a=%v b=%v x=%v, want false true false", missing("a"), missing("b"), missing("x"))
	}

	if err := s.TouchBooks(ctx, []string{"b"}, 120); err != nil {
		t.Fatal(err)
	}
	if missing("b") {
		t.Error("TouchBooks did not clear missing")
	}
	g, _, _ := s.GetBook(ctx, "b")
	if g.LastSeen != 120 {
		t.Errorf("LastSeen = %d, want 120", g.LastSeen)
	}
	// a (last_seen 100) agora é mais velho que o token 120 e fica ausente; b não.
	n, err = s.MarkMissing(ctx, "r", 120)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || !missing("a") || missing("b") || missing("x") {
		t.Errorf("after second pass n=%d a=%v b=%v x=%v", n, missing("a"), missing("b"), missing("x"))
	}
	// Sem nada novo para marcar: 0.
	if n, err := s.MarkMissing(ctx, "r", 120); err != nil || n != 0 {
		t.Errorf("repeat MarkMissing = %d, %v, want 0", n, err)
	}
}

func TestTouchBooksEmptyAndUnknown(t *testing.T) {
	s := newTestStore(t)
	if err := s.TouchBooks(context.Background(), nil, 1); err != nil {
		t.Errorf("empty: %v", err)
	}
	if err := s.TouchBooks(context.Background(), []string{"ghost"}, 1); err != nil {
		t.Errorf("unknown id: %v", err)
	}
}

func TestListBooksNaturalOrderWithProgress(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustUpsertLibs(t, s, mkRoot("r", "Raiz"), mkLib("f", "r", "r", "F"))
	mustUpsertBooks(t, s,
		mkBook("b2", "r", "HQ 2.cbz"),
		mkBook("b10", "r", "HQ 10.cbz"),
		mkBook("b1", "r", "HQ 1.cbz"),
		mkBook("bx", "r", "hq 3.CBZ"),
		mkBook("sub", "f", "Zzz.cbz"), // de outra biblioteca: não entra
	)
	if err := s.SaveProgress(ctx, "b2", 4, 9, 77); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListBooks(ctx, "r")
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, b := range list {
		order = append(order, b.ID)
	}
	want := []string{"b1", "b2", "bx", "b10"}
	if fmt.Sprint(order) != fmt.Sprint(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for _, b := range list {
		if b.ID == "b2" {
			if b.Progress == nil {
				t.Fatal("b2 should carry Progress")
			}
			if *b.Progress != (Progress{BookID: "b2", CurrentPage: 4, FurthestPage: 9, UpdatedAt: 77}) {
				t.Errorf("Progress = %+v", *b.Progress)
			}
		} else if b.Progress != nil {
			t.Errorf("%s has unexpected progress %+v", b.ID, *b.Progress)
		}
	}
	empty, err := s.ListBooks(ctx, "nada")
	if err != nil || len(empty) != 0 {
		t.Errorf("ListBooks(unknown) = %v, %v", empty, err)
	}
}

func TestGetBookMissing(t *testing.T) {
	s := newTestStore(t)
	_, ok, err := s.GetBook(context.Background(), "nope")
	if err != nil || ok {
		t.Errorf("ok=%v err=%v, want false,nil", ok, err)
	}
}

// TestConcurrentReadDuringWrite: leituras (WAL, sem o mutex de escrita) nunca
// recebem "database is locked" enquanto um escritor grava lotes.
func TestConcurrentReadDuringWrite(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustUpsertLibs(t, s, mkRoot("r", "Raiz"))

	const total, batch = 2000, 100
	done := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			iter := 0
			for {
				select {
				case <-done:
					if iter == 0 {
						t.Errorf("reader %d never ran", i)
					}
					return
				default:
				}
				iter++
				if _, err := s.ListBooks(ctx, "r"); err != nil {
					t.Errorf("ListBooks: %v", err)
					return
				}
				if _, err := s.Stats(ctx, "r"); err != nil {
					t.Errorf("Stats: %v", err)
					return
				}
			}
		}(i)
	}
	for start := 0; start < total; start += batch {
		books := make([]Book, 0, batch)
		for i := start; i < start+batch; i++ {
			books = append(books, mkBook(fmt.Sprintf("b%05d", i), "r", fmt.Sprintf("HQ %d.cbz", i)))
		}
		if err := s.UpsertBooks(ctx, books); err != nil {
			t.Fatalf("UpsertBooks: %v", err)
		}
	}
	close(done)
	wg.Wait()
	if st, err := s.Stats(ctx, "r"); err != nil || st.Books != total {
		t.Errorf("Stats = %+v, %v, want %d books", st, err, total)
	}
}

// TestConcurrentWritersDoNotBusy reproduz o achado do spike: uma transação
// "lê e depois escreve" (SaveProgress) concorrente com um escritor em lotes
// (UpsertBooks) falhava com SQLITE_BUSY. O mutex de escrita do Store deve
// zerar os erros.
func TestConcurrentWritersDoNotBusy(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustUpsertLibs(t, s, mkRoot("r", "Raiz"))
	const seed = 500
	pre := make([]Book, 0, seed)
	for i := 0; i < seed; i++ {
		pre = append(pre, mkBook(fmt.Sprintf("p%04d", i), "r", fmt.Sprintf("P %d.cbz", i)))
	}
	mustUpsertBooks(t, s, pre...)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { // escritor 1: lotes de livros novos
		defer wg.Done()
		for start := 0; start < 3000; start += 100 {
			books := make([]Book, 0, 100)
			for i := start; i < start+100; i++ {
				books = append(books, mkBook(fmt.Sprintf("n%05d", i), "r", fmt.Sprintf("N %d.cbz", i)))
			}
			if err := s.UpsertBooks(ctx, books); err != nil {
				t.Errorf("UpsertBooks: %v", err)
				return
			}
		}
	}()
	go func() { // escritor 2: SaveProgress (lê o livro e depois grava)
		defer wg.Done()
		for i := 0; i < 500; i++ {
			id := fmt.Sprintf("p%04d", i%seed)
			if err := s.SaveProgress(ctx, id, i%20, i%30, int64(i)); err != nil {
				t.Errorf("SaveProgress #%d: %v", i, err)
				return
			}
		}
	}()
	wg.Wait()
	if n := countRows(t, s, "books"); n != seed+3000 {
		t.Errorf("books = %d, want %d", n, seed+3000)
	}
	if n := countRows(t, s, "reading_progress"); n != seed {
		t.Errorf("reading_progress = %d, want %d", n, seed)
	}
}
