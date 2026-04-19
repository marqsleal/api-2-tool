package repository

import (
	"context"
	"sync"
	"time"

	"github.com/marqsleal/api-2-tool/internal/domain"
)

type CircuitBreakerRepository interface {
	CreateIfNotExists(ctx context.Context, toolID string) error
	GetByToolID(ctx context.Context, toolID string) (domain.CircuitBreaker, bool, error)
	UpdateIfVersion(ctx context.Context, breaker domain.CircuitBreaker, expectedVersion int64) (bool, error)
}

type InMemoryCircuitBreakerRepository struct {
	mu    sync.Mutex
	items map[string]domain.CircuitBreaker
}

func NewInMemoryCircuitBreakerRepository() *InMemoryCircuitBreakerRepository {
	return &InMemoryCircuitBreakerRepository{
		items: map[string]domain.CircuitBreaker{},
	}
}

func (r *InMemoryCircuitBreakerRepository) CreateIfNotExists(_ context.Context, toolID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.items[toolID]; ok {
		return nil
	}

	r.items[toolID] = domain.CircuitBreaker{
		ToolID: toolID,
		State:  domain.CircuitClosed,
	}
	return nil
}

func (r *InMemoryCircuitBreakerRepository) GetByToolID(_ context.Context, toolID string) (domain.CircuitBreaker, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	item, ok := r.items[toolID]
	return item, ok, nil
}

func (r *InMemoryCircuitBreakerRepository) UpdateIfVersion(_ context.Context, breaker domain.CircuitBreaker, expectedVersion int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.items[breaker.ToolID]
	if !ok {
		return false, nil
	}
	if current.Version != expectedVersion {
		return false, nil
	}

	breaker.Version = expectedVersion + 1
	if breaker.State != domain.CircuitOpen {
		breaker.OpenedUntil = time.Time{}
	}
	r.items[breaker.ToolID] = breaker
	return true, nil
}
