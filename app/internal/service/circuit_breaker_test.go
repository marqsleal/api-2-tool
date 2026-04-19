package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/marqsleal/api-2-tool/internal/repository"
)

func TestCircuitBreakerTransitions(t *testing.T) {
	repo := repository.NewInMemoryCircuitBreakerRepository()
	svc := NewCircuitBreakerService(repo, 2, 200*time.Millisecond, 2)
	ctx := context.Background()
	now := time.Now()

	if err := svc.BeforeExecution(ctx, "tool_1", now); err != nil {
		t.Fatalf("before execution closed error: %v", err)
	}

	if err := svc.OnFailure(ctx, "tool_1", now); err != nil {
		t.Fatalf("failure 1 error: %v", err)
	}
	if err := svc.BeforeExecution(ctx, "tool_1", now); err != nil {
		t.Fatalf("before execution after first failure error: %v", err)
	}

	if err := svc.OnFailure(ctx, "tool_1", now); err != nil {
		t.Fatalf("failure 2 error: %v", err)
	}
	if err := svc.BeforeExecution(ctx, "tool_1", now); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected open circuit error, got %v", err)
	}

	afterCooldown := now.Add(250 * time.Millisecond)
	if err := svc.BeforeExecution(ctx, "tool_1", afterCooldown); err != nil {
		t.Fatalf("expected half-open probe grant, got %v", err)
	}
	if err := svc.OnSuccess(ctx, "tool_1", afterCooldown); err != nil {
		t.Fatalf("on success error: %v", err)
	}

	if err := svc.BeforeExecution(ctx, "tool_1", afterCooldown); err != nil {
		t.Fatalf("expected closed circuit after success, got %v", err)
	}
}
