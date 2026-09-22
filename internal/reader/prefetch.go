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
