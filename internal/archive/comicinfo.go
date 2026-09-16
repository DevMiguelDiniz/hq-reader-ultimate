package archive

import (
	"encoding/xml"
	"io"
)

// parseComicInfo decodifica um ComicInfo.xml a partir de r.
func parseComicInfo(r io.Reader) (*ComicInfo, error) {
	var info ComicInfo
	if err := xml.NewDecoder(r).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}
