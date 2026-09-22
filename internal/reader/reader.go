package reader

import (
	"bytes"
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
	prefetch    *prefetchCoordinator
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
	s.books[id] = &bookEntry{archive: a, prefetch: newPrefetchCoordinator(a)}
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
	if data, ok := entry.prefetch.Get(index); ok {
		return io.NopCloser(bytes.NewReader(data)), nil
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

	entry.prefetch.SetCurrent(index)
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
	entry.prefetch.Cancel()
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
