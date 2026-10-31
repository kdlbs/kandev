package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Validate assignment requirements in the existing completion transaction,
// before either a Done-column move or a state-only completion can commit.
// No existing task is given a new requirement, and completed tasks stay closed.
func (r *Repository) guardRequiredSubtaskAssignmentsTx(ctx context.Context, tx *sql.Tx, taskID string, current, next v1.TaskState, workflowID, stepID string) error {
	if current == v1.TaskStateCompleted {
		return nil
	}
	required, assigned, err := r.readRequiredSubtaskAssignmentsTx(ctx, tx, taskID)
	if err != nil {
		return err
	}
	if required <= assigned {
		return nil
	}
	if next != v1.TaskStateCompleted {
		var terminal int
		if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT complete_task_on_enter FROM workflow_steps WHERE id=? AND workflow_id=?`), stepID, workflowID).Scan(&terminal); err != nil {
			return err
		}
		if terminal == 0 {
			return nil
		}
	}
	return fmt.Errorf("%w: required subtask assignments incomplete (%d/%d)", repoerrors.ErrTaskCompletionGateBlocked, assigned, required)
}

func requiredSubtaskCount(raw string) (int, error) {
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		if json.Valid([]byte(raw)) || !strings.Contains(raw, `"`+taskmodels.MetaKeyRequiredSubtaskCount+`"`) {
			return 0, nil
		}
		return 0, fmt.Errorf("read required subtask assignments: %w", err)
	}
	encodedCount, ok := metadata[taskmodels.MetaKeyRequiredSubtaskCount]
	if !ok {
		return 0, nil
	}
	var count int
	if err := json.Unmarshal(encodedCount, &count); err != nil {
		return 0, fmt.Errorf("read required subtask assignments: %w", err)
	}
	if count < 0 {
		return 0, fmt.Errorf("%w: invalid required subtask assignments", repoerrors.ErrTaskCompletionGateBlocked)
	}
	return count, nil
}

// Used by both the write guard and bounded read models; workflow defaults are
// deliberately insufficient evidence of a task-specific durable assignment.
func assignedRoutineChildrenSQL(parent string) string {
	return `CASE WHEN COALESCE(` + parent + `.metadata, '{}') LIKE '%"kandev.office_required_subtask_count"%' THEN
		(SELECT COUNT(*) FROM tasks child
		WHERE child.parent_id=` + parent + `.id AND child.workspace_id=` + parent + `.workspace_id AND child.is_ephemeral=0
		AND child.state NOT IN ('CANCELLED','FAILED')
		AND (child.archived_at IS NULL OR child.state='COMPLETED')
		AND EXISTS (
			SELECT 1 FROM workflow_step_participants seat
			WHERE seat.task_id=child.id AND seat.role='runner'
			AND COALESCE(seat.agent_profile_id, '') != ''
			AND (seat.step_id=child.workflow_step_id OR child.state='COMPLETED')
		)) ELSE 0 END`
}

func (r *Repository) readRequiredSubtaskAssignmentsTx(ctx context.Context, tx *sql.Tx, taskID string) (int, int, error) {
	var raw string
	var state v1.TaskState
	var assigned int
	query := `SELECT COALESCE(parent.metadata, '{}'), parent.state, ` + assignedRoutineChildrenSQL("parent") + ` FROM tasks parent WHERE parent.id=?`
	if err := tx.QueryRowContext(ctx, r.db.Rebind(query), taskID).Scan(&raw, &state, &assigned); err != nil {
		return 0, 0, err
	}
	if state == v1.TaskStateCompleted {
		return 0, 0, nil
	}
	required, err := requiredSubtaskCount(raw)
	return required, assigned, err
}
