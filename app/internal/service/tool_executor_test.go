package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/marqsleal/api-2-tool/internal/domain"
)

func TestBuildHTTPRequestAndParseBody(t *testing.T) {
	getReq, err := buildHTTPRequest(domain.ToolDefinition{Method: "GET", URL: "https://example.com/path"}, map[string]any{"a": 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(getReq.URL.RawQuery, "a=1") {
		t.Fatalf("expected query parameter in %s", getReq.URL.RawQuery)
	}

	templatedReq, err := buildHTTPRequest(
		domain.ToolDefinition{Method: "GET", URL: "https://example.com/ws/{cep}/json/"},
		map[string]any{"cep": "01001000", "extra": "x"},
	)
	if err != nil {
		t.Fatalf("unexpected templated request error: %v", err)
	}
	if templatedReq.URL.Path != "/ws/01001000/json/" {
		t.Fatalf("unexpected templated path: %s", templatedReq.URL.Path)
	}
	if templatedReq.URL.Query().Get("cep") != "" {
		t.Fatalf("expected consumed placeholder key to be removed from query")
	}
	if templatedReq.URL.Query().Get("extra") != "x" {
		t.Fatalf("expected extra query key to remain")
	}

	if _, err := buildHTTPRequest(
		domain.ToolDefinition{Method: "GET", URL: "https://example.com/ws/{cep}/json/"},
		map[string]any{"other": "x"},
	); err == nil {
		t.Fatalf("expected missing placeholder argument error")
	}

	postReq, err := buildHTTPRequest(domain.ToolDefinition{Method: "POST", URL: "https://example.com/path"}, map[string]any{"x": "y"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if postReq.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("expected content-type header")
	}

	if _, err := buildHTTPRequest(domain.ToolDefinition{Method: "GET", URL: "::://bad"}, nil); err == nil {
		t.Fatalf("expected invalid url error")
	}

	badArgs := map[string]any{"bad": make(chan int)}
	if _, err := buildHTTPRequest(domain.ToolDefinition{Method: "POST", URL: "https://example.com"}, badArgs); err == nil {
		t.Fatalf("expected marshal error")
	}

	if out := parseBody(nil); out == nil {
		t.Fatalf("expected empty object")
	}
	parsed := parseBody([]byte(`{"ok":true}`))
	if m, ok := parsed.(map[string]any); !ok || m["ok"] != true {
		t.Fatalf("unexpected parsed body: %#v", parsed)
	}
	text := parseBody([]byte("not json"))
	if s, ok := text.(string); !ok || s != "not json" {
		t.Fatalf("unexpected fallback body: %#v", text)
	}
}

func TestToolExecutorExecuteScenarios(t *testing.T) {
	repo := newTestRepo()
	defSvc := NewToolDefinitionService(repo)
	execSvc := NewToolExecutorService(defSvc)

	if _, err := execSvc.Execute("missing", ExecuteToolInput{}); !errors.Is(err, ErrDefinitionNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}

	repo.getErr = errors.New("db")
	if _, err := execSvc.Execute("x", ExecuteToolInput{}); err == nil {
		t.Fatalf("expected get error")
	}
	repo.getErr = nil

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET")
		}
		if r.Header.Get("X-Test") != "1" {
			t.Fatalf("missing header")
		}
		if got := r.URL.Path; got != "/ws/01001000/json/" {
			t.Fatalf("expected path arg replacement, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer upstream.Close()

	repo.items["tool_get"] = domain.ToolDefinition{
		ID:      "tool_get",
		Name:    "viacep",
		Method:  "GET",
		URL:     upstream.URL + "/ws/{cep}/json/",
		Headers: map[string]string{"X-Test": "1"},
		Active:  true,
	}

	out, err := execSvc.Execute("tool_get", ExecuteToolInput{CallID: "c1", Arguments: map[string]any{"cep": "01001000"}})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if out.Type != "function_call_output" || out.CallID != "c1" {
		t.Fatalf("unexpected output metadata: %+v", out)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(out.Output), &payload); err != nil {
		t.Fatalf("invalid output json: %v", err)
	}
	if payload["status_code"].(float64) != 200 {
		t.Fatalf("unexpected status code payload: %v", payload["status_code"])
	}

	postUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("expected json content-type")
		}
		_, _ = w.Write([]byte(`ok`))
	}))
	defer postUpstream.Close()

	repo.items["tool_post"] = domain.ToolDefinition{ID: "tool_post", Name: "post", Method: "POST", URL: postUpstream.URL, Active: true}
	if _, err := execSvc.Execute("tool_post", ExecuteToolInput{Arguments: map[string]any{"x": "y"}}); err != nil {
		t.Fatalf("unexpected post execute error: %v", err)
	}

	repo.items["tool_down"] = domain.ToolDefinition{ID: "tool_down", Name: "down", Method: "GET", URL: "http://127.0.0.1:1", Active: true}
	if _, err := execSvc.Execute("tool_down", ExecuteToolInput{}); err == nil || !strings.Contains(err.Error(), "upstream request failed") {
		t.Fatalf("expected upstream error, got %v", err)
	}
}
