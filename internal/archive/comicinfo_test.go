package archive

import (
	"strings"
	"testing"
)

func TestParseComicInfo(t *testing.T) {
	xmlData := `<?xml version="1.0"?>
<ComicInfo>
  <Title>Os Fabulosos X-Men</Title>
  <Series>X-Men</Series>
  <Number>175</Number>
  <Writer>Chris Claremont</Writer>
</ComicInfo>`

	info, err := parseComicInfo(strings.NewReader(xmlData))
	if err != nil {
		t.Fatalf("parseComicInfo: %v", err)
	}
	if info.Title != "Os Fabulosos X-Men" {
		t.Errorf("Title = %q", info.Title)
	}
	if info.Series != "X-Men" {
		t.Errorf("Series = %q", info.Series)
	}
	if info.Number != "175" {
		t.Errorf("Number = %q", info.Number)
	}
	if info.Writer != "Chris Claremont" {
		t.Errorf("Writer = %q", info.Writer)
	}
}

func TestParseComicInfoInvalidXML(t *testing.T) {
	_, err := parseComicInfo(strings.NewReader("isso nao e xml"))
	if err == nil {
		t.Fatal("esperava erro para XML inválido, veio nil")
	}
}
