package archive

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// maxComicInfoSize limita quanto de ComicInfo.xml Inspect lê para memória.
const maxComicInfoSize = 1 << 20 // 1 MiB

// Info é o resultado leve de Inspect: o suficiente para o scanner da
// biblioteca catalogar um arquivo sem abri-lo por completo.
type Info struct {
	// Format é o formato REAL pelo conteúdo ("cbz" ou "cbr"), não o da
	// extensão (há .cbr que são ZIP renomeado).
	Format    string
	PageCount int
	// ComicInfo é nil se não há ComicInfo.xml ou se ele está malformado
	// (o título cai depois para o nome do arquivo).
	ComicInfo *ComicInfo
}

// Inspect lê só o necessário para contar páginas e obter ComicInfo.xml.
// ZIP: usa o diretório central (barato). RAR: percorre os cabeçalhos sem
// extrair nada para disco (nenhuma pasta temporária) e só lê para memória
// o corpo de ComicInfo.xml.
//
// Limitação: em RAR sólido (hdr.Solid) o rardecode precisa decodificar os
// corpos anteriores para avançar até a próxima entrada, então o percurso
// deixa de ser só de cabeçalhos e custa quase tanto quanto extrair (só sem
// escrita em disco). O resultado continua correto, apenas não é barato.
func Inspect(path string) (Info, error) {
	format, err := detectFormat(path)
	if err != nil {
		return Info{}, err
	}
	if format == "cbr" {
		return inspectRar(path)
	}
	return inspectZip(path)
}

func inspectZip(path string) (Info, error) {
	a, err := newZipArchive(path)
	if err != nil {
		return Info{}, err
	}
	defer a.Close()

	info := Info{Format: "cbz", PageCount: len(a.Pages())}
	// ComicInfo ilegível/malformado não derruba a inspeção.
	if ci, err := a.Metadata(); err == nil {
		info.ComicInfo = ci
	}
	return info, nil
}

func inspectRar(path string) (Info, error) {
	r, err := openRarReader(path)
	if err != nil {
		return Info{}, err
	}
	defer r.Close()

	var entries []rawEntry
	var comicInfo *ComicInfo
	for {
		hdr, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Info{}, classifyRarError(err)
		}
		if hdr.IsDir {
			continue
		}
		name, err := sanitizeEntryName(hdr.Name)
		if err != nil {
			return Info{}, err
		}
		if hdr.Encrypted {
			// Mesmo desfecho de Open: o corpo não pode ser lido sem senha.
			return Info{}, fmt.Errorf("archive: entrada %q criptografada: %w", name, ErrPasswordProtected)
		}
		entries = append(entries, rawEntry{Name: name, Size: hdr.UnPackedSize})

		if strings.EqualFold(hdr.Name, "ComicInfo.xml") {
			data, err := io.ReadAll(io.LimitReader(r, maxComicInfoSize))
			if err != nil {
				return Info{}, classifyRarError(err)
			}
			// Malformado → nil, sem erro.
			if ci, err := parseComicInfo(bytes.NewReader(data)); err == nil {
				comicInfo = ci
			}
		}
	}
	return Info{Format: "cbr", PageCount: len(buildPageList(entries)), ComicInfo: comicInfo}, nil
}
