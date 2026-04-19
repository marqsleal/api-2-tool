package repository

import (
	"context"
	"fmt"
	"sync"

	"github.com/marqsleal/api-2-tool/internal/domain"
)

type ToolDefinitionRepository interface {
	Create(ctx context.Context, definition domain.ToolDefinition) (domain.ToolDefinition, error)
	List(ctx context.Context) ([]domain.ToolDefinition, error)
	GetByID(ctx context.Context, id string) (domain.ToolDefinition, bool, error)
	Patch(ctx context.Context, id string, patch domain.ToolDefinitionPatch) (domain.ToolDefinition, bool, error)
	Deactivate(ctx context.Context, id string) (bool, error)
}

type InMemoryToolDefinitionRepository struct {
	mu      sync.RWMutex
	items   map[string]domain.ToolDefinition
	counter int64
}

func NewInMemoryToolDefinitionRepository() *InMemoryToolDefinitionRepository {
	return &InMemoryToolDefinitionRepository{
		items: make(map[string]domain.ToolDefinition),
	}
}

func (r *InMemoryToolDefinitionRepository) Create(_ context.Context, definition domain.ToolDefinition) (domain.ToolDefinition, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.counter++
	definition.ID = fmt.Sprintf("tool_%d", r.counter)
	definition.Active = true
	r.items[definition.ID] = definition

	return definition, nil
}

func (r *InMemoryToolDefinitionRepository) List(_ context.Context) ([]domain.ToolDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	definitions := make([]domain.ToolDefinition, 0, len(r.items))
	for _, definition := range r.items {
		if !definition.Active {
			continue
		}
		definitions = append(definitions, definition)
	}

	return definitions, nil
}

func (r *InMemoryToolDefinitionRepository) GetByID(_ context.Context, id string) (domain.ToolDefinition, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	definition, ok := r.items[id]
	if !ok || !definition.Active {
		return domain.ToolDefinition{}, false, nil
	}

	return definition, true, nil
}

func (r *InMemoryToolDefinitionRepository) Patch(_ context.Context, id string, patch domain.ToolDefinitionPatch) (domain.ToolDefinition, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	definition, ok := r.items[id]
	if !ok || !definition.Active {
		return domain.ToolDefinition{}, false, nil
	}

	if patch.Name != nil {
		definition.Name = *patch.Name
	}
	if patch.Description != nil {
		definition.Description = *patch.Description
	}
	if patch.Method != nil {
		definition.Method = *patch.Method
	}
	if patch.URL != nil {
		definition.URL = *patch.URL
	}
	if patch.Headers != nil {
		definition.Headers = *patch.Headers
	}
	if patch.Parameters != nil {
		definition.Parameters = *patch.Parameters
	}
	if patch.Strict != nil {
		definition.Strict = *patch.Strict
	}

	r.items[id] = definition
	return definition, true, nil
}

func (r *InMemoryToolDefinitionRepository) Deactivate(_ context.Context, id string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	definition, ok := r.items[id]
	if !ok || !definition.Active {
		return false, nil
	}

	definition.Active = false
	r.items[id] = definition
	return true, nil
}
