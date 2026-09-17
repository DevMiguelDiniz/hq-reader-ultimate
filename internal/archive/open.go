package archive

import (
	"path/filepath"
	"strings"
)

// Open detecta o formato pela extensão do arquivo e retorna a
// implementação de Archive correspondente. É a única forma pela qual o
// resto da aplicação deve abrir um livro — nunca instanciar zipArchive/
// rarArchive diretamente.
func Open(path string) (Archive, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cbz", ".zip":
		return newZipArchive(path)
	case ".cbr", ".rar":
		return newRarArchive(path)
	default:
		return nil, ErrUnsupportedFormat
	}
}
