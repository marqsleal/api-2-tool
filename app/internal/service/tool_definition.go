package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/marqsleal/api-2-tool/internal/domain"
	"github.com/marqsleal/api-2-tool/internal/repository"
)

type ToolDefinitionInput struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers"`
	Parameters  map[string]any    `json:"parameters"`
	Strict      bool              `json:"strict"`
}

type ToolDefinitionPatchInput struct {
	Name        *string            `json:"name"`
	Description *string            `json:"description"`
	Method      *string            `json:"method"`
	URL         *string            `json:"url"`
	Headers     *map[string]string `json:"headers"`
	Parameters  *map[string]any    `json:"parameters"`
	Strict      *bool              `json:"strict"`
}

type ToolDefinitionService struct {
	repository repository.ToolDefinitionRepository
}

var ErrDefinitionNotFound = errors.New("definition not found")
var ErrDefinitionPatchEmpty = errors.New("at least one field must be provided for patch")

func NewToolDefinitionService(repository repository.ToolDefinitionRepository) ToolDefinitionService {
	return ToolDefinitionService{repository: repository}
}

func (s ToolDefinitionService) Create(ctx context.Context, input ToolDefinitionInput) (domain.ToolDefinition, error) {
	method := strings.ToUpper(strings.TrimSpace(input.Method))
	if input.Name == "" || method == "" || input.URL == "" {
		return domain.ToolDefinition{}, errors.New("name, method and url are required")
	}

	parameters := input.Parameters
	if parameters == nil {
		parameters = map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}

	definition := domain.ToolDefinition{
		Name:        input.Name,
		Description: input.Description,
		Method:      method,
		URL:         input.URL,
		Headers:     input.Headers,
		Parameters:  parameters,
		Strict:      input.Strict,
		Active:      true,
	}

	created, err := s.repository.Create(ctx, definition)
	if err != nil {
		return domain.ToolDefinition{}, fmt.Errorf("create definition: %w", err)
	}

	return created, nil
}

func (s ToolDefinitionService) List(ctx context.Context) ([]domain.ToolDefinition, error) {
	definitions, err := s.repository.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list definitions: %w", err)
	}

	return definitions, nil
}

func (s ToolDefinitionService) GetByID(ctx context.Context, id string) (domain.ToolDefinition, bool, error) {
	definition, ok, err := s.repository.GetByID(ctx, id)
	if err != nil {
		return domain.ToolDefinition{}, false, fmt.Errorf("get definition by id: %w", err)
	}

	return definition, ok, nil
}

func (s ToolDefinitionService) Patch(ctx context.Context, id string, input ToolDefinitionPatchInput) (domain.ToolDefinition, error) {
	if input.Name == nil &&
		input.Description == nil &&
		input.Method == nil &&
		input.URL == nil &&
		input.Headers == nil &&
		input.Parameters == nil &&
		input.Strict == nil {
		return domain.ToolDefinition{}, ErrDefinitionPatchEmpty
	}

	patch := domain.ToolDefinitionPatch{
		Name:        input.Name,
		Description: input.Description,
		URL:         input.URL,
		Headers:     input.Headers,
		Parameters:  input.Parameters,
		Strict:      input.Strict,
	}

	if input.Method != nil {
		method := strings.ToUpper(strings.TrimSpace(*input.Method))
		if method == "" {
			return domain.ToolDefinition{}, errors.New("method cannot be empty")
		}
		patch.Method = &method
	}

	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return domain.ToolDefinition{}, errors.New("name cannot be empty")
	}
	if input.URL != nil && strings.TrimSpace(*input.URL) == "" {
		return domain.ToolDefinition{}, errors.New("url cannot be empty")
	}

	definition, ok, err := s.repository.Patch(ctx, id, patch)
	if err != nil {
		return domain.ToolDefinition{}, fmt.Errorf("patch definition: %w", err)
	}
	if !ok {
		return domain.ToolDefinition{}, ErrDefinitionNotFound
	}

	return definition, nil
}

func (s ToolDefinitionService) Deactivate(ctx context.Context, id string) error {
	ok, err := s.repository.Deactivate(ctx, id)
	if err != nil {
		return fmt.Errorf("deactivate definition: %w", err)
	}
	if !ok {
		return ErrDefinitionNotFound
	}

	return nil
}

func (s ToolDefinitionService) ToToolFunction(definition domain.ToolDefinition) domain.ToolFunctionDefinition {
	return domain.ToolFunctionDefinition{
		Type: "function",
		Function: domain.ToolFunction{
			Name:        definition.Name,
			Description: definition.Description,
			Parameters:  definition.Parameters,
			Strict:      definition.Strict,
		},
	}
}
