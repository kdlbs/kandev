package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kandev/kandev/internal/office/models"
)

// ListTaskCommentsWindow returns the newest `limit` comments for a task
// (by created_at DESC, id DESC), presented ascending by the same tiebreak
// columns, alongside the task's total comment count. The count and the
// window are read inside a single read-only transaction so a concurrent
// write is either fully visible to both or fully invisible to both
// (AC-003.7, AC-006.1/AC-006.2). ReadOnly+RepeatableRead is a no-op on
// SQLite (WAL already snapshots per-transaction) but documents intent and
// matches the internal/automation/export_store.go precedent.
func (r *Repository) ListTaskCommentsWindow(ctx context.Context, taskID string, limit int) ([]*models.TaskComment, int, error) {
	tx, err := r.ro.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, 0, fmt.Errorf("begin read tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var total int
	if err := tx.QueryRowxContext(ctx, tx.Rebind(
		`SELECT COUNT(*) FROM task_comments WHERE task_id = ?`), taskID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count comments: %w", err)
	}

	var comments []*models.TaskComment
	if err := tx.SelectContext(ctx, &comments, tx.Rebind(`
		SELECT * FROM task_comments
		WHERE task_id = ?
		ORDER BY created_at DESC, id DESC
		LIMIT ?
	`), taskID, limit); err != nil {
		return nil, 0, fmt.Errorf("select comments: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, 0, fmt.Errorf("commit read tx: %w", err)
	}

	for i, j := 0, len(comments)-1; i < j; i, j = i+1, j-1 {
		comments[i], comments[j] = comments[j], comments[i]
	}
	if comments == nil {
		comments = []*models.TaskComment{}
	}
	return comments, total, nil
}
