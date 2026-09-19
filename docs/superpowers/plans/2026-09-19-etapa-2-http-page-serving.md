# Etapa 2 — Página Servida por HTTP — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Um app Wails v3 real (não spike) de uma janela, sem estilo, que
serve páginas de um CBZ/CBR pelo pacote `internal/archive` da Etapa 1 através
de uma rota HTTP custom `/page/{bookID}/{n}`, com navegação básica
próximo/anterior.

**Architecture:** Um `internal/reader.Service` (Go puro, sem Wails, testável
por `httptest`) abre o livro via `archive.Open`, gera um `bookID` de sessão
opaco (nunca o caminho do arquivo) e serve os bytes crus pela rota HTTP já
validada GO na Etapa 0 (`AssetOptions.Handler`). O app Wails (`cmd/hqreader`)
só liga os fios: registra o `Service` como serviço Wails (bindável pelo
front) e como dono do handler HTTP, e abre um livro fixo no startup via
variável de ambiente para teste manual.

**Tech Stack:** Go 1.21+, Wails v3 (`v3.0.0-beta.23`, pinado — mesma versão
já validada na Etapa 0), `internal/archive` (Etapa 1). Frontend: HTML/JS puro
(vanilla), sem framework — Vite/React/Tailwind entram só quando a UI de
verdade começar na Etapa 3.

**Spec:** [docs/superpowers/specs/2026-09-16-hq-reader-design.md](../specs/2026-09-16-hq-reader-design.md) — §3.1 (rota HTTP), §3.2 (services sem janela), §3.4 (fluxo de dados), §6 (Etapa 2).

## Global Constraints

- Go 1.21+ (piso do projeto, já em uso na Etapa 1).
- Wails v3 pinado em `v3.0.0-beta.23` — mesma versão validada na Etapa 0 (`spikes/01-http-route-finding.md`, `spikes/04-multiwindow-finding.md`). Não instalar `@latest`.
- `bookID` é sempre um identificador opaco de sessão (16 bytes aleatórios em hex), **nunca** o caminho do arquivo — decisão de segurança do §3.1 do spec.
- Resposta do handler `/page/` leva `Cache-Control: immutable` (§3.1).
- Content-Type e Content-Length do handler vêm de `archive.PageEntry`, nunca hardcoded.
- Módulo Go: `github.com/DevMiguelDiniz/hq-reader-ultimate` (já estabelecido na Etapa 1) — o app Wails desta etapa faz parte do MESMO módulo, não um módulo Go separado, para poder importar `internal/archive` e `internal/reader` diretamente.
- Fora de escopo desta etapa (registrar explicitamente, não implementar): prefetch/ring buffer (§3.4 — entra na Etapa 3 junto do leitor de verdade), biblioteca/scanner (Etapa 4), múltiplas janelas (Etapa 7), qualquer estilo/polish visual (§6 diz explicitamente "zero estilo" pra esta etapa).
- Nenhuma amostra real de CBZ/CBR é commitada ao git (mesma política das Etapas 0-1).

---

### Task 1: Scaffold do app `hqreader` integrado ao módulo do projeto

**Files:**
- Create: `cmd/hqreader/` (gerado pelo CLI `wails3`, depois reconciliado com o módulo raiz)
- Modify: `go.mod`, `go.sum` (adicionar a dependência do Wails v3)
- Possibly create: `go.work`, `go.work.sum` (só se o Step 6 abaixo exigir o plano B)

**Interfaces:**
- Produces: um app Wails v3 rodável em `cmd/hqreader/`, parte do módulo `github.com/DevMiguelDiniz/hq-reader-ultimate`, que as Tasks 4-6 vão modificar.

- [ ] **Step 1: Confirmar toolchain (deve já estar pronto desde a Etapa 0)**

```bash
go version
wails3 version
```
Esperado: Go 1.21+, `wails3 version` reporta `v3.0.0-beta.23`. Se `wails3` não
estiver instalado nesta máquina (sessão nova), instalar a versão pinada:
```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.23
```

- [ ] **Step 2: Gerar o scaffold**

```bash
cd cmd
wails3 init -n hqreader -t vanilla
cd ..
```
Se a flag `-t vanilla` não existir nesta versão, rodar `wails3 init -help`
para achar o nome do template mínimo (JS/HTML puro, sem framework) e usar
esse.

- [ ] **Step 3: Remover o go.mod gerado (ele cria um módulo Go separado, que não queremos)**

```bash
rm cmd/hqreader/go.mod cmd/hqreader/go.sum
```

- [ ] **Step 4: Adicionar o Wails v3 ao módulo raiz, versão pinada**

Da raiz do repositório:
```bash
go get github.com/wailsapp/wails/v3@v3.0.0-beta.23
```

- [ ] **Step 5: Tentar buildar como parte do módulo único (caminho simples)**

```bash
go build ./...
```
Esperado: sucesso. `cmd/hqreader` agora é só mais um pacote dentro do módulo
raiz (igual `cmd/hqcli` já é desde a Etapa 1), sem precisar de módulo Go
separado nem de restrição de visibilidade (`internal/`) — ele já está dentro
da árvore do módulo.

- [ ] **Step 6: Verificar se o `wails3 dev` funciona sem `go.mod` próprio**

```bash
cd cmd/hqreader
wails3 dev
```
Esperado: uma janela nativa abre mostrando a página padrão do template.
Parar (Ctrl+C) depois de confirmar.

**Se este step falhar** com um erro de resolução de módulo (ex.: "no go.mod
found" ou similar — a CLI do Wails pode esperar rodar a partir do diretório
que contém seu próprio `go.mod`), aplicar o **plano B** antes de prosseguir:

```bash
# volte pra raiz do repo
cd ../..

# recriar um go.mod local em cmd/hqreader, mas com um module path que
# CONTINUA o mesmo namespace do módulo raiz — isso é o que permite
# importar internal/archive e internal/reader dali (a regra de
# visibilidade do Go pra pacotes "internal" é sobre o PREFIXO do import
# path, não sobre qual go.mod fisicamente governa o arquivo)
cd cmd/hqreader
go mod init github.com/DevMiguelDiniz/hq-reader-ultimate/cmd/hqreader
go mod edit -require=github.com/DevMiguelDiniz/hq-reader-ultimate@v0.0.0
go mod edit -require=github.com/wailsapp/wails/v3@v3.0.0-beta.23
cd ../..

# go.work faz o require acima resolver pra cópia local, sem precisar
# publicar o módulo raiz em lugar nenhum
go work init .
go work use ./cmd/hqreader
go work sync
```
Depois disso, repetir o Step 6 (`cd cmd/hqreader && wails3 dev`). Documentar
no commit desta task qual dos dois caminhos (simples ou plano B) foi
necessário.

- [ ] **Step 7: Confirmar que o import de `internal/archive` funciona do novo pacote**

Editar temporariamente `cmd/hqreader/main.go` adicionando o import
`"github.com/DevMiguelDiniz/hq-reader-ultimate/internal/archive"` e uma
linha inofensiva usando algo do pacote (ex.: `var _ archive.Archive`) só
para confirmar que compila:
```bash
go build ./cmd/hqreader/...
```
Esperado: sucesso, sem erro de "use of internal package ... not allowed".
Depois de confirmar, reverter essa linha de teste (ela será substituída de
verdade na Task 4).

- [ ] **Step 8: Atualizar `.gitignore` e commitar o scaffold**

O template do `wails3 init` já gera um `.gitignore` próprio dentro de
`cmd/hqreader/` cobrindo `bin/`, `frontend/dist/`, `frontend/node_modules/`
— conferir que existe (`cat cmd/hqreader/.gitignore`) e que essas pastas não
aparecem em `git status --short`. Se `go.work`/`go.work.sum` foram criados
(plano B), eles DEVEM ser commitados (não são artefato de build, são
configuração do workspace).

```bash
git add cmd/hqreader go.mod go.sum
# se o plano B foi necessário:
git add go.work go.work.sum
git commit -m "hqreader: scaffold do app Wails v3 integrado ao modulo do projeto"
```

---

### Task 2: `internal/reader.Service` — abrir livro e registro de sessão

**Files:**
- Create: `internal/reader/reader.go`
- Test: `internal/reader/reader_test.go`

**Interfaces:**
- Consumes: `archive.Open`, `archive.Archive`, `archive.PageEntry` (pacote público `internal/archive` da Etapa 1).
- Produces: `type OpenResult struct{ BookID string; PageCount int }`, `var ErrBookNotFound error`, `type Service struct{...}`, `func NewService() *Service`, `func (s *Service) Open(path string) (OpenResult, error)`, `func (s *Service) Current() (OpenResult, error)`, `func (s *Service) PageInfo(bookID string, index int) (archive.PageEntry, error)`, `func (s *Service) PageReader(bookID string, index int) (io.ReadCloser, error)`, `func (s *Service) Close(bookID string) error` — todos consumidos pela Task 3 (handler HTTP) e Task 4 (wiring do main.go).

- [ ] **Step 1: Escrever o teste (falhando)**

Criar `internal/reader/reader_test.go`:

```go
package reader

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

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

func TestServiceOpenAndPageReader(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{
		"page1.jpg": tinyJPEG,
		"page2.jpg": tinyJPEG,
	})

	s := NewService()
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if res.BookID == "" {
		t.Fatal("BookID vazio")
	}
	if res.PageCount != 2 {
		t.Fatalf("PageCount = %d, want 2", res.PageCount)
	}

	info, err := s.PageInfo(res.BookID, 0)
	if err != nil {
		t.Fatalf("PageInfo: %v", err)
	}
	if info.ContentType != "image/jpeg" {
		t.Errorf("ContentType = %q", info.ContentType)
	}

	rc, err := s.PageReader(res.BookID, 0)
	if err != nil {
		t.Fatalf("PageReader: %v", err)
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(data) != len(tinyJPEG) {
		t.Errorf("len(data) = %d, want %d", len(data), len(tinyJPEG))
	}
}

func TestServiceUnknownBookID(t *testing.T) {
	s := NewService()
	_, err := s.PageInfo("nao-existe", 0)
	if !errors.Is(err, ErrBookNotFound) {
		t.Errorf("err = %v, want ErrBookNotFound", err)
	}
}

func TestServiceOpenNonexistentFile(t *testing.T) {
	s := NewService()
	_, err := s.Open(filepath.Join(t.TempDir(), "nao-existe.cbz"))
	if err == nil {
		t.Fatal("esperava erro ao abrir arquivo inexistente")
	}
}

func TestServiceTwoOpensAreDifferentBookIDs(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG})
	s := NewService()
	res1, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open 1: %v", err)
	}
	res2, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open 2: %v", err)
	}
	if res1.BookID == res2.BookID {
		t.Error("dois Open() geraram o mesmo bookID")
	}
}

func TestServiceClose(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG})
	s := NewService()
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s.Close(res.BookID); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := s.PageInfo(res.BookID, 0); !errors.Is(err, ErrBookNotFound) {
		t.Errorf("PageInfo apos Close: err = %v, want ErrBookNotFound", err)
	}
}

func TestServiceCurrent(t *testing.T) {
	s := NewService()
	if _, err := s.Current(); !errors.Is(err, ErrBookNotFound) {
		t.Errorf("Current() antes de qualquer Open: err = %v, want ErrBookNotFound", err)
	}

	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG, "page2.jpg": tinyJPEG})
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	cur, err := s.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if cur.BookID != res.BookID || cur.PageCount != res.PageCount {
		t.Errorf("Current() = %+v, want %+v", cur, res)
	}
}
```

- [ ] **Step 2: Rodar e confirmar que falha**

```bash
go test ./internal/reader/... -v
```
Esperado: erro de compilação (`undefined: NewService`, etc — o pacote ainda
não existe).

- [ ] **Step 3: Implementar**

Criar `internal/reader/reader.go`:

```go
package reader

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"sync"

	"github.com/DevMiguelDiniz/hq-reader-ultimate/internal/archive"
)

// ErrBookNotFound é retornado quando um bookID não corresponde a nenhum
// livro aberto nesta sessão (nunca esteve aberto, ou já foi fechado).
var ErrBookNotFound = errors.New("reader: livro nao encontrado")

// OpenResult é o que o frontend recebe ao abrir (ou consultar) um livro.
type OpenResult struct {
	BookID    string
	PageCount int
}

// Service mantém o registro de livros abertos nesta sessão, indexados por
// um bookID opaco (nunca o caminho do arquivo, por segurança — ver §3.1 do
// spec: a rota HTTP nunca deve expor o caminho real no disco).
type Service struct {
	mu         sync.RWMutex
	books      map[string]archive.Archive
	lastOpened string
}

// NewService cria um Service vazio, sem nenhum livro aberto.
func NewService() *Service {
	return &Service{books: make(map[string]archive.Archive)}
}

// Open abre um livro em path e retorna um bookID de sessão opaco.
func (s *Service) Open(path string) (OpenResult, error) {
	a, err := archive.Open(path)
	if err != nil {
		return OpenResult{}, err
	}

	id, err := newBookID()
	if err != nil {
		a.Close()
		return OpenResult{}, err
	}

	s.mu.Lock()
	s.books[id] = a
	s.lastOpened = id
	s.mu.Unlock()

	return OpenResult{BookID: id, PageCount: len(a.Pages())}, nil
}

// Current retorna o último livro aberto nesta sessão — atalho pro frontend
// consultar o bookID atual sem precisar reabrir o arquivo.
func (s *Service) Current() (OpenResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.lastOpened == "" {
		return OpenResult{}, ErrBookNotFound
	}
	a, ok := s.books[s.lastOpened]
	if !ok {
		return OpenResult{}, ErrBookNotFound
	}
	return OpenResult{BookID: s.lastOpened, PageCount: len(a.Pages())}, nil
}

// PageInfo retorna os metadados (tamanho, content-type) da página index do
// livro bookID, sem ler os bytes da página.
func (s *Service) PageInfo(bookID string, index int) (archive.PageEntry, error) {
	a, err := s.get(bookID)
	if err != nil {
		return archive.PageEntry{}, err
	}
	pages := a.Pages()
	if index < 0 || index >= len(pages) {
		return archive.PageEntry{}, archive.ErrCorrupted
	}
	return pages[index], nil
}

// PageReader abre um leitor pros bytes crus da página index do livro
// bookID. Quem chama é responsável por fechar o reader retornado.
func (s *Service) PageReader(bookID string, index int) (io.ReadCloser, error) {
	a, err := s.get(bookID)
	if err != nil {
		return nil, err
	}
	return a.Open(index)
}

// Close fecha o livro bookID e remove seu registro. Chamar com um bookID
// desconhecido não é erro (idempotente).
func (s *Service) Close(bookID string) error {
	s.mu.Lock()
	a, ok := s.books[bookID]
	if ok {
		delete(s.books, bookID)
		if s.lastOpened == bookID {
			s.lastOpened = ""
		}
	}
	s.mu.Unlock()
	if !ok {
		return nil
	}
	return a.Close()
}

func (s *Service) get(bookID string) (archive.Archive, error) {
	s.mu.RLock()
	a, ok := s.books[bookID]
	s.mu.RUnlock()
	if !ok {
		return nil, ErrBookNotFound
	}
	return a, nil
}

func newBookID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
```

- [ ] **Step 4: Rodar e confirmar que passa**

```bash
go test ./internal/reader/... -v
```
Esperado: todos os 6 testes em PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/reader/reader.go internal/reader/reader_test.go
git commit -m "reader: Service com registro de sessao por bookID opaco"
```

---

### Task 3: Handler HTTP `/page/{bookID}/{n}`

**Files:**
- Create: `internal/reader/handler.go`
- Test: `internal/reader/handler_test.go`

**Interfaces:**
- Consumes: `Service`, `ErrBookNotFound`, `OpenResult` (Task 2), `writeTestZip`/`tinyJPEG` (helpers já definidos em `reader_test.go` da Task 2 — mesma pacote, não redefinir).
- Produces: `func PageHandler(svc *Service) http.Handler` — consumido pela Task 4 (wiring do `main.go`).

- [ ] **Step 1: Escrever o teste (falhando)**

Criar `internal/reader/handler_test.go`:

```go
package reader

import (
	"net/http/httptest"
	"testing"
)

func TestPageHandlerServesBytes(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG})
	svc := NewService()
	res, err := svc.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	handler := PageHandler(svc)
	req := httptest.NewRequest("GET", "/page/"+res.BookID+"/0", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Cache-Control") != "immutable" {
		t.Errorf("Cache-Control = %q, want immutable", rec.Header().Get("Cache-Control"))
	}
	if rec.Body.String() != string(tinyJPEG) {
		t.Errorf("body = %v, want %v", rec.Body.Bytes(), tinyJPEG)
	}
}

func TestPageHandlerUnknownBook(t *testing.T) {
	svc := NewService()
	handler := PageHandler(svc)
	req := httptest.NewRequest("GET", "/page/nao-existe/0", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestPageHandlerIndexOutOfRange(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG})
	svc := NewService()
	res, err := svc.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	handler := PageHandler(svc)
	req := httptest.NewRequest("GET", "/page/"+res.BookID+"/99", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 500 {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestPageHandlerMalformedPath(t *testing.T) {
	svc := NewService()
	handler := PageHandler(svc)
	req := httptest.NewRequest("GET", "/page/semindice", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
```

- [ ] **Step 2: Rodar e confirmar que falha**

```bash
go test ./internal/reader/... -run TestPageHandler -v
```
Esperado: erro de compilação (`undefined: PageHandler`).

- [ ] **Step 3: Implementar**

Criar `internal/reader/handler.go`:

```go
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
```

- [ ] **Step 4: Rodar e confirmar que passa**

```bash
go test ./internal/reader/... -v
```
Esperado: todos os testes (Task 2 + Task 3) em PASS — 10 no total.

- [ ] **Step 5: Commit**

```bash
git add internal/reader/handler.go internal/reader/handler_test.go
git commit -m "reader: handler HTTP /page/{bookID}/{n}"
```

---

### Task 4: Wiring — registrar o Service e o handler no app Wails

**Files:**
- Modify: `cmd/hqreader/main.go` (substituir o conteúdo gerado pelo template)

**Interfaces:**
- Consumes: `reader.NewService`, `reader.Service`, `reader.PageHandler` (Task 2-3).

- [ ] **Step 1: Verificar a API real de `application.Options`/`WebviewWindowOptions` da versão instalada**

```bash
cd cmd/hqreader
go doc github.com/wailsapp/wails/v3/pkg/application Options
go doc github.com/wailsapp/wails/v3/pkg/application AssetOptions
go doc github.com/wailsapp/wails/v3/pkg/application WebviewWindowOptions
```
A Etapa 0 já validou, na MESMA versão pinada (`v3.0.0-beta.23`), que
`AssetOptions.Handler` aceita um `http.Handler` custom e que janelas se
criam via `app.Window.NewWithOptions(application.WebviewWindowOptions{...})`
(não `app.NewWebviewWindow()`, que não existe nesta versão — ver
`spikes/04-multiwindow-finding.md`). Como é a mesma versão já validada, não
se espera drift aqui — mas confirme os nomes de campo de
`WebviewWindowOptions` (`Title`, `Width`, `Height` são os assumidos abaixo)
antes de prosseguir.

- [ ] **Step 2: Substituir `cmd/hqreader/main.go`**

```go
package main

import (
	"embed"
	"log"
	"net/http"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/DevMiguelDiniz/hq-reader-ultimate/internal/reader"
)

//go:embed all:frontend
var assets embed.FS

func main() {
	svc := reader.NewService()

	bookPath := os.Getenv("HQREADER_BOOK")
	if bookPath == "" {
		log.Fatal("defina a variavel de ambiente HQREADER_BOOK com o caminho de um .cbz/.cbr para testar (ex.: HQREADER_BOOK=C:\\comics\\teste.cbz)")
	}
	if _, err := svc.Open(bookPath); err != nil {
		log.Fatalf("erro ao abrir %s: %v", bookPath, err)
	}

	mux := http.NewServeMux()
	mux.Handle("/page/", reader.PageHandler(svc))
	mux.Handle("/", application.AssetFileServerFS(assets))

	app := application.New(application.Options{
		Name: "hqreader",
		Services: []application.Service{
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler: mux,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "hqreader",
		Width:  900,
		Height: 700,
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
```

Se algum nome de campo do Step 1 divergir do que está acima (`Title`,
`Width`, `Height`), ajustar para o nome real antes de prosseguir.

- [ ] **Step 3: Remover o `greetservice.go` (ou equivalente) gerado pelo template, se existir**

O template `vanilla` do `wails3 init` normalmente gera um serviço de
exemplo (`greetservice.go` com um método `Greet`). Como `main.go` foi
totalmente substituído acima e não referencia mais esse serviço, apagar o
arquivo gerado para não deixar código morto:
```bash
rm -f cmd/hqreader/greetservice.go
```

- [ ] **Step 4: Build**

```bash
go build ./...
```
Esperado: sucesso. Se houver erro de import não usado ou tipo não
encontrado, é sinal de que o Step 1 revelou um nome de campo diferente —
ajuste `main.go` de acordo.

- [ ] **Step 5: Commit**

```bash
git add cmd/hqreader/main.go
git rm -f cmd/hqreader/greetservice.go 2>/dev/null || true
git commit -m "hqreader: liga o Service e o handler HTTP no app Wails"
```

---

### Task 5: Frontend mínimo — `<img>` + navegação

**Files:**
- Modify: `cmd/hqreader/frontend/index.html` (substituir o conteúdo gerado pelo template)

**Interfaces:**
- Consumes: o binding gerado pelo Wails para `reader.Service.Current()` (Task 2/4) — o caminho exato do import e os nomes de campo JSON de `OpenResult` **precisam ser confirmados no Step 1 abaixo**, não assumidos.

- [ ] **Step 1: Gerar os bindings e confirmar o caminho de import real**

```bash
cd cmd/hqreader
wails3 dev
```
Deixar rodar por alguns segundos (isso gera os arquivos em
`frontend/bindings/`), depois parar (Ctrl+C). Inspecionar a árvore gerada:
```bash
find frontend/bindings -iname '*reader*'
```
Isso deve revelar o caminho real do binding do `reader.Service` (esperado
algo como `frontend/bindings/github.com/DevMiguelDiniz/hq-reader-ultimate/internal/reader/service.js`
ou similar, já que o Wails organiza bindings pelo caminho de import completo
do pacote Go — ver `frontend/bindings/github.com/wailsapp/wails/...` como
precedente do mesmo padrão na Etapa 0). Abrir o arquivo `.d.ts` correspondente
e conferir os nomes de campo gerados para `OpenResult` (podem vir como
`BookID`/`PageCount` — nomes Go verbatim — ou camelCase, dependendo de como
o Wails serializa; ajustar o Step 2 abaixo para o que for encontrado).

- [ ] **Step 2: Substituir `cmd/hqreader/frontend/index.html`**

Usar o caminho de import e os nomes de campo confirmados no Step 1 (o
exemplo abaixo assume o caminho completo por import path e os nomes Go
verbatim — ajustar as duas primeiras coisas se o Step 1 encontrar diferente,
mantendo o resto da estrutura):

```html
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8" />
  <title>hqreader</title>
  <style>
    html, body { margin: 0; height: 100%; background: #222; color: #eee; font-family: sans-serif; }
    #viewport { display: flex; flex-direction: column; align-items: center; height: 100%; justify-content: center; }
    #page { max-width: 90vw; max-height: 80vh; }
    #controls { margin-top: 12px; }
    button { font-size: 16px; padding: 8px 16px; margin: 0 8px; }
  </style>
</head>
<body>
  <div id="viewport">
    <img id="page" src="" alt="pagina" />
    <div id="controls">
      <button id="prev">Anterior</button>
      <span id="counter"></span>
      <button id="next">Proxima</button>
    </div>
  </div>
  <script type="module">
    import { Current } from './bindings/github.com/DevMiguelDiniz/hq-reader-ultimate/internal/reader/service.js';

    let bookID = '';
    let pageCount = 0;
    let index = 0;

    const img = document.getElementById('page');
    const counter = document.getElementById('counter');

    function render() {
      img.src = `/page/${bookID}/${index}`;
      counter.textContent = `${index + 1} / ${pageCount}`;
    }

    document.getElementById('prev').addEventListener('click', () => {
      if (index > 0) { index--; render(); }
    });
    document.getElementById('next').addEventListener('click', () => {
      if (index < pageCount - 1) { index++; render(); }
    });

    Current().then((result) => {
      bookID = result.BookID;
      pageCount = result.PageCount;
      index = 0;
      render();
    }).catch((err) => {
      document.body.textContent = 'Erro ao abrir o livro: ' + err;
    });
  </script>
</body>
</html>
```

- [ ] **Step 3: Commit**

```bash
git add cmd/hqreader/frontend/index.html
git commit -m "hqreader: frontend minimo com img e navegacao proximo/anterior"
```

---

### Task 6: Verificação manual de ponta a ponta

**Files:**
- Nenhum arquivo de produto novo — esta task só valida o que as Tasks 1-5 construíram, com um script de verificação descartável.

**Interfaces:**
- Consumes: o app `cmd/hqreader` completo (Tasks 1-5).

- [ ] **Step 1: Criar um CBZ de teste com várias páginas**

```bash
mkdir -p /tmp/hqreader-manual-test
cat <<'EOF' > /tmp/hqreader-manual-test/make_test_cbz.go
package main

import (
	"archive/zip"
	"os"
)

func main() {
	f, _ := os.Create(os.Args[1])
	defer f.Close()
	zw := zip.NewWriter(f)
	pages := [][]byte{
		{0xFF, 0xD8, 0xFF, 0xD9},
		{0xFF, 0xD8, 0xFF, 0xD9},
		{0xFF, 0xD8, 0xFF, 0xD9},
	}
	names := []string{"page1.jpg", "page2.jpg", "page3.jpg"}
	for i, data := range pages {
		w, _ := zw.Create(names[i])
		w.Write(data)
	}
	zw.Close()
}
EOF
go run /tmp/hqreader-manual-test/make_test_cbz.go /tmp/hqreader-manual-test/teste.cbz
```

- [ ] **Step 2: Rodar em modo dev e verificar visualmente/via UI Automation**

```bash
cd cmd/hqreader
```
No Windows PowerShell (variável de ambiente + `wails3 dev`):
```powershell
$env:HQREADER_BOOK = "C:\caminho\completo\para\teste.cbz"
wails3 dev
```
Confirmar manualmente (ou reutilizando a técnica de UI Automation já
validada na Etapa 0 — ver `spikes/04-multiwindow-verify.ps1` como
referência de como ler o texto de um elemento da janela via PowerShell):
- A janela abre mostrando a primeira página (contador "1 / 3").
- Clicar "Proxima" avança pro contador "2 / 3" e a imagem muda de origem
  (`/page/{bookID}/1`).
- Clicar "Proxima" de novo mostra "3 / 3"; o botão "Proxima" não avança
  além disso (índice grudado no último).
- Clicar "Anterior" volta o contador e a imagem correspondente.

Parar o processo (Ctrl+C) depois de confirmar.

- [ ] **Step 3: Repetir a verificação central no build de produção**

Assim como a Etapa 0 validou a rota HTTP tanto em `wails3 dev` quanto em
`wails3 build` (ver `spikes/01-http-route-prod-verify.ps1`), repetir aqui
pro fluxo completo:
```bash
wails3 build
```
Rodar o binário gerado (em `cmd/hqreader/bin/`) com a mesma variável de
ambiente `HQREADER_BOOK` setada, e confirmar que a navegação funciona
igual — isso exercita o caminho de assets embutidos (`AssetFileServerFS`),
diferente do proxy do Vite usado em `wails3 dev`.

- [ ] **Step 4: Registrar o resultado**

Criar `cmd/hqreader/MANUAL_VERIFICATION.md`:

```markdown
# Verificação manual — Etapa 2

**Data:** <preencher>
**Cenário:** CBZ sintético de 3 páginas, `HQREADER_BOOK` apontando pro arquivo.

- [ ] `wails3 dev`: primeira página carrega, contador mostra "1 / 3"
- [ ] Navegação "Proxima"/"Anterior" funciona e atualiza a imagem
- [ ] Índice não ultrapassa os limites (0 a pageCount-1)
- [ ] `wails3 build` + binário: mesmo comportamento no build de produção

**Observações:** <qualquer desvio do esperado, ou "nenhum">
```

Preencher com o resultado real observado nos Steps 2-3.

- [ ] **Step 5: Commit**

```bash
git add cmd/hqreader/MANUAL_VERIFICATION.md
git commit -m "hqreader: registra verificacao manual de ponta a ponta da etapa 2"
```

---

## Self-Review

**Cobertura do spec:** rota HTTP `/page/{bookID}/{n}` com `Cache-Control:
immutable` e Content-Type/Length vindos do `PageEntry` (§3.1) → Tasks 2-3;
`bookID` opaco nunca-é-o-caminho (§3.1) → Task 2 (`newBookID` via
`crypto/rand`, nunca deriva do path); services sem dependência de janela
(§3.2) → `reader.Service` não importa nada de `application`, só é
registrado como Wails service de fora (Task 4); fluxo de dados básico
abrir→servir página (§3.4, sem prefetch — explicitamente fora de escopo
nas Global Constraints) → Tasks 2-4; artefato "app de uma janela, zero
estilo, img + próximo/anterior" (§6, Etapa 2) → Tasks 1, 4, 5; validar tanto
dev quanto build de produção (mesmo padrão de rigor da Etapa 0) → Task 6.

**Placeholders:** nenhum "TBD"/"TODO". As duas únicas partes com
investigação-antes-de-codar (nomes de campo de `WebviewWindowOptions` na
Task 4, caminho de import do binding + nomes JSON de `OpenResult` na Task 5)
seguem o mesmo padrão bem-sucedido das Etapas 0-1: código completo e
funcional como ponto de partida, com o passo de verificação explícito
declarado antes de qualquer linha que dependa do resultado da investigação.

**Consistência de tipos:** `OpenResult{BookID,PageCount}`, `Service`,
`ErrBookNotFound`, `PageHandler(svc *Service) http.Handler` usados com a
mesma assinatura em todas as tasks que os consomem. `writeTestZip`/
`tinyJPEG` (definidos em `reader_test.go` na Task 2) são reaproveitados sem
redefinição em `handler_test.go` na Task 3 (mesmo pacote). A Task 5 depende
do binding gerado pela Task 4's `application.NewService(svc)` — a ordem das
tasks garante que o binding já existe quando a Task 5 precisa inspecioná-lo.
