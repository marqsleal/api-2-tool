package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/marqsleal/api-2-tool/internal/service"
)

func TestHealthHandler(t *testing.T) {
	h := NewHealthHandler(service.NewHealthService())

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/health", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405")
	}
}
