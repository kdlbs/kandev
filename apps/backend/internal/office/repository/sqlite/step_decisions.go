package sqlite

import (
	"context"
	"database/sql"
	"errors"
)

// HasActiveStepDecision reports whether an active (non-superseded) row exists
// in workflow_step_decisions for the given task, step, and decider. It backs
// the review/approval completion check that flags a reviewer or approver
// turn that ended without recording a verdict via record_step_decision_kandev.
func (r *Repository) HasActiveStepDecision(ctx context.Context, taskID, stepID, deciderID string) (bool, error) {
	var exists int
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT 1 FROM workflow_step_decisions
		WHERE task_id = ? AND step_id = ? AND decider_id = ? AND superseded_at IS NULL
		LIMIT 1
	`), taskID, stepID, deciderID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
