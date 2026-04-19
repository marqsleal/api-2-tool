package domain

import "time"

type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
)

type ToolJob struct {
	ID           string
	DefinitionID string
	CallID       string
	Arguments    map[string]any
	Status       JobStatus
	Attempt      int
	MaxAttempts  int
	NextRunAt    time.Time
	LeaseUntil   time.Time
	Result       string
	Error        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
