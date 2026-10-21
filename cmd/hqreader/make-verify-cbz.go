//go:build ignore

// Gera o CBZ sintetico de 10 paginas usado por verify-reader-interactions.ps1
// e pelas verificacoes manuais da Etapa 3c. Nao faz parte do build.
//
// Toda pagina nao corrompida e uma cor solida com uma faixa branca horizontal
// (40 px, centralizada na vertical, largura total) e uma faixa magenta
// vertical (40 px, centralizada na horizontal, altura total) usadas como
// marcadores de posicao; a pagina corrompida (indice 6) sao bytes que nao
// sao PNG.
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

// Faixa marcadora: branco puro (255,255,255), largura total da pagina,
// markerHeight px de altura, centralizada na vertical (paginas de 900 px:
// linhas 430..469). As cores solidas das paginas nunca sao branco puro, entao
// verify-reader-interactions.ps1 acha a faixa por pixel numa captura da
// janela e mede o deslocamento vertical do pan mesmo quando a pagina ampliada
// e mais alta que o viewport (as bordas da pagina somem).
const markerHeight = 40

// Faixa marcadora vertical: magenta puro (255,0,255), altura total da pagina,
// markerWidth px de largura, centralizada na horizontal (paginas de 600 px:
// colunas 280..319; spread de 1200 px: colunas 580..619). Na intersecao com a
// faixa branca vale o branco, entao a faixa branca continua de ponta a ponta.
// Nenhuma cor de pagina tem R=255, entao o magenta e inequivoco; com ele o
// script mede o pan HORIZONTAL quando a pagina ampliada e mais larga que o
// viewport (as bordas laterais nunca entram no viewport por causa do clamp).
const markerWidth = 40

func solidPNG(w, h int, c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	magenta := color.RGBA{R: 255, G: 0, B: 255, A: 255}
	markerTop := (h - markerHeight) / 2
	markerLeft := (w - markerWidth) / 2
	for y := 0; y < h; y++ {
		inWhite := y >= markerTop && y < markerTop+markerHeight
		for x := 0; x < w; x++ {
			px := c
			if inWhite {
				px = white
			} else if x >= markerLeft && x < markerLeft+markerWidth {
				px = magenta
			}
			img.SetRGBA(x, y, px)
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
