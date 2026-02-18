package sqlite

import (
	"context"
	"fmt"
	"time"
)

// UpdateSessionWorkflowStep updates the workflow step for a session
func (r *Repository) UpdateSessionWorkflowStep(ctx context.Context, sessionID string, stepID string) error {
	now := time.Now().UTC()

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions SET workflow_step_id = ?, updated_at = ? WHERE id = ?
	`), stepID, now, sessionID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	return nil
}

// UpdateSessionReviewStatus updates the review status of a session
func (r *Repository) UpdateSessionReviewStatus(ctx context.Context, sessionID string, status string) error {
	now := time.Now().UTC()

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions SET review_status = ?, updated_at = ? WHERE id = ?
	`), status, now, sessionID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	return nil
}
