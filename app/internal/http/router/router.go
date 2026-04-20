package router

import (
	"net/http"

	"github.com/marqsleal/api-2-tool/internal/http/handler"
	"github.com/marqsleal/api-2-tool/internal/http/middleware"
)

func New(
	healthHandler handler.HealthHandler,
	toolHandler handler.ToolHandler,
	swaggerUIHandler http.Handler,
	openAPISpecHandler handler.OpenAPISpecHandler,
) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/health", healthHandler)
	mux.Handle("/tool", toolHandler)
	mux.Handle("/tool/definitions", toolHandler)
	mux.Handle("/tool/definitions/", toolHandler)
	mux.Handle("/tool/execute/", toolHandler)
	mux.Handle("/tool/jobs/", toolHandler)
	mux.Handle("/swagger/", swaggerUIHandler)
	mux.Handle("/swagger/doc.yaml", openAPISpecHandler)
	return middleware.RequestID(middleware.RequestLogger(mux))
}
