package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func mustLib(t *testing.T, s *Store, id string) Library {
	t.Helper()
	l, ok, err := s.GetLibrary(context.Background(), id)
	if err != nil || !ok {
		t.Fatalf("GetLibrary(%s): ok=%v err=%v", id, ok, err)
	}
	return l
}

func TestUpsertLibrariesRejectsSelfParent(t *testing.T) {
	s := newTestStore(t)
	mustUpsertLibs(t, s, mkRoot("r", "Raiz"))
	// Nova biblioteca que é pai dela mesma.
	err := s.UpsertLibraries(context.Background(), []Library{mkLib("x", "x", "r", "X")})
	if !errors.Is(err, ErrInvalidTree) {
		t.Fatalf("self-parent insert: err = %v, want ErrInvalidTree", err)
	}
	// Biblioteca existente sendo re-pai de si mesma.
	mustUpsertLibs(t, s, mkLib("a", "r", "r", "A"))
	err = s.UpsertLibraries(context.Background(), []Library{mkLib("a", "a", "r", "A")})
	if !errors.Is(err, ErrInvalidTree) {
		t.Fatalf("self-parent reupsert: err = %v, want ErrInvalidTree", err)
	}
	if n := countRows(t, s, "libraries"); n != 2 {
		t.Errorf("libraries = %d, want 2 (r, a)", n)
	}
	if a := mustLib(t, s, "a"); a.ParentID != "r" {
		t.Errorf("a.ParentID = %q, want r (unchanged)", a.ParentID)
	}
}

func TestUpsertLibrariesRejectsCycle(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustUpsertLibs(t, s,
		mkRoot("r", "Raiz"),
		mkLib("a", "r", "r", "A"),
		mkLib("b", "a", "r", "B"),
		mkLib("c", "b", "r", "C"),
		mkLib("d", "r", "r", "D"),
	)
	snapshot := func() map[string]string {
		m := map[string]string{}
		for _, id := range []string{"r", "a", "b", "c", "d"} {
			m[id] = mustLib(t, s, id).ParentID
		}
		return m
	}
	before := snapshot()

	// A sob B (B é filho de A): ciclo de 2.
	if err := s.UpsertLibraries(ctx, []Library{mkLib("a", "b", "r", "A")}); !errors.Is(err, ErrInvalidTree) {
		t.Errorf("2-cycle: err = %v, want ErrInvalidTree", err)
	}
	// A sob C (A -> B -> C): ciclo de 3.
	if err := s.UpsertLibraries(ctx, []Library{mkLib("a", "c", "r", "A")}); !errors.Is(err, ErrInvalidTree) {
		t.Errorf("3-cycle: err = %v, want ErrInvalidTree", err)
	}
	// O lote inteiro desfaz: a nova biblioteca "n" (válida) vem antes do ciclo.
	err := s.UpsertLibraries(ctx, []Library{mkLib("n", "r", "r", "N"), mkLib("a", "b", "r", "A")})
	if !errors.Is(err, ErrInvalidTree) {
		t.Errorf("batch with cycle: err = %v, want ErrInvalidTree", err)
	}
	if _, ok, _ := s.GetLibrary(ctx, "n"); ok {
		t.Error("batch was not rolled back: n exists")
	}
	after := snapshot()
	for id, p := range before {
		if after[id] != p {
			t.Errorf("tree changed: %s parent %q -> %q", id, p, after[id])
		}
	}
	// Árvore ainda navegável.
	anc, err := s.Ancestors(ctx, "c")
	if err != nil || fmt.Sprint(ids(anc)) != "[r a b c]" {
		t.Errorf("Ancestors(c) = %v, %v", ids(anc), err)
	}

	// Mover uma sub-árvore para outro ramo da mesma raiz continua permitido.
	if err := s.UpsertLibraries(ctx, []Library{mkLib("a", "d", "r", "A")}); err != nil {
		t.Fatalf("valid re-parent: %v", err)
	}
	anc, _ = s.Ancestors(ctx, "c")
	if fmt.Sprint(ids(anc)) != "[r d a b c]" {
		t.Errorf("Ancestors(c) after move = %v", ids(anc))
	}
}

func TestUpsertLibrariesRejectsBadRoot(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustUpsertLibs(t, s, mkRoot("r", "Raiz"), mkLib("a", "r", "r", "A"), mkRoot("r2", "Outra"))
	valid := mkLib("ok", "r", "r", "Ok") // vem antes: tem que desfazer junto
	cases := []struct {
		name string
		bad  Library
	}{
		{"root with RootID != ID", Library{ID: "x", Path: "/lib/x", Name: "X", RootID: "r", AddedAt: 1}},
		{"root with empty RootID", Library{ID: "x", Path: "/lib/x", Name: "X", AddedAt: 1}},
		{"child RootID differs from parent's", mkLib("x", "a", "r2", "X")},
		{"child with unknown parent", mkLib("x", "ghost", "r", "X")},
		{"existing child turned into a root", Library{ID: "a", Path: "/lib/a", Name: "A", RootID: "a", AddedAt: 1}},
		{"existing library moved to another root", mkLib("a", "r2", "r2", "A")},
	}
	for _, c := range cases {
		err := s.UpsertLibraries(ctx, []Library{valid, c.bad})
		if !errors.Is(err, ErrInvalidTree) {
			t.Errorf("%s: err = %v, want ErrInvalidTree", c.name, err)
		}
		if _, ok, _ := s.GetLibrary(ctx, "ok"); ok {
			t.Errorf("%s: batch not rolled back (ok exists)", c.name)
		}
		if _, ok, _ := s.GetLibrary(ctx, "x"); ok {
			t.Errorf("%s: x was written", c.name)
		}
	}
	if a := mustLib(t, s, "a"); a.ParentID != "r" || a.RootID != "r" {
		t.Errorf("a changed: %+v", a)
	}
	// Dentro do mesmo lote, o pai que vem antes vale.
	err := s.UpsertLibraries(ctx, []Library{mkRoot("r3", "Terceira"), mkLib("r3a", "r3", "r3", "A")})
	if err != nil {
		t.Errorf("valid batch with parent earlier in the batch: %v", err)
	}
}

// TestStatsAndAncestorsTerminateOnPoisonedTree: mesmo que um banco editado à
// mão (ou um bug futuro) contenha ciclos, as consultas recursivas terminam.
func TestStatsAndAncestorsTerminateOnPoisonedTree(t *testing.T) {
	s := newTestStore(t)
	mustUpsertLibs(t, s, mkRoot("r", "Raiz"))
	mustUpsertBooks(t, s, mkBook("b1", "r", "1.cbz"))

	// Contorna o UpsertLibraries com SQL cru. defer_foreign_keys adia a FK até o
	// commit (quando todas as linhas já existem) e volta ao normal sozinho.
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		"PRAGMA defer_foreign_keys = ON",
		"INSERT INTO libraries (id, path, name, parent_id, root_id, added_at) VALUES ('a', '/p/a', 'A', 'b', 'r', 1)",
		"INSERT INTO libraries (id, path, name, parent_id, root_id, added_at) VALUES ('b', '/p/b', 'B', 'a', 'r', 1)",
		"INSERT INTO libraries (id, path, name, parent_id, root_id, added_at) VALUES ('c', '/p/c', 'C', 'c', 'r', 1)",
		"INSERT INTO books (id, library_id, path, size, mtime, format, added_at) VALUES ('ba', 'a', '/p/a/1.cbz', 1, 1, 'cbz', 1)",
	} {
		if _, err := tx.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Cada chamada com timeout: uma regressão devolve erro em vez de travar.
	run := func(name string, fn func(ctx context.Context) error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		start := time.Now()
		if err := fn(ctx); err != nil {
			t.Errorf("%s: %v (after %v)", name, err, time.Since(start))
		}
	}
	run("Stats(a)", func(ctx context.Context) error { _, err := s.Stats(ctx, "a"); return err })
	run("Stats(c)", func(ctx context.Context) error { _, err := s.Stats(ctx, "c"); return err })
	run("Ancestors(a)", func(ctx context.Context) error {
		anc, err := s.Ancestors(ctx, "a")
		if err == nil && len(anc) > maxTreeDepth+1 {
			t.Errorf("Ancestors returned %d rows, want <= %d", len(anc), maxTreeDepth+1)
		}
		return err
	})
	run("Ancestors(c)", func(ctx context.Context) error { _, err := s.Ancestors(ctx, "c"); return err })
	run("ListChildren(a)", func(ctx context.Context) error { _, err := s.ListChildren(ctx, "a"); return err })
	run("Fingerprints(r)", func(ctx context.Context) error { _, err := s.Fingerprints(ctx, "r"); return err })
	run("MarkMissing(r)", func(ctx context.Context) error { _, err := s.MarkMissing(ctx, "r", 5); return err })
	run("DeleteRoot(r)", func(ctx context.Context) error { return s.DeleteRoot(ctx, "r") })
	if n := countRows(t, s, "libraries"); n != 0 {
		t.Errorf("libraries = %d after DeleteRoot, want 0", n)
	}
}

func TestSaveProgressClampsInputs(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustUpsertLibs(t, s, mkRoot("r", "Raiz"))
	mustUpsertBooks(t, s, mkBook("b", "r", "b.cbz"))
	get := func() Progress {
		g, ok, err := s.GetBook(ctx, "b")
		if err != nil || !ok || g.Progress == nil {
			t.Fatalf("GetBook: ok=%v err=%v", ok, err)
		}
		return *g.Progress
	}

	// Negativos viram 0.
	if err := s.SaveProgress(ctx, "b", -3, -7, 1); err != nil {
		t.Fatal(err)
	}
	if p := get(); p.CurrentPage != 0 || p.FurthestPage != 0 {
		t.Errorf("negatives: %+v, want 0/0", p)
	}
	// furthest abaixo de current sobe até current.
	if err := s.SaveProgress(ctx, "b", 9, 2, 2); err != nil {
		t.Fatal(err)
	}
	if p := get(); p.CurrentPage != 9 || p.FurthestPage != 9 {
		t.Errorf("furthest<current: %+v, want 9/9", p)
	}
	// Continua monotônico: voltar páginas não derruba o furthest.
	if err := s.SaveProgress(ctx, "b", 1, 0, 3); err != nil {
		t.Fatal(err)
	}
	if p := get(); p.CurrentPage != 1 || p.FurthestPage != 9 {
		t.Errorf("after going back: %+v, want current 1 furthest 9", p)
	}
	// Primeira gravação de um livro novo com furthest < current.
	mustUpsertBooks(t, s, mkBook("b2", "r", "b2.cbz"))
	if err := s.SaveProgress(ctx, "b2", 4, 1, 4); err != nil {
		t.Fatal(err)
	}
	if g, _, _ := s.GetBook(ctx, "b2"); g.Progress.FurthestPage != 4 {
		t.Errorf("first save furthest = %d, want 4", g.Progress.FurthestPage)
	}
	if err := s.SaveProgress(ctx, "inexistente", -1, -1, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown book: %v, want ErrNotFound", err)
	}
}

func TestMarkScannedOnlyAffectsRoots(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustUpsertLibs(t, s, mkRoot("r", "Raiz"), mkLib("f", "r", "r", "Filha"))
	if err := s.MarkScanned(ctx, "f", 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("MarkScanned(non-root) = %v, want ErrNotFound", err)
	}
	for _, id := range []string{"r", "f"} {
		if l := mustLib(t, s, id); l.ScannedAt != 0 {
			t.Errorf("%s.ScannedAt = %d, want 0", id, l.ScannedAt)
		}
	}
}

func TestTouchBooksManyIDs(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustUpsertLibs(t, s, mkRoot("r", "Raiz"))
	const total = 5000
	var all []string
	for start := 0; start < total; start += 500 {
		batch := make([]Book, 0, 500)
		for i := start; i < start+500; i++ {
			b := mkBook(fmt.Sprintf("b%05d", i), "r", fmt.Sprintf("%d.cbz", i))
			b.LastSeen = 10
			batch = append(batch, b)
			all = append(all, b.ID)
		}
		mustUpsertBooks(t, s, batch...)
	}
	if _, err := s.db.Exec("UPDATE books SET missing = 1"); err != nil {
		t.Fatal(err)
	}
	if err := s.TouchBooks(ctx, all, 777); err != nil {
		t.Fatalf("TouchBooks(%d ids): %v", total, err)
	}
	var seen, missing int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM books WHERE last_seen = 777").Scan(&seen); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM books WHERE missing = 1").Scan(&missing); err != nil {
		t.Fatal(err)
	}
	if seen != total || missing != 0 {
		t.Errorf("last_seen=777 rows = %d (want %d), missing rows = %d (want 0)", seen, total, missing)
	}
}
