package store

import (
	"context"
	"testing"
)

func TestSaveProgressFurthestIsMonotonic(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustUpsertLibs(t, s, mkRoot("r", "Raiz"))
	mustUpsertBooks(t, s, mkBook("b", "r", "b.cbz"))

	if err := s.SaveProgress(ctx, "b", 5, 8, 10); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProgress(ctx, "b", 2, 3, 20); err != nil {
		t.Fatal(err)
	}
	g, ok, err := s.GetBook(ctx, "b")
	if err != nil || !ok || g.Progress == nil {
		t.Fatalf("GetBook: ok=%v err=%v progress=%v", ok, err, g.Progress)
	}
	if g.Progress.CurrentPage != 2 || g.Progress.FurthestPage != 8 || g.Progress.UpdatedAt != 20 {
		t.Errorf("Progress = %+v, want current 2 furthest 8 updated 20", *g.Progress)
	}
	// Avançar de novo sobe o furthest.
	if err := s.SaveProgress(ctx, "b", 9, 12, 30); err != nil {
		t.Fatal(err)
	}
	g, _, _ = s.GetBook(ctx, "b")
	if g.Progress.FurthestPage != 12 || g.Progress.CurrentPage != 9 {
		t.Errorf("Progress = %+v", *g.Progress)
	}

	if err := s.SaveProgress(ctx, "inexistente", 1, 1, 1); !errorsIs(err, ErrNotFound) {
		t.Errorf("SaveProgress(unknown) = %v, want ErrNotFound", err)
	}
	if n := countRows(t, s, "reading_progress"); n != 1 {
		t.Errorf("reading_progress = %d, want 1", n)
	}
}

func TestProgressSurvivesBookReupsert(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustUpsertLibs(t, s, mkRoot("r", "Raiz"))
	b := mkBook("b", "r", "b.cbz")
	mustUpsertBooks(t, s, b)
	if err := s.SaveProgress(ctx, "b", 3, 6, 5); err != nil {
		t.Fatal(err)
	}
	b.Size = 12345 // o scan reupserta (INSERT OR REPLACE apagaria o progresso via cascata)
	mustUpsertBooks(t, s, b)
	g, _, _ := s.GetBook(ctx, "b")
	if g.Progress == nil || g.Progress.FurthestPage != 6 {
		t.Errorf("progress lost on reupsert: %+v", g.Progress)
	}
}
