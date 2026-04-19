package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/marqsleal/api-2-tool/internal/domain"
)

type SQLiteCircuitBreakerRepository struct {
	db *sql.DB
}

func NewSQLiteCircuitBreakerRepository(dbPath string) (*SQLiteCircuitBreakerRepository, error) {
	if err := ensureDir(dbPath); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	repo := &SQLiteCircuitBreakerRepository{db: db}
	if err := repo.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return repo, nil
}

func (r *SQLiteCircuitBreakerRepository) Close() error {
	return r.db.Close()
}

func (r *SQLiteCircuitBreakerRepository) CreateIfNotExists(ctx context.Context, toolID string) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO tool_circuit_breakers (tool_id, state, consecutive_failures, opened_until_unix, half_open_remaining_probes, version, updated_at_unix)
		 VALUES (?, ?, 0, 0, 0, 0, ?)
		 ON CONFLICT(tool_id) DO NOTHING`,
		toolID,
		string(domain.CircuitClosed),
		time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("create breaker: %w", err)
	}
	return nil
}

func (r *SQLiteCircuitBreakerRepository) GetByToolID(ctx context.Context, toolID string) (domain.CircuitBreaker, bool, error) {
	var (
		b          domain.CircuitBreaker
		state      string
		openedUnix int64
	)

	row := r.db.QueryRowContext(
		ctx,
		`SELECT tool_id, state, consecutive_failures, opened_until_unix, half_open_remaining_probes, version
		 FROM tool_circuit_breakers
		 WHERE tool_id = ?`,
		toolID,
	)
	err := row.Scan(
		&b.ToolID,
		&state,
		&b.ConsecutiveFailures,
		&openedUnix,
		&b.HalfOpenRemainingProbes,
		&b.Version,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.CircuitBreaker{}, false, nil
		}
		return domain.CircuitBreaker{}, false, fmt.Errorf("get breaker: %w", err)
	}

	b.State = domain.CircuitState(state)
	if openedUnix > 0 {
		b.OpenedUntil = time.Unix(openedUnix, 0).UTC()
	}
	return b, true, nil
}

func (r *SQLiteCircuitBreakerRepository) UpdateIfVersion(ctx context.Context, breaker domain.CircuitBreaker, expectedVersion int64) (bool, error) {
	openedUntil := int64(0)
	if !breaker.OpenedUntil.IsZero() {
		openedUntil = breaker.OpenedUntil.UTC().Unix()
	}

	result, err := r.db.ExecContext(
		ctx,
		`UPDATE tool_circuit_breakers
		 SET state = ?, consecutive_failures = ?, opened_until_unix = ?, half_open_remaining_probes = ?, version = version + 1, updated_at_unix = ?
		 WHERE tool_id = ? AND version = ?`,
		string(breaker.State),
		breaker.ConsecutiveFailures,
		openedUntil,
		breaker.HalfOpenRemainingProbes,
		time.Now().Unix(),
		breaker.ToolID,
		expectedVersion,
	)
	if err != nil {
		return false, fmt.Errorf("update breaker: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return affected == 1, nil
}

func (r *SQLiteCircuitBreakerRepository) migrate() error {
	_, err := r.db.Exec(`
CREATE TABLE IF NOT EXISTS tool_circuit_breakers (
	tool_id TEXT PRIMARY KEY,
	state TEXT NOT NULL,
	consecutive_failures INTEGER NOT NULL,
	opened_until_unix INTEGER NOT NULL,
	half_open_remaining_probes INTEGER NOT NULL,
	version INTEGER NOT NULL,
	updated_at_unix INTEGER NOT NULL
);`)
	if err != nil {
		return fmt.Errorf("migrate tool_circuit_breakers: %w", err)
	}

	return nil
}
