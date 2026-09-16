package archive

import (
	"archive/zip"
	"io"
	"strings"
)

type zipArchive struct {
	reader *zip.ReadCloser
	pages  []PageEntry
	byName map[string]*zip.File
}

func newZipArchive(path string) (Archive, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, ErrCorrupted
	}

	var entries []rawEntry
	byName := make(map[string]*zip.File)
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		entries = append(entries, rawEntry{Name: f.Name, Size: int64(f.UncompressedSize64)})
		byName[f.Name] = f
	}

	return &zipArchive{
		reader: r,
		pages:  buildPageList(entries),
		byName: byName,
	}, nil
}

func (z *zipArchive) Pages() []PageEntry { return z.pages }

func (z *zipArchive) Open(index int) (io.ReadCloser, error) {
	if index < 0 || index >= len(z.pages) {
		return nil, ErrCorrupted
	}
	f, ok := z.byName[z.pages[index].Name]
	if !ok {
		return nil, ErrCorrupted
	}
	rc, err := f.Open()
	if err != nil {
		return nil, ErrCorrupted
	}
	return rc, nil
}

func (z *zipArchive) Metadata() (*ComicInfo, error) {
	for name, f := range z.byName {
		if strings.EqualFold(name, "ComicInfo.xml") {
			rc, err := f.Open()
			if err != nil {
				return nil, ErrCorrupted
			}
			defer rc.Close()
			return parseComicInfo(rc)
		}
	}
	return nil, nil
}

func (z *zipArchive) Close() error {
	return z.reader.Close()
}
