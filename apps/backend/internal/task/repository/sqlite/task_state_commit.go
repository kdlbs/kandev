package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// CommitTaskState applies one guarded state change and returns the committed
// task snapshot with its authoritative previous state.
func (r *Repository) CommitTaskState(
	ctx context.Context, taskID string, write models.TaskStateWrite,
) (models.TaskStateChange, error) {
	write.State = normalizeCommittedTaskState(write.State)
	tx, err := r.db.BeginTx(ctx, r.hierarchyTxOptions())
	if err != nil {
		return models.TaskStateChange{}, err
	}
	defer func() { _ = tx.Rollback() }()

	rawState, workflowID, stepID, err := r.readTaskStateForCommit(ctx, tx, taskID)
	if err != nil {
		return models.TaskStateChange{}, err
	}
	currentState := normalizeCommittedTaskState(rawState)
	if !taskStateWriteMatches(write, currentState, stepID) && !taskStateWriteAcceptsAlreadyAtTarget(write, currentState, stepID) {
		return models.TaskStateChange{}, nil
	}
	if currentState != write.State {
		if err := r.guardTaskCompletionStateTx(ctx, tx, taskID, currentState, write.State, workflowID, stepID); err != nil {
			return models.TaskStateChange{}, err
		}
		if err := r.writeTaskStateCommit(ctx, tx, taskID, rawState, stepID, write); err != nil {
			return models.TaskStateChange{}, err
		}
	}

	task, err := r.readTaskStateSnapshot(ctx, tx, taskID)
	if err != nil {
		return models.TaskStateChange{}, err
	}
	task.State = normalizeCommittedTaskState(task.State)
	if err := tx.Commit(); err != nil {
		return models.TaskStateChange{}, err
	}
	return models.TaskStateChange{
		Matched:  true,
		Changed:  currentState != write.State,
		OldState: currentState,
		Task:     task,
	}, nil
}

func taskStateWriteAcceptsAlreadyAtTarget(write models.TaskStateWrite, currentState v1.TaskState, stepID string) bool {
	if !write.AcceptIfAlreadyAtTarget || write.ExpectedState == nil || currentState != write.State {
		return false
	}
	return write.ExpectedWorkflowStepID == nil || stepID == *write.ExpectedWorkflowStepID
}

func (r *Repository) readTaskStateForCommit(
	ctx context.Context, tx *sql.Tx, taskID string,
) (v1.TaskState, string, string, error) {
	query := `SELECT state, workflow_id, workflow_step_id FROM tasks WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	var state v1.TaskState
	var workflowID, stepID string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(query), taskID).Scan(&state, &workflowID, &stepID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", "", fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
		}
		return "", "", "", err
	}
	return state, workflowID, stepID, nil
}

func taskStateWriteMatches(write models.TaskStateWrite, state v1.TaskState, stepID string) bool {
	if write.ExpectedState != nil && state != normalizeCommittedTaskState(*write.ExpectedState) {
		return false
	}
	return write.ExpectedWorkflowStepID == nil || stepID == *write.ExpectedWorkflowStepID
}

func normalizeCommittedTaskState(state v1.TaskState) v1.TaskState {
	return v1.TaskState(strings.ToUpper(string(state)))
}

func (r *Repository) writeTaskStateCommit(
	ctx context.Context, tx *sql.Tx, taskID string, currentState v1.TaskState, currentStepID string,
	write models.TaskStateWrite,
) error {
	query := `UPDATE tasks SET state = ?, updated_at = ? WHERE id = ? AND state = ? AND COALESCE(workflow_step_id, '') = ?`
	args := []any{write.State, r.nowUTC(), taskID, currentState, currentStepID}
	if write.ExpectedWorkflowStepID != nil {
		query += ` AND COALESCE(workflow_step_id, '') = ?`
		args = append(args, *write.ExpectedWorkflowStepID)
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("task state changed while committing task %s", taskID)
	}
	return nil
}

func (r *Repository) readTaskStateSnapshot(ctx context.Context, tx *sql.Tx, taskID string) (*models.Task, error) {
	task, err := r.scanSingleTask(tx.QueryRowContext(ctx,
		r.db.Rebind(`SELECT `+taskSelectColumns("t")+` FROM tasks t WHERE t.id = ?`), taskID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}
	return task, err
}
