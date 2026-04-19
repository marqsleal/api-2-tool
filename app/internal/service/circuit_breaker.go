package service

import (
	"context"
	"errors"
	"time"

	"github.com/marqsleal/api-2-tool/internal/domain"
	"github.com/marqsleal/api-2-tool/internal/repository"
)

var ErrCircuitOpen = errors.New("circuit breaker open")

type CircuitBreakerService struct {
	repository repository.CircuitBreakerRepository
	threshold  int
	cooldown   time.Duration
	halfProbes int
}

func NewCircuitBreakerService(repository repository.CircuitBreakerRepository, threshold int, cooldown time.Duration, halfProbes int) CircuitBreakerService {
	if threshold < 1 {
		threshold = 1
	}
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	if halfProbes < 1 {
		halfProbes = 1
	}
	return CircuitBreakerService{
		repository: repository,
		threshold:  threshold,
		cooldown:   cooldown,
		halfProbes: halfProbes,
	}
}

func (s CircuitBreakerService) BeforeExecution(ctx context.Context, toolID string, now time.Time) error {
	for i := 0; i < 6; i++ {
		if err := s.repository.CreateIfNotExists(ctx, toolID); err != nil {
			return err
		}

		breaker, ok, err := s.repository.GetByToolID(ctx, toolID)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}

		switch breaker.State {
		case domain.CircuitClosed:
			return nil
		case domain.CircuitOpen:
			if now.Before(breaker.OpenedUntil) {
				return ErrCircuitOpen
			}

			next := breaker
			next.State = domain.CircuitHalfOpen
			next.HalfOpenRemainingProbes = s.halfProbes
			next.ConsecutiveFailures = 0
			next.OpenedUntil = time.Time{}
			updated, err := s.repository.UpdateIfVersion(ctx, next, breaker.Version)
			if err != nil {
				return err
			}
			if updated {
				continue
			}
		case domain.CircuitHalfOpen:
			if breaker.HalfOpenRemainingProbes <= 0 {
				return ErrCircuitOpen
			}

			next := breaker
			next.HalfOpenRemainingProbes--
			updated, err := s.repository.UpdateIfVersion(ctx, next, breaker.Version)
			if err != nil {
				return err
			}
			if updated {
				return nil
			}
		default:
			return nil
		}
	}

	return ErrCircuitOpen
}

func (s CircuitBreakerService) OnSuccess(ctx context.Context, toolID string, now time.Time) error {
	for i := 0; i < 6; i++ {
		breaker, ok, err := s.repository.GetByToolID(ctx, toolID)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}

		if breaker.State == domain.CircuitClosed && breaker.ConsecutiveFailures == 0 {
			return nil
		}

		next := breaker
		next.State = domain.CircuitClosed
		next.ConsecutiveFailures = 0
		next.HalfOpenRemainingProbes = 0
		next.OpenedUntil = time.Time{}
		updated, err := s.repository.UpdateIfVersion(ctx, next, breaker.Version)
		if err != nil {
			return err
		}
		if updated {
			return nil
		}
	}
	_ = now
	return nil
}

func (s CircuitBreakerService) OnFailure(ctx context.Context, toolID string, now time.Time) error {
	for i := 0; i < 6; i++ {
		if err := s.repository.CreateIfNotExists(ctx, toolID); err != nil {
			return err
		}
		breaker, ok, err := s.repository.GetByToolID(ctx, toolID)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}

		next := breaker
		switch breaker.State {
		case domain.CircuitHalfOpen:
			next.State = domain.CircuitOpen
			next.ConsecutiveFailures = s.threshold
			next.HalfOpenRemainingProbes = 0
			next.OpenedUntil = now.Add(s.cooldown)
		case domain.CircuitOpen:
			next.OpenedUntil = now.Add(s.cooldown)
		default:
			next.ConsecutiveFailures++
			if next.ConsecutiveFailures >= s.threshold {
				next.State = domain.CircuitOpen
				next.OpenedUntil = now.Add(s.cooldown)
				next.HalfOpenRemainingProbes = 0
			}
		}

		updated, err := s.repository.UpdateIfVersion(ctx, next, breaker.Version)
		if err != nil {
			return err
		}
		if updated {
			return nil
		}
	}

	return nil
}
