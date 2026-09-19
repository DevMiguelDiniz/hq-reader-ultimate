package reader

import (
	"net/http/httptest"
	"testing"
)

func TestPageHandlerServesBytes(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG})
	svc := NewService()
	res, err := svc.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// necessário no Windows: sem isso, t.TempDir() falha ao limpar porque o arquivo zip aberto pelo Service ainda segura um handle
	defer svc.Close(res.BookID)

	handler := PageHandler(svc)
	req := httptest.NewRequest("GET", "/page/"+res.BookID+"/0", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Cache-Control") != "immutable" {
		t.Errorf("Cache-Control = %q, want immutable", rec.Header().Get("Cache-Control"))
	}
	if rec.Body.String() != string(tinyJPEG) {
		t.Errorf("body = %v, want %v", rec.Body.Bytes(), tinyJPEG)
	}
}

func TestPageHandlerUnknownBook(t *testing.T) {
	svc := NewService()
	handler := PageHandler(svc)
	req := httptest.NewRequest("GET", "/page/nao-existe/0", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestPageHandlerIndexOutOfRange(t *testing.T) {
	path := writeTestZip(t, map[string][]byte{"page1.jpg": tinyJPEG})
	svc := NewService()
	res, err := svc.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// necessário no Windows: sem isso, t.TempDir() falha ao limpar porque o arquivo zip aberto pelo Service ainda segura um handle
	defer svc.Close(res.BookID)

	handler := PageHandler(svc)
	req := httptest.NewRequest("GET", "/page/"+res.BookID+"/99", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 500 {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestPageHandlerMalformedPath(t *testing.T) {
	svc := NewService()
	handler := PageHandler(svc)
	req := httptest.NewRequest("GET", "/page/semindice", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
