package archive

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var rar5Signature = []byte{0x52, 0x61, 0x72, 0x21, 0x1A, 0x07, 0x01, 0x00}

// copyAs copia o arquivo src para um novo diretório temporário com o nome
// dado, para testar o mesmo conteúdo sob extensões diferentes.
func copyAs(t *testing.T, src, name string) string {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return writeBytes(t, name, data)
}

func writeBytes(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func syntheticRar(t *testing.T, name string) string {
	t.Helper()
	data := append(append([]byte{}, rar5Signature...), make([]byte, 24)...)
	return writeBytes(t, name, data)
}

func TestInspectZip(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{
		"page1.jpg":     tinyJPEG,
		"page2.jpg":     tinyJPEG,
		"page10.jpg":    tinyJPEG,
		"Thumbs.db":     []byte("lixo"),
		"ComicInfo.xml": []byte(`<ComicInfo><Title>Edição Especial</Title><Series>S</Series><Number>7</Number></ComicInfo>`),
	})
	info, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if info.PageCount != 3 {
		t.Errorf("PageCount = %d, want 3", info.PageCount)
	}
	if info.Format != "cbz" {
		t.Errorf("Format = %q, want cbz", info.Format)
	}
	if info.ComicInfo == nil {
		t.Fatal("ComicInfo = nil, want preenchido")
	}
	if info.ComicInfo.Title != "Edição Especial" || info.ComicInfo.Series != "S" || info.ComicInfo.Number != "7" {
		t.Errorf("ComicInfo = %+v", *info.ComicInfo)
	}
}

func TestInspectZipWithoutComicInfo(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG, "page2.png": tinyJPEG})
	info, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if info.ComicInfo != nil {
		t.Errorf("ComicInfo = %+v, want nil", info.ComicInfo)
	}
	if info.PageCount != 2 {
		t.Errorf("PageCount = %d, want 2", info.PageCount)
	}
}

func TestInspectZipMalformedComicInfo(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{
		"page1.jpg":     tinyJPEG,
		"page2.jpg":     tinyJPEG,
		"ComicInfo.xml": []byte(`<ComicInfo><Title>quebrado`),
	})
	info, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v (ComicInfo malformado não deve falhar)", err)
	}
	if info.ComicInfo != nil {
		t.Errorf("ComicInfo = %+v, want nil", info.ComicInfo)
	}
	if info.PageCount != 2 {
		t.Errorf("PageCount = %d, want 2", info.PageCount)
	}
}

func TestInspectZipCorrupted(t *testing.T) {
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i*7 + 3)
	}
	path := writeBytes(t, "lixo.cbz", data)
	_, err := Inspect(path)
	if !errors.Is(err, ErrCorrupted) {
		t.Errorf("err = %v, want ErrCorrupted", err)
	}
}

func TestInspectUnsupportedExtension(t *testing.T) {
	path := writeBytes(t, "doc.pdf", []byte("%PDF-1.7 isto nao e zip nem rar"))
	_, err := Inspect(path)
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("err = %v, want ErrUnsupportedFormat", err)
	}
}

func TestInspectCbrThatIsZip(t *testing.T) {
	src := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG, "page2.jpg": tinyJPEG})
	path := copyAs(t, src, "disfarcado.cbr")
	info, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if info.Format != "cbz" {
		t.Errorf("Format = %q, want cbz (formato real pelo conteúdo)", info.Format)
	}
	if info.PageCount != 2 {
		t.Errorf("PageCount = %d, want 2", info.PageCount)
	}
}

func TestInspectCbzThatIsRar(t *testing.T) {
	path := syntheticRar(t, "disfarcado.cbz")
	_, err := Inspect(path)
	if !errors.Is(err, ErrCorrupted) {
		t.Errorf("err = %v, want ErrCorrupted (tratado como RAR, não como zip)", err)
	}
}

func TestDetectFormat(t *testing.T) {
	zipSrc := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG})
	tests := []struct {
		name    string
		path    string
		want    string
		wantErr error
	}{
		{"zip com .cbz", copyAs(t, zipSrc, "a.cbz"), "cbz", nil},
		{"zip com .cbr", copyAs(t, zipSrc, "a.cbr"), "cbz", nil},
		{"zip com .zip", copyAs(t, zipSrc, "a.zip"), "cbz", nil},
		{"zip vazio com .cbr", writeBytes(t, "vazio.cbr", []byte{'P', 'K', 0x05, 0x06, 0, 0, 0, 0}), "cbz", nil},
		{"rar5 com .cbr", syntheticRar(t, "a.cbr"), "cbr", nil},
		{"rar com .cbz", syntheticRar(t, "a.cbz"), "cbr", nil},
		{"3 bytes com .cbz cai na extensão", writeBytes(t, "curto.cbz", []byte{1, 2, 3}), "cbz", nil},
		{"3 bytes com .cbr cai na extensão", writeBytes(t, "curto.cbr", []byte{1, 2, 3}), "cbr", nil},
		{"vazio com .txt", writeBytes(t, "vazio.txt", nil), "", ErrUnsupportedFormat},
		{"conteúdo desconhecido com .pdf", writeBytes(t, "a.pdf", []byte("%PDF-1.7 conteudo")), "", ErrUnsupportedFormat},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := detectFormat(tc.path)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("detectFormat: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestInspectMissingFile(t *testing.T) {
	_, err := Inspect(filepath.Join(t.TempDir(), "nao-existe.cbz"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want errors.Is(err, fs.ErrNotExist)", err)
	}
	if errors.Is(err, ErrCorrupted) {
		t.Errorf("err = %v, ausência de arquivo não deve satisfazer ErrCorrupted", err)
	}
}

func TestInspectRarCorruptedSynthetic(t *testing.T) {
	_, err := Inspect(syntheticRar(t, "sintetico.cbr"))
	if !errors.Is(err, ErrCorrupted) {
		t.Errorf("err = %v, want ErrCorrupted", err)
	}
}

func TestInspectRarRealSample(t *testing.T) {
	path := realCBRPath(t)

	root := filepath.Join(t.TempDir(), "extract")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	orig := tempDirRoot
	tempDirRoot = func() string { return root }
	defer func() { tempDirRoot = orig }()

	info, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if info.Format != "cbr" {
		t.Skipf("amostra %s é %q pelo conteúdo, não RAR; este teste exige RAR", path, info.Format)
	}

	left, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("Inspect criou %d entradas no diretório temporário, want 0", len(left))
	}

	a, err := newRarArchive(path)
	if err != nil {
		t.Fatalf("newRarArchive: %v", err)
	}
	defer a.Close()
	if want := len(a.Pages()); info.PageCount != want {
		t.Errorf("PageCount = %d, want %d (Pages() da extração completa)", info.PageCount, want)
	}
	wantCI, err := a.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if !reflect.DeepEqual(info.ComicInfo, wantCI) {
		t.Errorf("ComicInfo = %+v, want %+v", info.ComicInfo, wantCI)
	}
	t.Logf("amostra real: PageCount=%d ComicInfo=%v", info.PageCount, info.ComicInfo != nil)
}
