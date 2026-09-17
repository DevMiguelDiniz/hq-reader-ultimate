package archive

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
			return nil, fmt.Errorf("archive: nao foi possivel abrir %s: %w", path, err)
		}
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

func (z *zipArchive) Pages() []PageEntry {
	pagesCopy := make([]PageEntry, len(z.pages))
	copy(pagesCopy, z.pages)
	return pagesCopy
}

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
			info, err := parseComicInfo(rc)
			if err != nil {
				return nil, fmt.Errorf("archive: ComicInfo.xml malformado: %w", ErrCorrupted)
			}
			return info, nil
		}
	}
	return nil, nil
}

func (z *zipArchive) Close() error {
	return z.reader.Close()
}
