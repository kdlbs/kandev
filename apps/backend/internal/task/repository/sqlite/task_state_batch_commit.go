package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type pendingTaskStateBatchChange struct {
	taskID   string
	oldState v1.TaskState
}

// CommitTaskStateBatch commits every changed task state and related checkout
// release before returning any publication receipts.
func (r *Repository) CommitTaskStateBatch(
	ctx context.Context, write models.TaskStateBatchWrite,
) ([]models.TaskStateChange, error) {
	write.State = normalizeCommittedTaskState(write.State)
	tx, err := r.db.BeginTx(ctx, r.hierarchyTxOptions())
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	changes := make([]pendingTaskStateBatchChange, 0, len(write.TaskIDs))
	seen := make(map[string]struct{}, len(write.TaskIDs))
	for _, taskID := range write.TaskIDs {
		if taskID == "" {
			continue
		}
		if _, ok := seen[taskID]; ok {
			continue
		}
		seen[taskID] = struct{}{}

		change, changed, err := r.commitTaskStateBatchMemberTx(ctx, tx, taskID, write.State)
		if err != nil {
			return nil, err
		}
		if changed {
			changes = append(changes, change)
		}
	}

	if err := r.releaseTaskCheckoutsTx(ctx, tx, write.ReleaseCheckoutTaskIDs); err != nil {
		return nil, err
	}

	receipts := make([]models.TaskStateChange, 0, len(changes))
	for _, change := range changes {
		task, err := r.readTaskStateSnapshot(ctx, tx, change.taskID)
		if err != nil {
			return nil, err
		}
		task.State = normalizeCommittedTaskState(task.State)
		receipts = append(receipts, models.TaskStateChange{
			Matched:  true,
			Changed:  true,
			OldState: change.oldState,
			Task:     task,
		})
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return receipts, nil
}

func (r *Repository) commitTaskStateBatchMemberTx(
	ctx context.Context, tx *sql.Tx, taskID string, newState v1.TaskState,
) (pendingTaskStateBatchChange, bool, error) {
	rawState, workflowID, stepID, err := r.readTaskStateForCommit(ctx, tx, taskID)
	if errors.Is(err, ErrTaskNotFound) {
		return pendingTaskStateBatchChange{}, false, nil
	}
	if err != nil {
		return pendingTaskStateBatchChange{}, false, err
	}
	oldState := normalizeCommittedTaskState(rawState)
	if oldState == newState {
		return pendingTaskStateBatchChange{}, false, nil
	}
	if err := r.guardTaskCompletionStateTx(ctx, tx, taskID, oldState, newState, workflowID, stepID); err != nil {
		return pendingTaskStateBatchChange{}, false, err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(
		`UPDATE tasks SET state = ?, updated_at = ? WHERE id = ? AND state = ? AND COALESCE(workflow_step_id, '') = ?`,
	), newState, r.nowUTC(), taskID, rawState, stepID)
	if err != nil {
		return pendingTaskStateBatchChange{}, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return pendingTaskStateBatchChange{}, false, err
	}
	if rows == 0 {
		return pendingTaskStateBatchChange{}, false, fmt.Errorf("task state changed while committing task %s", taskID)
	}
	return pendingTaskStateBatchChange{taskID: taskID, oldState: oldState}, true, nil
}

func (r *Repository) releaseTaskCheckoutsTx(ctx context.Context, tx *sql.Tx, taskIDs []string) error {
	uniqueIDs := make([]string, 0, len(taskIDs))
	seen := make(map[string]struct{}, len(taskIDs))
	for _, taskID := range taskIDs {
		if taskID == "" {
			continue
		}
		if _, ok := seen[taskID]; ok {
			continue
		}
		seen[taskID] = struct{}{}
		uniqueIDs = append(uniqueIDs, taskID)
	}
	if len(uniqueIDs) == 0 {
		return nil
	}
	placeholders := make([]byte, 0, len(uniqueIDs)*2)
	args := make([]any, 0, len(uniqueIDs)+1)
	args = append(args, r.nowUTC())
	for i, taskID := range uniqueIDs {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args = append(args, taskID)
	}
	query := fmt.Sprintf(
		`UPDATE tasks SET checkout_agent_id = '', checkout_at = NULL, checkout_run_id = NULL, updated_at = ? WHERE id IN (%s)`,
		placeholders,
	)
	_, err := tx.ExecContext(ctx, r.db.Rebind(query), args...)
	return err
}
