package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	_ "modernc.org/sqlite"

	"github.com/marqsleal/api-2-tool/internal/domain"
)

type SQLiteToolJobRepository struct {
	db *sql.DB
}

func NewSQLiteToolJobRepository(dbPath string) (*SQLiteToolJobRepository, error) {
	if err := ensureDir(dbPath); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	repo := &SQLiteToolJobRepository{db: db}
	if err := repo.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return repo, nil
}

func (r *SQLiteToolJobRepository) Close() error {
	return r.db.Close()
}

func (r *SQLiteToolJobRepository) Create(ctx context.Context, definitionID string, input map[string]any, callID string, maxAttempts int, now time.Time) (domain.ToolJob, error) {
	argumentsJSON, err := json.Marshal(input)
	if err != nil {
		return domain.ToolJob{}, fmt.Errorf("marshal arguments: %w", err)
	}

	id := "job_" + uuid.NewString()
	nextRun := now.UTC().Unix()
	_, err = r.db.ExecContext(
		ctx,
		`INSERT INTO tool_jobs (id, definition_id, call_id, arguments_json, status, attempt, max_attempts, next_run_at_unix, lease_until_unix, result_json, error_message, created_at_unix, updated_at_unix)
		 VALUES (?, ?, ?, ?, ?, 0, ?, ?, 0, '', '', ?, ?)`,
		id,
		definitionID,
		callID,
		string(argumentsJSON),
		string(domain.JobPending),
		maxAttempts,
		nextRun,
		nextRun,
		nextRun,
	)
	if err != nil {
		return domain.ToolJob{}, fmt.Errorf("insert job: %w", err)
	}

	job, ok, err := r.GetByID(ctx, id)
	if err != nil {
		return domain.ToolJob{}, err
	}
	if !ok {
		return domain.ToolJob{}, fmt.Errorf("created job not found")
	}
	return job, nil
}

func (r *SQLiteToolJobRepository) GetByID(ctx context.Context, jobID string) (domain.ToolJob, bool, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, definition_id, call_id, arguments_json, status, attempt, max_attempts, next_run_at_unix, lease_until_unix, result_json, error_message, created_at_unix, updated_at_unix
		 FROM tool_jobs
		 WHERE id = ?`,
		jobID,
	)
	job, err := scanToolJob(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.ToolJob{}, false, nil
		}
		return domain.ToolJob{}, false, err
	}
	return job, true, nil
}

func (r *SQLiteToolJobRepository) ClaimNextPending(ctx context.Context, now time.Time, leaseDuration time.Duration) (domain.ToolJob, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.ToolJob{}, false, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(
		ctx,
		`SELECT id, definition_id, call_id, arguments_json, status, attempt, max_attempts, next_run_at_unix, lease_until_unix, result_json, error_message, created_at_unix, updated_at_unix
		 FROM tool_jobs
		 WHERE (status = ? AND next_run_at_unix <= ?)
		    OR (status = ? AND lease_until_unix > 0 AND lease_until_unix <= ?)
		 ORDER BY created_at_unix ASC
		 LIMIT 1`,
		string(domain.JobPending),
		now.UTC().Unix(),
		string(domain.JobRunning),
		now.UTC().Unix(),
	)

	job, err := scanToolJob(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.ToolJob{}, false, nil
		}
		return domain.ToolJob{}, false, err
	}

	leaseUntil := now.Add(leaseDuration).UTC().Unix()
	res, err := tx.ExecContext(
		ctx,
		`UPDATE tool_jobs
		 SET status = ?, attempt = attempt + 1, lease_until_unix = ?, updated_at_unix = ?
		 WHERE id = ? AND (status = ? OR (status = ? AND lease_until_unix > 0 AND lease_until_unix <= ?))`,
		string(domain.JobRunning),
		leaseUntil,
		now.UTC().Unix(),
		job.ID,
		string(domain.JobPending),
		string(domain.JobRunning),
		now.UTC().Unix(),
	)
	if err != nil {
		return domain.ToolJob{}, false, fmt.Errorf("claim job: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.ToolJob{}, false, fmt.Errorf("claim rows affected: %w", err)
	}
	if affected == 0 {
		return domain.ToolJob{}, false, nil
	}

	if err := tx.Commit(); err != nil {
		return domain.ToolJob{}, false, fmt.Errorf("commit claim: %w", err)
	}

	job.Attempt++
	job.Status = domain.JobRunning
	job.LeaseUntil = time.Unix(leaseUntil, 0).UTC()
	job.UpdatedAt = now.UTC()
	return job, true, nil
}

func (r *SQLiteToolJobRepository) MarkSucceeded(ctx context.Context, jobID string, result string, now time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE tool_jobs
		 SET status = ?, result_json = ?, error_message = '', lease_until_unix = 0, updated_at_unix = ?
		 WHERE id = ?`,
		string(domain.JobSucceeded),
		result,
		now.UTC().Unix(),
		jobID,
	)
	if err != nil {
		return fmt.Errorf("mark succeeded: %w", err)
	}
	return nil
}

func (r *SQLiteToolJobRepository) MarkRetryPending(ctx context.Context, jobID string, nextRunAt time.Time, errMsg string, now time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE tool_jobs
		 SET status = ?, next_run_at_unix = ?, error_message = ?, lease_until_unix = 0, updated_at_unix = ?
		 WHERE id = ?`,
		string(domain.JobPending),
		nextRunAt.UTC().Unix(),
		errMsg,
		now.UTC().Unix(),
		jobID,
	)
	if err != nil {
		return fmt.Errorf("mark retry pending: %w", err)
	}
	return nil
}

func (r *SQLiteToolJobRepository) MarkFailed(ctx context.Context, jobID string, errMsg string, now time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE tool_jobs
		 SET status = ?, error_message = ?, lease_until_unix = 0, updated_at_unix = ?
		 WHERE id = ?`,
		string(domain.JobFailed),
		errMsg,
		now.UTC().Unix(),
		jobID,
	)
	if err != nil {
		return fmt.Errorf("mark failed: %w", err)
	}
	return nil
}

func (r *SQLiteToolJobRepository) DeleteTerminalOlderThan(ctx context.Context, threshold time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`DELETE FROM tool_jobs
		 WHERE (status = ? OR status = ?) AND updated_at_unix < ?`,
		string(domain.JobSucceeded),
		string(domain.JobFailed),
		threshold.UTC().Unix(),
	)
	if err != nil {
		return fmt.Errorf("delete old terminal jobs: %w", err)
	}
	return nil
}

func (r *SQLiteToolJobRepository) migrate() error {
	_, err := r.db.Exec(`
CREATE TABLE IF NOT EXISTS tool_jobs (
	id TEXT PRIMARY KEY,
	definition_id TEXT NOT NULL,
	call_id TEXT NOT NULL,
	arguments_json TEXT NOT NULL,
	status TEXT NOT NULL,
	attempt INTEGER NOT NULL,
	max_attempts INTEGER NOT NULL,
	next_run_at_unix INTEGER NOT NULL,
	lease_until_unix INTEGER NOT NULL,
	result_json TEXT NOT NULL,
	error_message TEXT NOT NULL,
	created_at_unix INTEGER NOT NULL,
	updated_at_unix INTEGER NOT NULL
);`)
	if err != nil {
		return fmt.Errorf("migrate tool_jobs: %w", err)
	}

	_, err = r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_tool_jobs_status_next_run ON tool_jobs (status, next_run_at_unix)`)
	if err != nil {
		return fmt.Errorf("migrate tool_jobs index: %w", err)
	}
	return nil
}

type toolJobScanner interface {
	Scan(dest ...any) error
}

func scanToolJob(s toolJobScanner) (domain.ToolJob, error) {
	var (
		job            domain.ToolJob
		argumentsJSON  string
		status         string
		nextRunAtUnix  int64
		leaseUntilUnix int64
		createdAtUnix  int64
		updatedAtUnix  int64
	)

	err := s.Scan(
		&job.ID,
		&job.DefinitionID,
		&job.CallID,
		&argumentsJSON,
		&status,
		&job.Attempt,
		&job.MaxAttempts,
		&nextRunAtUnix,
		&leaseUntilUnix,
		&job.Result,
		&job.Error,
		&createdAtUnix,
		&updatedAtUnix,
	)
	if err != nil {
		return domain.ToolJob{}, err
	}

	job.Status = domain.JobStatus(status)
	job.NextRunAt = time.Unix(nextRunAtUnix, 0).UTC()
	if leaseUntilUnix > 0 {
		job.LeaseUntil = time.Unix(leaseUntilUnix, 0).UTC()
	}
	job.CreatedAt = time.Unix(createdAtUnix, 0).UTC()
	job.UpdatedAt = time.Unix(updatedAtUnix, 0).UTC()

	if err := json.Unmarshal([]byte(argumentsJSON), &job.Arguments); err != nil {
		return domain.ToolJob{}, fmt.Errorf("unmarshal job arguments: %w", err)
	}
	if job.Arguments == nil {
		job.Arguments = map[string]any{}
	}
	return job, nil
}
