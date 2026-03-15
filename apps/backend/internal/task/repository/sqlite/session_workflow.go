package sqlite

import (
	"context"
	"fmt"
	"time"
)

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
