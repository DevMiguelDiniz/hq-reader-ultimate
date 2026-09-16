package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/nwaples/rardecode/v2"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("uso: rar-cost <arquivo.cbr>")
		os.Exit(1)
	}
	path := os.Args[1]

	start := time.Now()
	r, err := rardecode.OpenReader(path)
	if err != nil {
		fmt.Printf("erro ao abrir %s: %v\n", path, err)
		os.Exit(1)
	}
	defer r.Close()

	entryCount := 0
	solidEntryCount := 0
	var firstEntryAt time.Duration

	for {
		hdr, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("erro lendo cabecalho da entrada %d: %v\n", entryCount, err)
			os.Exit(1)
		}

		if _, err := io.Copy(io.Discard, r); err != nil {
			fmt.Printf("erro decodificando entrada %d (%s): %v\n", entryCount, hdr.Name, err)
			os.Exit(1)
		}

		if hdr.Solid {
			solidEntryCount++
		}

		entryCount++
		if entryCount == 1 {
			firstEntryAt = time.Since(start)
		}
	}

	total := time.Since(start)
	fmt.Printf("arquivo: %s\n", path)
	fmt.Printf("entradas decodificadas: %d\n", entryCount)
	fmt.Printf("entradas marcadas como solid (hdr.Solid): %d\n", solidEntryCount)
	fmt.Printf("tempo ate a 1a entrada: %v\n", firstEntryAt)
	fmt.Printf("tempo total (todas as entradas ate o fim): %v\n", total)
}
