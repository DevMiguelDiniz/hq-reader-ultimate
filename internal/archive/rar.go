package archive

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nwaples/rardecode/v2"
)

type rarArchive struct {
	tempDir       string
	pages         []PageEntry
	comicInfoPath string
}

// sanitizeEntryName limpa o nome de uma entrada do archive e rejeita
// qualquer tentativa de escapar do diretório de extração (zip slip) — um
// CBR malicioso com uma entrada tipo "../../algo" nunca deve conseguir
// escrever fora de tempDir. Ver nota de risco no spec §3.3.
func sanitizeEntryName(name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
		return "", fmt.Errorf("archive: nome de entrada inseguro: %q", name)
	}
	return clean, nil
}

func newRarArchive(path string) (Archive, error) {
	root := tempDirRoot()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	tempDir, err := os.MkdirTemp(root, "*")
	if err != nil {
		return nil, err
	}

	r, err := rardecode.OpenReader(path)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, classifyRarError(err)
	}
	defer r.Close()

	var entries []rawEntry
	var comicInfoPath string

	for {
		hdr, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			os.RemoveAll(tempDir)
			return nil, classifyRarError(err)
		}
		if hdr.IsDir {
			continue
		}

		destName, err := sanitizeEntryName(hdr.Name)
		if err != nil {
			os.RemoveAll(tempDir)
			return nil, err
		}

		entries = append(entries, rawEntry{Name: hdr.Name, Size: hdr.UnPackedSize})

		destPath := filepath.Join(tempDir, destName)
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			os.RemoveAll(tempDir)
			return nil, err
		}
		out, err := os.Create(destPath)
		if err != nil {
			os.RemoveAll(tempDir)
			return nil, err
		}
		if _, err := io.Copy(out, r); err != nil {
			out.Close()
			os.RemoveAll(tempDir)
			return nil, classifyRarError(err)
		}
		out.Close()

		if strings.EqualFold(filepath.Base(hdr.Name), "ComicInfo.xml") {
			comicInfoPath = destPath
		}
	}

	return &rarArchive{
		tempDir:       tempDir,
		pages:         buildPageList(entries),
		comicInfoPath: comicInfoPath,
	}, nil
}

func (r *rarArchive) Pages() []PageEntry { return r.pages }

func (r *rarArchive) Open(index int) (io.ReadCloser, error) {
	if index < 0 || index >= len(r.pages) {
		return nil, ErrCorrupted
	}
	name, err := sanitizeEntryName(r.pages[index].Name)
	if err != nil {
		return nil, err
	}
	return os.Open(filepath.Join(r.tempDir, name))
}

func (r *rarArchive) Metadata() (*ComicInfo, error) {
	if r.comicInfoPath == "" {
		return nil, nil
	}
	f, err := os.Open(r.comicInfoPath)
	if err != nil {
		return nil, ErrCorrupted
	}
	defer f.Close()
	return parseComicInfo(f)
}

func (r *rarArchive) Close() error {
	return os.RemoveAll(r.tempDir)
}

// classifyRarError é um placeholder mínimo nesta task — toda a
// classificação real (distinguir senha de corrompido) é implementada na
// Task 5. Aqui só existe pra rar.go compilar e pros testes desta task
// (que não testam classificação de erro) passarem.
func classifyRarError(err error) error {
	if err == nil {
		return nil
	}
	return ErrCorrupted
}

// tempDirRoot é um placeholder temporário desta task — a Task 6 move isso
// para tempdirs.go e não duplica.
func tempDirRoot() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "hqreader", "tmp-extract")
}
