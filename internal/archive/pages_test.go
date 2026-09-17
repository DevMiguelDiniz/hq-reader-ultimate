package archive

import "testing"

func TestNaturalLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"page2.jpg", "page10.jpg", true},
		{"page10.jpg", "page2.jpg", false},
		{"page1.jpg", "page1.jpg", false},
		{"a/page2.jpg", "a/page10.jpg", true},
		{"b/page1.jpg", "a/page1.jpg", false},
	}
	for _, c := range cases {
		if got := naturalLess(c.a, c.b); got != c.want {
			t.Errorf("naturalLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestContentTypeForName(t *testing.T) {
	cases := []struct {
		name   string
		wantCT string
		wantOK bool
	}{
		{"page1.jpg", "image/jpeg", true},
		{"page1.JPEG", "image/jpeg", true},
		{"page1.png", "image/png", true},
		{"page1.webp", "image/webp", true},
		{"ComicInfo.xml", "", false},
		{"Thumbs.db", "", false},
		{"noext", "", false},
	}
	for _, c := range cases {
		ct, ok := contentTypeForName(c.name)
		if ct != c.wantCT || ok != c.wantOK {
			t.Errorf("contentTypeForName(%q) = (%q, %v), want (%q, %v)", c.name, ct, ok, c.wantCT, c.wantOK)
		}
	}
}

func TestBuildPageList(t *testing.T) {
	entries := []rawEntry{
		{Name: "ComicInfo.xml", Size: 100},
		{Name: "chapter1/page10.jpg", Size: 200},
		{Name: "chapter1/page2.jpg", Size: 150},
		{Name: "Thumbs.db", Size: 50},
		{Name: "chapter1/page1.png", Size: 300},
	}
	pages := buildPageList(entries)
	if len(pages) != 3 {
		t.Fatalf("got %d pages, want 3 (ComicInfo.xml e Thumbs.db devem ser filtrados)", len(pages))
	}
	wantOrder := []string{"chapter1/page1.png", "chapter1/page2.jpg", "chapter1/page10.jpg"}
	for i, w := range wantOrder {
		if pages[i].Name != w {
			t.Errorf("pages[%d].Name = %q, want %q", i, pages[i].Name, w)
		}
		if pages[i].Index != i {
			t.Errorf("pages[%d].Index = %d, want %d", i, pages[i].Index, i)
		}
	}
	if pages[0].ContentType != "image/png" {
		t.Errorf("pages[0].ContentType = %q, want image/png", pages[0].ContentType)
	}
}
