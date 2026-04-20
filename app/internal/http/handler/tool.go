package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/marqsleal/api-2-tool/internal/http/response"
	"github.com/marqsleal/api-2-tool/internal/service"
)

type ToolHandler struct {
	definitionService service.ToolDefinitionService
	executorService   service.ToolExecutorService
	jobService        *service.ToolJobService
}

func NewToolHandler(
	definitionService service.ToolDefinitionService,
	executorService service.ToolExecutorService,
	jobService *service.ToolJobService,
) ToolHandler {
	return ToolHandler{
		definitionService: definitionService,
		executorService:   executorService,
		jobService:        jobService,
	}
}

func (h ToolHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/tool" && r.Method == http.MethodPost:
		h.createTool(w, r)
		return
	case r.URL.Path == "/tool/definitions" && r.Method == http.MethodGet:
		h.listDefinitions(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/tool/definitions/") && r.Method == http.MethodGet:
		h.getDefinitionByID(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/tool/definitions/") && r.Method == http.MethodPatch:
		h.patchDefinitionByID(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/tool/definitions/") && r.Method == http.MethodDelete:
		h.deleteDefinitionByID(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/tool/execute/") && r.Method == http.MethodPost:
		if strings.HasSuffix(r.URL.Path, "/jobs") {
			h.enqueueToolJob(w, r)
			return
		}
		h.executeTool(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/tool/jobs/") && r.Method == http.MethodGet:
		h.getToolJobByID(w, r)
		return
	}

	response.Error(w, http.StatusNotFound, "route not found")
}

func (h ToolHandler) createTool(w http.ResponseWriter, r *http.Request) {
	var input service.ToolDefinitionInput
	if err := decodeJSON(r.Body, &input); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	definition, err := h.definitionService.Create(r.Context(), input)
	if err != nil {
		if err.Error() == "name, method and url are required" {
			response.Error(w, http.StatusBadRequest, err.Error())
			return
		}

		response.Error(w, http.StatusInternalServerError, "failed to create definition")
		return
	}

	response.JSON(w, http.StatusCreated, map[string]any{
		"id":     definition.ID,
		"active": definition.Active,
		"tool":   h.definitionService.ToToolFunction(definition),
		"upstream": map[string]any{
			"method":  definition.Method,
			"url":     definition.URL,
			"headers": definition.Headers,
		},
		"cache": map[string]any{
			"enabled":     definition.Cache.Enabled,
			"ttl_seconds": definition.Cache.TTLSeconds,
			"max_entries": definition.Cache.MaxEntries,
		},
	})
}

func (h ToolHandler) listDefinitions(w http.ResponseWriter, r *http.Request) {
	definitions, err := h.definitionService.List(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to list definitions")
		return
	}
	items := make([]map[string]any, 0, len(definitions))

	for _, definition := range definitions {
		items = append(items, map[string]any{
			"id":     definition.ID,
			"active": definition.Active,
			"tool":   h.definitionService.ToToolFunction(definition),
			"upstream": map[string]any{
				"method":  definition.Method,
				"url":     definition.URL,
				"headers": definition.Headers,
			},
			"cache": map[string]any{
				"enabled":     definition.Cache.Enabled,
				"ttl_seconds": definition.Cache.TTLSeconds,
				"max_entries": definition.Cache.MaxEntries,
			},
		})
	}

	response.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h ToolHandler) getDefinitionByID(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r.URL.Path, "/tool/definitions/")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	definition, ok, err := h.definitionService.GetByID(r.Context(), id)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to get definition")
		return
	}
	if !ok {
		response.Error(w, http.StatusNotFound, "definition not found")
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{
		"id":     definition.ID,
		"active": definition.Active,
		"tool":   h.definitionService.ToToolFunction(definition),
		"upstream": map[string]any{
			"method":  definition.Method,
			"url":     definition.URL,
			"headers": definition.Headers,
		},
		"cache": map[string]any{
			"enabled":     definition.Cache.Enabled,
			"ttl_seconds": definition.Cache.TTLSeconds,
			"max_entries": definition.Cache.MaxEntries,
		},
	})
}

func (h ToolHandler) executeTool(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r.URL.Path, "/tool/execute/")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	var input service.ExecuteToolInput
	if err := decodeJSON(r.Body, &input); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	output, err := h.executorService.Execute(r.Context(), id, input)
	if err != nil {
		if errors.Is(err, service.ErrDefinitionNotFound) {
			response.Error(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, service.ErrCircuitOpen) {
			response.Error(w, http.StatusServiceUnavailable, err.Error())
			return
		}

		if strings.Contains(err.Error(), "upstream request failed") {
			response.Error(w, http.StatusBadGateway, err.Error())
			return
		}

		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	response.JSON(w, http.StatusOK, output)
}

func (h ToolHandler) enqueueToolJob(w http.ResponseWriter, r *http.Request) {
	if h.jobService == nil {
		response.Error(w, http.StatusNotImplemented, "job service not configured")
		return
	}

	id, err := pathID(strings.TrimSuffix(r.URL.Path, "/jobs"), "/tool/execute/")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	var input service.ExecuteToolInput
	if err := decodeJSON(r.Body, &input); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	job, err := h.jobService.Enqueue(r.Context(), id, input)
	if err != nil {
		if errors.Is(err, service.ErrDefinitionNotFound) {
			response.Error(w, http.StatusNotFound, err.Error())
			return
		}
		response.Error(w, http.StatusInternalServerError, "failed to enqueue job")
		return
	}

	response.JSON(w, http.StatusAccepted, map[string]any{
		"job_id":        job.ID,
		"definition_id": job.DefinitionID,
		"status":        job.Status,
		"attempt":       job.Attempt,
		"max_attempts":  job.MaxAttempts,
	})
}

func (h ToolHandler) getToolJobByID(w http.ResponseWriter, r *http.Request) {
	if h.jobService == nil {
		response.Error(w, http.StatusNotImplemented, "job service not configured")
		return
	}

	jobID, err := pathID(r.URL.Path, "/tool/jobs/")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	job, ok, err := h.jobService.GetByID(r.Context(), jobID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to get job")
		return
	}
	if !ok {
		response.Error(w, http.StatusNotFound, "job not found")
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"job_id":        job.ID,
		"definition_id": job.DefinitionID,
		"status":        job.Status,
		"attempt":       job.Attempt,
		"max_attempts":  job.MaxAttempts,
		"result":        job.Result,
		"error":         job.Error,
		"created_at":    job.CreatedAt,
		"updated_at":    job.UpdatedAt,
	})
}

func (h ToolHandler) patchDefinitionByID(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r.URL.Path, "/tool/definitions/")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	var input service.ToolDefinitionPatchInput
	if err := decodeToolPatchJSON(r.Body, &input); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	definition, err := h.definitionService.Patch(r.Context(), id, input)
	if err != nil {
		if errors.Is(err, service.ErrDefinitionNotFound) {
			response.Error(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, service.ErrDefinitionPatchEmpty) || strings.Contains(err.Error(), "cannot be empty") {
			response.Error(w, http.StatusBadRequest, err.Error())
			return
		}

		response.Error(w, http.StatusInternalServerError, "failed to patch definition")
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{
		"id":     definition.ID,
		"active": definition.Active,
		"tool":   h.definitionService.ToToolFunction(definition),
		"upstream": map[string]any{
			"method":  definition.Method,
			"url":     definition.URL,
			"headers": definition.Headers,
		},
		"cache": map[string]any{
			"enabled":     definition.Cache.Enabled,
			"ttl_seconds": definition.Cache.TTLSeconds,
			"max_entries": definition.Cache.MaxEntries,
		},
	})
}

func (h ToolHandler) deleteDefinitionByID(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r.URL.Path, "/tool/definitions/")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.definitionService.Deactivate(r.Context(), id); err != nil {
		if errors.Is(err, service.ErrDefinitionNotFound) {
			response.Error(w, http.StatusNotFound, err.Error())
			return
		}

		response.Error(w, http.StatusInternalServerError, "failed to delete definition")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeJSON(body io.ReadCloser, out any) error {
	defer body.Close()

	if err := json.NewDecoder(body).Decode(out); err != nil {
		return errors.New("invalid json body")
	}

	return nil
}

func decodeToolPatchJSON(body io.ReadCloser, out *service.ToolDefinitionPatchInput) error {
	defer body.Close()

	rawBody, err := io.ReadAll(body)
	if err != nil {
		return errors.New("invalid json body")
	}

	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &rawMap); err != nil {
		return errors.New("invalid json body")
	}

	if _, hasActive := rawMap["active"]; hasActive {
		return errors.New("active field cannot be patched")
	}

	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return errors.New("invalid json body")
	}

	return nil
}

func pathID(path string, prefix string) (string, error) {
	id := strings.TrimPrefix(path, prefix)
	if id == "" || strings.Contains(id, "/") {
		return "", errors.New("invalid id")
	}

	return id, nil
}
