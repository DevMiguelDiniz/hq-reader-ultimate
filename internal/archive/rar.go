package archive

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
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
		return "", fmt.Errorf("archive: nome de entrada inseguro: %q: %w", name, ErrCorrupted)
	}
	return clean, nil
}

func newRarArchive(path string) (Archive, error) {
	root := tempDirRoot()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("archive: nao foi possivel criar diretorio temporario: %w", ErrCorrupted)
	}
	tempDir, err := os.MkdirTemp(root, "*")
	if err != nil {
		return nil, fmt.Errorf("archive: nao foi possivel criar diretorio temporario: %w", ErrCorrupted)
	}

	r, err := rardecode.OpenReader(path)
	if err != nil {
		os.RemoveAll(tempDir)
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
			return nil, fmt.Errorf("archive: nao foi possivel abrir %s: %w", path, err)
		}
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

		destPath := filepath.Join(tempDir, destName)
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			os.RemoveAll(tempDir)
			return nil, fmt.Errorf("archive: nao foi possivel criar diretorio para %q: %w", destName, ErrCorrupted)
		}
		out, err := os.Create(destPath)
		if err != nil {
			os.RemoveAll(tempDir)
			return nil, fmt.Errorf("archive: nao foi possivel criar arquivo temporario para %q: %w", destName, ErrCorrupted)
		}
		written, err := io.Copy(out, r)
		if err != nil {
			out.Close()
			os.RemoveAll(tempDir)
			return nil, classifyRarError(err)
		}
		out.Close()

		entries = append(entries, rawEntry{Name: destName, Size: written})

		if strings.EqualFold(hdr.Name, "ComicInfo.xml") {
			comicInfoPath = destPath
		}
	}

	return &rarArchive{
		tempDir:       tempDir,
		pages:         buildPageList(entries),
		comicInfoPath: comicInfoPath,
	}, nil
}

func (r *rarArchive) Pages() []PageEntry {
	pagesCopy := make([]PageEntry, len(r.pages))
	copy(pagesCopy, r.pages)
	return pagesCopy
}

func (r *rarArchive) Open(index int) (io.ReadCloser, error) {
	if index < 0 || index >= len(r.pages) {
		return nil, ErrCorrupted
	}
	name, err := sanitizeEntryName(r.pages[index].Name)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(r.tempDir, name))
	if err != nil {
		return nil, fmt.Errorf("archive: pagina ausente no diretorio temporario: %w", ErrCorrupted)
	}
	return f, nil
}

func (r *rarArchive) Metadata() (*ComicInfo, error) {
	if r.comicInfoPath == "" {
		return nil, nil
	}
	f, err := os.Open(r.comicInfoPath)
	if err != nil {
		return nil, fmt.Errorf("archive: nao foi possivel abrir ComicInfo.xml: %w", ErrCorrupted)
	}
	defer f.Close()
	info, err := parseComicInfo(f)
	if err != nil {
		return nil, fmt.Errorf("archive: ComicInfo.xml malformado: %w", ErrCorrupted)
	}
	return info, nil
}

func (r *rarArchive) Close() error {
	return os.RemoveAll(r.tempDir)
}

// classifyRarError traduz os erros de github.com/nwaples/rardecode/v2 (pinned
// v2.4.1) para o vocabulário de erro deste pacote.
//
// Investigação (Task 5): o pacote nunca é aberto com a Option Password(...)
// neste código, ou seja `a.pass` fica sempre nil dentro do rardecode. Isso
// significa que, na prática, um RAR protegido por senha NUNCA produz
// rardecode.ErrBadPassword (esse erro só é retornado por getKeys() em
// archive50.go quando uma senha FOI fornecida e não bate com o hash de
// verificação — ver archive50.go:229-231, chamado a partir de
// parseEncryptionBlock, que já retorna cedo com ErrArchiveEncrypted quando
// a.pass == nil, antes de chegar em getKeys). O sinal real que aparece no
// nosso fluxo (sem senha) é:
//   - rardecode.ErrArchiveEncrypted: cabeçalho do archive está criptografado
//     (nomes de arquivo inclusive); retornado por OpenReader/Next (RAR4 em
//     archive15.go:352, RAR5 em archive50.go:501,543).
//   - rardecode.ErrArchivedFileEncrypted: só o conteúdo do arquivo está
//     criptografado (cabeçalho legível); o erro só aparece ao tentar Read()
//     o corpo do arquivo (reader.go:378, via errorFile), não em Next().
//
// rardecode.ErrBadPassword é mantido na checagem abaixo por segurança/
// futuro-proofing (caso este pacote um dia passe a usar a Option
// Password(...)), mas hoje é inalcançável pelos caminhos de newRarArchive.
func classifyRarError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, rardecode.ErrArchiveEncrypted) ||
		errors.Is(err, rardecode.ErrArchivedFileEncrypted) ||
		errors.Is(err, rardecode.ErrBadPassword) {
		return ErrPasswordProtected
	}
	return ErrCorrupted
}
