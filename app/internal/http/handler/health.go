package handler

import (
	"net/http"

	"github.com/marqsleal/api-2-tool/internal/http/response"
	"github.com/marqsleal/api-2-tool/internal/service"
)

type HealthHandler struct {
	service service.HealthService
}

func NewHealthHandler(service service.HealthService) HealthHandler {
	return HealthHandler{service: service}
}

func (h HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	response.JSON(w, http.StatusOK, h.service.Status())
}
