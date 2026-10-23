// SPIKE (Etapa 4, Task 1) — custo de inspecionar CBR. Descartavel.
// Compara (A) percorrer so cabecalhos com rardecode Next() guardando apenas o
// ComicInfo.xml em memoria contra (B) o archive.Open atual (extrai tudo para
// disco) + Pages() + Metadata() + Close().
// Uso: go run ./cmd/cbrbench <arquivo.cbr>...
package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/DevMiguelDiniz/hq-reader-ultimate/internal/archive"
	"github.com/nwaples/rardecode/v2"
)

var imageExt = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".bmp": true, ".avif": true}

func magic(p string) string {
	f, err := os.Open(p)
	if err != nil {
		return err.Error()
	}
	defer f.Close()
	b := make([]byte, 8)
	io.ReadFull(f, b)
	switch {
	case bytes.HasPrefix(b, []byte("Rar!\x1a\x07\x01\x00")):
		return "RAR5"
	case bytes.HasPrefix(b, []byte("Rar!\x1a\x07\x00")):
		return "RAR4"
	case bytes.HasPrefix(b, []byte("PK")):
		return "ZIP"
	}
	return fmt.Sprintf("unknown % x", b)
}

type walk struct {
	entries, images, solid, firstSolidIdx, comicInfoIdx int
	comicInfoLen                                        int
	firstHeader, total                                  time.Duration
	err                                                 error
}

func headerWalk(p string) walk {
	w := walk{firstSolidIdx: -1, comicInfoIdx: -1}
	t0 := time.Now()
	r, err := rardecode.OpenReader(p)
	if err != nil {
		w.err = err
		return w
	}
	defer r.Close()
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			w.err = err
			break
		}
		if w.entries == 0 {
			w.firstHeader = time.Since(t0)
		}
		if h.Solid {
			w.solid++
			if w.firstSolidIdx < 0 {
				w.firstSolidIdx = w.entries
			}
		}
		if !h.IsDir && imageExt[strings.ToLower(path.Ext(h.Name))] {
			w.images++
		}
		if strings.EqualFold(path.Base(strings.ReplaceAll(h.Name, `\`, "/")), "ComicInfo.xml") {
			b, err := io.ReadAll(r) // unico corpo lido
			if err != nil {
				w.err = err
			}
			w.comicInfoIdx = w.entries
			w.comicInfoLen = len(b)
		}
		w.entries++
	}
	w.total = time.Since(t0)
	return w
}

func fullOpen(p string) (pages int, hasMeta bool, open, total time.Duration, err error) {
	t0 := time.Now()
	a, err := archive.Open(p)
	if err != nil {
		return 0, false, 0, 0, err
	}
	open = time.Since(t0)
	pages = len(a.Pages())
	md, _ := a.Metadata()
	hasMeta = md != nil
	a.Close()
	return pages, hasMeta, open, time.Since(t0), nil
}

// decodeAll le o corpo de TODAS as entradas para io.Discard: e o trabalho que
// Next() e obrigado a fazer para pular entradas num RAR solido (medida-proxy
// do pior caso quando nao ha amostra solida).
func decodeAll(p string) (time.Duration, int64, error) {
	t0 := time.Now()
	r, err := rardecode.OpenReader(p)
	if err != nil {
		return 0, 0, err
	}
	defer r.Close()
	var n int64
	for {
		_, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, n, err
		}
		c, err := io.Copy(io.Discard, r)
		n += c
		if err != nil {
			return 0, n, err
		}
	}
	return time.Since(t0), n, nil
}

func zipList(p string) (entries, images int, el time.Duration, err error) {
	t0 := time.Now()
	z, err := zip.OpenReader(p)
	if err != nil {
		return 0, 0, 0, err
	}
	for _, f := range z.File {
		entries++
		if imageExt[strings.ToLower(path.Ext(f.Name))] {
			images++
		}
	}
	z.Close()
	return entries, images, time.Since(t0), nil
}

const iters = 20

func main() {
	for _, p := range os.Args[1:] {
		st, err := os.Stat(p)
		if err != nil {
			fmt.Println("stat:", err)
			continue
		}
		m := magic(p)
		fmt.Printf("== %s\n   size=%d bytes magic=%s\n", path.Base(strings.ReplaceAll(p, `\`, "/")), st.Size(), m)

		pages, meta, open, total, err := fullOpen(p)
		fmt.Printf("   B archive.Open(as named): pages=%d metadata=%v open=%s total=%s err=%v\n", pages, meta, open.Round(time.Millisecond), total.Round(time.Millisecond), err)

		if m == "ZIP" {
			e, im, _, err := zipList(p)
			t0 := time.Now()
			for i := 0; i < iters; i++ {
				zipList(p)
			}
			fmt.Printf("   D archive/zip listing: entries=%d images=%d avg over %d=%s err=%v\n", e, im, iters, (time.Since(t0) / iters).Round(time.Microsecond), err)
			link := os.TempDir() + `\spike06-sniff.cbz`
			os.Remove(link)
			if err := os.Link(p, link); err != nil {
				fmt.Println("   link:", err)
				continue
			}
			for run := 1; run <= 3; run++ {
				pages, meta, open, total, err := fullOpen(link)
				fmt.Printf("   run%d E archive.Open(same bytes renamed .cbz): pages=%d metadata=%v open=%s total=%s err=%v\n", run, pages, meta, open.Round(time.Millisecond), total.Round(time.Millisecond), err)
			}
			os.Remove(link)
			continue
		}

		cold := headerWalk(p)
		fmt.Printf("   A header-walk (1st): entries=%d images=%d solidEntries=%d firstSolidIdx=%d comicInfoIdx=%d comicInfoBytes=%d total=%s err=%v\n",
			cold.entries, cold.images, cold.solid, cold.firstSolidIdx, cold.comicInfoIdx, cold.comicInfoLen, cold.total.Round(time.Microsecond), cold.err)
		t0 := time.Now()
		for i := 0; i < iters; i++ {
			headerWalk(p)
		}
		fmt.Printf("   A header-walk avg over %d: %s\n", iters, (time.Since(t0) / iters).Round(time.Microsecond))
		for run := 1; run <= 3; run++ {
			el, n, err := decodeAll(p)
			fmt.Printf("   run%d C decode-all to io.Discard: %s (%d bytes) err=%v\n", run, el.Round(time.Millisecond), n, err)
			pages, meta, open, total, err := fullOpen(p)
			fmt.Printf("   run%d B archive.Open: pages=%d metadata=%v open=%s open+pages+meta+close=%s err=%v\n",
				run, pages, meta, open.Round(time.Millisecond), total.Round(time.Millisecond), err)
		}
	}
}
