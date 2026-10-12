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
		SELECT id, workspace_id, COALESCE(workflow_id, ''), COALESCE(workflow_step_id, ''),
			COALESCE(title, ''), state, COALESCE(identifier, ''), updated_at
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

// GetTaskVisibilitySnapshot reads the current compact task projection and its
// workspace-bound lifecycle evidence from one read transaction. The caller
// uses the pair together, so a lifecycle write cannot combine one version of
// the task with another version of its receipt.
func (r *Repository) GetTaskVisibilitySnapshot(ctx context.Context, workspaceID, taskID string) (*models.Task, *models.TaskLifecycleReceipt, error) {
	tx, err := r.ro.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	task, err := r.taskLifecycleSnapshotInTx(ctx, tx, taskID)
	if err != nil {
		return nil, nil, err
	}
	var receipt models.TaskLifecycleReceipt
	err = tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT task_id, workspace_id, identifier, title, workflow_id, workflow_step_id,
			state, disposition, changed_at, revision, provenance
		FROM task_lifecycle_receipts WHERE workspace_id = ? AND task_id = ?
	`), workspaceID, taskID).Scan(&receipt.TaskID, &receipt.WorkspaceID, &receipt.Identifier,
		&receipt.Title, &receipt.WorkflowID, &receipt.WorkflowStepID, &receipt.State,
		&receipt.Disposition, &receipt.ChangedAt, &receipt.Revision, &receipt.Provenance)
	if errors.Is(err, sql.ErrNoRows) {
		receipt = models.TaskLifecycleReceipt{}
	} else if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	if receipt.TaskID == "" {
		return task, nil, nil
	}
	return task, &receipt, nil
}
