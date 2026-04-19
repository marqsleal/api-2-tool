package service

import (
	"context"
	"fmt"
	"time"

	"github.com/marqsleal/api-2-tool/internal/domain"
	"github.com/marqsleal/api-2-tool/internal/repository"
)

type ToolJobService struct {
	repository    repository.ToolJobRepository
	executor      ToolExecutorService
	maxAttempts   int
	leaseDuration time.Duration
	pollInterval  time.Duration
	retention     time.Duration
	lastCleanup   time.Time
}

func NewToolJobService(repository repository.ToolJobRepository, executor ToolExecutorService) ToolJobService {
	return ToolJobService{
		repository:    repository,
		executor:      executor,
		maxAttempts:   3,
		leaseDuration: 15 * time.Second,
		pollInterval:  150 * time.Millisecond,
		retention:     7 * 24 * time.Hour,
	}
}

func (s *ToolJobService) SetRetention(retention time.Duration) {
	if retention > 0 {
		s.retention = retention
	}
}

func (s ToolJobService) Enqueue(ctx context.Context, definitionID string, input ExecuteToolInput) (domain.ToolJob, error) {
	_, ok, err := s.executor.definitionService.GetByID(ctx, definitionID)
	if err != nil {
		return domain.ToolJob{}, err
	}
	if !ok {
		return domain.ToolJob{}, ErrDefinitionNotFound
	}

	now := time.Now().UTC()
	return s.repository.Create(ctx, definitionID, input.Arguments, input.CallID, s.maxAttempts, now)
}

func (s ToolJobService) GetByID(ctx context.Context, jobID string) (domain.ToolJob, bool, error) {
	return s.repository.GetByID(ctx, jobID)
}

func (s *ToolJobService) RunWorker(ctx context.Context) {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.processOnce(ctx)
			_ = s.cleanup(ctx)
		}
	}
}

func (s *ToolJobService) processOnce(ctx context.Context) error {
	now := time.Now().UTC()
	job, ok, err := s.repository.ClaimNextPending(ctx, now, s.leaseDuration)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	output, execErr := s.executor.Execute(ctx, job.DefinitionID, ExecuteToolInput{
		CallID:    job.CallID,
		Arguments: job.Arguments,
	})
	if execErr == nil {
		return s.repository.MarkSucceeded(ctx, job.ID, output.Output, time.Now().UTC())
	}

	if job.Attempt < job.MaxAttempts {
		nextRun := time.Now().UTC().Add(backoffForJobAttempt(job.Attempt))
		return s.repository.MarkRetryPending(ctx, job.ID, nextRun, execErr.Error(), time.Now().UTC())
	}

	return s.repository.MarkFailed(ctx, job.ID, execErr.Error(), time.Now().UTC())
}

func backoffForJobAttempt(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	base := 300 * time.Millisecond
	return base * time.Duration(1<<(attempt-1))
}

func (s *ToolJobService) StartWorkers(ctx context.Context, count int) error {
	if count < 1 {
		return fmt.Errorf("worker count must be >= 1")
	}
	for i := 0; i < count; i++ {
		go s.RunWorker(ctx)
	}
	return nil
}

func (s *ToolJobService) cleanup(ctx context.Context) error {
	if s.retention <= 0 {
		return nil
	}
	now := time.Now().UTC()
	if !s.lastCleanup.IsZero() && now.Sub(s.lastCleanup) < time.Minute {
		return nil
	}
	s.lastCleanup = now
	return s.repository.DeleteTerminalOlderThan(ctx, now.Add(-s.retention))
}
