package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/marqsleal/api-2-tool/internal/domain"
)

type ExecuteToolInput struct {
	CallID    string         `json:"call_id"`
	Arguments map[string]any `json:"arguments"`
}

type ToolExecutorService struct {
	definitionService ToolDefinitionService
	client            *http.Client
}

func NewToolExecutorService(definitionService ToolDefinitionService) ToolExecutorService {
	return ToolExecutorService{
		definitionService: definitionService,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s ToolExecutorService) Execute(definitionID string, input ExecuteToolInput) (domain.FunctionCallOutput, error) {
	definition, ok, err := s.definitionService.GetByID(definitionID)
	if err != nil {
		return domain.FunctionCallOutput{}, err
	}
	if !ok {
		return domain.FunctionCallOutput{}, ErrDefinitionNotFound
	}

	request, err := buildHTTPRequest(definition, input.Arguments)
	if err != nil {
		return domain.FunctionCallOutput{}, err
	}

	for key, value := range definition.Headers {
		request.Header.Set(key, value)
	}

	response, err := s.client.Do(request)
	if err != nil {
		return domain.FunctionCallOutput{}, fmt.Errorf("upstream request failed: %w", err)
	}
	defer response.Body.Close()

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return domain.FunctionCallOutput{}, fmt.Errorf("failed to read upstream response: %w", err)
	}

	responseBody := parseBody(bodyBytes)
	payload := map[string]any{
		"definition_id": definition.ID,
		"name":          definition.Name,
		"status_code":   response.StatusCode,
		"response":      responseBody,
	}

	outputBytes, err := json.Marshal(payload)
	if err != nil {
		return domain.FunctionCallOutput{}, fmt.Errorf("failed to encode tool output: %w", err)
	}

	return domain.FunctionCallOutput{
		Type:   "function_call_output",
		CallID: input.CallID,
		Output: string(outputBytes),
	}, nil
}

func buildHTTPRequest(definition domain.ToolDefinition, arguments map[string]any) (*http.Request, error) {
	if arguments == nil {
		arguments = map[string]any{}
	}
	normalizedArgs := make(map[string]any, len(arguments))
	for k, v := range arguments {
		normalizedArgs[k] = v
	}

	resolvedURL, consumedKeys, err := resolveURLPlaceholders(definition.URL, normalizedArgs)
	if err != nil {
		return nil, err
	}
	for _, key := range consumedKeys {
		delete(normalizedArgs, key)
	}

	method := strings.ToUpper(definition.Method)
	if method == http.MethodGet {
		parsedURL, err := url.Parse(resolvedURL)
		if err != nil {
			return nil, fmt.Errorf("invalid url: %w", err)
		}

		query := parsedURL.Query()
		for key, value := range normalizedArgs {
			query.Set(key, fmt.Sprint(value))
		}
		parsedURL.RawQuery = query.Encode()

		return http.NewRequest(method, parsedURL.String(), nil)
	}

	bodyBytes, err := json.Marshal(normalizedArgs)
	if err != nil {
		return nil, fmt.Errorf("invalid arguments body: %w", err)
	}

	request, err := http.NewRequest(method, resolvedURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")

	return request, nil
}

func resolveURLPlaceholders(rawURL string, arguments map[string]any) (string, []string, error) {
	placeholderRegex := regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)
	matches := placeholderRegex.FindAllStringSubmatch(rawURL, -1)
	if len(matches) == 0 {
		return rawURL, nil, nil
	}

	consumed := make([]string, 0, len(matches))
	resolvedURL := rawURL

	for _, match := range matches {
		key := match[1]
		rawPlaceholder := match[0]
		value, ok := arguments[key]
		if !ok {
			return "", nil, fmt.Errorf("missing url placeholder argument: %s", key)
		}
		consumed = append(consumed, key)
		replacement := url.PathEscape(fmt.Sprint(value))
		resolvedURL = strings.ReplaceAll(resolvedURL, rawPlaceholder, replacement)
	}

	return resolvedURL, consumed, nil
}

func parseBody(body []byte) any {
	if len(body) == 0 {
		return map[string]any{}
	}

	var parsed any
	if err := json.Unmarshal(body, &parsed); err == nil {
		return parsed
	}

	return string(body)
}
