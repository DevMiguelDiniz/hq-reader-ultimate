//go:build ignore

// Gera o CBZ sintetico de 10 paginas usado por verify-reader-interactions.ps1
// e pelas verificacoes manuais da Etapa 3c. Nao faz parte do build.
//
// Uso (de cmd/hqreader): go run make-verify-cbz.go <saida.cbz>
package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

type page struct {
	name    string
	w, h    int
	corrupt bool
}

func solidPNG(w, h int, c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "uso: go run make-verify-cbz.go <saida.cbz>")
		os.Exit(1)
	}

	pages := []page{
		{name: "page01.png", w: 600, h: 900},                // 0 capa
		{name: "page02.png", w: 600, h: 900},                // 1
		{name: "page03.png", w: 600, h: 900},                // 2
		{name: "page04.png", w: 600, h: 900},                // 3
		{name: "page05.png", w: 1200, h: 900},               // 4 spread
		{name: "page06.png", w: 600, h: 900},                // 5
		{name: "page07.png", w: 600, h: 900, corrupt: true}, // 6 corrompida
		{name: "page08.png", w: 600, h: 900},                // 7
		{name: "page09.png", w: 600, h: 900},                // 8
		{name: "page10.png", w: 600, h: 900},                // 9
	}

	f, err := os.Create(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for i, p := range pages {
		w, err := zw.Create(p.name)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		var data []byte
		if p.corrupt {
			data = []byte("isto nao e um png")
		} else {
			data = solidPNG(p.w, p.h, color.RGBA{R: uint8(40 + i*20), G: uint8(200 - i*15), B: 120, A: 255})
		}
		if _, err := w.Write(data); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if err := zw.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
