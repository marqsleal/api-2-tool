package repository

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestInMemoryToolJobLeaseReclaimAndCleanup(t *testing.T) {
	repo := NewInMemoryToolJobRepository()
	now := time.Now().UTC()

	job, err := repo.Create(context.Background(), "tool_1", map[string]any{"x": "y"}, "c1", 3, now.Add(-time.Second))
	if err != nil {
		t.Fatalf("create job error: %v", err)
	}

	claimed, ok, err := repo.ClaimNextPending(context.Background(), now, 10*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("expected claimed job")
	}
	if claimed.ID != job.ID || claimed.Status != "running" {
		t.Fatalf("unexpected claimed job: %+v", claimed)
	}

	claimedAgain, ok, err := repo.ClaimNextPending(context.Background(), now.Add(20*time.Millisecond), 10*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("expected lease-reclaimed job")
	}
	if claimedAgain.ID != job.ID || claimedAgain.Attempt != 2 {
		t.Fatalf("unexpected reclaimed job: %+v", claimedAgain)
	}

	if err := repo.MarkFailed(context.Background(), job.ID, "error", now.Add(-time.Hour)); err != nil {
		t.Fatalf("mark failed error: %v", err)
	}
	if err := repo.DeleteTerminalOlderThan(context.Background(), now.Add(-time.Minute)); err != nil {
		t.Fatalf("cleanup error: %v", err)
	}
	if _, ok, _ := repo.GetByID(context.Background(), job.ID); ok {
		t.Fatalf("expected cleaned terminal job")
	}
}

func TestSQLiteToolJobLeaseReclaimAndCleanup(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "jobs.db")
	repo, err := NewSQLiteToolJobRepository(dbPath)
	if err != nil {
		t.Fatalf("new sqlite job repo error: %v", err)
	}
	defer repo.Close()

	now := time.Now().UTC()
	job, err := repo.Create(context.Background(), "tool_1", map[string]any{"x": "y"}, "c1", 3, now.Add(-time.Second))
	if err != nil {
		t.Fatalf("create job error: %v", err)
	}

	claimed, ok, err := repo.ClaimNextPending(context.Background(), now, 10*time.Millisecond)
	if err != nil || !ok || claimed.ID != job.ID {
		t.Fatalf("expected claimed job")
	}

	claimedAgain, ok, err := repo.ClaimNextPending(context.Background(), now.Add(20*time.Millisecond), 10*time.Millisecond)
	if err != nil || !ok || claimedAgain.ID != job.ID {
		t.Fatalf("expected reclaimed job")
	}

	if err := repo.MarkFailed(context.Background(), job.ID, "error", now.Add(-time.Hour)); err != nil {
		t.Fatalf("mark failed error: %v", err)
	}
	if err := repo.DeleteTerminalOlderThan(context.Background(), now.Add(-time.Minute)); err != nil {
		t.Fatalf("cleanup error: %v", err)
	}
	if _, ok, _ := repo.GetByID(context.Background(), job.ID); ok {
		t.Fatalf("expected cleaned terminal job")
	}
}
