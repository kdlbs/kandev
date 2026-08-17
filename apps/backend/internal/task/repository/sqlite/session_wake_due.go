package sqlite

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) ListDueTaskSessionWakes(ctx context.Context, now time.Time, limit int) ([]*models.TaskSessionWake, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`SELECT `+sessionWakeColumns+` FROM task_session_wakes WHERE next_run_at <= ? AND expires_at > ? ORDER BY next_run_at ASC LIMIT ?`), now.UTC(), now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var wakes []*models.TaskSessionWake
	for rows.Next() {
		wake, scanErr := scanSessionWake(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		wakes = append(wakes, wake)
	}
	return wakes, rows.Err()
}

// ClaimTaskSessionWake advances the schedule before dispatch. The expected
// timestamp is the compare-and-swap fence shared backend processes use to
// prevent duplicate fire delivery.
func (r *Repository) ClaimTaskSessionWake(ctx context.Context, id string, expectedNextRunAt, nextRunAt, attemptedAt time.Time) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE task_session_wakes SET next_run_at = ?, last_attempt_at = ?, updated_at = ? WHERE id = ? AND next_run_at = ? AND expires_at > ?`), nextRunAt.UTC(), attemptedAt.UTC(), attemptedAt.UTC(), id, expectedNextRunAt.UTC(), attemptedAt.UTC())
	if err != nil {
		return false, err
	}
	count, _ := result.RowsAffected()
	return count == 1, nil
}

func (r *Repository) RecordTaskSessionWakeDelivery(ctx context.Context, id, status, lastError string, firedAt *time.Time) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE task_session_wakes SET last_delivery_status = ?, last_error = ?, last_fired_at = ?, updated_at = ? WHERE id = ?`), status, lastError, firedAt, time.Now().UTC(), id)
	return err
}
