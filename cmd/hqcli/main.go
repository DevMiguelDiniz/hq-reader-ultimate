package main

import (
	"fmt"
	"io"
	"os"

	"github.com/DevMiguelDiniz/hq-reader-ultimate/internal/archive"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "uso: hqcli list <arquivo> | hqcli dump <arquivo> <indice>")
		os.Exit(1)
	}

	cmd, path := os.Args[1], os.Args[2]

	a, err := archive.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro ao abrir %s: %v\n", path, err)
		os.Exit(1)
	}
	defer a.Close()

	switch cmd {
	case "list":
		runList(a)
	case "dump":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "uso: hqcli dump <arquivo> <indice>")
			os.Exit(1)
		}
		runDump(a, os.Args[3])
	default:
		fmt.Fprintf(os.Stderr, "comando desconhecido: %s\n", cmd)
		os.Exit(1)
	}
}

func runList(a archive.Archive) {
	info, err := a.Metadata()
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro ao ler metadados: %v\n", err)
	} else if info != nil {
		fmt.Printf("Título: %s | Série: %s #%s\n", info.Title, info.Series, info.Number)
	}
	for _, p := range a.Pages() {
		fmt.Printf("%3d  %-40s  %8d bytes  %s\n", p.Index, p.Name, p.Size, p.ContentType)
	}
}

func runDump(a archive.Archive, indexArg string) {
	var index int
	if _, err := fmt.Sscanf(indexArg, "%d", &index); err != nil {
		fmt.Fprintf(os.Stderr, "índice inválido: %s\n", indexArg)
		os.Exit(1)
	}
	rc, err := a.Open(index)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro ao abrir página %d: %v\n", index, err)
		os.Exit(1)
	}
	defer rc.Close()
	if _, err := io.Copy(os.Stdout, rc); err != nil {
		fmt.Fprintf(os.Stderr, "erro ao copiar: %v\n", err)
		os.Exit(1)
	}
}
