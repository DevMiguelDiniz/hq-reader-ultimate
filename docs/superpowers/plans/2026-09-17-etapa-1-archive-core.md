# Etapa 1 — Núcleo Archive — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Um pacote Go (`internal/archive`) que implementa a interface `Archive`
para CBZ e CBR, testado com fixtures reais, mais uma CLI de debug (`hqcli`).

**Architecture:** Uma interface única (`Pages`/`Open`/`Metadata`/`Close`)
escondendo duas implementações (`zipArchive` via `archive/zip` da stdlib,
`rarArchive` via `nwaples/rardecode/v2`). Filtragem de páginas por extensão e
ordenação natural são lógica pura e compartilhada entre as duas
implementações. `rarArchive` extrai tudo para um diretório temporário no
primeiro `Open()` (RAR não tem acesso aleatório); `zipArchive` lê direto do
arquivo. Um factory `archive.Open(path)` escolhe a implementação pela
extensão.

**Tech Stack:** Go 1.21+, `archive/zip` (stdlib), `github.com/nwaples/rardecode/v2 v2.4.1` (já validado na Etapa 0).

**Spec:** [docs/superpowers/specs/2026-09-16-hq-reader-design.md](../specs/2026-09-16-hq-reader-design.md) — §3.3 (interface Archive) e §5 (casos de borda de formato).

## Global Constraints

- Go 1.21+ (piso do projeto, já usado na Etapa 0).
- Módulo Go raiz: `github.com/DevMiguelDiniz/hq-reader-ultimate` (mesmo path do remote `origin`).
- `github.com/nwaples/rardecode/v2` fixado em `v2.4.1` (versão validada na Etapa 0) — não usar `@latest`.
- Nomes de erro sentinela exatos, usados em todo o pacote: `ErrPasswordProtected`, `ErrCorrupted`, `ErrUnsupportedFormat`.
- Layout de módulos por `internal/archive` e `cmd/hqcli`, conforme §3 do spec.
- Nenhuma amostra real de CBZ/CBR é commitada ao git. Fixtures de CBZ são geradas em memória/tempfile pelos próprios testes (`archive/zip` da stdlib cria arquivos válidos sem depender de ferramenta externa). Fixtures de CBR reaproveitam a amostra real já deixada (fora do git) pela Etapa 0 em `spikes/rar-cost/testdata/sample-solid.cbr` — testes que dependem dela pulam (`t.Skip`) graciosamente se o arquivo não existir nesta máquina.
- Fora de escopo desta etapa (fica para a Etapa 3, "Leitor de verdade"): detecção de página dupla por proporção — isso exige decodificar dimensões da imagem, o que é preocupação do leitor, não do `Archive` (cuja responsabilidade é só servir bytes crus e metadados).
- `PageEntry` ganha um campo `Name` além dos três do esboço original do spec (`Index`, `Size`, `ContentType`) — necessário para localizar a entrada correspondente dentro de `zipArchive`/`rarArchive` e para a listagem do `hqcli`. Aditivo, não remove nada do design original.

---

### Task 1: Interface `Archive`, tipos base, filtro e ordenação de páginas

**Files:**
- Create: `go.mod`
- Create: `internal/archive/archive.go`
- Create: `internal/archive/pages.go`
- Test: `internal/archive/pages_test.go`

**Interfaces:**
- Produces: `type PageEntry struct{ Index int; Name string; Size int64; ContentType string }`, `type ComicInfo struct{...}`, `type Archive interface{ Pages() []PageEntry; Open(index int) (io.ReadCloser, error); Metadata() (*ComicInfo, error); Close() error }`, sentinelas `ErrPasswordProtected`, `ErrCorrupted`, `ErrUnsupportedFormat`, `type rawEntry struct{ Name string; Size int64 }`, `func buildPageList(entries []rawEntry) []PageEntry`, `func naturalLess(a, b string) bool`, `func contentTypeForName(name string) (string, bool)`. Todas as tasks seguintes consomem esses tipos/funções.

- [ ] **Step 1: Inicializar o módulo Go**

```bash
go mod init github.com/DevMiguelDiniz/hq-reader-ultimate
```

- [ ] **Step 2: Escrever o teste de `naturalLess` e `contentTypeForName` (falhando)**

Criar `internal/archive/pages_test.go`:

```go
package archive

import "testing"

func TestNaturalLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"page2.jpg", "page10.jpg", true},
		{"page10.jpg", "page2.jpg", false},
		{"page1.jpg", "page1.jpg", false},
		{"a/page2.jpg", "a/page10.jpg", true},
		{"b/page1.jpg", "a/page1.jpg", false},
	}
	for _, c := range cases {
		if got := naturalLess(c.a, c.b); got != c.want {
			t.Errorf("naturalLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestContentTypeForName(t *testing.T) {
	cases := []struct {
		name    string
		wantCT  string
		wantOK  bool
	}{
		{"page1.jpg", "image/jpeg", true},
		{"page1.JPEG", "image/jpeg", true},
		{"page1.png", "image/png", true},
		{"page1.webp", "image/webp", true},
		{"ComicInfo.xml", "", false},
		{"Thumbs.db", "", false},
		{"noext", "", false},
	}
	for _, c := range cases {
		ct, ok := contentTypeForName(c.name)
		if ct != c.wantCT || ok != c.wantOK {
			t.Errorf("contentTypeForName(%q) = (%q, %v), want (%q, %v)", c.name, ct, ok, c.wantCT, c.wantOK)
		}
	}
}
```

- [ ] **Step 3: Rodar e confirmar que falha (funções ainda não existem)**

```bash
go test ./internal/archive/... -run 'TestNaturalLess|TestContentTypeForName' -v
```
Esperado: erro de compilação (`undefined: naturalLess`, `undefined: contentTypeForName`).

- [ ] **Step 4: Criar `internal/archive/archive.go` com os tipos base**

```go
package archive

import (
	"encoding/xml"
	"errors"
	"io"
)

var (
	ErrPasswordProtected = errors.New("archive: protegido por senha")
	ErrCorrupted         = errors.New("archive: arquivo corrompido ou truncado")
	ErrUnsupportedFormat = errors.New("archive: formato não suportado")
)

// PageEntry descreve uma página já filtrada (só imagens) e ordenada
// naturalmente dentro do archive.
type PageEntry struct {
	Index       int
	Name        string
	Size        int64
	ContentType string
}

// ComicInfo mapeia o subconjunto de ComicInfo.xml usado pela aplicação.
type ComicInfo struct {
	XMLName xml.Name `xml:"ComicInfo"`
	Title   string   `xml:"Title"`
	Series  string   `xml:"Series"`
	Number  string   `xml:"Number"`
	Summary string   `xml:"Summary"`
	Writer  string   `xml:"Writer"`
}

// Archive é a interface comum entre CBZ e CBR. Pages() já vem ordenado
// naturalmente e filtrado só para entradas de imagem suportada.
type Archive interface {
	Pages() []PageEntry
	Open(index int) (io.ReadCloser, error)
	Metadata() (*ComicInfo, error)
	Close() error
}
```

- [ ] **Step 5: Criar `internal/archive/pages.go` com a lógica de filtro/ordenação**

```go
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
```

- [ ] **Step 6: Rodar de novo e confirmar que passa**

```bash
go test ./internal/archive/... -run 'TestNaturalLess|TestContentTypeForName' -v
```
Esperado: PASS nos dois testes.

- [ ] **Step 7: Escrever e rodar o teste de `buildPageList` (subpasta + filtro)**

Adicionar a `pages_test.go`:

```go
func TestBuildPageList(t *testing.T) {
	entries := []rawEntry{
		{Name: "ComicInfo.xml", Size: 100},
		{Name: "chapter1/page10.jpg", Size: 200},
		{Name: "chapter1/page2.jpg", Size: 150},
		{Name: "Thumbs.db", Size: 50},
		{Name: "chapter1/page1.png", Size: 300},
	}
	pages := buildPageList(entries)
	if len(pages) != 3 {
		t.Fatalf("got %d pages, want 3 (ComicInfo.xml e Thumbs.db devem ser filtrados)", len(pages))
	}
	wantOrder := []string{"chapter1/page1.png", "chapter1/page2.jpg", "chapter1/page10.jpg"}
	for i, w := range wantOrder {
		if pages[i].Name != w {
			t.Errorf("pages[%d].Name = %q, want %q", i, pages[i].Name, w)
		}
		if pages[i].Index != i {
			t.Errorf("pages[%d].Index = %d, want %d", i, pages[i].Index, i)
		}
	}
	if pages[0].ContentType != "image/png" {
		t.Errorf("pages[0].ContentType = %q, want image/png", pages[0].ContentType)
	}
}
```

```bash
go test ./internal/archive/... -run TestBuildPageList -v
```
Esperado: PASS.

- [ ] **Step 8: Commit**

```bash
git add go.mod internal/archive/archive.go internal/archive/pages.go internal/archive/pages_test.go
git commit -m "archive: interface base, filtro e ordenacao natural de paginas"
```

---

### Task 2: Parsing de `ComicInfo.xml`

**Files:**
- Create: `internal/archive/comicinfo.go`
- Test: `internal/archive/comicinfo_test.go`

**Interfaces:**
- Consumes: `ComicInfo` (Task 1).
- Produces: `func parseComicInfo(r io.Reader) (*ComicInfo, error)` — usado por `zipArchive.Metadata()` e `rarArchive.Metadata()` nas Tasks 3 e 4.

- [ ] **Step 1: Escrever o teste (falhando)**

Criar `internal/archive/comicinfo_test.go`:

```go
package archive

import (
	"strings"
	"testing"
)

func TestParseComicInfo(t *testing.T) {
	xmlData := `<?xml version="1.0"?>
<ComicInfo>
  <Title>Os Fabulosos X-Men</Title>
  <Series>X-Men</Series>
  <Number>175</Number>
  <Writer>Chris Claremont</Writer>
</ComicInfo>`

	info, err := parseComicInfo(strings.NewReader(xmlData))
	if err != nil {
		t.Fatalf("parseComicInfo: %v", err)
	}
	if info.Title != "Os Fabulosos X-Men" {
		t.Errorf("Title = %q", info.Title)
	}
	if info.Series != "X-Men" {
		t.Errorf("Series = %q", info.Series)
	}
	if info.Number != "175" {
		t.Errorf("Number = %q", info.Number)
	}
	if info.Writer != "Chris Claremont" {
		t.Errorf("Writer = %q", info.Writer)
	}
}

func TestParseComicInfoInvalidXML(t *testing.T) {
	_, err := parseComicInfo(strings.NewReader("isso nao e xml"))
	if err == nil {
		t.Fatal("esperava erro para XML inválido, veio nil")
	}
}
```

- [ ] **Step 2: Rodar e confirmar que falha**

```bash
go test ./internal/archive/... -run TestParseComicInfo -v
```
Esperado: erro de compilação (`undefined: parseComicInfo`).

- [ ] **Step 3: Implementar**

Criar `internal/archive/comicinfo.go`:

```go
package archive

import (
	"encoding/xml"
	"io"
)

// parseComicInfo decodifica um ComicInfo.xml a partir de r.
func parseComicInfo(r io.Reader) (*ComicInfo, error) {
	var info ComicInfo
	if err := xml.NewDecoder(r).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}
```

- [ ] **Step 4: Rodar e confirmar que passa**

```bash
go test ./internal/archive/... -run TestParseComicInfo -v
```
Esperado: PASS nos dois testes.

- [ ] **Step 5: Commit**

```bash
git add internal/archive/comicinfo.go internal/archive/comicinfo_test.go
git commit -m "archive: parsing de ComicInfo.xml"
```

---

### Task 3: `zipArchive` — implementação CBZ

**Files:**
- Create: `internal/archive/zip.go`
- Test: `internal/archive/zip_test.go`

**Interfaces:**
- Consumes: `Archive`, `PageEntry`, `ComicInfo`, `rawEntry`, `buildPageList`, `parseComicInfo`, sentinelas (Tasks 1-2).
- Produces: `func newZipArchive(path string) (Archive, error)` — consumido pelo factory na Task 7. Helper de teste `func writeTestZip(t *testing.T, files map[string][]byte) string` e `var tinyJPEG []byte` — reaproveitados pela Task 7's `open_test.go`.

- [ ] **Step 1: Escrever o teste (falhando)**

Criar `internal/archive/zip_test.go`:

```go
package archive

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// tinyJPEG é o conteúdo mínimo usado nos testes — só os bytes importam,
// nada decodifica a imagem de verdade nesta camada.
var tinyJPEG = []byte{0xFF, 0xD8, 0xFF, 0xD9}

func writeTestZip(t *testing.T, files map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cbz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return path
}

func TestZipArchive(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{
		"chapter1/page2.jpg": tinyJPEG,
		"chapter1/page1.jpg": tinyJPEG,
		"Thumbs.db":          []byte("lixo"),
		"ComicInfo.xml":      []byte(`<ComicInfo><Title>Teste</Title></ComicInfo>`),
	})

	a, err := newZipArchive(path)
	if err != nil {
		t.Fatalf("newZipArchive: %v", err)
	}
	defer a.Close()

	pages := a.Pages()
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	if pages[0].Name != "chapter1/page1.jpg" || pages[1].Name != "chapter1/page2.jpg" {
		t.Errorf("ordem errada: %+v", pages)
	}

	rc, err := a.Open(0)
	if err != nil {
		t.Fatalf("Open(0): %v", err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(data, tinyJPEG) {
		t.Errorf("bytes da página 0 não batem com o esperado")
	}

	info, err := a.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if info == nil || info.Title != "Teste" {
		t.Errorf("Metadata = %+v", info)
	}
}

func TestZipArchiveCorrupted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupted.cbz")
	if err := os.WriteFile(path, []byte("isso nao e um zip"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := newZipArchive(path)
	if !errors.Is(err, ErrCorrupted) {
		t.Errorf("err = %v, want ErrCorrupted", err)
	}
}

func TestZipArchiveNoMetadata(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG})
	a, err := newZipArchive(path)
	if err != nil {
		t.Fatalf("newZipArchive: %v", err)
	}
	defer a.Close()

	info, err := a.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if info != nil {
		t.Errorf("Metadata = %+v, want nil (sem ComicInfo.xml)", info)
	}
}
```

- [ ] **Step 2: Rodar e confirmar que falha**

```bash
go test ./internal/archive/... -run TestZipArchive -v
```
Esperado: erro de compilação (`undefined: newZipArchive`).

- [ ] **Step 3: Implementar**

Criar `internal/archive/zip.go`:

```go
package archive

import (
	"archive/zip"
	"io"
	"strings"
)

type zipArchive struct {
	reader *zip.ReadCloser
	pages  []PageEntry
	byName map[string]*zip.File
}

func newZipArchive(path string) (Archive, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, ErrCorrupted
	}

	var entries []rawEntry
	byName := make(map[string]*zip.File)
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		entries = append(entries, rawEntry{Name: f.Name, Size: int64(f.UncompressedSize64)})
		byName[f.Name] = f
	}

	return &zipArchive{
		reader: r,
		pages:  buildPageList(entries),
		byName: byName,
	}, nil
}

func (z *zipArchive) Pages() []PageEntry { return z.pages }

func (z *zipArchive) Open(index int) (io.ReadCloser, error) {
	if index < 0 || index >= len(z.pages) {
		return nil, ErrCorrupted
	}
	f, ok := z.byName[z.pages[index].Name]
	if !ok {
		return nil, ErrCorrupted
	}
	rc, err := f.Open()
	if err != nil {
		return nil, ErrCorrupted
	}
	return rc, nil
}

func (z *zipArchive) Metadata() (*ComicInfo, error) {
	for name, f := range z.byName {
		if strings.EqualFold(name, "ComicInfo.xml") {
			rc, err := f.Open()
			if err != nil {
				return nil, ErrCorrupted
			}
			defer rc.Close()
			return parseComicInfo(rc)
		}
	}
	return nil, nil
}

func (z *zipArchive) Close() error {
	return z.reader.Close()
}
```

- [ ] **Step 4: Rodar e confirmar que passa**

```bash
go test ./internal/archive/... -run TestZipArchive -v
```
Esperado: PASS nos três testes (`TestZipArchive`, `TestZipArchiveCorrupted`, `TestZipArchiveNoMetadata`).

- [ ] **Step 5: Commit**

```bash
git add internal/archive/zip.go internal/archive/zip_test.go
git commit -m "archive: implementacao zipArchive (CBZ)"
```

---

### Task 4: `rarArchive` — implementação CBR (caminho feliz + ciclo de vida do temp dir)

**Files:**
- Create: `internal/archive/rar.go`
- Test: `internal/archive/rar_test.go`
- Modify: `go.mod`, `go.sum` (via `go get`)

**Interfaces:**
- Consumes: `Archive`, `PageEntry`, `ComicInfo`, `rawEntry`, `buildPageList`, `parseComicInfo`, `naturalLess`, sentinelas (Tasks 1-2).
- Produces: `func newRarArchive(path string) (Archive, error)`, `type rarArchive struct{ tempDir string; ... }` (campo `tempDir` acessado diretamente pelo teste via type assertion), `func sanitizeEntryName(name string) (string, error)`, `func realCBRPath(t *testing.T) string` — reaproveitado pela Task 5 e pela Task 7's `open_test.go`. `func classifyRarError(err error) error` — implementada de fato na Task 5, mas **referenciada** aqui (`newRarArchive` já chama `classifyRarError` nos pontos de erro); esta task escreve um `classifyRarError` mínimo (todo erro não-nil vira `ErrCorrupted`) só pra `rar.go` compilar — a Task 5 substitui o corpo pela versão que distingue senha de corrompido.

**IMPORTANTE — API do rardecode v2.4.1 já tem drift conhecido (visto na Etapa 0):** a assinatura de `OpenReader` mudou de uma versão pra outra, e nomes de campo de `FileHeader` não foram 100% confirmados na Etapa 0 (só `Solid bool` foi verificado contra o código-fonte). Antes de escrever `rar.go`, confirme os nomes reais.

- [ ] **Step 1: Adicionar a dependência pinada**

```bash
go get github.com/nwaples/rardecode/v2@v2.4.1
```

- [ ] **Step 2: Verificar a API real de `FileHeader` e `OpenReader`**

```bash
go doc github.com/nwaples/rardecode/v2 OpenReader
go doc github.com/nwaples/rardecode/v2 FileHeader
```

Confirme especificamente: o nome do campo com o caminho da entrada (assumido `Name string` abaixo — convenção idêntica a `tar.Header`/`zip.FileHeader`), o campo de tamanho descomprimido (assumido `UnPackedSize int64` abaixo), e um campo booleano de diretório (assumido `IsDir bool` abaixo). Se qualquer um desses nomes for diferente do assumido, ajuste o código do Step 4 para o nome real antes de prosseguir — não prossiga com um nome incerto sem checar.

- [ ] **Step 3: Escrever o teste (falhando)**

Criar `internal/archive/rar_test.go`:

```go
package archive

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// realCBRPath retorna o caminho de uma amostra real de CBR deixada pelo
// spike da Etapa 0 (não committada ao git). Se não existir nesta máquina,
// os testes que dependem dela são pulados — gap de fixture conhecido, ver
// docs/superpowers/specs/2026-09-16-hq-reader-design.md §5.
func realCBRPath(t *testing.T) string {
	t.Helper()
	candidates := []string{
		filepath.Join("..", "..", "spikes", "rar-cost", "testdata", "sample-solid.cbr"),
	}
	if env := os.Getenv("HQREADER_TEST_CBR"); env != "" {
		candidates = append([]string{env}, candidates...)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Skip("amostra real de CBR não encontrada — copie um .cbr real para spikes/rar-cost/testdata/sample-solid.cbr (Etapa 0) ou defina HQREADER_TEST_CBR")
	return ""
}

func TestRarArchiveRealSample(t *testing.T) {
	path := realCBRPath(t)

	a, err := newRarArchive(path)
	if err != nil {
		t.Fatalf("newRarArchive: %v", err)
	}
	defer a.Close()

	pages := a.Pages()
	if len(pages) == 0 {
		t.Fatal("nenhuma página encontrada na amostra real")
	}
	for i := 1; i < len(pages); i++ {
		if !naturalLess(pages[i-1].Name, pages[i].Name) {
			t.Errorf("ordem não-natural entre %q e %q", pages[i-1].Name, pages[i].Name)
		}
	}

	rc, err := a.Open(0)
	if err != nil {
		t.Fatalf("Open(0): %v", err)
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if int64(len(data)) != pages[0].Size {
		t.Errorf("tamanho lido = %d, PageEntry.Size = %d", len(data), pages[0].Size)
	}
}

func TestRarArchiveCloseRemovesTempDir(t *testing.T) {
	path := realCBRPath(t)

	a, err := newRarArchive(path)
	if err != nil {
		t.Fatalf("newRarArchive: %v", err)
	}
	ra := a.(*rarArchive)
	tempDir := ra.tempDir

	if _, err := os.Stat(tempDir); err != nil {
		t.Fatalf("tempDir deveria existir logo após newRarArchive: %v", err)
	}

	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Errorf("tempDir ainda existe depois do Close: %v", err)
	}
}

func TestSanitizeEntryNameRejectsTraversal(t *testing.T) {
	cases := []struct {
		name    string
		wantErr bool
	}{
		{"page1.jpg", false},
		{"chapter1/page1.jpg", false},
		{"../escape.jpg", true},
		{"chapter1/../../escape.jpg", true},
	}
	for _, c := range cases {
		_, err := sanitizeEntryName(c.name)
		if (err != nil) != c.wantErr {
			t.Errorf("sanitizeEntryName(%q) err = %v, wantErr %v", c.name, err, c.wantErr)
		}
	}
}
```

- [ ] **Step 4: Rodar e confirmar que falha**

```bash
go test ./internal/archive/... -run TestRarArchive -v
go test ./internal/archive/... -run TestSanitizeEntryName -v
```
Esperado: erro de compilação (`undefined: newRarArchive`, `undefined: sanitizeEntryName`).

- [ ] **Step 5: Implementar**

Criar `internal/archive/rar.go` (ajuste os nomes de campo do `FileHeader` para o que o Step 2 confirmou, se diferente do assumido aqui):

```go
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
```

Note: `tempDirRoot()` ainda não existe — é criado na Task 6. Para esta task
compilar e os testes desta task passarem, adicione temporariamente ao final
de `rar.go` (a Task 6 vai mover isso para `tempdirs.go` e não duplicar):

```go
func tempDirRoot() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "hqreader", "tmp-extract")
}
```

- [ ] **Step 6: Rodar e confirmar que passa**

```bash
go test ./internal/archive/... -run 'TestRarArchive|TestSanitizeEntryName' -v
```
Esperado: PASS em `TestSanitizeEntryNameRejectsTraversal` sempre. `TestRarArchiveRealSample` e `TestRarArchiveCloseRemovesTempDir` passam se `spikes/rar-cost/testdata/sample-solid.cbr` existir nesta máquina, ou aparecem como `SKIP` (não `FAIL`) se não existir — os dois resultados são aceitáveis, só uma falha real (`FAIL`) bloqueia o commit.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/archive/rar.go internal/archive/rar_test.go
git commit -m "archive: implementacao rarArchive (CBR) - caminho feliz e zip slip"
```

---

### Task 5: `rarArchive` — classificação de erros (corrompido / protegido por senha)

**Files:**
- Modify: `internal/archive/rar.go` (substituir o `classifyRarError` mínimo da Task 4)
- Test: `internal/archive/rar_test.go` (adicionar casos)

**Interfaces:**
- Consumes: `classifyRarError` (assinatura já existe da Task 4, só o corpo muda), `ErrPasswordProtected`, `ErrCorrupted` (Task 1), `realCBRPath` (Task 4).

**IMPORTANTE:** o nome exato do erro/sinal que `rardecode/v2` usa para "arquivo protegido por senha" não foi confirmado na Etapa 0 (só `Solid bool`/`ErrSolidOpen` foram). Investigue antes de codar.

- [ ] **Step 1: Investigar o sinal real de senha do rardecode**

```bash
go doc github.com/nwaples/rardecode/v2
```

Procure por qualquer símbolo exportado com "password" ou "encrypt" no nome
(ex.: `ErrBadPassword`, `ErrEncrypted`, ou uma `Option` como `Password(...)`
que, se omitida, causa um erro específico ao tentar ler uma entrada
criptografada). Se `go doc` não deixar claro, abra o código-fonte do módulo
já baixado pelo `go get` da Task 4 (o caminho aparece em
`go list -m -f '{{.Dir}}' github.com/nwaples/rardecode/v2`) e procure por
"password" no texto.

- [ ] **Step 2: Escrever o teste de classificação (falhando)**

Adicionar a `internal/archive/rar_test.go`:

```go
func TestClassifyRarError(t *testing.T) {
	if got := classifyRarError(nil); got != nil {
		t.Errorf("classifyRarError(nil) = %v, want nil", got)
	}
	// Ajuste rardecode.ErrBadPassword abaixo para o nome real encontrado no
	// Step 1 se for diferente.
	if got := classifyRarError(rardecode.ErrBadPassword); !errors.Is(got, ErrPasswordProtected) {
		t.Errorf("classifyRarError(ErrBadPassword) = %v, want ErrPasswordProtected", got)
	}
	if got := classifyRarError(io.ErrUnexpectedEOF); !errors.Is(got, ErrCorrupted) {
		t.Errorf("classifyRarError(generic) = %v, want ErrCorrupted", got)
	}
}
```

Adicionar os imports necessários (`errors`, `github.com/nwaples/rardecode/v2`) ao topo de `rar_test.go`.

- [ ] **Step 3: Rodar e confirmar que falha**

```bash
go test ./internal/archive/... -run TestClassifyRarError -v
```
Esperado: falha — ou compilação (se `rardecode.ErrBadPassword` não existir com esse nome exato, o que já é uma confirmação de que o Step 1 precisa achar o nome certo antes de prosseguir), ou asserção (`classifyRarError` ainda mapeia tudo pra `ErrCorrupted`).

- [ ] **Step 4: Implementar a classificação real**

Substituir o `classifyRarError` de `rar.go` (ajustar o símbolo pro nome real achado no Step 1):

```go
func classifyRarError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, rardecode.ErrBadPassword) {
		return ErrPasswordProtected
	}
	return ErrCorrupted
}
```

Adicionar `"errors"` aos imports de `rar.go`.

- [ ] **Step 5: Rodar e confirmar que passa**

```bash
go test ./internal/archive/... -run TestClassifyRarError -v
```
Esperado: PASS.

- [ ] **Step 6: Escrever e rodar o teste de arquivo truncado (usa a amostra real)**

Adicionar a `rar_test.go`:

```go
func TestRarArchiveCorrupted(t *testing.T) {
	realPath := realCBRPath(t)

	data, err := os.ReadFile(realPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) < 200 {
		t.Skip("amostra real pequena demais para truncar com segurança")
	}

	dir := t.TempDir()
	truncatedPath := filepath.Join(dir, "truncated.cbr")
	if err := os.WriteFile(truncatedPath, data[:100], 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = newRarArchive(truncatedPath)
	if !errors.Is(err, ErrCorrupted) {
		t.Errorf("err = %v, want ErrCorrupted", err)
	}
}
```

```bash
go test ./internal/archive/... -run TestRarArchiveCorrupted -v
```
Esperado: PASS se a amostra real existir, `SKIP` (não `FAIL`) se não existir.

- [ ] **Step 7: Rodar a suíte inteira do pacote antes de commitar**

```bash
go test ./internal/archive/... -v
```
Esperado: todos os testes `PASS` ou `SKIP` (os que dependem da amostra real), nenhum `FAIL`.

- [ ] **Step 8: Commit**

```bash
git add internal/archive/rar.go internal/archive/rar_test.go
git commit -m "archive: classificacao de erro rar (senha vs corrompido)"
```

---

### Task 6: Limpeza de diretórios órfãos (`CleanOrphanedTempDirs`)

**Files:**
- Create: `internal/archive/tempdirs.go`
- Modify: `internal/archive/rar.go` (remover a função `tempDirRoot` duplicada da Task 4, que passa a viver só em `tempdirs.go`)
- Test: `internal/archive/tempdirs_test.go`

**Interfaces:**
- Produces: `var tempDirRoot func() string` (variável, não const — permite o teste substituí-la por um diretório isolado), `func CleanOrphanedTempDirs() error` — será chamada pelo `main()` da futura Etapa 2 no startup do app (não usada por nenhuma task desta etapa além do próprio teste).

- [ ] **Step 1: Escrever o teste (falhando)**

Criar `internal/archive/tempdirs_test.go`:

```go
package archive

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanOrphanedTempDirs(t *testing.T) {
	dir := t.TempDir()
	orig := tempDirRoot
	tempDirRoot = func() string { return dir }
	defer func() { tempDirRoot = orig }()

	orphan := filepath.Join(dir, "orphan-123")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(orphan, "page1.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := CleanOrphanedTempDirs(); err != nil {
		t.Fatalf("CleanOrphanedTempDirs: %v", err)
	}

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("orphan ainda existe: %v", err)
	}
}

func TestCleanOrphanedTempDirsNoDir(t *testing.T) {
	orig := tempDirRoot
	tempDirRoot = func() string { return filepath.Join(t.TempDir(), "nao-existe") }
	defer func() { tempDirRoot = orig }()

	if err := CleanOrphanedTempDirs(); err != nil {
		t.Errorf("CleanOrphanedTempDirs em diretório inexistente: %v", err)
	}
}
```

- [ ] **Step 2: Rodar e confirmar que falha**

```bash
go test ./internal/archive/... -run TestCleanOrphanedTempDirs -v
```
Esperado: erro de compilação (`undefined: CleanOrphanedTempDirs`, ou conflito porque `tempDirRoot` ainda é uma função declarada em `rar.go`, não uma variável).

- [ ] **Step 3: Remover a função `tempDirRoot` de `rar.go`**

Em `internal/archive/rar.go`, apagar o bloco:

```go
func tempDirRoot() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "hqreader", "tmp-extract")
}
```

- [ ] **Step 4: Criar `internal/archive/tempdirs.go`**

```go
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
```

- [ ] **Step 5: Rodar a suíte inteira do pacote**

```bash
go test ./internal/archive/... -v
```
Esperado: todos `PASS` ou `SKIP` (amostra real de CBR), nenhum `FAIL`. Isso confirma que `rar.go` ainda compila e funciona sem a `tempDirRoot` duplicada.

- [ ] **Step 6: Commit**

```bash
git add internal/archive/tempdirs.go internal/archive/tempdirs_test.go internal/archive/rar.go
git commit -m "archive: limpeza de diretorios temp orfaos"
```

---

### Task 7: Factory `Open()` — dispatch por extensão

**Files:**
- Create: `internal/archive/open.go`
- Test: `internal/archive/open_test.go`

**Interfaces:**
- Consumes: `newZipArchive` (Task 3), `newRarArchive` (Task 4), `ErrUnsupportedFormat` (Task 1), `writeTestZip`/`tinyJPEG` (Task 3), `realCBRPath` (Task 4).
- Produces: `func Open(path string) (Archive, error)` — **única função exportada do pacote além dos tipos** — consumida pelo `hqcli` na Task 8, e por toda a aplicação a partir da Etapa 2.

- [ ] **Step 1: Escrever o teste (falhando)**

Criar `internal/archive/open_test.go`:

```go
package archive

import (
	"errors"
	"testing"
)

func TestOpenUnsupportedFormat(t *testing.T) {
	_, err := Open("comic.pdf")
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("err = %v, want ErrUnsupportedFormat", err)
	}
}

func TestOpenDispatchesCBZ(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG})
	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	if len(a.Pages()) != 1 {
		t.Errorf("got %d pages, want 1", len(a.Pages()))
	}
}

func TestOpenDispatchesCBR(t *testing.T) {
	path := realCBRPath(t)
	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	if len(a.Pages()) == 0 {
		t.Error("esperava ao menos uma página")
	}
}
```

- [ ] **Step 2: Rodar e confirmar que falha**

```bash
go test ./internal/archive/... -run TestOpen -v
```
Esperado: erro de compilação (`undefined: Open`).

- [ ] **Step 3: Implementar**

Criar `internal/archive/open.go`:

```go
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
```

- [ ] **Step 4: Rodar e confirmar que passa**

```bash
go test ./internal/archive/... -run TestOpen -v
```
Esperado: `TestOpenUnsupportedFormat` e `TestOpenDispatchesCBZ` em PASS; `TestOpenDispatchesCBR` em PASS ou SKIP.

- [ ] **Step 5: Rodar a suíte inteira do pacote**

```bash
go test ./internal/archive/... -v
```
Esperado: todos `PASS` ou `SKIP`, nenhum `FAIL`.

- [ ] **Step 6: Commit**

```bash
git add internal/archive/open.go internal/archive/open_test.go
git commit -m "archive: factory Open() com dispatch por extensao"
```

---

### Task 8: `hqcli` — CLI de debug

**Files:**
- Create: `cmd/hqcli/main.go`

**Interfaces:**
- Consumes: `archive.Open`, `archive.Archive`, `archive.PageEntry`, `archive.ComicInfo` (pacote público do `internal/archive`, todas as tasks anteriores).

- [ ] **Step 1: Implementar**

Criar `cmd/hqcli/main.go`:

```go
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/DevMiguelDiniz/hq-reader-ultimate/internal/archive"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "uso: hqcli list <arquivo> | hqcli dump <arquivo> <indice>")
		os.Exit(1)
	}

	cmd, path := os.Args[1], os.Args[2]

	a, err := archive.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro ao abrir %s: %v\n", path, err)
		os.Exit(1)
	}
	defer a.Close()

	switch cmd {
	case "list":
		runList(a)
	case "dump":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "uso: hqcli dump <arquivo> <indice>")
			os.Exit(1)
		}
		runDump(a, os.Args[3])
	default:
		fmt.Fprintf(os.Stderr, "comando desconhecido: %s\n", cmd)
		os.Exit(1)
	}
}

func runList(a archive.Archive) {
	info, err := a.Metadata()
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro ao ler metadados: %v\n", err)
	} else if info != nil {
		fmt.Printf("Título: %s | Série: %s #%s\n", info.Title, info.Series, info.Number)
	}
	for _, p := range a.Pages() {
		fmt.Printf("%3d  %-40s  %8d bytes  %s\n", p.Index, p.Name, p.Size, p.ContentType)
	}
}

func runDump(a archive.Archive, indexArg string) {
	var index int
	if _, err := fmt.Sscanf(indexArg, "%d", &index); err != nil {
		fmt.Fprintf(os.Stderr, "índice inválido: %s\n", indexArg)
		os.Exit(1)
	}
	rc, err := a.Open(index)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro ao abrir página %d: %v\n", index, err)
		os.Exit(1)
	}
	defer rc.Close()
	if _, err := io.Copy(os.Stdout, rc); err != nil {
		fmt.Fprintf(os.Stderr, "erro ao copiar: %v\n", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Build**

```bash
go build -o /tmp/hqcli ./cmd/hqcli
```
Esperado: build sem erros.

- [ ] **Step 3: Verificar manualmente com um CBZ sintético**

```bash
cat <<'EOF' > /tmp/make_test_cbz.go
package main

import (
	"archive/zip"
	"os"
)

func main() {
	f, _ := os.Create("/tmp/test.cbz")
	defer f.Close()
	zw := zip.NewWriter(f)
	w, _ := zw.Create("page1.jpg")
	w.Write([]byte{0xFF, 0xD8, 0xFF, 0xD9})
	w2, _ := zw.Create("ComicInfo.xml")
	w2.Write([]byte("<ComicInfo><Title>Debug</Title></ComicInfo>"))
	zw.Close()
}
EOF
go run /tmp/make_test_cbz.go
/tmp/hqcli list /tmp/test.cbz
```
Esperado: imprime `Título: Debug | Série: #` seguido de uma linha listando `page1.jpg`.

```bash
/tmp/hqcli dump /tmp/test.cbz 0 | xxd | head -1
```
Esperado: mostra os bytes `ffd8 ffd9` (o conteúdo exato escrito no fixture).

- [ ] **Step 4: Verificar manualmente com a amostra real de CBR, se disponível**

```bash
/tmp/hqcli list spikes/rar-cost/testdata/sample-solid.cbr
```
Esperado (se o arquivo existir nesta máquina): lista as páginas da amostra real. Se o arquivo não existir, pule este step — não é bloqueante.

- [ ] **Step 5: Commit**

```bash
git add cmd/hqcli/main.go
git commit -m "hqcli: CLI de debug para list/dump de paginas"
```

---

## Self-Review

**Cobertura do spec:** interface `Archive` e tipos (§3.3) → Task 1; `ComicInfo.xml` → Task 2; `zipArchive` com corrupção/subpasta/filtro → Task 3; `rarArchive` com extração lazy, ciclo de vida do temp dir e mitigação de zip slip (nota de segurança do §3.3) → Tasks 4 e 6; classificação de erro senha/corrompido (§5) → Task 5; `hqcli` como "artefato tangível" (§6, Etapa 1) → Task 8; dispatch por extensão único ponto de entrada → Task 7. Página dupla/proporção fica explicitamente fora de escopo (pertence à Etapa 3, registrado nas Global Constraints) — não há requisito do §5 sem task correspondente.

**Placeholders:** nenhum "TBD"/"TODO" solto. O único ponto que parece incompleto de propósito é o `classifyRarError` mínimo da Task 4 (sempre retorna `ErrCorrupted`) — isso é código real, completo e correto (falha de forma segura), não um placeholder; a Task 5 substitui o corpo por uma versão mais específica, com a assinatura já fixada desde a Task 4 para não quebrar `rar.go`.

**Consistência de tipos:** `Archive`, `PageEntry{Index,Name,Size,ContentType}`, `ComicInfo`, as três sentinelas, `buildPageList`, `naturalLess`, `parseComicInfo`, `newZipArchive`, `newRarArchive`, `sanitizeEntryName`, `classifyRarError`, `CleanOrphanedTempDirs`, `Open` — usados com a mesma assinatura em todas as tasks que os consomem. `realCBRPath` e `writeTestZip`/`tinyJPEG` (definidos como helpers de teste nas Tasks 4 e 3) são reaproveitados sem redefinição nas Tasks 5 e 7, já que arquivos `_test.go` do mesmo pacote compartilham escopo.
