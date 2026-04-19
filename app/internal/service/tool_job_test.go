package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/marqsleal/api-2-tool/internal/domain"
	"github.com/marqsleal/api-2-tool/internal/repository"
)

func TestToolJobServiceEnqueueAndProcess(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	defRepo := repository.NewInMemoryToolDefinitionRepository()
	created, err := defRepo.Create(context.Background(), domain.ToolDefinition{
		ID:     "tool_1",
		Name:   "t",
		Method: "GET",
		URL:    upstream.URL,
		Active: true,
	})
	if err != nil {
		t.Fatalf("create definition error: %v", err)
	}
	definitionID := created.ID

	defSvc := NewToolDefinitionService(defRepo)
	execSvc := NewToolExecutorService(defSvc)
	jobRepo := repository.NewInMemoryToolJobRepository()
	jobSvc := NewToolJobService(jobRepo, execSvc)

	job, err := jobSvc.Enqueue(context.Background(), definitionID, ExecuteToolInput{CallID: "c1", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("enqueue error: %v", err)
	}
	if job.Status != domain.JobPending {
		t.Fatalf("expected pending job")
	}

	if err := jobSvc.processOnce(context.Background()); err != nil {
		t.Fatalf("process error: %v", err)
	}

	got, ok, err := jobSvc.GetByID(context.Background(), job.ID)
	if err != nil || !ok {
		t.Fatalf("expected created job")
	}
	if got.Status != domain.JobSucceeded {
		t.Fatalf("expected succeeded status, got %s", got.Status)
	}
}
