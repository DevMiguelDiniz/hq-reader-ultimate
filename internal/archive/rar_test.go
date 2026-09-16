package archive

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/nwaples/rardecode/v2"
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

// TestClassifyRarError verifica a classificação de erros do rardecode/v2.
//
// Nota da investigação (Task 5): rardecode.ErrBadPassword existe com esse
// nome exato (a suposição do brief estava certa quanto ao nome), mas ele só
// é retornado quando uma senha FOI fornecida via Option Password(...) e não
// confere (archive50.go:229-231). Este pacote nunca usa essa Option, então
// o sinal real de "protegido por senha" nos nossos caminhos de código é
// rardecode.ErrArchiveEncrypted (cabeçalho criptografado, retornado por
// OpenReader/Next) ou rardecode.ErrArchivedFileEncrypted (corpo do arquivo
// criptografado, retornado só ao ler os dados). Testamos os dois abaixo,
// além de manter ErrBadPassword coberto por completude.
func TestClassifyRarError(t *testing.T) {
	if got := classifyRarError(nil); got != nil {
		t.Errorf("classifyRarError(nil) = %v, want nil", got)
	}
	if got := classifyRarError(rardecode.ErrArchiveEncrypted); !errors.Is(got, ErrPasswordProtected) {
		t.Errorf("classifyRarError(ErrArchiveEncrypted) = %v, want ErrPasswordProtected", got)
	}
	if got := classifyRarError(rardecode.ErrArchivedFileEncrypted); !errors.Is(got, ErrPasswordProtected) {
		t.Errorf("classifyRarError(ErrArchivedFileEncrypted) = %v, want ErrPasswordProtected", got)
	}
	if got := classifyRarError(rardecode.ErrBadPassword); !errors.Is(got, ErrPasswordProtected) {
		t.Errorf("classifyRarError(ErrBadPassword) = %v, want ErrPasswordProtected", got)
	}
	if got := classifyRarError(io.ErrUnexpectedEOF); !errors.Is(got, ErrCorrupted) {
		t.Errorf("classifyRarError(generic) = %v, want ErrCorrupted", got)
	}
}

func TestRarArchiveCorrupted(t *testing.T) {
	realPath := realCBRPath(t)

	data, err := os.ReadFile(realPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) < 200 {
		t.Skip("amostra real pequena demais para truncar com segurança")
	}

	dir := t.TempDir()
	truncatedPath := filepath.Join(dir, "truncated.cbr")
	if err := os.WriteFile(truncatedPath, data[:100], 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = newRarArchive(truncatedPath)
	if !errors.Is(err, ErrCorrupted) {
		t.Errorf("err = %v, want ErrCorrupted", err)
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
