package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/marqsleal/api-2-tool/internal/repository"
	"github.com/marqsleal/api-2-tool/internal/service"
)

func newToolHandlerForTest() ToolHandler {
	repo := repository.NewInMemoryToolDefinitionRepository()
	defSvc := service.NewToolDefinitionService(repo)
	execSvc := service.NewToolExecutorService(defSvc)
	return NewToolHandler(defSvc, execSvc, nil)
}

func doReq(t *testing.T, h http.Handler, method string, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
	}
	h.ServeHTTP(rr, r)
	return rr
}

func createToolAndID(t *testing.T, h http.Handler, url string) string {
	t.Helper()
	payload := `{"name":"tool_test","description":"d","method":"GET","url":"` + url + `","headers":{},"parameters":{"type":"object","properties":{}},"strict":true}`
	rr := doReq(t, h, http.MethodPost, "/tool", payload)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create tool expected 201 got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	id, _ := resp["id"].(string)
	if id == "" {
		t.Fatalf("missing id in response")
	}
	if active, ok := resp["active"].(bool); !ok || !active {
		t.Fatalf("expected active=true in create response")
	}
	return id
}

func TestToolHandlerRoutesAndCreateValidation(t *testing.T) {
	h := newToolHandlerForTest()

	rr := doReq(t, h, http.MethodGet, "/unknown", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404")
	}

	rr = doReq(t, h, http.MethodPost, "/tool", "{bad")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid json")
	}

	rr = doReq(t, h, http.MethodPost, "/tool", `{"name":"","method":"","url":""}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for validation")
	}
}

func TestToolHandlerListGetPatchDeleteAndExecuteLifecycle(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	h := newToolHandlerForTest()
	id := createToolAndID(t, h, upstream.URL)

	rr := doReq(t, h, http.MethodGet, "/tool/definitions", "")
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), id) {
		t.Fatalf("expected list with id, got %d body=%s", rr.Code, rr.Body.String())
	}

	rr = doReq(t, h, http.MethodGet, "/tool/definitions/bad/id", "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid id path")
	}

	rr = doReq(t, h, http.MethodGet, "/tool/definitions/missing", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing tool")
	}

	rr = doReq(t, h, http.MethodGet, "/tool/definitions/"+id, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for get by id")
	}

	rr = doReq(t, h, http.MethodPatch, "/tool/definitions/"+id, `{"active":false}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when patching active")
	}

	rr = doReq(t, h, http.MethodPatch, "/tool/definitions/"+id, `{"unknown":1}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field")
	}

	rr = doReq(t, h, http.MethodPatch, "/tool/definitions/"+id, `{}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty patch")
	}

	rr = doReq(t, h, http.MethodPatch, "/tool/definitions/"+id, `{"description":"patched"}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "patched") {
		t.Fatalf("expected patch success")
	}

	rr = doReq(t, h, http.MethodPost, "/tool/execute/"+id, "{bad")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 invalid execute body")
	}

	rr = doReq(t, h, http.MethodPost, "/tool/execute/"+id, `{"call_id":"c1","arguments":{"x":"y"}}`)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "function_call_output") {
		t.Fatalf("expected execute success, got %d body=%s", rr.Code, rr.Body.String())
	}

	rr = doReq(t, h, http.MethodDelete, "/tool/definitions/"+id, "")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on logical delete")
	}

	rr = doReq(t, h, http.MethodGet, "/tool/definitions/"+id, "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after logical delete")
	}

	rr = doReq(t, h, http.MethodPost, "/tool/execute/"+id, `{"call_id":"c2","arguments":{}}`)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 execute for inactive")
	}
}

func TestToolHandlerExecuteUpstreamError(t *testing.T) {
	h := newToolHandlerForTest()
	id := createToolAndID(t, h, "http://127.0.0.1:1")

	rr := doReq(t, h, http.MethodPost, "/tool/execute/"+id, `{"call_id":"c1","arguments":{}}`)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for upstream error, got %d", rr.Code)
	}
}

func TestToolHandlerNotFoundAndInvalidExecutePath(t *testing.T) {
	h := newToolHandlerForTest()

	rr := doReq(t, h, http.MethodPatch, "/tool/definitions/tool_missing", `{"description":"x"}`)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected patch 404 for missing tool")
	}

	rr = doReq(t, h, http.MethodDelete, "/tool/definitions/tool_missing", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected delete 404 for missing tool")
	}

	rr = doReq(t, h, http.MethodPost, "/tool/execute/invalid/path", `{"call_id":"c","arguments":{}}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected execute invalid path 400, got %d", rr.Code)
	}

	rr = doReq(t, h, http.MethodPost, "/tool/execute/tool_missing", `{"call_id":"c","arguments":{}}`)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected execute missing 404, got %d", rr.Code)
	}
}
