package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// ---- log em arquivo: toda evidencia do spike (Go e o que o frontend reporta)
// vai para SPIKE_LOG (ou %TEMP%\spike06.log), para ser capturada depois.

var logMu sync.Mutex

func logf(format string, args ...any) {
	path := os.Getenv("SPIKE_LOG")
	if path == "" {
		path = filepath.Join(os.TempDir(), "spike06.log")
	}
	line := fmt.Sprintf("%s pid=%d %s\n", time.Now().Format("15:04:05.000"), os.Getpid(), fmt.Sprintf(format, args...))
	logMu.Lock()
	defer logMu.Unlock()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprint(os.Stderr, line)
		return
	}
	defer f.Close()
	f.WriteString(line)
}

// ---- tipos para o item (c): como os bindings tipam cada forma.

type Item struct {
	Name string
	Size int64
}

type Inner struct {
	Label string
	Items []Item
}

type SampleResult struct {
	ID         int64
	Title      string
	Done       bool
	Count      int
	Ratio      float64
	Inner      Inner
	InnerPtr   *Inner // deixado nil de proposito
	FilledPtr  *Inner
	Items      []Item
	NilItems   []Item // nil de proposito
	Tags       map[string]int
	Tagged     string `json:"taggedField"`
	Omitted    string `json:"omittedField,omitempty"`
	unexported string
}

// BurstEvent: payload de evento SEM tags json (item b: caixa dos campos no JS).
type BurstEvent struct {
	Worker int
	Seq    int
	Global int64
	Path   string
	Ok     bool
}

type SpikeService struct {
	mu       sync.Mutex
	startCtx context.Context
}

// ---- (d) ciclo de vida

func (s *SpikeService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	s.startCtx = ctx
	app := application.Get()
	logf("ServiceStartup: called; application.Get()!=nil=%v; options.Name=%q options.Route=%q; windows=%d",
		app != nil, options.Name, options.Route, len(app.Window.GetAll()))
	go func() {
		<-ctx.Done()
		logf("ServiceStartup ctx: Done (err=%v)", ctx.Err())
	}()
	// Emitir durante o startup: o frontend ainda nao existe; ele reporta se viu.
	app.Event.Emit("startup-ping", "emitido dentro de ServiceStartup")
	return nil
}

func (s *SpikeService) ServiceShutdown() error {
	logf("ServiceShutdown: called; startCtx.Err()=%v", s.startCtx.Err())
	return nil
}

// ---- (a) dialogo nativo de pasta

func (s *SpikeService) PickFolder() (string, error) {
	t0 := time.Now()
	logf("PickFolder: opening dialog")
	path, err := application.Get().Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		CanChooseDirectories: true,
		CanChooseFiles:       false,
		Title:                "Escolha uma pasta (spike06)",
		Directory:            os.Getenv("SPIKE_PICK_DIR"), // pasta inicial (verify-ui.ps1 -Select)
	}).PromptForSingleSelection()
	errType := "<nil>"
	if err != nil {
		errType = fmt.Sprintf("%T", err)
	}
	logf("PickFolder: returned path=%q (len=%d) err=%v errType=%s errNil=%v elapsed=%s",
		path, len(path), err, errType, err == nil, time.Since(t0))
	return path, err
}

// ---- (b) eventos de goroutines

func (s *SpikeService) EmitBurst(n int) int {
	const workers = 8
	app := application.Get()
	logf("EmitBurst: n=%d workers=%d start", n, workers)
	t0 := time.Now()
	var global int64
	var gmu sync.Mutex
	var wg sync.WaitGroup
	per := n / workers
	for w := 0; w < workers; w++ {
		count := per
		if w == workers-1 {
			count = n - per*(workers-1)
		}
		wg.Add(1)
		go func(w, count int) {
			defer wg.Done()
			for i := 0; i < count; i++ {
				gmu.Lock()
				global++
				g := global
				gmu.Unlock()
				app.Event.Emit("burst", BurstEvent{Worker: w, Seq: i, Global: g, Path: fmt.Sprintf(`C:\Comics\Ação %d\%d.cbz`, w, i), Ok: true})
			}
		}(w, count)
	}
	wg.Wait()
	logf("EmitBurst: all %d Emit calls returned in %s", n, time.Since(t0))
	app.Event.Emit("burst-done", n)
	return n
}

// ---- (c) formato dos bindings

func (s *SpikeService) Sample() SampleResult {
	return SampleResult{
		ID:        9007199254740993, // 2^53+1: perde precisao em number do JS?
		Title:     "Edição Especial",
		Done:      true,
		Count:     3,
		Ratio:     0.5,
		Inner:     Inner{Label: "inner", Items: []Item{{Name: "a.png", Size: 10}}},
		InnerPtr:  nil,
		FilledPtr: &Inner{Label: "filled"},
		Items:     []Item{{Name: "x", Size: 1}, {Name: "y", Size: 2}},
		NilItems:  nil,
		Tags:      map[string]int{"k": 1},
		Tagged:    "t",
	}
}

// Slow bloqueia ms milissegundos (testa se chamadas concorrentes do frontend
// sao atendidas em paralelo).
func (s *SpikeService) Slow(ms int) int {
	logf("Slow: start ms=%d", ms)
	time.Sleep(time.Duration(ms) * time.Millisecond)
	logf("Slow: end ms=%d", ms)
	return ms
}

// Report: canal do frontend para o log (evidencia do lado JS).
func (s *SpikeService) Report(line string) {
	logf("JS: %s", line)
}
