package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/marqsleal/api-2-tool/internal/http/response"
	"github.com/marqsleal/api-2-tool/internal/service"
)

type ToolHandler struct {
	definitionService service.ToolDefinitionService
	executorService   service.ToolExecutorService
}

func NewToolHandler(
	definitionService service.ToolDefinitionService,
	executorService service.ToolExecutorService,
) ToolHandler {
	return ToolHandler{
		definitionService: definitionService,
		executorService:   executorService,
	}
}

func (h ToolHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/tool" && r.Method == http.MethodPost:
		h.createTool(w, r)
		return
	case r.URL.Path == "/tool/definitions" && r.Method == http.MethodGet:
		h.listDefinitions(w)
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
		h.executeTool(w, r)
		return
	}

	response.Error(w, http.StatusNotFound, "route not found")
}

func (h ToolHandler) createTool(w http.ResponseWriter, r *http.Request) {
	var input service.ToolDefinitionInput
	if err := decodeJSON(r.Body, &input); err != nil {
		log.Printf("tool create failed: invalid request body")
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	log.Printf(
		"tool create started name=%q method=%s url=%s",
		input.Name,
		input.Method,
		input.URL,
	)

	definition, err := h.definitionService.Create(input)
	if err != nil {
		if err.Error() == "name, method and url are required" {
			log.Printf("tool create failed: validation error=%v", err)
			response.Error(w, http.StatusBadRequest, err.Error())
			return
		}

		log.Printf("tool create failed: internal error=%v", err)
		response.Error(w, http.StatusInternalServerError, "failed to create definition")
		return
	}

	log.Printf(
		"tool create finished id=%s name=%q method=%s",
		definition.ID,
		definition.Name,
		definition.Method,
	)

	response.JSON(w, http.StatusCreated, map[string]any{
		"id":     definition.ID,
		"active": definition.Active,
		"tool":   h.definitionService.ToToolFunction(definition),
		"upstream": map[string]any{
			"method":  definition.Method,
			"url":     definition.URL,
			"headers": definition.Headers,
		},
	})
}

func (h ToolHandler) listDefinitions(w http.ResponseWriter) {
	log.Printf("tool definitions list started")
	definitions, err := h.definitionService.List()
	if err != nil {
		log.Printf("tool definitions list failed: %v", err)
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
		})
	}

	log.Printf("tool definitions list finished count=%d", len(items))
	response.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h ToolHandler) getDefinitionByID(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r.URL.Path, "/tool/definitions/")
	if err != nil {
		log.Printf("tool definition get failed: invalid id path=%s", r.URL.Path)
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	log.Printf("tool definition get started id=%s", id)

	definition, ok, err := h.definitionService.GetByID(id)
	if err != nil {
		log.Printf("tool definition get failed id=%s err=%v", id, err)
		response.Error(w, http.StatusInternalServerError, "failed to get definition")
		return
	}
	if !ok {
		log.Printf("tool definition get not found id=%s", id)
		response.Error(w, http.StatusNotFound, "definition not found")
		return
	}

	log.Printf("tool definition get finished id=%s name=%q", definition.ID, definition.Name)
	response.JSON(w, http.StatusOK, map[string]any{
		"id":     definition.ID,
		"active": definition.Active,
		"tool":   h.definitionService.ToToolFunction(definition),
		"upstream": map[string]any{
			"method":  definition.Method,
			"url":     definition.URL,
			"headers": definition.Headers,
		},
	})
}

func (h ToolHandler) executeTool(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r.URL.Path, "/tool/execute/")
	if err != nil {
		log.Printf("tool execute failed: invalid id path=%s", r.URL.Path)
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	var input service.ExecuteToolInput
	if err := decodeJSON(r.Body, &input); err != nil {
		log.Printf("tool execute failed id=%s: invalid request body", id)
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	log.Printf("tool execute started id=%s call_id=%q", id, input.CallID)

	output, err := h.executorService.Execute(r.Context(), id, input)
	if err != nil {
		if errors.Is(err, service.ErrDefinitionNotFound) {
			log.Printf("tool execute not found id=%s", id)
			response.Error(w, http.StatusNotFound, err.Error())
			return
		}

		if strings.Contains(err.Error(), "upstream request failed") {
			log.Printf("tool execute upstream error id=%s err=%v", id, err)
			response.Error(w, http.StatusBadGateway, err.Error())
			return
		}

		log.Printf("tool execute failed id=%s err=%v", id, err)
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	log.Printf("tool execute finished id=%s call_id=%q", id, output.CallID)
	response.JSON(w, http.StatusOK, output)
}

func (h ToolHandler) patchDefinitionByID(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r.URL.Path, "/tool/definitions/")
	if err != nil {
		log.Printf("tool patch failed: invalid id path=%s", r.URL.Path)
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	var input service.ToolDefinitionPatchInput
	if err := decodeToolPatchJSON(r.Body, &input); err != nil {
		log.Printf("tool patch failed id=%s: %v", id, err)
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	log.Printf("tool patch started id=%s", id)
	definition, err := h.definitionService.Patch(id, input)
	if err != nil {
		if errors.Is(err, service.ErrDefinitionNotFound) {
			log.Printf("tool patch not found id=%s", id)
			response.Error(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, service.ErrDefinitionPatchEmpty) || strings.Contains(err.Error(), "cannot be empty") {
			log.Printf("tool patch validation error id=%s err=%v", id, err)
			response.Error(w, http.StatusBadRequest, err.Error())
			return
		}

		log.Printf("tool patch failed id=%s err=%v", id, err)
		response.Error(w, http.StatusInternalServerError, "failed to patch definition")
		return
	}

	log.Printf("tool patch finished id=%s", id)
	response.JSON(w, http.StatusOK, map[string]any{
		"id":     definition.ID,
		"active": definition.Active,
		"tool":   h.definitionService.ToToolFunction(definition),
		"upstream": map[string]any{
			"method":  definition.Method,
			"url":     definition.URL,
			"headers": definition.Headers,
		},
	})
}

func (h ToolHandler) deleteDefinitionByID(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r.URL.Path, "/tool/definitions/")
	if err != nil {
		log.Printf("tool delete failed: invalid id path=%s", r.URL.Path)
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	log.Printf("tool delete(logical) started id=%s", id)
	if err := h.definitionService.Deactivate(id); err != nil {
		if errors.Is(err, service.ErrDefinitionNotFound) {
			log.Printf("tool delete(logical) not found id=%s", id)
			response.Error(w, http.StatusNotFound, err.Error())
			return
		}

		log.Printf("tool delete(logical) failed id=%s err=%v", id, err)
		response.Error(w, http.StatusInternalServerError, "failed to delete definition")
		return
	}

	log.Printf("tool delete(logical) finished id=%s", id)
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
