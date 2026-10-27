package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestStore abre um Store novo num arquivo temporário e o fecha ao fim do teste.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func pragmaInt(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	var v int
	if err := db.QueryRow("PRAGMA " + name).Scan(&v); err != nil {
		t.Fatalf("PRAGMA %s: %v", name, err)
	}
	return v
}

func pragmaString(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	var v string
	if err := db.QueryRow("PRAGMA " + name).Scan(&v); err != nil {
		t.Fatalf("PRAGMA %s: %v", name, err)
	}
	return v
}

func countRows(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// mkRoot / mkLib / mkBook constroem fixtures mínimas.
func mkRoot(id, name string) Library {
	return Library{ID: id, Path: "/lib/" + id, Name: name, RootID: id, AddedAt: 1000}
}

func mkLib(id, parent, root, name string) Library {
	return Library{ID: id, Path: "/lib/" + id, Name: name, ParentID: parent, RootID: root, AddedAt: 1000}
}

func mkBook(id, lib, file string) Book {
	return Book{
		ID: id, LibraryID: lib, Path: "/lib/" + lib + "/" + file, Format: "cbz",
		Title: strings.TrimSuffix(file, ".cbz"), Size: 100, MTime: 200, AddedAt: 300,
		PageCount: 10, LastSeen: 400,
	}
}

func mustUpsertLibs(t *testing.T, s *Store, libs ...Library) {
	t.Helper()
	if err := s.UpsertLibraries(context.Background(), libs); err != nil {
		t.Fatalf("UpsertLibraries: %v", err)
	}
}

func mustUpsertBooks(t *testing.T, s *Store, books ...Book) {
	t.Helper()
	if err := s.UpsertBooks(context.Background(), books); err != nil {
		t.Fatalf("UpsertBooks: %v", err)
	}
}

func TestOpenCreatesSchemaV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if v := pragmaInt(t, s.db, "user_version"); v != 1 {
		t.Errorf("user_version = %d, want 1", v)
	}
	if SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", SchemaVersion)
	}
	if m := pragmaString(t, s.db, "journal_mode"); m != "wal" {
		t.Errorf("journal_mode = %q, want wal", m)
	}
	if fk := pragmaInt(t, s.db, "foreign_keys"); fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
	mustUpsertLibs(t, s, mkRoot("r", "Raiz"))
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Reabrir não refaz a migração (refazer falharia com "table already exists"
	// ou apagaria os dados).
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	if v := pragmaInt(t, s2.db, "user_version"); v != 1 {
		t.Errorf("after reopen user_version = %d, want 1", v)
	}
	if _, ok, err := s2.GetLibrary(context.Background(), "r"); err != nil || !ok {
		t.Errorf("library lost after reopen: ok=%v err=%v", ok, err)
	}
}

func fileHash(t *testing.T, path string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

func TestOpenRefusesNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("PRAGMA user_version = 99"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	before := fileHash(t, path)
	s2, err := Open(path)
	if err == nil {
		s2.Close()
		t.Fatal("Open succeeded on a newer schema")
	}
	if !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("err = %v, want ErrSchemaTooNew", err)
	}
	if after := fileHash(t, path); after != before {
		t.Error("database file changed after refused Open")
	}
}

func TestOpenRefusesNewerSchemaInRollbackJournalMode(t *testing.T) {
	// Um banco "do futuro" que não está em WAL: o Open não pode nem trocar o
	// modo de journal (isso reescreve o cabeçalho do arquivo).
	path := filepath.Join(t.TempDir(), "t.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec("PRAGMA user_version = 99; CREATE TABLE x(a)"); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	before := fileHash(t, path)
	if s, err := Open(path); !errors.Is(err, ErrSchemaTooNew) {
		if s != nil {
			s.Close()
		}
		t.Fatalf("err = %v, want ErrSchemaTooNew", err)
	}
	if after := fileHash(t, path); after != before {
		t.Error("database file changed after refused Open")
	}
}

func TestOpenPathWithSpecialChars(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Pasta Ação com espaço #1 & 100%")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "t.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database not at expected path: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(dir) {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("unexpected entries beside the folder: %v", names)
	}
	if m := pragmaString(t, s.db, "journal_mode"); m != "wal" {
		t.Errorf("journal_mode = %q, want wal", m)
	}
	if fk := pragmaInt(t, s.db, "foreign_keys"); fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}

func TestOpenCreatesMissingParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hqreader", "sub", "library.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("db not created: %v", err)
	}
}

func TestOpenRejectsQuestionMarkInPath(t *testing.T) {
	// "?" separaria o caminho da query string da DSN.
	if s, err := Open(filepath.Join(t.TempDir(), "a?b.db")); err == nil {
		s.Close()
		t.Error("expected an error for a path containing '?'")
	}
}

func TestPragmasApplyToEveryPooledConnection(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	// Segura várias conexões ao mesmo tempo para forçar o pool a abrir novas.
	var conns []*sql.Conn
	for i := 0; i < 4; i++ {
		c, err := s.db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}
	for i, c := range conns {
		var fk, bt int
		if err := c.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
			t.Errorf("conn %d foreign_keys=%d err=%v", i, fk, err)
		}
		if err := c.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&bt); err != nil || bt != 5000 {
			t.Errorf("conn %d busy_timeout=%d err=%v", i, bt, err)
		}
		_ = c.Close()
	}
}

func TestNaturalLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"HQ 2.cbz", "HQ 10.cbz", true},
		{"HQ 10.cbz", "HQ 2.cbz", false},
		{"hq 1", "HQ 2", true},
		{"abc", "ABD", true},
		{"ABC", "abd", true},
		{"a", "a", false},
		{"a", "ab", true},
		{"ab", "a", false},
		{"x99999999999999999999999", "x100000000000000000000000", true},
		{"x007", "x7a", true},
		{"Ação 2", "ação 10", true},
	}
	for _, c := range cases {
		if got := naturalLess(c.a, c.b); got != c.want {
			t.Errorf("naturalLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
