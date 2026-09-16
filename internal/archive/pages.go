package archive

import (
	"sort"
	"strconv"
	"strings"
)

var imageExtensions = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
}

// contentTypeForName retorna o content-type pela extensão do arquivo
// (case-insensitive) e um bool indicando se é uma imagem suportada.
func contentTypeForName(name string) (string, bool) {
	i := strings.LastIndex(name, ".")
	if i < 0 {
		return "", false
	}
	ext := strings.ToLower(name[i:])
	ct, ok := imageExtensions[ext]
	return ct, ok
}

// rawEntry é a informação mínima que zipArchive/rarArchive extraem de cada
// entrada do arquivo antes da filtragem e ordenação.
type rawEntry struct {
	Name string
	Size int64
}

// buildPageList filtra só entradas de imagem suportada (por extensão) e
// ordena naturalmente pelo caminho completo — o que também resolve o caso
// de subpastas internas: o caminho relativo completo participa da
// ordenação, então "achatar" a árvore é uma consequência natural de ordenar
// pelo nome completo, sem lógica extra de subpasta.
func buildPageList(entries []rawEntry) []PageEntry {
	var images []rawEntry
	for _, e := range entries {
		if _, ok := contentTypeForName(e.Name); ok {
			images = append(images, e)
		}
	}
	sort.SliceStable(images, func(i, j int) bool {
		return naturalLess(images[i].Name, images[j].Name)
	})

	pages := make([]PageEntry, len(images))
	for i, e := range images {
		ct, _ := contentTypeForName(e.Name)
		pages[i] = PageEntry{Index: i, Name: e.Name, Size: e.Size, ContentType: ct}
	}
	return pages
}

// naturalLess compara dois nomes tratando sequências de dígitos como
// números (page2 vem antes de page10), preservando a comparação lexical
// pro resto da string (incluindo separadores de pasta).
func naturalLess(a, b string) bool {
	ia, ib := 0, 0
	for ia < len(a) && ib < len(b) {
		ca, cb := a[ia], b[ib]
		if isDigit(ca) && isDigit(cb) {
			na, ea := scanNumber(a, ia)
			nb, eb := scanNumber(b, ib)
			if na != nb {
				return na < nb
			}
			ia, ib = ea, eb
			continue
		}
		if ca != cb {
			return ca < cb
		}
		ia++
		ib++
	}
	return len(a)-ia < len(b)-ib
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func scanNumber(s string, start int) (int64, int) {
	end := start
	for end < len(s) && isDigit(s[end]) {
		end++
	}
	n, _ := strconv.ParseInt(s[start:end], 10, 64)
	return n, end
}
