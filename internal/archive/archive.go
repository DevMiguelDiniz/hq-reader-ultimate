package archive

import (
	"encoding/xml"
	"errors"
	"io"
)

var (
	ErrPasswordProtected = errors.New("archive: protegido por senha")
	ErrCorrupted         = errors.New("archive: arquivo corrompido ou truncado")
	ErrUnsupportedFormat = errors.New("archive: formato não suportado")
)

// PageEntry descreve uma página já filtrada (só imagens) e ordenada
// naturalmente dentro do archive.
type PageEntry struct {
	Index       int
	Name        string
	Size        int64
	ContentType string
}

// ComicInfo mapeia o subconjunto de ComicInfo.xml usado pela aplicação.
type ComicInfo struct {
	XMLName xml.Name `xml:"ComicInfo"`
	Title   string   `xml:"Title"`
	Series  string   `xml:"Series"`
	Number  string   `xml:"Number"`
	Summary string   `xml:"Summary"`
	Writer  string   `xml:"Writer"`
}

// Archive é a interface comum entre CBZ e CBR. Pages() já vem ordenado
// naturalmente e filtrado só para entradas de imagem suportada.
type Archive interface {
	Pages() []PageEntry
	Open(index int) (io.ReadCloser, error)
	Metadata() (*ComicInfo, error)
	Close() error
}
