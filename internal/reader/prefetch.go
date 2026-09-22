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
	wg     sync.WaitGroup
	closed bool
	slots  map[int][]byte
	cancel map[int]*fetchHandle
}

// fetchHandle identifica um fetch específico em andamento. Como
// context.CancelFunc não é comparável entre si de forma útil (duas funções
// distintas nunca são ==, mesmo cancelando "a mesma coisa" conceitualmente),
// usamos o ponteiro do próprio handle como identidade: a goroutine que
// disparou o fetch guarda uma referência a ESTE handle e, ao terminar, só
// mexe em p.cancel[index] se o valor lá ainda for este mesmo ponteiro — ver
// comentário no startFetch sobre o bug ABA que isso evita.
type fetchHandle struct {
	cancel context.CancelFunc
}

func newPrefetchCoordinator(a archive.Archive) *prefetchCoordinator {
	return &prefetchCoordinator{
		archive: a,
		slots:   make(map[int][]byte),
		cancel:  make(map[int]*fetchHandle),
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
	if p.closed {
		p.mu.Unlock()
		return
	}
	for i, handle := range p.cancel {
		if !inWindow[i] {
			handle.cancel()
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

// startFetch dispara uma goroutine que lê a página index do archive e guarda
// o resultado em p.slots. Cada fetch tem um *fetchHandle próprio registrado
// em p.cancel[index]; a goroutine só remove essa entrada (ou escreve em
// p.slots) se p.cancel[index] ainda for O MESMO handle que ela criou.
//
// Isso evita um bug ABA: se SetCurrent cancela este fetch (índice saiu da
// janela) e remove p.cancel[index], mas a goroutine continua rodando
// (cancelamento é best-effort — io.ReadAll não é interrompido) e o usuário
// volta pro mesmo índice antes dela terminar, um NOVO fetch é registrado em
// p.cancel[index] com um handle diferente. Sem essa checagem de identidade,
// quando a goroutine antiga finalmente termina ela apagaria incondicionalmente
// o registro do fetch NOVO, tornando-o invisível a cancelamentos futuros.
func (p *prefetchCoordinator) startFetch(index int) {
	ctx, cancel := context.WithCancel(context.Background())
	handle := &fetchHandle{cancel: cancel}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.cancel[index] = handle
	p.wg.Add(1)
	p.mu.Unlock()

	go func() {
		defer p.wg.Done()

		rc, err := p.archive.Open(index)
		if err != nil {
			p.mu.Lock()
			if h, ok := p.cancel[index]; ok && h == handle {
				delete(p.cancel, index)
			}
			p.mu.Unlock()
			return
		}
		data, readErr := io.ReadAll(rc)
		rc.Close()

		p.mu.Lock()
		defer p.mu.Unlock()
		h, ok := p.cancel[index]
		if !ok || h != handle {
			// p.cancel[index] já foi assumido por um fetch mais novo pro
			// mesmo índice (ou removido) — este resultado está obsoleto,
			// descarta sem tocar no registro alheio.
			return
		}
		delete(p.cancel, index)
		if ctx.Err() != nil {
			// cancelado enquanto lia — descarta o resultado
			return
		}
		if readErr == nil && !p.closed {
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

// Cancel interrompe (best-effort) todos os fetches em andamento e ESPERA
// eles terminarem antes de retornar — chamado quando o livro é fechado, pra
// garantir que nenhuma goroutine ainda tenha um handle aberto no archive
// (via archive.Open) quando o chamador em seguida fecha o archive de fato.
// Sem esse Wait(), Service.Close() podia chamar archive.Close() enquanto uma
// goroutine de prefetch ainda segurava um arquivo aberto dentro do diretório
// temporário de um CBR — no Windows, remover um arquivo com handle aberto
// falha, o que propagava erro do Close() e vazava o diretório temporário.
//
// Marca p.closed = true (sob o lock) ANTES de cancelar, pra que fetches em
// andamento ou disparados numa corrida com SetCurrent não escrevam mais em
// p.slots nem sejam relançados por startFetch enquanto o Cancel() já está em
// curso.
func (p *prefetchCoordinator) Cancel() {
	p.mu.Lock()
	p.closed = true
	for i, handle := range p.cancel {
		handle.cancel()
		delete(p.cancel, i)
	}
	p.mu.Unlock()

	// Wait() é chamado FORA do lock: as goroutines em andamento precisam
	// adquirir p.mu pra rodar seu Done() (via defer, dentro do handler de
	// conclusão), então esperar aqui com o lock ainda seguro causaria
	// deadlock.
	p.wg.Wait()
}
