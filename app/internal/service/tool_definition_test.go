package service

import (
	"context"
	"errors"
	"testing"

	"github.com/marqsleal/api-2-tool/internal/domain"
)

type testRepo struct {
	items            map[string]domain.ToolDefinition
	createErr        error
	listErr          error
	getErr           error
	patchErr         error
	deactivateErr    error
	patchShouldExist bool
}

func newTestRepo() *testRepo {
	return &testRepo{items: map[string]domain.ToolDefinition{}}
}

func (r *testRepo) Create(_ context.Context, definition domain.ToolDefinition) (domain.ToolDefinition, error) {
	if r.createErr != nil {
		return domain.ToolDefinition{}, r.createErr
	}
	if definition.ID == "" {
		definition.ID = "tool_test"
	}
	r.items[definition.ID] = definition
	return definition, nil
}

func (r *testRepo) List(_ context.Context) ([]domain.ToolDefinition, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	out := make([]domain.ToolDefinition, 0, len(r.items))
	for _, item := range r.items {
		out = append(out, item)
	}
	return out, nil
}

func (r *testRepo) GetByID(_ context.Context, id string) (domain.ToolDefinition, bool, error) {
	if r.getErr != nil {
		return domain.ToolDefinition{}, false, r.getErr
	}
	item, ok := r.items[id]
	return item, ok, nil
}

func (r *testRepo) Patch(_ context.Context, id string, patch domain.ToolDefinitionPatch) (domain.ToolDefinition, bool, error) {
	if r.patchErr != nil {
		return domain.ToolDefinition{}, false, r.patchErr
	}
	item, ok := r.items[id]
	if !ok && !r.patchShouldExist {
		return domain.ToolDefinition{}, false, nil
	}
	if patch.Name != nil {
		item.Name = *patch.Name
	}
	if patch.Description != nil {
		item.Description = *patch.Description
	}
	if patch.Method != nil {
		item.Method = *patch.Method
	}
	if patch.URL != nil {
		item.URL = *patch.URL
	}
	if patch.Headers != nil {
		item.Headers = *patch.Headers
	}
	if patch.Parameters != nil {
		item.Parameters = *patch.Parameters
	}
	if patch.Strict != nil {
		item.Strict = *patch.Strict
	}
	r.items[id] = item
	return item, true, nil
}

func (r *testRepo) Deactivate(_ context.Context, id string) (bool, error) {
	if r.deactivateErr != nil {
		return false, r.deactivateErr
	}
	item, ok := r.items[id]
	if !ok {
		return false, nil
	}
	item.Active = false
	r.items[id] = item
	return true, nil
}

func TestToolDefinitionServiceCreateAndDefaults(t *testing.T) {
	repo := newTestRepo()
	svc := NewToolDefinitionService(repo)

	created, err := svc.Create(context.Background(), ToolDefinitionInput{
		Name:   "x",
		Method: "get",
		URL:    "https://example.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created.Method != "GET" {
		t.Fatalf("expected method GET, got %s", created.Method)
	}
	if !created.Active {
		t.Fatalf("expected active true")
	}
	if created.Parameters == nil {
		t.Fatalf("expected default parameters")
	}
}

func TestToolDefinitionServiceCreateValidationAndRepoError(t *testing.T) {
	repo := newTestRepo()
	svc := NewToolDefinitionService(repo)

	_, err := svc.Create(context.Background(), ToolDefinitionInput{})
	if err == nil {
		t.Fatalf("expected validation error")
	}

	repo.createErr = errors.New("db")
	_, err = svc.Create(context.Background(), ToolDefinitionInput{Name: "n", Method: "GET", URL: "https://x"})
	if err == nil {
		t.Fatalf("expected repo error")
	}
}

func TestToolDefinitionServiceListAndGet(t *testing.T) {
	repo := newTestRepo()
	repo.items["tool_1"] = domain.ToolDefinition{ID: "tool_1", Name: "n", Method: "GET", URL: "https://x", Active: true}
	svc := NewToolDefinitionService(repo)

	list, err := svc.List(context.Background())
	if err != nil || len(list) != 1 {
		t.Fatalf("expected list with one item")
	}

	_, ok, err := svc.GetByID(context.Background(), "tool_1")
	if err != nil || !ok {
		t.Fatalf("expected found item")
	}

	repo.listErr = errors.New("list")
	if _, err := svc.List(context.Background()); err == nil {
		t.Fatalf("expected list error")
	}

	repo.getErr = errors.New("get")
	if _, _, err := svc.GetByID(context.Background(), "tool_1"); err == nil {
		t.Fatalf("expected get error")
	}
}

func TestToolDefinitionServicePatch(t *testing.T) {
	repo := newTestRepo()
	repo.items["tool_1"] = domain.ToolDefinition{ID: "tool_1", Name: "old", Method: "GET", URL: "https://x", Active: true}
	svc := NewToolDefinitionService(repo)

	_, err := svc.Patch(context.Background(), "tool_1", ToolDefinitionPatchInput{})
	if !errors.Is(err, ErrDefinitionPatchEmpty) {
		t.Fatalf("expected ErrDefinitionPatchEmpty, got %v", err)
	}

	empty := ""
	_, err = svc.Patch(context.Background(), "tool_1", ToolDefinitionPatchInput{Name: &empty})
	if err == nil {
		t.Fatalf("expected name validation error")
	}

	_, err = svc.Patch(context.Background(), "tool_1", ToolDefinitionPatchInput{URL: &empty})
	if err == nil {
		t.Fatalf("expected url validation error")
	}

	_, err = svc.Patch(context.Background(), "tool_1", ToolDefinitionPatchInput{Method: &empty})
	if err == nil {
		t.Fatalf("expected method validation error")
	}

	name := "new"
	method := "post"
	patched, err := svc.Patch(context.Background(), "tool_1", ToolDefinitionPatchInput{Name: &name, Method: &method})
	if err != nil {
		t.Fatalf("unexpected patch error: %v", err)
	}
	if patched.Name != "new" || patched.Method != "POST" {
		t.Fatalf("unexpected patched values: %+v", patched)
	}

	_, err = svc.Patch(context.Background(), "missing", ToolDefinitionPatchInput{Name: &name})
	if !errors.Is(err, ErrDefinitionNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}

	repo.patchErr = errors.New("patch")
	_, err = svc.Patch(context.Background(), "tool_1", ToolDefinitionPatchInput{Name: &name})
	if err == nil {
		t.Fatalf("expected patch repo error")
	}
}

func TestToolDefinitionServiceDeactivateAndToToolFunction(t *testing.T) {
	repo := newTestRepo()
	repo.items["tool_1"] = domain.ToolDefinition{ID: "tool_1", Name: "n", Description: "d", Method: "GET", URL: "https://x", Active: true}
	svc := NewToolDefinitionService(repo)

	if err := svc.Deactivate(context.Background(), "tool_1"); err != nil {
		t.Fatalf("unexpected deactivate error: %v", err)
	}
	if repo.items["tool_1"].Active {
		t.Fatalf("expected inactive")
	}

	if err := svc.Deactivate(context.Background(), "missing"); !errors.Is(err, ErrDefinitionNotFound) {
		t.Fatalf("expected not found")
	}

	repo.deactivateErr = errors.New("deactivate")
	if err := svc.Deactivate(context.Background(), "tool_1"); err == nil {
		t.Fatalf("expected deactivate error")
	}

	tool := svc.ToToolFunction(domain.ToolDefinition{Name: "n", Description: "d", Parameters: map[string]any{"x": "y"}, Strict: true})
	if tool.Type != "function" || tool.Function.Name != "n" || !tool.Function.Strict {
		t.Fatalf("unexpected tool mapping: %+v", tool)
	}
}
