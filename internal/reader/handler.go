package reader

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// PageHandler retorna um http.Handler que serve GET /page/{bookID}/{n},
// lendo os bytes crus da página n do livro bookID através de svc. Registrar
// esta rota ANTES do fallback de assets estáticos no AssetServer do Wails
// (ver §3.1 do spec e spikes/01-http-route-finding.md, validado na Etapa 0).
func PageHandler(svc *Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bookID, index, ok := parsePagePath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}

		info, err := svc.PageInfo(bookID, index)
		if err != nil {
			if errors.Is(err, ErrBookNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "erro ao ler pagina", http.StatusInternalServerError)
			return
		}

		rc, err := svc.PageReader(bookID, index)
		if err != nil {
			http.Error(w, "erro ao ler pagina", http.StatusInternalServerError)
			return
		}
		defer rc.Close()

		w.Header().Set("Content-Type", info.ContentType)
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
		w.Header().Set("Cache-Control", "immutable")
		io.Copy(w, rc)
	})
}

// parsePagePath extrai bookID e índice de um caminho "/page/{bookID}/{n}".
func parsePagePath(path string) (bookID string, index int, ok bool) {
	trimmed := strings.TrimPrefix(path, "/page/")
	if trimmed == path {
		return "", 0, false
	}
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "", 0, false
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil || n < 0 {
		return "", 0, false
	}
	return parts[0], n, true
}
