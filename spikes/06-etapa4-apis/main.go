// SPIKE (Etapa 4, Task 1) — codigo descartavel. Valida contra o Wails v3
// v3.0.0-beta.23 real: dialogo nativo de pasta, Emit de goroutines,
// formato dos bindings e ciclo ServiceStartup/ServiceShutdown.
// Resultados: spikes/06-etapa4-apis-finding.md.
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	// Evento registrado: o gerador de bindings tipa e.data no JS, e o Emit
	// valida o tipo do payload em tempo de execucao.
	application.RegisterEvent[BurstEvent]("burst")
}

func main() {
	logf("main: process start")
	svc := &SpikeService{}

	app := application.New(application.Options{
		Name:        "spike06",
		Description: "Spike Etapa 4 (APIs do Wails)",
		Services: []application.Service{
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "spike06",
		Width:            900,
		Height:           600,
		BackgroundColour: application.NewRGB(255, 255, 255),
		URL:              "/",
	})

	logf("main: calling app.Run")
	err := app.Run()
	logf("main: app.Run returned err=%v", err)
	if err != nil {
		log.Fatal(err)
	}
}
