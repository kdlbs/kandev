package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// MarkTaskWorktreeDetached records that the user removed a worktree from a
// task. Detection must not adopt the path again until the agent reports it.
func (r *Repository) MarkTaskWorktreeDetached(ctx context.Context, taskID, worktreePath string, detachedAt time.Time) error {
	if taskID == "" || worktreePath == "" {
		return fmt.Errorf("task_id and worktree_path are required")
	}
	if detachedAt.IsZero() {
		detachedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_detached_worktrees (task_id, worktree_path, detached_at)
		VALUES (?, ?, ?)
		ON CONFLICT (task_id, worktree_path) DO UPDATE SET detached_at = EXCLUDED.detached_at
	`), taskID, worktreePath, detachedAt.UTC())
	return err
}

// ClearTaskWorktreeDetached removes a detachment record after an explicit
// agent report re-attaches the worktree.
func (r *Repository) ClearTaskWorktreeDetached(ctx context.Context, taskID, worktreePath string) error {
	if taskID == "" || worktreePath == "" {
		return nil
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM task_detached_worktrees WHERE task_id = ? AND worktree_path = ?
	`), taskID, worktreePath)
	return err
}

// IsTaskWorktreeDetached reports whether the path was removed from the task.
func (r *Repository) IsTaskWorktreeDetached(ctx context.Context, taskID, worktreePath string) (bool, error) {
	if taskID == "" || worktreePath == "" {
		return false, nil
	}
	var exists int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT 1 FROM task_detached_worktrees WHERE task_id = ? AND worktree_path = ?
	`), taskID, worktreePath).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// DeleteTaskDetachedWorktreesByTask clears the task's suppression rows.
func (r *Repository) DeleteTaskDetachedWorktreesByTask(ctx context.Context, taskID string) error {
	if taskID == "" {
		return nil
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_detached_worktrees WHERE task_id = ?`), taskID)
	return err
}

// DetachTaskRepositoryAttachment atomically records the suppression row (when
// requested), marks the environment inventory row detached for later cleanup,
// and removes the task-side attachment. A failure rolls back all three.
func (r *Repository) DetachTaskRepositoryAttachment(ctx context.Context, taskID, taskRepositoryID, environmentRepoID, worktreePath string, markDetached bool) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if markDetached && worktreePath != "" {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO task_detached_worktrees (task_id, worktree_path, detached_at)
			VALUES (?, ?, ?)
			ON CONFLICT (task_id, worktree_path) DO UPDATE SET detached_at = EXCLUDED.detached_at
		`), taskID, worktreePath, time.Now().UTC()); err != nil {
			return fmt.Errorf("record worktree detachment: %w", err)
		}
	}
	now := time.Now().UTC()
	if environmentRepoID != "" {
		result, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE task_environment_repos
			SET status = ?, updated_at = ?
			WHERE id = ? AND EXISTS (
				SELECT 1 FROM task_environments WHERE id = task_environment_id AND task_id = ?
			)
		`), models.TaskEnvironmentRepoStatusDetached, now, environmentRepoID, taskID)
		if err != nil {
			return fmt.Errorf("mark environment repository detached: %w", err)
		}
		rows, _ := result.RowsAffected()
		if rows != 1 {
			return fmt.Errorf("environment repository not found: %s", environmentRepoID)
		}
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_repositories WHERE id = ? AND task_id = ?`), taskRepositoryID, taskID)
	if err != nil {
		return fmt.Errorf("delete task repository: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return fmt.Errorf("task repository not found: %s", taskRepositoryID)
	}
	return tx.Commit()
}
