package archive

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// realCBRPath retorna o caminho de uma amostra real de CBR deixada pelo
// spike da Etapa 0 (não committada ao git). Se não existir nesta máquina,
// os testes que dependem dela são pulados — gap de fixture conhecido, ver
// docs/superpowers/specs/2026-09-16-hq-reader-design.md §5.
func realCBRPath(t *testing.T) string {
	t.Helper()
	candidates := []string{
		filepath.Join("..", "..", "spikes", "rar-cost", "testdata", "sample-solid.cbr"),
	}
	if env := os.Getenv("HQREADER_TEST_CBR"); env != "" {
		candidates = append([]string{env}, candidates...)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Skip("amostra real de CBR não encontrada — copie um .cbr real para spikes/rar-cost/testdata/sample-solid.cbr (Etapa 0) ou defina HQREADER_TEST_CBR")
	return ""
}

func TestRarArchiveRealSample(t *testing.T) {
	path := realCBRPath(t)

	a, err := newRarArchive(path)
	if err != nil {
		t.Fatalf("newRarArchive: %v", err)
	}
	defer a.Close()

	pages := a.Pages()
	if len(pages) == 0 {
		t.Fatal("nenhuma página encontrada na amostra real")
	}
	for i := 1; i < len(pages); i++ {
		if !naturalLess(pages[i-1].Name, pages[i].Name) {
			t.Errorf("ordem não-natural entre %q e %q", pages[i-1].Name, pages[i].Name)
		}
	}

	rc, err := a.Open(0)
	if err != nil {
		t.Fatalf("Open(0): %v", err)
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if int64(len(data)) != pages[0].Size {
		t.Errorf("tamanho lido = %d, PageEntry.Size = %d", len(data), pages[0].Size)
	}
}

func TestRarArchiveCloseRemovesTempDir(t *testing.T) {
	path := realCBRPath(t)

	a, err := newRarArchive(path)
	if err != nil {
		t.Fatalf("newRarArchive: %v", err)
	}
	ra := a.(*rarArchive)
	tempDir := ra.tempDir

	if _, err := os.Stat(tempDir); err != nil {
		t.Fatalf("tempDir deveria existir logo após newRarArchive: %v", err)
	}

	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Errorf("tempDir ainda existe depois do Close: %v", err)
	}
}

func TestSanitizeEntryNameRejectsTraversal(t *testing.T) {
	cases := []struct {
		name    string
		wantErr bool
	}{
		{"page1.jpg", false},
		{"chapter1/page1.jpg", false},
		{"../escape.jpg", true},
		{"chapter1/../../escape.jpg", true},
	}
	for _, c := range cases {
		_, err := sanitizeEntryName(c.name)
		if (err != nil) != c.wantErr {
			t.Errorf("sanitizeEntryName(%q) err = %v, wantErr %v", c.name, err, c.wantErr)
		}
	}
}
