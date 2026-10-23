package pathnorm

import (
	"archive/zip"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSameFolderSameID(t *testing.T) {
	base := PathID(`C:\Comics\X`)
	for _, p := range []string{`C:\Comics\X`, `c:/comics/x/`, `C:\Comics\.\X`, `C:\Comics\X\`, `C:\Comics\Y\..\X`, `C:/COMICS/X//`} {
		t.Logf("%-22q -> norm=%q id=%s same=%v", p, NormalizePath(p), PathID(p), PathID(p) == base)
		if PathID(p) != base {
			t.Errorf("%q should collapse to the id of C:\\Comics\\X", p)
		}
	}
	if PathID(`C:\Comics\Y`) == base {
		t.Errorf("C:\\Comics\\Y must differ")
	}
	t.Logf("%-22q -> norm=%q id=%s same=%v", `C:\Comics\Y`, NormalizePath(`C:\Comics\Y`), PathID(`C:\Comics\Y`), PathID(`C:\Comics\Y`) == base)
}

func TestEdgeCases(t *testing.T) {
	cwd, _ := os.Getwd()
	t.Logf("cwd=%q", cwd)
	for _, p := range []string{
		`C:\`, `C:`, `c:/`,
		`\\server\share\Comics`, `//server/share/comics/`,
		`\\?\C:\Comics\X`,
		`C:\Comics\Ação Especial`, `c:/comics/AÇÃO ESPECIAL/`,
		"C:\\Comics\\A\u0303o", // NFD: 'A' + til combinante
		"C:\\Comics\\\u00c3o",  // NFC: 'Ã'
		`Comics\X`,             // relativo
	} {
		t.Logf("%-28q -> %q id=%s", p, NormalizePath(p), PathID(p))
	}
}

// Pasta real com acentos/espacos e caixa diferente: o Windows abre as duas
// grafias (NTFS sem distincao de caixa) e o id colapsa; NFC e NFD sao nomes
// DIFERENTES no NTFS (duas pastas coexistem).
func TestRealFoldersAccentsCaseNFCNFD(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "Ação Especial")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	upper := filepath.Join(root, "AÇÃO ESPECIAL")
	st, err := os.Stat(upper)
	t.Logf("stat with other case: err=%v isDir=%v; sameID=%v", err, err == nil && st.IsDir(), PathID(real) == PathID(upper))

	nfc := filepath.Join(root, "\u00c3o")
	nfd := filepath.Join(root, "A\u0303o")
	e1 := os.Mkdir(nfc, 0o755)
	e2 := os.Mkdir(nfd, 0o755)
	entries, _ := os.ReadDir(root)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	t.Logf("mkdir NFC err=%v, mkdir NFD err=%v, dir entries=%q, sameID(NFC,NFD)=%v", e1, e2, names, PathID(nfc) == PathID(nfd))
}

// Caminho de 300+ caracteres sob t.TempDir(): Go abre sozinho?
func TestLongPath(t *testing.T) {
	root := t.TempDir()
	p := root
	for i := 0; len(p) < 320; i++ {
		p = filepath.Join(p, "pasta longa com acentuação "+strings.Repeat("x", 10))
	}
	t.Logf("len(path)=%d", len(p))
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	file := filepath.Join(p, "HQ 01.cbz")
	f, err := os.Create(file)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	zw := zip.NewWriter(f)
	w, _ := zw.Create("001.png")
	w.Write([]byte("x"))
	zw.Close()
	f.Close()
	st, err := os.Stat(file)
	t.Logf("len(file)=%d Stat err=%v size=%d", len(file), err, st.Size())
	zr, err := zip.OpenReader(file)
	if err == nil {
		t.Logf("zip.OpenReader ok, entries=%d", len(zr.File))
		zr.Close()
	} else {
		t.Errorf("zip.OpenReader: %v", err)
	}
	var found []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			found = append(found, path)
		}
		return nil
	})
	t.Logf("WalkDir err=%v files=%d (len of found=%d)", err, len(found), func() int {
		if len(found) > 0 {
			return len(found[0])
		}
		return 0
	}())
	if len(found) != 1 || found[0] != file {
		t.Errorf("WalkDir did not find the file")
	}
	n := NormalizePath(file)
	t.Logf("NormalizePath(long) len=%d stable=%v id=%s", len(n), NormalizePath(n) == n, PathID(file))
	t.Logf("long path with \\\\?\\ prefix same id? %v", PathID(`\\?\`+file) == PathID(file))
	if err := os.RemoveAll(root); err != nil {
		t.Errorf("RemoveAll: %v", err)
	}
}
