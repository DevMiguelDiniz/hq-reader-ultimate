package store

import (
	"context"
	"reflect"
	"testing"
)

func ids(libs []Library) []string {
	var out []string
	for _, l := range libs {
		out = append(out, l.ID)
	}
	return out
}

func TestLibraryTree(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustUpsertLibs(t, s,
		mkRoot("r", "Raiz"),
		mkLib("f1", "r", "r", "Filha 2"),
		mkLib("f2", "r", "r", "Filha 10"),
		mkLib("n", "f1", "r", "Neta"),
	)

	kids, err := s.ListChildren(ctx, "r")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(kids), []string{"f1", "f2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListChildren(r) = %v, want %v", got, want)
	}
	anc, err := s.Ancestors(ctx, "n")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(anc), []string{"r", "f1", "n"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Ancestors(n) = %v, want %v", got, want)
	}
	roots, err := s.ListRoots(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(roots), []string{"r"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListRoots = %v, want %v", got, want)
	}
	if roots[0].ParentID != "" || roots[0].RootID != "r" {
		t.Errorf("root fields wrong: %+v", roots[0])
	}

	// Reupsert muda o nome (e o caminho), mas nunca added_at nem scanned_at.
	if err := s.MarkScanned(ctx, "r", 777); err != nil {
		t.Fatal(err)
	}
	renamed := mkLib("f1", "r", "r", "Renomeada")
	renamed.AddedAt = 9999
	renamed.Path = "/novo/caminho"
	rootAgain := mkRoot("r", "Raiz Nova")
	rootAgain.AddedAt = 5555
	mustUpsertLibs(t, s, rootAgain, renamed)
	got, ok, err := s.GetLibrary(ctx, "f1")
	if err != nil || !ok {
		t.Fatalf("GetLibrary: ok=%v err=%v", ok, err)
	}
	if got.Name != "Renomeada" || got.Path != "/novo/caminho" {
		t.Errorf("name/path not updated: %+v", got)
	}
	if got.AddedAt != 1000 {
		t.Errorf("AddedAt = %d, want 1000 (must not be overwritten)", got.AddedAt)
	}
	r, _, _ := s.GetLibrary(ctx, "r")
	if r.Name != "Raiz Nova" || r.AddedAt != 1000 || r.ScannedAt != 777 {
		t.Errorf("root after reupsert: %+v (want AddedAt 1000, ScannedAt 777)", r)
	}
}

func TestGetLibraryMissing(t *testing.T) {
	s := newTestStore(t)
	_, ok, err := s.GetLibrary(context.Background(), "nope")
	if err != nil || ok {
		t.Errorf("ok=%v err=%v, want false,nil", ok, err)
	}
}

func TestListRootsAndChildrenNaturalCaseInsensitive(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustUpsertLibs(t, s,
		mkRoot("a", "HQ 10"),
		mkRoot("b", "hq 2"),
		mkRoot("c", "Zebra"),
		mkRoot("d", "alfa"),
		mkLib("k1", "a", "a", "Vol 10"),
		mkLib("k2", "a", "a", "vol 9"),
		mkLib("k3", "a", "a", "Vol 1"),
	)
	roots, err := s.ListRoots(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(roots), []string{"d", "b", "a", "c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListRoots order = %v, want %v", got, want)
	}
	kids, err := s.ListChildren(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(kids), []string{"k3", "k2", "k1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListChildren order = %v, want %v", got, want)
	}
	none, err := s.ListChildren(ctx, "b")
	if err != nil || len(none) != 0 {
		t.Errorf("ListChildren(leaf) = %v, %v", none, err)
	}
}

func TestUpsertLibrariesRejectsMissingParent(t *testing.T) {
	s := newTestStore(t)
	err := s.UpsertLibraries(context.Background(), []Library{mkLib("f", "ghost", "ghost", "F")})
	if err == nil {
		t.Fatal("expected a foreign-key error for an unknown parent")
	}
	// A transação é atômica: nada foi gravado.
	if n := countRows(t, s, "libraries"); n != 0 {
		t.Errorf("libraries = %d, want 0", n)
	}
}

func TestDeleteRootCascades(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustUpsertLibs(t, s,
		mkRoot("r1", "Um"), mkLib("r1a", "r1", "r1", "A"), mkLib("r1b", "r1a", "r1", "B"),
		mkRoot("r2", "Dois"), mkLib("r2a", "r2", "r2", "A"),
	)
	mustUpsertBooks(t, s,
		mkBook("b1", "r1", "1.cbz"), mkBook("b2", "r1a", "2.cbz"), mkBook("b3", "r1b", "3.cbz"),
		mkBook("c1", "r2", "1.cbz"), mkBook("c2", "r2a", "2.cbz"),
	)
	for _, id := range []string{"b1", "b3", "c1", "c2"} {
		if err := s.SaveProgress(ctx, id, 1, 2, 50); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.DeleteRoot(ctx, "r1"); err != nil {
		t.Fatal(err)
	}
	var n int
	for _, q := range []string{
		"SELECT COUNT(*) FROM libraries WHERE root_id='r1' OR id='r1'",
		"SELECT COUNT(*) FROM books WHERE id IN ('b1','b2','b3')",
		"SELECT COUNT(*) FROM reading_progress WHERE book_id IN ('b1','b2','b3')",
	} {
		if err := s.db.QueryRow(q).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s = %d (err %v), want 0", q, n, err)
		}
	}
	if got := countRows(t, s, "libraries"); got != 2 {
		t.Errorf("libraries = %d, want 2 (second root intact)", got)
	}
	if got := countRows(t, s, "books"); got != 2 {
		t.Errorf("books = %d, want 2", got)
	}
	if got := countRows(t, s, "reading_progress"); got != 2 {
		t.Errorf("reading_progress = %d, want 2", got)
	}

	// Idempotente: apagar de novo (ou uma raiz inexistente) não é erro.
	if err := s.DeleteRoot(ctx, "r1"); err != nil {
		t.Errorf("second DeleteRoot: %v", err)
	}
	// Não serve para apagar uma sub-biblioteca avulsa.
	if err := s.DeleteRoot(ctx, "r2a"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.GetLibrary(ctx, "r2a"); !ok {
		t.Error("DeleteRoot removed a non-root library")
	}
}

func TestStatsCountsSubtreeNonMissing(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustUpsertLibs(t, s,
		mkRoot("r", "Raiz"), mkLib("f", "r", "r", "Filha"), mkLib("n", "f", "r", "Neta"),
		mkRoot("other", "Outra"),
	)
	mustUpsertBooks(t, s,
		mkBook("r1", "r", "1.cbz"), mkBook("r2", "r", "2.cbz"), mkBook("r3", "r", "3.cbz"),
		mkBook("f1", "f", "f1.cbz"), mkBook("f2", "f", "f2.cbz"),
		mkBook("n1", "n", "n1.cbz"),
		mkBook("o1", "other", "o1.cbz"),
	)
	if _, err := s.db.Exec("UPDATE books SET missing=1 WHERE id='f2'"); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		lib  string
		want Stats
	}{
		{"r", Stats{Books: 5, Folders: 1}},
		{"f", Stats{Books: 2, Folders: 1}},
		{"n", Stats{Books: 1, Folders: 0}},
		{"other", Stats{Books: 1, Folders: 0}},
		{"ghost", Stats{}},
	} {
		got, err := s.Stats(ctx, c.lib)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("Stats(%s) = %+v, want %+v", c.lib, got, c.want)
		}
	}
}

func TestMarkScanned(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustUpsertLibs(t, s, mkRoot("r", "Raiz"))
	if l, _, _ := s.GetLibrary(ctx, "r"); l.ScannedAt != 0 {
		t.Errorf("new library ScannedAt = %d, want 0", l.ScannedAt)
	}
	if err := s.MarkScanned(ctx, "r", 4242); err != nil {
		t.Fatal(err)
	}
	if l, _, _ := s.GetLibrary(ctx, "r"); l.ScannedAt != 4242 {
		t.Errorf("ScannedAt = %d, want 4242", l.ScannedAt)
	}
	if err := s.MarkScanned(ctx, "ghost", 1); !errorsIs(err, ErrNotFound) {
		t.Errorf("MarkScanned(ghost) = %v, want ErrNotFound", err)
	}
}
