package handler

import (
	"errors"
	"net/http"
	"testing"

	"github.com/marqsleal/api-2-tool/internal/domain"
	"github.com/marqsleal/api-2-tool/internal/service"
)

type errRepo struct {
	createErr     error
	listErr       error
	getErr        error
	patchErr      error
	deactivateErr error
}

func (r *errRepo) Create(definition domain.ToolDefinition) (domain.ToolDefinition, error) {
	if r.createErr != nil {
		return domain.ToolDefinition{}, r.createErr
	}
	definition.ID = "tool_x"
	definition.Active = true
	return definition, nil
}
func (r *errRepo) List() ([]domain.ToolDefinition, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return []domain.ToolDefinition{}, nil
}
func (r *errRepo) GetByID(string) (domain.ToolDefinition, bool, error) {
	if r.getErr != nil {
		return domain.ToolDefinition{}, false, r.getErr
	}
	return domain.ToolDefinition{}, false, nil
}
func (r *errRepo) Patch(string, domain.ToolDefinitionPatch) (domain.ToolDefinition, bool, error) {
	if r.patchErr != nil {
		return domain.ToolDefinition{}, false, r.patchErr
	}
	return domain.ToolDefinition{}, false, nil
}
func (r *errRepo) Deactivate(string) (bool, error) {
	if r.deactivateErr != nil {
		return false, r.deactivateErr
	}
	return false, nil
}

func TestToolHandlerInternalErrorBranches(t *testing.T) {
	repo := &errRepo{createErr: errors.New("db")}
	h := NewToolHandler(service.NewToolDefinitionService(repo), service.NewToolExecutorService(service.NewToolDefinitionService(repo)))

	rr := doReq(t, h, http.MethodPost, "/tool", `{"name":"n","method":"GET","url":"https://x"}`)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected create 500, got %d", rr.Code)
	}

	repo = &errRepo{listErr: errors.New("db")}
	h = NewToolHandler(service.NewToolDefinitionService(repo), service.NewToolExecutorService(service.NewToolDefinitionService(repo)))
	rr = doReq(t, h, http.MethodGet, "/tool/definitions", "")
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected list 500")
	}

	repo = &errRepo{getErr: errors.New("db")}
	h = NewToolHandler(service.NewToolDefinitionService(repo), service.NewToolExecutorService(service.NewToolDefinitionService(repo)))
	rr = doReq(t, h, http.MethodGet, "/tool/definitions/tool_1", "")
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected get 500")
	}

	rr = doReq(t, h, http.MethodPost, "/tool/execute/tool_1", `{"call_id":"c1","arguments":{}}`)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected execute 500 on get error")
	}

	repo = &errRepo{patchErr: errors.New("db")}
	h = NewToolHandler(service.NewToolDefinitionService(repo), service.NewToolExecutorService(service.NewToolDefinitionService(repo)))
	rr = doReq(t, h, http.MethodPatch, "/tool/definitions/tool_1", `{"description":"x"}`)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected patch 500")
	}

	rr = doReq(t, h, http.MethodPatch, "/tool/definitions/", `{"description":"x"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected patch invalid id 400")
	}

	repo = &errRepo{deactivateErr: errors.New("db")}
	h = NewToolHandler(service.NewToolDefinitionService(repo), service.NewToolExecutorService(service.NewToolDefinitionService(repo)))
	rr = doReq(t, h, http.MethodDelete, "/tool/definitions/tool_1", "")
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected delete 500")
	}

	rr = doReq(t, h, http.MethodDelete, "/tool/definitions/", "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected delete invalid id 400")
	}
}
