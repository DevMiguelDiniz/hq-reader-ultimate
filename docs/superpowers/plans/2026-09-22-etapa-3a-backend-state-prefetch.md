# Etapa 3a — Estado da Página no Backend + Prefetch — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rastrear a página atual de cada livro no backend (`internal/reader.Service`) e prefetch-ar em RAM as páginas próximas dela, servindo-as via cache quando disponíveis — tudo em Go puro, sem nenhuma dependência do Wails.

**Architecture:** `Service` passa a guardar, por livro, um `bookEntry{archive, currentPage, prefetch}`. `SetCurrentPage` atualiza `currentPage` e aciona um `prefetchCoordinator` dedicado àquele livro, que recalcula a janela `[index-2, index+2]` (recortada aos limites do livro), cancela fetches fora da nova janela e dispara os que faltam em goroutines próprias. `PageReader` passa a consultar esse cache antes de cair no caminho de leitura direta do archive — o handler HTTP da Etapa 2 (`PageHandler`) não muda nenhuma linha, já que o cache fica transparente dentro de `PageReader`.

**Tech Stack:** Go 1.25.0 (piso atual do projeto, herdado da dependência do Wails v3 desde a Etapa 2). Nenhuma dependência nova.

**Spec:** [docs/superpowers/specs/2026-09-16-hq-reader-design.md](../specs/2026-09-16-hq-reader-design.md) — §3.2 (services sem dependência de janela), §3.4 (fluxo de dados: prefetch coordinator e ring buffer).

## Global Constraints

- Go 1.25.0 (não regredir — Wails v3 exige essa versão mínima desde a Etapa 2).
- Assinaturas públicas já existentes de `internal/reader` não podem quebrar: `NewService() *Service`, `(*Service).Open(path string) (OpenResult, error)`, `(*Service).Current() (OpenResult, error)`, `(*Service).PageInfo(bookID string, index int) (archive.PageEntry, error)`, `(*Service).PageReader(bookID string, index int) (io.ReadCloser, error)`, `(*Service).Close(bookID string) error`, `OpenResult{BookID string; PageCount int}`, `ErrBookNotFound`, `PageHandler(svc *Service) http.Handler`.
- `internal/reader/handler.go` e `internal/reader/handler_test.go` (da Etapa 2) não são modificados por este plano, exceto pela adição de UM novo teste na Task 4 — o cache fica inteiramente transparente dentro de `PageReader`.
- Testes que abrem um archive de verdade via `Service` (direto ou indiretamente) precisam de `defer s.Close(bookID)` — lição da Etapa 2: no Windows, `t.TempDir()` falha ao limpar se o `Service` ainda segura um handle de arquivo aberto.
- "Ring buffer de 5 slots" (§3.4) é implementado como um mapa limitado pela própria janela `[index-2, index+2]` (no máximo 5 entradas) em vez de um array circular literal — a janela nunca cresce além de 5 posições, então a capacidade fica garantida pelo próprio algoritmo de `SetCurrent`, não por uma estrutura de dados de tamanho fixo. Decisão de design deliberada, não uma simplificação incompleta.
- Cancelamento de prefetch via `context.Context` é *best-effort*: como `io.ReadAll` não observa `ctx.Done()`, cancelar um fetch em andamento não interrompe a leitura — só descarta o resultado quando ela termina. Aceitável porque `Archive.Open(index)` é rápido (zip de acesso aleatório, ou arquivo já extraído em disco no caso do RAR) — não vale a complexidade de plumbing de contexto através de `io.Reader`.
- `zipArchive.Open()`/`rarArchive.Open()` (Etapa 1) já são seguros para chamadas concorrentes em índices diferentes (e no mesmo índice) — nenhuma sincronização adicional é necessária na camada `Archive` para o prefetch rodar em paralelo com leituras do handler HTTP.

---

### Task 1: `SetCurrentPage`/`CurrentPage` — estado da página no backend

**Files:**
- Modify: `internal/reader/reader.go` (substituir o conteúdo inteiro pelo dado no Step 3 — é um refactor que toca praticamente todos os métodos existentes)
- Modify: `internal/reader/reader_test.go` (adicionar testes novos ao final do arquivo, sem tocar nos existentes)

**Interfaces:**
- Consumes: `archive.Open`, `archive.Archive`, `archive.PageEntry`, `archive.ErrCorrupted` (Etapa 1).
- Produces: `type bookEntry struct{ archive archive.Archive; currentPage int }` (sem campo `prefetch` ainda — adicionado na Task 3), `func (s *Service) SetCurrentPage(bookID string, index int) error`, `func (s *Service) CurrentPage(bookID string) (int, error)` — consumidos pelas Tasks 2-4.

- [ ] **Step 1: Escrever os testes novos (falhando)**

Adicionar ao final de `internal/reader/reader_test.go`:

```go
func TestServiceSetAndGetCurrentPage(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{
		"page1.jpg": tinyJPEG,
		"page2.jpg": tinyJPEG,
		"page3.jpg": tinyJPEG,
	})
	s := NewService()
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close(res.BookID)

	if err := s.SetCurrentPage(res.BookID, 2); err != nil {
		t.Fatalf("SetCurrentPage: %v", err)
	}
	got, err := s.CurrentPage(res.BookID)
	if err != nil {
		t.Fatalf("CurrentPage: %v", err)
	}
	if got != 2 {
		t.Errorf("CurrentPage = %d, want 2", got)
	}
}

func TestServiceCurrentPageDefaultsToZero(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG})
	s := NewService()
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close(res.BookID)

	got, err := s.CurrentPage(res.BookID)
	if err != nil {
		t.Fatalf("CurrentPage: %v", err)
	}
	if got != 0 {
		t.Errorf("CurrentPage antes de qualquer SetCurrentPage = %d, want 0", got)
	}
}

func TestServiceCurrentPageUnknownBook(t *testing.T) {
	s := NewService()
	_, err := s.CurrentPage("nao-existe")
	if !errors.Is(err, ErrBookNotFound) {
		t.Errorf("err = %v, want ErrBookNotFound", err)
	}
}

func TestServiceSetCurrentPageUnknownBook(t *testing.T) {
	s := NewService()
	err := s.SetCurrentPage("nao-existe", 0)
	if !errors.Is(err, ErrBookNotFound) {
		t.Errorf("err = %v, want ErrBookNotFound", err)
	}
}

func TestServiceSetCurrentPageOutOfRange(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG})
	s := NewService()
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close(res.BookID)

	err = s.SetCurrentPage(res.BookID, 5)
	if !errors.Is(err, archive.ErrCorrupted) {
		t.Errorf("err = %v, want archive.ErrCorrupted", err)
	}
}
```

Adicionar `"github.com/DevMiguelDiniz/hq-reader-ultimate/internal/archive"` aos imports de `reader_test.go` (necessário para `archive.ErrCorrupted` no último teste).

- [ ] **Step 2: Rodar e confirmar que falha**

```bash
go test ./internal/reader/... -v
```
Esperado: erro de compilação (`undefined: SetCurrentPage`, `undefined: CurrentPage`).

- [ ] **Step 3: Substituir `internal/reader/reader.go` inteiro**

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

// bookEntry agrupa o archive aberto e o estado de sessão associado a ele
// (página atual). Esse estado vive no backend desde a Etapa 3 — nunca só
// em useState do React — para que uma futura janela destacada (Etapa 7)
// possa compartilhar o mesmo estado sem duplicar lógica no front.
type bookEntry struct {
	archive     archive.Archive
	currentPage int
}

// Service mantém o registro de livros abertos nesta sessão, indexados por
// um bookID opaco (nunca o caminho do arquivo, por segurança — ver §3.1 do
// spec: a rota HTTP nunca deve expor o caminho real no disco).
type Service struct {
	mu         sync.RWMutex
	books      map[string]*bookEntry
	lastOpened string
}

// NewService cria um Service vazio, sem nenhum livro aberto.
func NewService() *Service {
	return &Service{books: make(map[string]*bookEntry)}
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
	s.books[id] = &bookEntry{archive: a}
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
	entry, ok := s.books[s.lastOpened]
	if !ok {
		return OpenResult{}, ErrBookNotFound
	}
	return OpenResult{BookID: s.lastOpened, PageCount: len(entry.archive.Pages())}, nil
}

// PageInfo retorna os metadados (tamanho, content-type) da página index do
// livro bookID, sem ler os bytes da página.
func (s *Service) PageInfo(bookID string, index int) (archive.PageEntry, error) {
	entry, err := s.get(bookID)
	if err != nil {
		return archive.PageEntry{}, err
	}
	pages := entry.archive.Pages()
	if index < 0 || index >= len(pages) {
		return archive.PageEntry{}, archive.ErrCorrupted
	}
	return pages[index], nil
}

// PageReader abre um leitor pros bytes crus da página index do livro
// bookID. Quem chama é responsável por fechar o reader retornado.
func (s *Service) PageReader(bookID string, index int) (io.ReadCloser, error) {
	entry, err := s.get(bookID)
	if err != nil {
		return nil, err
	}
	return entry.archive.Open(index)
}

// SetCurrentPage registra qual página está sendo exibida agora para o
// livro bookID. Esse estado vive no backend (não no frontend) — ver
// comentário de bookEntry.
func (s *Service) SetCurrentPage(bookID string, index int) error {
	s.mu.Lock()
	entry, ok := s.books[bookID]
	if !ok {
		s.mu.Unlock()
		return ErrBookNotFound
	}
	pages := entry.archive.Pages()
	if index < 0 || index >= len(pages) {
		s.mu.Unlock()
		return archive.ErrCorrupted
	}
	entry.currentPage = index
	s.mu.Unlock()
	return nil
}

// CurrentPage retorna a última página definida via SetCurrentPage para o
// livro bookID (0 se SetCurrentPage nunca foi chamado para ele).
func (s *Service) CurrentPage(bookID string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.books[bookID]
	if !ok {
		return 0, ErrBookNotFound
	}
	return entry.currentPage, nil
}

// Close fecha o livro bookID e remove seu registro. Chamar com um bookID
// desconhecido não é erro (idempotente).
func (s *Service) Close(bookID string) error {
	s.mu.Lock()
	entry, ok := s.books[bookID]
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
	return entry.archive.Close()
}

func (s *Service) get(bookID string) (*bookEntry, error) {
	s.mu.RLock()
	entry, ok := s.books[bookID]
	s.mu.RUnlock()
	if !ok {
		return nil, ErrBookNotFound
	}
	return entry, nil
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
Esperado: todos os testes (os 6 já existentes da Etapa 2 + os 5 novos desta task) em PASS.

- [ ] **Step 5: Confirmar que o resto do projeto ainda compila**

```bash
go build ./...
go vet ./...
```
Esperado: sucesso (o refactor de `books map[string]archive.Archive` para `books map[string]*bookEntry` é inteiramente interno a `reader.go` — `handler.go` só usa os métodos públicos de `Service`, então não deveria precisar de nenhuma mudança).

- [ ] **Step 6: Commit**

```bash
git add internal/reader/reader.go internal/reader/reader_test.go
git commit -m "reader: estado da pagina atual no backend (SetCurrentPage/CurrentPage)"
```

---

### Task 2: `prefetchCoordinator` — janela, cancelamento, cache em RAM

**Files:**
- Create: `internal/reader/prefetch.go`
- Test: `internal/reader/prefetch_test.go`

**Interfaces:**
- Consumes: `archive.Archive`, `archive.Open` (Etapa 1), `writeTestZip`/`tinyJPEG` (Etapa 2, `reader_test.go`, mesmo pacote).
- Produces: `func newPrefetchCoordinator(a archive.Archive) *prefetchCoordinator`, `func (p *prefetchCoordinator) SetCurrent(index int)`, `func (p *prefetchCoordinator) Get(index int) ([]byte, bool)`, `func (p *prefetchCoordinator) Cancel()` — consumidos pela Task 3 (que passa a criar/guardar um `*prefetchCoordinator` por `bookEntry`) e pela Task 4 (`PageReader` consulta `Get`).

- [ ] **Step 1: Escrever o teste (falhando)**

Criar `internal/reader/prefetch_test.go`:

```go
package reader

import (
	"fmt"
	"testing"
	"time"

	"github.com/DevMiguelDiniz/hq-reader-ultimate/internal/archive"
)

func openTestArchiveWithPages(t *testing.T, n int) archive.Archive {
	t.Helper()
	files := make(map[string][]byte, n)
	for i := 0; i < n; i++ {
		files[fmt.Sprintf("page%02d.jpg", i)] = tinyJPEG
	}
	path := writeTestZip(t, files)
	a, err := archive.Open(path)
	if err != nil {
		t.Fatalf("archive.Open: %v", err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

func waitForSlot(t *testing.T, p *prefetchCoordinator, index int) []byte {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if data, ok := p.Get(index); ok {
			return data
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("pagina %d nunca apareceu no cache dentro do prazo", index)
	return nil
}

func TestPrefetchCoordinatorFetchesWindow(t *testing.T) {
	a := openTestArchiveWithPages(t, 10)
	p := newPrefetchCoordinator(a)

	p.SetCurrent(5)

	for _, idx := range []int{3, 4, 5, 6, 7} {
		data := waitForSlot(t, p, idx)
		if len(data) != len(tinyJPEG) {
			t.Errorf("pagina %d: len(data) = %d, want %d", idx, len(data), len(tinyJPEG))
		}
	}
}

func TestPrefetchCoordinatorClipsAtBookBoundaries(t *testing.T) {
	a := openTestArchiveWithPages(t, 3)
	p := newPrefetchCoordinator(a)

	p.SetCurrent(0)

	for _, idx := range []int{0, 1, 2} {
		waitForSlot(t, p, idx)
	}

	if _, ok := p.Get(-1); ok {
		t.Error("Get(-1) deveria retornar false, indice invalido")
	}
	if _, ok := p.Get(3); ok {
		t.Error("Get(3) deveria retornar false, fora dos limites do livro (3 paginas: 0,1,2)")
	}
}

func TestPrefetchCoordinatorEvictsOutOfWindowSlots(t *testing.T) {
	a := openTestArchiveWithPages(t, 20)
	p := newPrefetchCoordinator(a)

	p.SetCurrent(0)
	waitForSlot(t, p, 0)

	p.SetCurrent(15)

	if _, ok := p.Get(0); ok {
		t.Error("pagina 0 deveria ter sido removida do cache apos SetCurrent(15) — janela nao inclui mais o indice 0")
	}
}

func TestPrefetchCoordinatorGetMissingReturnsFalse(t *testing.T) {
	a := openTestArchiveWithPages(t, 5)
	p := newPrefetchCoordinator(a)

	if _, ok := p.Get(2); ok {
		t.Error("Get(2) antes de qualquer SetCurrent deveria retornar false")
	}
}

func TestPrefetchCoordinatorCancel(t *testing.T) {
	a := openTestArchiveWithPages(t, 10)
	p := newPrefetchCoordinator(a)

	p.SetCurrent(5)
	waitForSlot(t, p, 5)

	p.Cancel()

	p.mu.Lock()
	pending := len(p.cancel)
	p.mu.Unlock()
	if pending != 0 {
		t.Errorf("apos Cancel(), %d fetches ainda registrados como em andamento, want 0", pending)
	}
}
```

- [ ] **Step 2: Rodar e confirmar que falha**

```bash
go test ./internal/reader/... -run TestPrefetchCoordinator -v
```
Esperado: erro de compilação (`undefined: newPrefetchCoordinator`).

- [ ] **Step 3: Implementar**

Criar `internal/reader/prefetch.go`:

```go
package reader

import (
	"context"
	"io"
	"sync"

	"github.com/DevMiguelDiniz/hq-reader-ultimate/internal/archive"
)

// prefetchWindow é o raio da janela de prefetch ao redor da página atual
// (N±2 = até 5 páginas no total) — ver §3.4 do spec.
const prefetchWindow = 2

// prefetchCoordinator mantém em memória os bytes já lidos das páginas
// próximas da atual (uma janela de até 5 posições), evitando reabrir o
// archive a cada requisição HTTP subsequente. Uma instância por livro
// aberto. Cancelamento via context é best-effort: como io.ReadAll não é
// sensível a contexto, cancelar um fetch em andamento não interrompe a
// leitura — só descarta o resultado quando ela termina.
type prefetchCoordinator struct {
	archive archive.Archive

	mu     sync.Mutex
	slots  map[int][]byte
	cancel map[int]context.CancelFunc
}

func newPrefetchCoordinator(a archive.Archive) *prefetchCoordinator {
	return &prefetchCoordinator{
		archive: a,
		slots:   make(map[int][]byte),
		cancel:  make(map[int]context.CancelFunc),
	}
}

// SetCurrent recalcula a janela [index-2, index+2] (recortada aos limites
// válidos do livro): cancela prefetches fora da nova janela, descarta do
// cache o que saiu da janela, e dispara prefetch para os índices que
// entraram na janela e ainda não estão prontos nem em andamento.
func (p *prefetchCoordinator) SetCurrent(index int) {
	pages := p.archive.Pages()
	if len(pages) == 0 {
		return
	}

	windowStart := index - prefetchWindow
	if windowStart < 0 {
		windowStart = 0
	}
	windowEnd := index + prefetchWindow
	if windowEnd > len(pages)-1 {
		windowEnd = len(pages) - 1
	}

	inWindow := make(map[int]bool, windowEnd-windowStart+1)
	for i := windowStart; i <= windowEnd; i++ {
		inWindow[i] = true
	}

	p.mu.Lock()
	for i, cancel := range p.cancel {
		if !inWindow[i] {
			cancel()
			delete(p.cancel, i)
		}
	}
	for i := range p.slots {
		if !inWindow[i] {
			delete(p.slots, i)
		}
	}
	var toFetch []int
	for i := windowStart; i <= windowEnd; i++ {
		_, hasSlot := p.slots[i]
		_, inFlight := p.cancel[i]
		if !hasSlot && !inFlight {
			toFetch = append(toFetch, i)
		}
	}
	p.mu.Unlock()

	for _, i := range toFetch {
		p.startFetch(i)
	}
}

func (p *prefetchCoordinator) startFetch(index int) {
	ctx, cancel := context.WithCancel(context.Background())

	p.mu.Lock()
	p.cancel[index] = cancel
	p.mu.Unlock()

	go func() {
		rc, err := p.archive.Open(index)
		if err != nil {
			p.mu.Lock()
			delete(p.cancel, index)
			p.mu.Unlock()
			return
		}
		data, readErr := io.ReadAll(rc)
		rc.Close()

		p.mu.Lock()
		defer p.mu.Unlock()
		delete(p.cancel, index)
		if ctx.Err() != nil {
			// cancelado enquanto lia — descarta o resultado
			return
		}
		if readErr == nil {
			p.slots[index] = data
		}
	}()
}

// Get retorna os bytes em cache pra página index, se já estiverem prontos.
func (p *prefetchCoordinator) Get(index int) ([]byte, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	data, ok := p.slots[index]
	return data, ok
}

// Cancel interrompe (best-effort) todos os fetches em andamento — chamado
// quando o livro é fechado, pra não deixar goroutines tentando ler de um
// archive que está prestes a ser fechado.
func (p *prefetchCoordinator) Cancel() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, cancel := range p.cancel {
		cancel()
		delete(p.cancel, i)
	}
}
```

- [ ] **Step 4: Rodar e confirmar que passa**

```bash
go test ./internal/reader/... -v
```
Esperado: todos os testes em PASS (Task 1 + os 6 novos desta task).

- [ ] **Step 5: Rodar com o detector de race, já que este código usa goroutines**

```bash
go test ./internal/reader/... -race -v
```
Esperado: PASS, sem avisos de race condition.

- [ ] **Step 6: Commit**

```bash
git add internal/reader/prefetch.go internal/reader/prefetch_test.go
git commit -m "reader: prefetchCoordinator com janela, cancelamento e cache em RAM"
```

---

### Task 3: Ligar o `prefetchCoordinator` ao ciclo de vida do livro

**Files:**
- Modify: `internal/reader/reader.go`
- Modify: `internal/reader/reader_test.go` (adicionar testes ao final)

**Interfaces:**
- Consumes: `newPrefetchCoordinator`, `(*prefetchCoordinator).SetCurrent`, `(*prefetchCoordinator).Cancel` (Task 2), `waitForSlot` (Task 2, `prefetch_test.go`, mesmo pacote).
- Produces: `bookEntry` ganha o campo `prefetch *prefetchCoordinator`, preenchido em `Open()`, consumido por `SetCurrentPage` (aciona) e `Close` (cancela). A Task 4 depende de `entry.prefetch` estar sempre não-nil para todo `bookEntry` vivo.

- [ ] **Step 1: Escrever os testes novos (falhando)**

Adicionar ao final de `internal/reader/reader_test.go`:

```go
func TestServiceOpenCreatesPrefetchCoordinator(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG, "page2.jpg": tinyJPEG})
	s := NewService()
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close(res.BookID)

	s.mu.RLock()
	entry := s.books[res.BookID]
	s.mu.RUnlock()
	if entry.prefetch == nil {
		t.Fatal("entry.prefetch é nil apos Open()")
	}
}

func TestServiceSetCurrentPageTriggersPrefetch(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{
		"page0.jpg": tinyJPEG, "page1.jpg": tinyJPEG, "page2.jpg": tinyJPEG,
	})
	s := NewService()
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close(res.BookID)

	if err := s.SetCurrentPage(res.BookID, 1); err != nil {
		t.Fatalf("SetCurrentPage: %v", err)
	}

	s.mu.RLock()
	entry := s.books[res.BookID]
	s.mu.RUnlock()

	waitForSlot(t, entry.prefetch, 1)
}
```

- [ ] **Step 2: Rodar e confirmar que falha**

```bash
go test ./internal/reader/... -run 'TestServiceOpenCreatesPrefetchCoordinator|TestServiceSetCurrentPageTriggersPrefetch' -v
```
Esperado: erro de compilação (`entry.prefetch undefined` — o campo ainda não existe em `bookEntry`).

- [ ] **Step 3: Adicionar o campo `prefetch` a `bookEntry` e popular em `Open()`**

Em `internal/reader/reader.go`, alterar a definição de `bookEntry`:

```go
type bookEntry struct {
	archive     archive.Archive
	currentPage int
	prefetch    *prefetchCoordinator
}
```

E em `Open()`, alterar a linha que cria o `bookEntry`:

```go
	s.mu.Lock()
	s.books[id] = &bookEntry{archive: a, prefetch: newPrefetchCoordinator(a)}
	s.lastOpened = id
	s.mu.Unlock()
```

- [ ] **Step 4: Fazer `SetCurrentPage` acionar o prefetch**

Substituir o corpo de `SetCurrentPage` em `internal/reader/reader.go`:

```go
func (s *Service) SetCurrentPage(bookID string, index int) error {
	s.mu.Lock()
	entry, ok := s.books[bookID]
	if !ok {
		s.mu.Unlock()
		return ErrBookNotFound
	}
	pages := entry.archive.Pages()
	if index < 0 || index >= len(pages) {
		s.mu.Unlock()
		return archive.ErrCorrupted
	}
	entry.currentPage = index
	s.mu.Unlock()

	entry.prefetch.SetCurrent(index)
	return nil
}
```

- [ ] **Step 5: Fazer `Close` cancelar o prefetch em andamento**

Substituir o corpo de `Close` em `internal/reader/reader.go`:

```go
func (s *Service) Close(bookID string) error {
	s.mu.Lock()
	entry, ok := s.books[bookID]
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
	entry.prefetch.Cancel()
	return entry.archive.Close()
}
```

- [ ] **Step 6: Rodar e confirmar que passa**

```bash
go test ./internal/reader/... -race -v
```
Esperado: todos os testes (Tasks 1-2 + os 2 novos desta task) em PASS, sem avisos de race.

- [ ] **Step 7: Confirmar que o resto do projeto ainda compila**

```bash
go build ./...
go vet ./...
```

- [ ] **Step 8: Commit**

```bash
git add internal/reader/reader.go internal/reader/reader_test.go
git commit -m "reader: liga prefetchCoordinator ao ciclo de vida do livro (Open/SetCurrentPage/Close)"
```

---

### Task 4: `PageReader` consulta o cache de prefetch

**Files:**
- Modify: `internal/reader/reader.go`
- Modify: `internal/reader/reader_test.go` (adicionar testes ao final)
- Modify: `internal/reader/handler_test.go` (adicionar um teste ao final — `handler.go` em si não muda)

**Interfaces:**
- Consumes: `(*prefetchCoordinator).Get` (Task 2), `entry.prefetch` (Task 3).
- Produces: `PageReader` passa a servir bytes do cache quando disponíveis, mantendo a mesma assinatura pública — nenhuma task futura depende de algo novo aqui além do comportamento (o `PageHandler` da Etapa 2 já consome `PageReader` sem saber que o cache existe).

- [ ] **Step 1: Escrever os testes novos (falhando)**

Adicionar ao final de `internal/reader/reader_test.go`:

```go
func TestServicePageReaderUsesPrefetchedBytes(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{
		"page0.jpg": tinyJPEG, "page1.jpg": tinyJPEG, "page2.jpg": tinyJPEG,
	})
	s := NewService()
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close(res.BookID)

	if err := s.SetCurrentPage(res.BookID, 1); err != nil {
		t.Fatalf("SetCurrentPage: %v", err)
	}

	s.mu.RLock()
	entry := s.books[res.BookID]
	s.mu.RUnlock()
	waitForSlot(t, entry.prefetch, 1)

	rc, err := s.PageReader(res.BookID, 1)
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

func TestServicePageReaderFallsBackWhenNotPrefetched(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page0.jpg": tinyJPEG})
	s := NewService()
	res, err := s.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close(res.BookID)

	// Nenhum SetCurrentPage foi chamado, entao o cache esta vazio — o
	// caminho de leitura direta do archive (cache-through) deve funcionar
	// normalmente.
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
```

Adicionar ao final de `internal/reader/handler_test.go`:

```go
func TestPageHandlerServesPrefetchedBytes(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{
		"page0.jpg": tinyJPEG, "page1.jpg": tinyJPEG, "page2.jpg": tinyJPEG,
	})
	svc := NewService()
	res, err := svc.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer svc.Close(res.BookID)

	if err := svc.SetCurrentPage(res.BookID, 1); err != nil {
		t.Fatalf("SetCurrentPage: %v", err)
	}
	svc.mu.RLock()
	entry := svc.books[res.BookID]
	svc.mu.RUnlock()
	waitForSlot(t, entry.prefetch, 1)

	handler := PageHandler(svc)
	req := httptest.NewRequest("GET", "/page/"+res.BookID+"/1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != string(tinyJPEG) {
		t.Errorf("body = %v, want %v", rec.Body.Bytes(), tinyJPEG)
	}
}
```

- [ ] **Step 2: Rodar e confirmar que falha**

```bash
go test ./internal/reader/... -run 'TestServicePageReaderUsesPrefetchedBytes|TestPageHandlerServesPrefetchedBytes' -v
```
Esperado: o teste `TestServicePageReaderUsesPrefetchedBytes` PASSA mesmo antes da mudança (o cache-through já retorna os bytes corretos, só que sem usar o cache) — isso é esperado, TDD aqui está testando comportamento observável, não implementação interna. `TestServicePageReaderFallsBackWhenNotPrefetched` e `TestPageHandlerServesPrefetchedBytes` também devem passar já. Ainda assim, prossiga para o Step 3: o objetivo desta task não é fazer um teste vermelho ficar verde, é garantir que o caminho de cache realmente é exercitado — confirme isso adicionando temporariamente um `panic("cache nao usado")` no branch de cache-hit do Step 3 antes de implementá-lo de verdade, rodando `TestServicePageReaderUsesPrefetchedBytes` e confirmando que ele AGORA falha com esse panic (prova que o teste realmente passa pelo cache quando ele existe) — depois remova o panic e implemente a lógica real.

- [ ] **Step 3: Implementar**

Substituir o corpo de `PageReader` em `internal/reader/reader.go`:

```go
func (s *Service) PageReader(bookID string, index int) (io.ReadCloser, error) {
	entry, err := s.get(bookID)
	if err != nil {
		return nil, err
	}
	if data, ok := entry.prefetch.Get(index); ok {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	return entry.archive.Open(index)
}
```

Adicionar `"bytes"` aos imports de `internal/reader/reader.go`.

- [ ] **Step 4: Rodar e confirmar que passa**

```bash
go test ./internal/reader/... -race -v
```
Esperado: todos os testes (Tasks 1-3 + os 3 novos desta task) em PASS, sem avisos de race.

- [ ] **Step 5: Confirmar que o resto do projeto ainda compila**

```bash
go build ./...
go vet ./...
go test ./...
```
Esperado: sucesso em tudo — em particular, os testes de `internal/archive` (Etapa 1) e os testes de `handler.go` já existentes da Etapa 2 continuam passando sem modificação.

- [ ] **Step 6: Commit**

```bash
git add internal/reader/reader.go internal/reader/reader_test.go internal/reader/handler_test.go
git commit -m "reader: PageReader consulta o cache de prefetch antes do archive"
```

---

## Self-Review

**Cobertura do spec:** estado da página atual no backend (§3.2, "nunca só em useState do React") → Task 1 e Task 3 (`SetCurrentPage`/`CurrentPage`, acionado pelo backend, não pelo front); prefetch coordinator com janela N±2, cancelamento via context, ring buffer de 5 posições (§3.4) → Task 2; integração do coordinator ao ciclo de vida do livro (criado no `Open`, cancelado no `Close`) → Task 3; handler HTTP servindo do cache quando disponível, sem mudar sua própria assinatura ou lógica → Task 4. Nenhum requisito do §3.2/§3.4 relevante a esta etapa ficou sem task correspondente — zoom/pan, modos de página, atalhos e a migração de frontend ficam para a Etapa 3b (fora de escopo deste plano, conforme já declarado nas Global Constraints).

**Placeholders:** nenhum "TBD"/"TODO". O Step 2 da Task 4 pede uma verificação deliberada (inserir e remover um `panic` temporário) para confirmar que o caminho de cache é de fato exercitado, já que o teste em si passaria mesmo sem a mudança (está testando comportamento observável idêntico nos dois caminhos) — isso é uma instrução concreta, não uma lacuna.

**Consistência de tipos:** `bookEntry{archive, currentPage, prefetch}`, `prefetchCoordinator` e seus métodos (`SetCurrent`, `Get`, `Cancel`), `SetCurrentPage`/`CurrentPage` usados com a mesma assinatura em todas as tasks que os consomem. `openTestArchiveWithPages`/`waitForSlot` (definidos em `prefetch_test.go` na Task 2) são reaproveitados sem redefinição nas Tasks 3 e 4 (mesmo pacote). O acesso direto a `s.books[...]` e `s.mu` nos testes das Tasks 3-4 é válido porque os testes vivem no mesmo pacote `reader` (não é um pacote `_test` externo) — padrão já estabelecido nas Etapas 1-2 deste projeto para inspecionar estado interno em testes.
