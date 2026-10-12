package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type lifecycleReceiptExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type lifecycleReceiptQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (r *Repository) taskLifecycleSnapshotInTx(ctx context.Context, tx lifecycleReceiptQueryer, taskID string) (*models.Task, error) {
	var task models.Task
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT id, workspace_id, workflow_id, workflow_step_id, title, state, identifier, updated_at
		FROM tasks WHERE id = ?
	`), taskID).Scan(&task.ID, &task.WorkspaceID, &task.WorkflowID, &task.WorkflowStepID,
		&task.Title, &task.State, &task.Identifier, &task.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *Repository) recordTaskLifecycleReceipt(ctx context.Context, tx lifecycleReceiptExecutor, task *models.Task, disposition, provenance string, changedAt time.Time) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_lifecycle_receipts (
			task_id, workspace_id, identifier, title, workflow_id, workflow_step_id,
			state, disposition, changed_at, revision, provenance
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			workspace_id = excluded.workspace_id,
			identifier = excluded.identifier,
			title = excluded.title,
			workflow_id = excluded.workflow_id,
			workflow_step_id = excluded.workflow_step_id,
			state = excluded.state,
			disposition = excluded.disposition,
			changed_at = excluded.changed_at,
			revision = task_lifecycle_receipts.revision + 1,
			provenance = excluded.provenance
	`), task.ID, task.WorkspaceID, task.Identifier, task.Title, task.WorkflowID, task.WorkflowStepID,
		string(task.State), disposition, changedAt.UTC(), provenance)
	return err
}

// recordTaskLifecycleReceiptForCurrentTask records a transition from the row
// that the same transaction has just made authoritative. Keeping this lookup
// in the mutation transaction prevents a placement receipt from lagging a
// successful move or workspace transfer.
func (r *Repository) recordTaskLifecycleReceiptForCurrentTask(
	ctx context.Context,
	tx interface {
		lifecycleReceiptExecutor
		lifecycleReceiptQueryer
	},
	taskID, disposition, provenance string,
	changedAt time.Time,
) error {
	task, err := r.taskLifecycleSnapshotInTx(ctx, tx, taskID)
	if err != nil {
		return err
	}
	if task == nil {
		return errors.New("task lifecycle receipt target disappeared")
	}
	return r.recordTaskLifecycleReceipt(ctx, tx, task, disposition, provenance, changedAt)
}

// GetTaskLifecycleReceipt returns evidence only when the caller already has a
// trusted workspace identity. A foreign task is indistinguishable from absent.
func (r *Repository) GetTaskLifecycleReceipt(ctx context.Context, workspaceID, taskID string) (*models.TaskLifecycleReceipt, error) {
	var receipt models.TaskLifecycleReceipt
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT task_id, workspace_id, identifier, title, workflow_id, workflow_step_id,
			state, disposition, changed_at, revision, provenance
		FROM task_lifecycle_receipts WHERE workspace_id = ? AND task_id = ?
	`), workspaceID, taskID).Scan(&receipt.TaskID, &receipt.WorkspaceID, &receipt.Identifier,
		&receipt.Title, &receipt.WorkflowID, &receipt.WorkflowStepID, &receipt.State,
		&receipt.Disposition, &receipt.ChangedAt, &receipt.Revision, &receipt.Provenance)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &receipt, nil
}
