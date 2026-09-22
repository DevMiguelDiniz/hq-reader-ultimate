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

// TestPrefetchCoordinatorStaysWithinFiveSlotBound é a assertiva direta da
// garantia do §3.4 ("janela de até 5 páginas"): percorrer o livro inteiro,
// página a página, nunca deve deixar mais que 2*prefetchWindow+1 slots em
// cache simultaneamente. Antes do fix do bug ABA (goroutine antiga cancelada
// apagando incondicionalmente o registro de um fetch mais novo pro mesmo
// índice), esse teste podia transientemente ver mais de 5 slots: um fetch
// cancelado que termina tarde conseguia gravar em p.slots um índice que já
// devia ter sido evictado, porque seu cancelamento tinha sido "perdido"
// pela goroutine mais nova que reusou o mesmo índice.
func TestPrefetchCoordinatorStaysWithinFiveSlotBound(t *testing.T) {
	const totalPages = 20
	a := openTestArchiveWithPages(t, totalPages)
	p := newPrefetchCoordinator(a)

	const maxSlots = 2*prefetchWindow + 1

	for i := 0; i < totalPages; i++ {
		p.SetCurrent(i)

		// Da uma chance pras goroutines de fetch/cancelamento em andamento
		// rodarem, pra aumentar a chance de expor uma condição de corrida
		// (sem -race disponivel nesta maquina, isso e best-effort).
		time.Sleep(2 * time.Millisecond)

		p.mu.Lock()
		n := len(p.slots)
		p.mu.Unlock()
		if n > maxSlots {
			t.Fatalf("apos SetCurrent(%d): len(p.slots) = %d, want <= %d (janela de %d slots)", i, n, maxSlots, maxSlots)
		}
	}

	// Espera os ultimos fetches disparados por SetCurrent(totalPages-1)
	// terminarem e confirma que o bound se mantem no estado final.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		n := len(p.slots)
		pending := len(p.cancel)
		p.mu.Unlock()
		if pending == 0 {
			if n > maxSlots {
				t.Fatalf("estado final: len(p.slots) = %d, want <= %d", n, maxSlots)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("fetches nunca terminaram dentro do prazo")
}

// TestPrefetchCoordinatorCancelWaitsForInFlightGoroutines é a regressao do
// Finding 2: Cancel() precisa BLOQUEAR ate as goroutines de fetch em
// andamento realmente terminarem (via sync.WaitGroup), nao só sinalizar
// cancelamento e retornar. Isso importa porque quem chama Cancel()
// (Service.Close) em seguida fecha o archive — se uma goroutine ainda
// tivesse um arquivo aberto (caso de CBR/RAR com diretorio temporario), o
// Close() do archive podia falhar ou vazar o diretorio.
//
// Aqui verificamos isso indiretamente: apos SetCurrent disparar fetches e
// Cancel() retornar, tanto p.cancel quanto (por um curto periodo de
// observacao adicional) p.slots devem estar num estado estavel e vazio —
// nenhuma goroutine deveria estar escrevendo nada depois que Cancel()
// devolveu o controle, porque todas ja tem que ter terminado (Done() foi
// chamado antes do Wait() em Cancel() liberar).
func TestPrefetchCoordinatorCancelWaitsForInFlightGoroutines(t *testing.T) {
	a := openTestArchiveWithPages(t, 20)
	p := newPrefetchCoordinator(a)

	// Dispara fetches para varios indices e cancela imediatamente, sem dar
	// tempo pra nenhum deles terminar sozinho primeiro — o objetivo e pegar
	// Cancel() com fetches genuinamente em andamento.
	p.SetCurrent(10)
	p.Cancel()

	p.mu.Lock()
	pendingAfterCancel := len(p.cancel)
	slotsAfterCancel := len(p.slots)
	p.mu.Unlock()
	if pendingAfterCancel != 0 {
		t.Errorf("apos Cancel() retornar, p.cancel tem %d entradas, want 0 (Cancel deveria esperar todas as goroutines terminarem)", pendingAfterCancel)
	}
	if slotsAfterCancel != 0 {
		t.Errorf("apos Cancel() retornar, p.slots tem %d entradas, want 0 (nenhum fetch cancelado deveria ter conseguido escrever)", slotsAfterCancel)
	}

	// Observa por um periodo adicional: se Cancel() tivesse retornado cedo
	// demais (sem esperar as goroutines), poderiamos ver p.slots ou
	// p.cancel mudarem depois do ponto acima. Com o WaitGroup correto, nada
	// deveria mudar, porque nao ha mais nenhuma goroutine viva tocando
	// nesse prefetchCoordinator.
	time.Sleep(100 * time.Millisecond)
	p.mu.Lock()
	pendingLater := len(p.cancel)
	slotsLater := len(p.slots)
	p.mu.Unlock()
	if pendingLater != 0 || slotsLater != 0 {
		t.Errorf("estado mudou depois de Cancel() retornar: p.cancel=%d p.slots=%d, want 0/0 — indica goroutine ainda viva apos Cancel()", pendingLater, slotsLater)
	}
}
