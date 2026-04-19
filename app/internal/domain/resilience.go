package domain

import "time"

type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

type CircuitBreaker struct {
	ToolID                  string
	State                   CircuitState
	ConsecutiveFailures     int
	OpenedUntil             time.Time
	HalfOpenRemainingProbes int
	Version                 int64
}
