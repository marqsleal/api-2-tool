package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestToolJobServiceCleanupRetention(t *testing.T) {
	repo := repository.NewInMemoryToolJobRepository()
	defRepo := repository.NewInMemoryToolDefinitionRepository()
	created, err := defRepo.Create(context.Background(), domain.ToolDefinition{Name: "t", Method: "GET", URL: "https://example.com", Active: true})
	if err != nil {
		t.Fatalf("create definition error: %v", err)
	}
	defSvc := NewToolDefinitionService(defRepo)
	execSvc := NewToolExecutorService(defSvc)
	jobSvc := NewToolJobService(repo, execSvc)
	jobSvc.SetRetention(time.Millisecond)

	job, err := repo.Create(context.Background(), created.ID, map[string]any{}, "c", 1, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("create job error: %v", err)
	}
	if err := repo.MarkFailed(context.Background(), job.ID, "x", time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("mark failed error: %v", err)
	}

	if err := jobSvc.cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup error: %v", err)
	}
	_, ok, err := repo.GetByID(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("get job error: %v", err)
	}
	if ok {
		t.Fatalf("expected old terminal job deleted")
	}
}
