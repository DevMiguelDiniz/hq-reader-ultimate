// Package pathnorm: SPIKE (Etapa 4, Task 1) da Decisao 9 do plano, escrita
// literalmente para ser testada no Windows. Descartavel.
package pathnorm

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"runtime"
	"strings"
)

// NormalizePath = filepath.Abs + filepath.Clean, tira separador final; no
// Windows troca \ por / e passa para minusculas.
func NormalizePath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}
	c := filepath.Clean(abs)
	c = strings.TrimRight(c, `\/`)
	if runtime.GOOS == "windows" {
		c = strings.ToLower(strings.ReplaceAll(c, `\`, "/"))
	}
	return c
}

// PathID = primeiros 32 hex de sha256(NormalizePath(p)).
func PathID(p string) string {
	sum := sha256.Sum256([]byte(NormalizePath(p)))
	return hex.EncodeToString(sum[:])[:32]
}
