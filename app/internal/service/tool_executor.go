package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
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
	circuitBreaker    *CircuitBreakerService
	client            *http.Client
	retryMaxAttempts  int
	totalTimeout      time.Duration
	cacheRegistry     *toolCacheRegistry
	randSource        *rand.Rand
}

func NewToolExecutorService(definitionService ToolDefinitionService) ToolExecutorService {
	return ToolExecutorService{
		definitionService: definitionService,
		circuitBreaker:    nil,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		retryMaxAttempts: 3,
		totalTimeout:     30 * time.Second,
		cacheRegistry:    newToolCacheRegistry(128),
		randSource:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func NewToolExecutorServiceWithOptions(
	definitionService ToolDefinitionService,
	circuitBreaker *CircuitBreakerService,
	httpClient *http.Client,
) ToolExecutorService {
	service := NewToolExecutorService(definitionService)
	service.circuitBreaker = circuitBreaker
	if httpClient != nil {
		service.client = httpClient
	}
	return service
}

func (s ToolExecutorService) Execute(ctx context.Context, definitionID string, input ExecuteToolInput) (domain.FunctionCallOutput, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithTimeout(ctx, s.totalTimeout)
	defer cancel()

	definition, ok, err := s.definitionService.GetByID(ctx, definitionID)
	if err != nil {
		return domain.FunctionCallOutput{}, err
	}
	if !ok {
		return domain.FunctionCallOutput{}, ErrDefinitionNotFound
	}

	request, err := buildHTTPRequest(ctx, definition, input.Arguments)
	if err != nil {
		return domain.FunctionCallOutput{}, err
	}

	for key, value := range definition.Headers {
		request.Header.Set(key, value)
	}

	now := time.Now()
	cacheKey := ""
	cacheEnabled := strings.EqualFold(definition.Method, http.MethodGet) && definition.Cache.Enabled && definition.Cache.TTLSeconds > 0 && definition.Cache.MaxEntries > 0
	if cacheEnabled && s.cacheRegistry != nil {
		cacheKey = buildCacheKey(definitionID, request, input.Arguments, definition.Headers)
		cache := s.cacheRegistry.cacheFor(definitionID, definition.Cache.MaxEntries)
		if cached, remaining, hit := cache.Get(cacheKey, now); hit {
			outputBytes, err := applyExecutionMetadata(cached, map[string]any{
				"cache_hit":              true,
				"cache_key":              cacheKey,
				"cache_ttl_remaining_ms": remaining.Milliseconds(),
				"attempt_count":          0,
				"retry_reason":           "",
			})
			if err == nil {
				cached = outputBytes
			}
			return domain.FunctionCallOutput{
				Type:   "function_call_output",
				CallID: input.CallID,
				Output: cached,
			}, nil
		}
	}

	if s.circuitBreaker != nil {
		if err := s.circuitBreaker.BeforeExecution(ctx, definitionID, time.Now()); err != nil {
			if errors.Is(err, ErrCircuitOpen) {
				return domain.FunctionCallOutput{}, err
			}
			return domain.FunctionCallOutput{}, fmt.Errorf("circuit breaker check failed: %w", err)
		}
	}

	response, attemptCount, retryReason, err := s.doWithRetry(ctx, request)
	if err != nil {
		if s.circuitBreaker != nil {
			_ = s.circuitBreaker.OnFailure(ctx, definitionID, time.Now())
		}
		return domain.FunctionCallOutput{}, fmt.Errorf("upstream request failed: %w", err)
	}
	defer response.Body.Close()

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return domain.FunctionCallOutput{}, fmt.Errorf("failed to read upstream response: %w", err)
	}

	if response.StatusCode >= 200 && response.StatusCode < 500 && response.StatusCode != http.StatusTooManyRequests {
		if s.circuitBreaker != nil {
			_ = s.circuitBreaker.OnSuccess(ctx, definitionID, time.Now())
		}
	} else if s.circuitBreaker != nil {
		_ = s.circuitBreaker.OnFailure(ctx, definitionID, time.Now())
	}

	responseBody := parseBody(bodyBytes)
	payload := map[string]any{
		"definition_id": definition.ID,
		"name":          definition.Name,
		"status_code":   response.StatusCode,
		"response":      responseBody,
		"cache_hit":     false,
		"cache_key":     cacheKey,
		"attempt_count": attemptCount,
		"retry_reason":  retryReason,
		"retry": map[string]any{
			"attempt_count": attemptCount,
			"reason":        retryReason,
		},
	}

	outputBytes, err := json.Marshal(payload)
	if err != nil {
		return domain.FunctionCallOutput{}, fmt.Errorf("failed to encode tool output: %w", err)
	}

	if cacheEnabled && cacheKey != "" && response.StatusCode >= 200 && response.StatusCode < 300 {
		s.cacheRegistry.cacheFor(definitionID, definition.Cache.MaxEntries).Set(
			cacheKey,
			string(outputBytes),
			time.Duration(definition.Cache.TTLSeconds)*time.Second,
			now,
		)
	}

	return domain.FunctionCallOutput{
		Type:   "function_call_output",
		CallID: input.CallID,
		Output: string(outputBytes),
	}, nil
}

func (s ToolExecutorService) doWithRetry(ctx context.Context, request *http.Request) (*http.Response, int, string, error) {
	var lastErr error
	lastReason := ""
	for attempt := 1; attempt <= s.retryMaxAttempts; attempt++ {
		if ctx.Err() != nil {
			return nil, attempt, "context_cancelled", ctx.Err()
		}

		req := request.Clone(ctx)
		response, err := s.client.Do(req)
		if err == nil {
			if !shouldRetryStatus(response.StatusCode) {
				return response, attempt, lastReason, nil
			}
			lastErr = fmt.Errorf("status=%d", response.StatusCode)
			lastReason = fmt.Sprintf("status_%d", response.StatusCode)
			response.Body.Close()
		} else {
			lastErr = err
			if !shouldRetryError(err) {
				return nil, attempt, "non_retriable_error", err
			}
			lastReason = "transient_error"
		}

		if attempt == s.retryMaxAttempts {
			break
		}

		delay := s.backoffDelay(attempt)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, attempt, "context_cancelled", ctx.Err()
		case <-timer.C:
		}
	}
	return nil, s.retryMaxAttempts, lastReason, lastErr
}

func shouldRetryStatus(status int) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	return status >= 500
}

func shouldRetryError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return false
}

func (s ToolExecutorService) backoffDelay(attempt int) time.Duration {
	base := 100 * time.Millisecond
	multiplier := 1 << (attempt - 1)
	delay := time.Duration(multiplier) * base
	jitter := time.Duration(s.randSource.Intn(75)) * time.Millisecond
	return delay + jitter
}

func buildCacheKey(definitionID string, request *http.Request, arguments map[string]any, headers map[string]string) string {
	argBytes, _ := json.Marshal(arguments)

	relevantHeaderKeys := make([]string, 0, len(headers))
	for key := range headers {
		relevantHeaderKeys = append(relevantHeaderKeys, strings.ToLower(key))
	}
	sort.Strings(relevantHeaderKeys)

	headerValues := make([]string, 0, len(relevantHeaderKeys))
	for _, lowerKey := range relevantHeaderKeys {
		headerValues = append(headerValues, lowerKey+"="+request.Header.Get(lowerKey))
	}

	return fmt.Sprintf(
		"%s|%s|%s|%s|%s|%s",
		definitionID,
		request.Method,
		request.URL.EscapedPath(),
		request.URL.RawQuery,
		string(argBytes),
		strings.Join(headerValues, "&"),
	)
}

func applyExecutionMetadata(output string, metadata map[string]any) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		return "", err
	}
	for key, value := range metadata {
		payload[key] = value
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func buildHTTPRequest(ctx context.Context, definition domain.ToolDefinition, arguments map[string]any) (*http.Request, error) {
	if ctx == nil {
		ctx = context.Background()
	}

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

		return http.NewRequestWithContext(ctx, method, parsedURL.String(), nil)
	}

	bodyBytes, err := json.Marshal(normalizedArgs)
	if err != nil {
		return nil, fmt.Errorf("invalid arguments body: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, method, resolvedURL, bytes.NewReader(bodyBytes))
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
