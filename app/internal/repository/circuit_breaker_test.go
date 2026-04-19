package repository

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/marqsleal/api-2-tool/internal/domain"
)

func TestInMemoryCircuitBreakerVersionGuard(t *testing.T) {
	repo := NewInMemoryCircuitBreakerRepository()
	ctx := context.Background()

	if err := repo.CreateIfNotExists(ctx, "tool_1"); err != nil {
		t.Fatalf("create breaker error: %v", err)
	}
	current, ok, err := repo.GetByToolID(ctx, "tool_1")
	if err != nil || !ok {
		t.Fatalf("get breaker error: %v", err)
	}

	next := current
	next.State = domain.CircuitOpen
	next.OpenedUntil = time.Now().Add(time.Minute)
	updated, err := repo.UpdateIfVersion(ctx, next, current.Version)
	if err != nil || !updated {
		t.Fatalf("expected first update to succeed")
	}

	stale := current
	stale.State = domain.CircuitHalfOpen
	updated, err = repo.UpdateIfVersion(ctx, stale, current.Version)
	if err != nil {
		t.Fatalf("stale update err: %v", err)
	}
	if updated {
		t.Fatalf("expected stale update rejection")
	}
}

func TestSQLiteCircuitBreakerVersionGuard(t *testing.T) {
	repo, err := NewSQLiteCircuitBreakerRepository(filepath.Join(t.TempDir(), "cb.db"))
	if err != nil {
		t.Fatalf("new sqlite breaker repo error: %v", err)
	}
	defer repo.Close()
	ctx := context.Background()

	if err := repo.CreateIfNotExists(ctx, "tool_1"); err != nil {
		t.Fatalf("create breaker error: %v", err)
	}
	current, ok, err := repo.GetByToolID(ctx, "tool_1")
	if err != nil || !ok {
		t.Fatalf("get breaker error: %v", err)
	}

	next := current
	next.State = domain.CircuitOpen
	next.OpenedUntil = time.Now().Add(time.Minute)
	updated, err := repo.UpdateIfVersion(ctx, next, current.Version)
	if err != nil || !updated {
		t.Fatalf("expected first update to succeed")
	}

	stale := current
	stale.State = domain.CircuitHalfOpen
	updated, err = repo.UpdateIfVersion(ctx, stale, current.Version)
	if err != nil {
		t.Fatalf("stale update err: %v", err)
	}
	if updated {
		t.Fatalf("expected stale update rejection")
	}
}
