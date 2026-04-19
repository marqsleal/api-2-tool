package router

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/marqsleal/api-2-tool/internal/http/handler"
	"github.com/marqsleal/api-2-tool/internal/repository"
	"github.com/marqsleal/api-2-tool/internal/service"
)

func TestRouterRoutes(t *testing.T) {
	repo := repository.NewInMemoryToolDefinitionRepository()
	defSvc := service.NewToolDefinitionService(repo)
	execSvc := service.NewToolExecutorService(defSvc)

	healthHandler := handler.NewHealthHandler(service.NewHealthService())
	toolHandler := handler.NewToolHandler(defSvc, execSvc, nil)

	tmpDir := t.TempDir()
	spec := filepath.Join(tmpDir, "openapi.yaml")
	if err := os.WriteFile(spec, []byte("openapi: 3.0.3\n"), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	r := New(healthHandler, toolHandler, handler.NewSwaggerUIHandler("/swagger/doc.yaml"), handler.NewOpenAPISpecHandler(spec))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected health 200")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/swagger/doc.yaml", nil)
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected swagger doc 200")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil)
	r.ServeHTTP(rr, req)
	if rr.Code == http.StatusInternalServerError {
		t.Fatalf("unexpected 500 from swagger ui")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/tool/definitions", nil)
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected tool definitions 200, got %d", rr.Code)
	}
}
