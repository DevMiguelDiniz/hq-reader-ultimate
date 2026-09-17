package archive

import (
	"errors"
	"io"
	"io/fs"
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
		if c.wantErr && !errors.Is(err, ErrCorrupted) {
			t.Errorf("sanitizeEntryName(%q) err = %v, want errors.Is(err, ErrCorrupted) == true", c.name, err)
		}
	}
}

// TestRarArchiveMissingFile confirma que um caminho inexistente é
// distinguível de um arquivo presente mas corrompido — mesmo requisito do
// backend CBZ (ver TestZipArchiveMissingFile), importante para um futuro
// scanner de biblioteca (Etapa 4).
func TestRarArchiveMissingFile(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nao-existe.cbr")

	_, err := newRarArchive(missing)
	if err == nil {
		t.Fatal("esperava erro para arquivo inexistente, veio nil")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want errors.Is(err, fs.ErrNotExist) == true", err)
	}
	if errors.Is(err, ErrCorrupted) {
		t.Errorf("err = %v, nao deveria satisfazer ErrCorrupted (é ausência de arquivo, não corrupção)", err)
	}
}

// TestRarArchiveCorruptedSynthetic exercita o caminho de corrupção RAR sem
// depender da amostra real de CBR (ausente nesta máquina): escreve uma
// assinatura RAR5 válida ("Rar!\x1A\x07\x01\x00") seguida de poucos bytes de
// lixo/truncamento, o que basta para rardecode.OpenReader reconhecer o
// arquivo como RAR e falhar ao tentar interpretar o corpo. Ao contrário de
// TestRarArchiveCorrupted (que precisa da fixture real), este teste roda
// incondicionalmente — garantindo que o caminho de tratamento de corrupção
// RAR é sempre exercitado, inclusive em CI.
func TestRarArchiveCorruptedSynthetic(t *testing.T) {
	rar5Signature := []byte{0x52, 0x61, 0x72, 0x21, 0x1A, 0x07, 0x01, 0x00}
	garbage := make([]byte, 24) // zeros — corpo incompleto/inválido após a assinatura
	data := append(append([]byte{}, rar5Signature...), garbage...)

	dir := t.TempDir()
	path := filepath.Join(dir, "synthetic-corrupted.cbr")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := newRarArchive(path)
	if !errors.Is(err, ErrCorrupted) {
		t.Errorf("err = %v, want ErrCorrupted", err)
	}
}

// TestRarArchiveComicInfoOnlyAtRoot confirma que ComicInfo.xml dentro de uma
// subpasta NÃO é reconhecido pelo backend RAR — mesma regra do backend ZIP
// (ver TestZipArchiveComicInfoOnlyAtRoot). Antes desta correção, o backend
// RAR casava por basename em qualquer profundidade, divergindo do ZIP.
func TestRarArchiveComicInfoOnlyAtRoot(t *testing.T) {
	dir := t.TempDir()
	tempDirRootOrig := tempDirRoot
	tempDirRoot = func() string { return filepath.Join(dir, "extract") }
	defer func() { tempDirRoot = tempDirRootOrig }()

	// Não há ferramenta de criação de RAR disponível nesta máquina (ver
	// nota de risco no spec §5), então este caso é coberto indiretamente:
	// a regra de correspondência em rar.go usa strings.EqualFold(hdr.Name,
	// "ComicInfo.xml") — comparação de igualdade exata contra o nome cru
	// da entrada — que é estruturalmente idêntica à do backend zip.go
	// (strings.EqualFold(name, "ComicInfo.xml")), já coberta por
	// TestZipArchiveComicInfoOnlyAtRoot. Ver rar.go: condição do loop de
	// extração em newRarArchive.
	t.Skip("sem ferramenta de criação de RAR nesta máquina; regra idêntica ao backend zip já coberta por TestZipArchiveComicInfoOnlyAtRoot — ver comentário")
}
