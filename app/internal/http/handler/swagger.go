package handler

import (
	"net/http"

	httpSwagger "github.com/swaggo/http-swagger/v2"
)

type OpenAPISpecHandler struct {
	specPath string
}

func NewOpenAPISpecHandler(specPath string) OpenAPISpecHandler {
	return OpenAPISpecHandler{specPath: specPath}
}

func (h OpenAPISpecHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/yaml")
	http.ServeFile(w, r, h.specPath)
}

func NewSwaggerUIHandler(specURL string) http.Handler {
	return httpSwagger.Handler(
		httpSwagger.URL(specURL),
		httpSwagger.DocExpansion("none"),
		httpSwagger.DeepLinking(true),
	)
}
