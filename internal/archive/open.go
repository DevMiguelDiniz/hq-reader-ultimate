package archive

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var (
	zipMagic      = []byte("PK\x03\x04")
	zipEmptyMagic = []byte("PK\x05\x06") // zip vazio: só o end-of-central-directory
	rarMagic      = []byte("Rar!\x1a\x07")
)

// extensionFormat devolve o formato sugerido só pela extensão ("cbz",
// "cbr") ou "" se a extensão não é reconhecida.
func extensionFormat(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cbz", ".zip":
		return "cbz"
	case ".cbr", ".rar":
		return "cbr"
	}
	return ""
}

// detectFormat decide se o arquivo é "cbz" (ZIP) ou "cbr" (RAR) olhando os
// primeiros bytes; a extensão só desempata quando o conteúdo não decide
// (arquivo curto, ilegível ou com assinatura desconhecida — aí o backend
// escolhido pela extensão reporta o erro certo). Existem .cbr que na
// verdade são ZIP renomeados, por isso o conteúdo manda.
//
// Erros: arquivo inexistente/sem permissão devolve o erro de os.Open
// (errors.Is com fs.ErrNotExist/fs.ErrPermission), desde que a extensão
// seja reconhecida; com extensão desconhecida o resultado é
// ErrUnsupportedFormat, mesmo para caminho inexistente. Nem conteúdo nem
// extensão reconhecidos → ErrUnsupportedFormat.
func detectFormat(path string) (string, error) {
	ext := extensionFormat(path)

	f, err := os.Open(path)
	if err != nil {
		if ext == "" {
			return "", ErrUnsupportedFormat
		}
		return "", fmt.Errorf("archive: nao foi possivel abrir %s: %w", path, err)
	}
	defer f.Close()

	var head [8]byte
	n, _ := io.ReadFull(f, head[:])
	h := head[:n]
	switch {
	case bytes.HasPrefix(h, zipMagic), bytes.HasPrefix(h, zipEmptyMagic):
		return "cbz", nil
	case bytes.HasPrefix(h, rarMagic):
		return "cbr", nil
	}
	if ext == "" {
		return "", ErrUnsupportedFormat
	}
	return ext, nil
}

// Open detecta o formato pelo conteúdo do arquivo (extensão só como
// desempate, ver detectFormat) e retorna a implementação de Archive
// correspondente. É a única forma pela qual o resto da aplicação deve
// abrir um livro — nunca instanciar zipArchive/rarArchive diretamente.
func Open(path string) (Archive, error) {
	format, err := detectFormat(path)
	if err != nil {
		return nil, err
	}
	if format == "cbr" {
		return newRarArchive(path)
	}
	return newZipArchive(path)
}
