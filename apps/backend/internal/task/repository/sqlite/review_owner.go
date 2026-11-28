package sqlite

import "context"

// SetTaskReviewRunReviewer fences attribution against cancellation or execution.
// It updates only the existing reviewer identity, never the run's terminal state.
func (r *Repository) SetTaskReviewRunReviewer(ctx context.Context, runID, agentID, model string) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE task_review_runs SET agent_id = ?, model = ? WHERE id = ? AND status = 'pending'`), agentID, model, runID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// StartTaskReviewRunIfPending is the final pending-state claim for a native
// route. A cancel before the detached worker starts stays terminal.
func (r *Repository) StartTaskReviewRunIfPending(ctx context.Context, runID string) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE task_review_runs SET status = 'running' WHERE id = ? AND status = 'pending'`), runID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// SetTaskReviewRunRunningReviewer records a changed candidate before inference.
// Only the live running row can change; terminal history stays immutable.
func (r *Repository) SetTaskReviewRunRunningReviewer(ctx context.Context, runID, agentID, model string) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE task_review_runs SET agent_id = ?, model = ? WHERE id = ? AND status = 'running'`), agentID, model, runID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}
