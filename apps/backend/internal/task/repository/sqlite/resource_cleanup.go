package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
)

const taskResourceCleanupColumns = `
	id, operation_id, task_id, trigger, state, resource_snapshot, attempts,
	next_attempt_at, last_error, created_at, updated_at, completed_at`

func (r *Repository) CreateTaskResourceCleanupJob(ctx context.Context, job *models.TaskResourceCleanupJob) error {
	if job.ID == "" {
		job.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now
	if job.State == "" {
		job.State = models.TaskResourceCleanupStatePending
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_resource_cleanup_jobs (`+taskResourceCleanupColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(operation_id) DO NOTHING
	`), job.ID, job.OperationID, job.TaskID, job.Trigger, job.State,
		job.ResourceSnapshot, job.Attempts, job.NextAttemptAt, job.LastError,
		job.CreatedAt, job.UpdatedAt, job.CompletedAt)
	return err
}

func (r *Repository) GetTaskResourceCleanupJobByOperationID(ctx context.Context, operationID string) (*models.TaskResourceCleanupJob, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT `+taskResourceCleanupColumns+`
		FROM task_resource_cleanup_jobs WHERE operation_id = ?
	`), operationID)
	return scanTaskResourceCleanupJob(row)
}

func (r *Repository) GetTaskResourceCleanupJob(ctx context.Context, id string) (*models.TaskResourceCleanupJob, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT `+taskResourceCleanupColumns+`
		FROM task_resource_cleanup_jobs WHERE id = ?
	`), id)
	return scanTaskResourceCleanupJob(row)
}

func (r *Repository) ListPreparedTaskResourceCleanupJobs(ctx context.Context) ([]*models.TaskResourceCleanupJob, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+taskResourceCleanupColumns+`
		FROM task_resource_cleanup_jobs
		WHERE state = ? ORDER BY created_at ASC
	`), models.TaskResourceCleanupStatePrepared)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	jobs := make([]*models.TaskResourceCleanupJob, 0)
	for rows.Next() {
		job, scanErr := scanTaskResourceCleanupJob(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func scanTaskResourceCleanupJob(row interface{ Scan(...any) error }) (*models.TaskResourceCleanupJob, error) {
	job := &models.TaskResourceCleanupJob{}
	err := row.Scan(&job.ID, &job.OperationID, &job.TaskID, &job.Trigger, &job.State,
		&job.ResourceSnapshot, &job.Attempts, &job.NextAttemptAt, &job.LastError,
		&job.CreatedAt, &job.UpdatedAt, &job.CompletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("task resource cleanup job not found")
	}
	return job, err
}

func (r *Repository) ListDueTaskResourceCleanupJobs(ctx context.Context, now time.Time, limit int) ([]*models.TaskResourceCleanupJob, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+taskResourceCleanupColumns+`
		FROM task_resource_cleanup_jobs
		WHERE state = ? OR (state = ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?))
		ORDER BY created_at ASC LIMIT ?
	`), models.TaskResourceCleanupStatePending, models.TaskResourceCleanupStateRetryWait, now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	jobs := make([]*models.TaskResourceCleanupJob, 0)
	for rows.Next() {
		job, scanErr := scanTaskResourceCleanupJob(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (r *Repository) MarkTaskResourceCleanupJobRunning(ctx context.Context, id string) (bool, error) {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_resource_cleanup_jobs
		SET state = ?, attempts = attempts + 1, next_attempt_at = NULL, updated_at = ?
		WHERE id = ? AND state IN (?, ?)
	`), models.TaskResourceCleanupStateRunning, now, id,
		models.TaskResourceCleanupStatePending, models.TaskResourceCleanupStateRetryWait)
	if err != nil {
		return false, err
	}
	count, _ := result.RowsAffected()
	return count == 1, nil
}

func (r *Repository) StartPreparedTaskResourceCleanupJob(ctx context.Context, id string) (bool, error) {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_resource_cleanup_jobs
		SET state = ?, next_attempt_at = NULL, updated_at = ?
		WHERE id = ? AND state = ?
	`), models.TaskResourceCleanupStatePending, now, id, models.TaskResourceCleanupStatePrepared)
	if err != nil {
		return false, err
	}
	count, _ := result.RowsAffected()
	return count == 1, nil
}

func (r *Repository) CompleteTaskResourceCleanupJob(ctx context.Context, id string, state models.TaskResourceCleanupState, lastError string, nextAttemptAt *time.Time) error {
	now := time.Now().UTC()
	var completedAt *time.Time
	if state == models.TaskResourceCleanupStateSucceeded || state == models.TaskResourceCleanupStateCancelled {
		completedAt = &now
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_resource_cleanup_jobs
		SET state = ?, last_error = ?, next_attempt_at = ?, completed_at = ?, updated_at = ?
		WHERE id = ?
	`), state, lastError, nextAttemptAt, completedAt, now, id)
	return err
}

// CompleteClaimedTaskResourceCleanupJob applies a worker result only to the
// exact running claim that produced it. A concurrent cancellation or a newer
// retry generation wins and keeps its state and historical metadata.
func (r *Repository) CompleteClaimedTaskResourceCleanupJob(
	ctx context.Context,
	id string,
	attempt int,
	state models.TaskResourceCleanupState,
	lastError string,
	nextAttemptAt *time.Time,
) (bool, error) {
	now := time.Now().UTC()
	var completedAt *time.Time
	if state == models.TaskResourceCleanupStateSucceeded || state == models.TaskResourceCleanupStateCancelled {
		completedAt = &now
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_resource_cleanup_jobs
		SET state = ?, last_error = ?, next_attempt_at = ?, completed_at = ?, updated_at = ?
		WHERE id = ? AND state = ? AND attempts = ?
	`), state, lastError, nextAttemptAt, completedAt, now, id,
		models.TaskResourceCleanupStateRunning, attempt)
	if err != nil {
		return false, err
	}
	count, _ := result.RowsAffected()
	return count == 1, nil
}

func (r *Repository) CancelArchiveTaskResourceCleanupJobs(ctx context.Context, taskID string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_resource_cleanup_jobs
		SET state = ?, completed_at = ?, updated_at = ?
		WHERE task_id = ? AND trigger IN (?, ?) AND state IN (?, ?, ?, ?)
	`), models.TaskResourceCleanupStateCancelled, now, now, taskID,
		models.TaskResourceCleanupTriggerArchive, models.TaskResourceCleanupTriggerCascadeArchive,
		models.TaskResourceCleanupStatePrepared, models.TaskResourceCleanupStatePending,
		models.TaskResourceCleanupStateRunning,
		models.TaskResourceCleanupStateRetryWait)
	return err
}

func (r *Repository) ResetRunningTaskResourceCleanupJobs(ctx context.Context) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_resource_cleanup_jobs
		SET state = ?, next_attempt_at = ?, updated_at = ? WHERE state = ?
	`), models.TaskResourceCleanupStateRetryWait, now, now, models.TaskResourceCleanupStateRunning)
	return err
}
