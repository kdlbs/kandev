package sqlite

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// CreateTaskBlocker creates a blocker relationship between two tasks.
func (r *Repository) CreateTaskBlocker(ctx context.Context, blocker *models.TaskBlocker) error {
	blocker.CreatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_blockers (task_id, blocker_task_id, created_at)
		VALUES (?, ?, ?)
	`), blocker.TaskID, blocker.BlockerTaskID, blocker.CreatedAt)
	return err
}

// ListTaskBlockers returns all blockers for a task.
func (r *Repository) ListTaskBlockers(ctx context.Context, taskID string) ([]*models.TaskBlocker, error) {
	var blockers []*models.TaskBlocker
	err := r.ro.SelectContext(ctx, &blockers, r.ro.Rebind(
		`SELECT * FROM task_blockers WHERE task_id = ? ORDER BY created_at`), taskID)
	if err != nil {
		return nil, err
	}
	if blockers == nil {
		blockers = []*models.TaskBlocker{}
	}
	return blockers, nil
}

// DeleteTaskBlocker removes a blocker relationship.
func (r *Repository) DeleteTaskBlocker(ctx context.Context, taskID, blockerTaskID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM task_blockers WHERE task_id = ? AND blocker_task_id = ?`), taskID, blockerTaskID)
	return err
}
