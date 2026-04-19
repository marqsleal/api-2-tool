package repository

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/marqsleal/api-2-tool/internal/domain"
)

type ToolJobRepository interface {
	Create(ctx context.Context, definitionID string, input map[string]any, callID string, maxAttempts int, now time.Time) (domain.ToolJob, error)
	GetByID(ctx context.Context, jobID string) (domain.ToolJob, bool, error)
	ClaimNextPending(ctx context.Context, now time.Time, leaseDuration time.Duration) (domain.ToolJob, bool, error)
	MarkSucceeded(ctx context.Context, jobID string, result string, now time.Time) error
	MarkRetryPending(ctx context.Context, jobID string, nextRunAt time.Time, errMsg string, now time.Time) error
	MarkFailed(ctx context.Context, jobID string, errMsg string, now time.Time) error
}

type InMemoryToolJobRepository struct {
	mu    sync.Mutex
	items map[string]domain.ToolJob
}

func NewInMemoryToolJobRepository() *InMemoryToolJobRepository {
	return &InMemoryToolJobRepository{
		items: map[string]domain.ToolJob{},
	}
}

func (r *InMemoryToolJobRepository) Create(_ context.Context, definitionID string, input map[string]any, callID string, maxAttempts int, now time.Time) (domain.ToolJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := "job_" + uuid.NewString()
	job := domain.ToolJob{
		ID:           id,
		DefinitionID: definitionID,
		CallID:       callID,
		Arguments:    input,
		Status:       domain.JobPending,
		Attempt:      0,
		MaxAttempts:  maxAttempts,
		NextRunAt:    now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	r.items[id] = job
	return job, nil
}

func (r *InMemoryToolJobRepository) GetByID(_ context.Context, jobID string) (domain.ToolJob, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	item, ok := r.items[jobID]
	return item, ok, nil
}

func (r *InMemoryToolJobRepository) ClaimNextPending(_ context.Context, now time.Time, leaseDuration time.Duration) (domain.ToolJob, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var candidate domain.ToolJob
	found := false
	for _, item := range r.items {
		if item.Status != domain.JobPending {
			continue
		}
		if item.NextRunAt.After(now) {
			continue
		}
		if !found || item.CreatedAt.Before(candidate.CreatedAt) {
			candidate = item
			found = true
		}
	}
	if !found {
		return domain.ToolJob{}, false, nil
	}

	candidate.Status = domain.JobRunning
	candidate.Attempt++
	candidate.LeaseUntil = now.Add(leaseDuration)
	candidate.UpdatedAt = now
	r.items[candidate.ID] = candidate
	return candidate, true, nil
}

func (r *InMemoryToolJobRepository) MarkSucceeded(_ context.Context, jobID string, result string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	item, ok := r.items[jobID]
	if !ok {
		return nil
	}
	item.Status = domain.JobSucceeded
	item.Result = result
	item.Error = ""
	item.LeaseUntil = time.Time{}
	item.UpdatedAt = now
	r.items[jobID] = item
	return nil
}

func (r *InMemoryToolJobRepository) MarkRetryPending(_ context.Context, jobID string, nextRunAt time.Time, errMsg string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	item, ok := r.items[jobID]
	if !ok {
		return nil
	}
	item.Status = domain.JobPending
	item.NextRunAt = nextRunAt
	item.Error = errMsg
	item.LeaseUntil = time.Time{}
	item.UpdatedAt = now
	r.items[jobID] = item
	return nil
}

func (r *InMemoryToolJobRepository) MarkFailed(_ context.Context, jobID string, errMsg string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	item, ok := r.items[jobID]
	if !ok {
		return nil
	}
	item.Status = domain.JobFailed
	item.Error = errMsg
	item.LeaseUntil = time.Time{}
	item.UpdatedAt = now
	r.items[jobID] = item
	return nil
}
