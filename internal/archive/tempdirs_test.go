package archive

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanOrphanedTempDirs(t *testing.T) {
	dir := t.TempDir()
	orig := tempDirRoot
	tempDirRoot = func() string { return dir }
	defer func() { tempDirRoot = orig }()

	orphan := filepath.Join(dir, "orphan-123")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(orphan, "page1.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := CleanOrphanedTempDirs(); err != nil {
		t.Fatalf("CleanOrphanedTempDirs: %v", err)
	}

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("orphan ainda existe: %v", err)
	}
}

func TestCleanOrphanedTempDirsNoDir(t *testing.T) {
	orig := tempDirRoot
	tempDirRoot = func() string { return filepath.Join(t.TempDir(), "nao-existe") }
	defer func() { tempDirRoot = orig }()

	if err := CleanOrphanedTempDirs(); err != nil {
		t.Errorf("CleanOrphanedTempDirs em diretório inexistente: %v", err)
	}
}
