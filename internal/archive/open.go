package archive

import "strings"

// Open detecta o formato pela extensão do arquivo e retorna a
// implementação de Archive correspondente. É a única forma pela qual o
// resto da aplicação deve abrir um livro — nunca instanciar zipArchive/
// rarArchive diretamente.
func Open(path string) (Archive, error) {
	switch strings.ToLower(extOf(path)) {
	case ".cbz", ".zip":
		return newZipArchive(path)
	case ".cbr", ".rar":
		return newRarArchive(path)
	default:
		return nil, ErrUnsupportedFormat
	}
}

func extOf(path string) string {
	i := strings.LastIndex(path, ".")
	if i < 0 {
		return ""
	}
	return path[i:]
}
