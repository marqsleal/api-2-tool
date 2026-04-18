package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenAPISpecHandler(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "openapi.yaml")
	if err := os.WriteFile(specPath, []byte("openapi: 3.0.3\n"), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	h := NewOpenAPISpecHandler(specPath)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/swagger/doc.yaml", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "application/yaml" {
		t.Fatalf("expected yaml content-type")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/swagger/doc.yaml", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405")
	}
}

func TestSwaggerUIHandler(t *testing.T) {
	h := NewSwaggerUIHandler("/swagger/doc.yaml")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil)
	h.ServeHTTP(rr, req)
	if rr.Code == http.StatusInternalServerError {
		t.Fatalf("unexpected 500 from swagger ui")
	}
}
