package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

const taskResourceCleanupColumns = `
	id, operation_id, task_id, trigger, state, resource_snapshot, attempts,
	next_attempt_at, last_error, created_at, updated_at, completed_at`

func (r *Repository) CreateTaskResourceCleanupJob(ctx context.Context, job *models.TaskResourceCleanupJob) error {
	_, err := r.createTaskResourceCleanupJob(ctx, job, nil)
	return err
}

// CreateArchiveReclaimTaskResourceCleanupJob inserts a reclaim candidate only
// while the task still has the archive generation that produced it. The task
// row lock is shared with unarchive, which also checks for active archive jobs
// before clearing archived_at.
func (r *Repository) CreateArchiveReclaimTaskResourceCleanupJob(
	ctx context.Context,
	job *models.TaskResourceCleanupJob,
	archivedAt time.Time,
) (bool, error) {
	if job == nil || job.Trigger != models.TaskResourceCleanupTriggerArchiveReclaim {
		return false, errors.New("archive reclaim cleanup job is required")
	}
	if archivedAt.IsZero() {
		return false, errors.New("archive reclaim generation is required")
	}
	return r.createTaskResourceCleanupJob(ctx, job, &archivedAt)
}

func (r *Repository) createTaskResourceCleanupJob(
	ctx context.Context,
	job *models.TaskResourceCleanupJob,
	expectedArchivedAt *time.Time,
) (bool, error) {
	if job == nil {
		return false, errors.New("task resource cleanup job is nil")
	}
	if job.ID == "" {
		job.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now
	if job.State == "" {
		job.State = models.TaskResourceCleanupStatePending
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := recoveryclaim.EnsureTaskAvailableTx(ctx, r.db, tx, job.TaskID); err != nil {
		return false, err
	}
	if expectedArchivedAt != nil {
		var archivedAt sql.NullTime
		err := tx.QueryRowContext(ctx, r.db.Rebind(`
			SELECT archived_at FROM tasks WHERE id = ?
		`), job.TaskID).Scan(&archivedAt)
		matchesGeneration := err == nil && archivedAt.Valid && archivedAt.Time.Equal(*expectedArchivedAt)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		if !matchesGeneration {
			if commitErr := tx.Commit(); commitErr != nil {
				return false, commitErr
			}
			return false, nil
		}
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_resource_cleanup_jobs (`+taskResourceCleanupColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(operation_id) DO NOTHING
	`), job.ID, job.OperationID, job.TaskID, job.Trigger, job.State,
		job.ResourceSnapshot, job.Attempts, job.NextAttemptAt, job.LastError,
		job.CreatedAt, job.UpdatedAt, job.CompletedAt)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// UpdateTaskResourceCleanupSnapshot writes the resource inventory captured
// after the prepared barrier was reserved. The barrier row exists before the
// inventory query so concurrent session/worktree creation is rejected while
// the snapshot is being assembled.
func (r *Repository) UpdateTaskResourceCleanupSnapshot(ctx context.Context, operationID, snapshot string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_resource_cleanup_jobs
		SET resource_snapshot = ?, updated_at = ?
		WHERE operation_id = ? AND state = ?
	`), snapshot, time.Now().UTC(), operationID, models.TaskResourceCleanupStatePrepared)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task resource cleanup job %s not found or not prepared", operationID)
	}
	return nil
}

// UpdateClaimedTaskResourceCleanupSnapshot persists outcomes produced by one
// exact running cleanup attempt. A newer retry or cancellation wins when the
// claim no longer matches.
func (r *Repository) UpdateClaimedTaskResourceCleanupSnapshot(ctx context.Context, id string, attempt int, snapshot string) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_resource_cleanup_jobs
		SET resource_snapshot = ?, updated_at = ?
		WHERE id = ? AND state = ? AND attempts = ?
	`), snapshot, time.Now().UTC(), id, models.TaskResourceCleanupStateRunning, attempt)
	if err != nil {
		return false, err
	}
	rows, _ := result.RowsAffected()
	return rows == 1, nil
}

// HasActiveTaskResourceCleanupJob reports whether teardown has been admitted
// for a task. The prepared state is included because the cleanup intent is
// persisted before task deletion and before the worker is allowed to run.
func (r *Repository) HasActiveTaskResourceCleanupJob(ctx context.Context, taskID string) (bool, error) {
	var active bool
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT EXISTS (
			SELECT 1 FROM task_resource_cleanup_jobs
			WHERE task_id = ? AND state IN (?, ?, ?, ?, ?)
		)
	`), taskID,
		models.TaskResourceCleanupStatePrepared,
		models.TaskResourceCleanupStatePending,
		models.TaskResourceCleanupStateRunning,
		models.TaskResourceCleanupStateRetryWait,
		models.TaskResourceCleanupStateWaitingForClean,
	).Scan(&active)
	return active, err
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

func (r *Repository) ListArchiveTaskResourceCleanupJobs(
	ctx context.Context, taskID string,
) ([]*models.TaskResourceCleanupJob, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+taskResourceCleanupColumns+`
		FROM task_resource_cleanup_jobs
		WHERE task_id = ? AND trigger IN (?, ?, ?)
		ORDER BY created_at ASC
	`), taskID, models.TaskResourceCleanupTriggerArchive,
		models.TaskResourceCleanupTriggerCascadeArchive,
		models.TaskResourceCleanupTriggerArchiveReclaim)
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
		WHERE state = ? OR ((state = ? OR state = ?) AND (next_attempt_at IS NULL OR next_attempt_at <= ?))
		ORDER BY created_at ASC LIMIT ?
	`), models.TaskResourceCleanupStatePending,
		models.TaskResourceCleanupStateRetryWait, models.TaskResourceCleanupStateWaitingForClean,
		now.UTC(), limit)
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

func (r *Repository) ListArchivedActiveWorktreeReclaimCandidates(
	ctx context.Context,
	taskID string,
	afterWorktreeID string,
	limit int,
) ([]*models.TaskArchiveReclaimCandidate, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `
		SELECT t.id, t.archived_at, ter.worktree_id, ter.worktree_path, COALESCE(r.local_path, '')
		FROM tasks t
		INNER JOIN task_environments te ON te.task_id = t.id
		INNER JOIN task_environment_repos ter ON ter.task_environment_id = te.id
		LEFT JOIN repositories r ON r.id = ter.repository_id
		WHERE t.archived_at IS NOT NULL
			AND ter.status = 'active' AND ter.deleted_at IS NULL
			AND COALESCE(ter.worktree_id, '') <> ''
			AND COALESCE(ter.worktree_path, '') <> ''
			AND COALESCE(r.local_path, '') <> ''
			AND ter.worktree_id > ?`
	args := []any{afterWorktreeID}
	if taskID != "" {
		query += ` AND t.id = ?`
		args = append(args, taskID)
	}
	query += ` ORDER BY ter.worktree_id ASC LIMIT ?`
	args = append(args, limit)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	candidates := make([]*models.TaskArchiveReclaimCandidate, 0, limit)
	for rows.Next() {
		candidate := &models.TaskArchiveReclaimCandidate{}
		if err := rows.Scan(
			&candidate.TaskID, &candidate.ArchivedAt, &candidate.WorktreeID,
			&candidate.WorktreePath, &candidate.RepositoryPath,
		); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func (r *Repository) MarkTaskResourceCleanupJobRunning(ctx context.Context, id string) (bool, error) {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_resource_cleanup_jobs
		SET state = ?, attempts = attempts + 1, next_attempt_at = NULL, updated_at = ?
		WHERE id = ? AND state IN (?, ?, ?)
	`), models.TaskResourceCleanupStateRunning, now, id,
		models.TaskResourceCleanupStatePending, models.TaskResourceCleanupStateRetryWait,
		models.TaskResourceCleanupStateWaitingForClean)
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

func (r *Repository) CompleteTaskResourceCleanupJob(
	ctx context.Context,
	id string,
	state models.TaskResourceCleanupState,
	lastError string,
	nextAttemptAt *time.Time,
) error {
	now := time.Now().UTC()
	var completedAt *time.Time
	if state == models.TaskResourceCleanupStateSucceeded ||
		state == models.TaskResourceCleanupStateFailed ||
		state == models.TaskResourceCleanupStateCancelled {
		completedAt = &now
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_resource_cleanup_jobs
		SET state = ?, last_error = ?, next_attempt_at = ?, completed_at = ?, updated_at = ?
		WHERE id = ?
	`), state, lastError, nextAttemptAt, completedAt, now, id)
	return err
}

// RestoreCancelledTaskResourceCleanupJobIfUnchanged re-prepares only the
// cancelled cleanup generation that the caller inspected. The state and
// attempt predicates prevent a delayed restore from overwriting a newer
// prepared, pending, or running generation.
func (r *Repository) RestoreCancelledTaskResourceCleanupJobIfUnchanged(
	ctx context.Context,
	id string,
	attempts int,
	lastError string,
) (bool, error) {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_resource_cleanup_jobs
		SET state = ?, last_error = ?, next_attempt_at = NULL, completed_at = NULL, updated_at = ?
		WHERE id = ? AND state = ? AND attempts = ?
	`), models.TaskResourceCleanupStatePrepared, lastError, now,
		id, models.TaskResourceCleanupStateCancelled, attempts)
	if err != nil {
		return false, err
	}
	count, _ := result.RowsAffected()
	return count == 1, nil
}

// CancelTaskResourceCleanupJobIfPending cancels only an eligible cleanup
// generation. Running claims are left untouched for physical reconciliation.
func (r *Repository) CancelTaskResourceCleanupJobIfPending(ctx context.Context, id string) (bool, error) {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_resource_cleanup_jobs
		SET state = ?, next_attempt_at = NULL, completed_at = ?, updated_at = ?
		WHERE id = ? AND state IN (?, ?, ?, ?)
	`),
		models.TaskResourceCleanupStateCancelled, now, now, id,
		models.TaskResourceCleanupStatePrepared,
		models.TaskResourceCleanupStatePending,
		models.TaskResourceCleanupStateRetryWait,
		models.TaskResourceCleanupStateWaitingForClean,
	)
	if err != nil {
		return false, err
	}
	count, _ := result.RowsAffected()
	return count == 1, nil
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
	if state == models.TaskResourceCleanupStateSucceeded ||
		state == models.TaskResourceCleanupStateFailed ||
		state == models.TaskResourceCleanupStateCancelled {
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
		WHERE task_id = ? AND trigger IN (?, ?, ?) AND state IN (?, ?, ?, ?)
	`), models.TaskResourceCleanupStateCancelled, now, now, taskID,
		models.TaskResourceCleanupTriggerArchive, models.TaskResourceCleanupTriggerCascadeArchive,
		models.TaskResourceCleanupTriggerArchiveReclaim,
		models.TaskResourceCleanupStatePrepared, models.TaskResourceCleanupStatePending,
		models.TaskResourceCleanupStateRetryWait, models.TaskResourceCleanupStateWaitingForClean)
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
