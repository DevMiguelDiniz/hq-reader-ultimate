package archive

import (
	"os"
	"path/filepath"
)

// tempDirRoot é onde rarArchive extrai arquivos temporariamente. É uma
// variável (não uma const) para o teste poder apontar para um diretório
// isolado sem tocar no cache real do usuário.
var tempDirRoot = func() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "hqreader", "tmp-extract")
}

// CleanOrphanedTempDirs apaga qualquer diretório deixado por uma sessão
// anterior que terminou sem chamar Close() (ex.: o app crashou). Deve ser
// chamada uma vez no startup do app, antes de abrir qualquer livro — a
// chamada em si fica para a Etapa 2 (main do app ainda não existe).
func CleanOrphanedTempDirs() error {
	root := tempDirRoot()
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(root, e.Name())); err != nil {
			return err
		}
	}
	return nil
}
