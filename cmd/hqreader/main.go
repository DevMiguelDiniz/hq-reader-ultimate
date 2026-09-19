package main

import (
	"embed"
	"log"
	"net/http"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/DevMiguelDiniz/hq-reader-ultimate/internal/archive"
	"github.com/DevMiguelDiniz/hq-reader-ultimate/internal/reader"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Clean up orphaned temp directories from previous sessions that may have crashed
	if err := archive.CleanOrphanedTempDirs(); err != nil {
		log.Printf("aviso: falha ao limpar diretorios temp orfaos: %v", err)
	}

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
