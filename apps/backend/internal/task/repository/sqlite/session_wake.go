package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
)

const sessionWakeColumns = `id, task_id, session_id, marker, prompt, cron_expression, timezone,
	next_run_at, expires_at, last_attempt_at, last_fired_at, last_delivery_status, last_error, created_at, updated_at`

func (r *Repository) ListTaskSessionWakes(ctx context.Context, taskID, sessionID string) ([]*models.TaskSessionWake, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`SELECT `+sessionWakeColumns+` FROM task_session_wakes WHERE task_id = ? AND session_id = ? ORDER BY created_at ASC`), taskID, sessionID)
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

func (r *Repository) UpsertTaskSessionWake(ctx context.Context, wake *models.TaskSessionWake) (created bool, err error) {
	if wake.ID == "" {
		wake.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	wake.CreatedAt = now
	wake.UpdatedAt = now
	var existing string
	err = r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT id FROM task_session_wakes WHERE task_id = ? AND session_id = ? AND marker = ?`), wake.TaskID, wake.SessionID, wake.Marker).Scan(&existing)
	if err == nil {
		created = false
		wake.ID = existing
	} else if errors.Is(err, sql.ErrNoRows) {
		created = true
	} else {
		return false, err
	}
	_, err = r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO task_session_wakes (`+sessionWakeColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, session_id, marker) DO UPDATE SET prompt=excluded.prompt, cron_expression=excluded.cron_expression, timezone=excluded.timezone, next_run_at=excluded.next_run_at, expires_at=excluded.expires_at, last_attempt_at=NULL, last_fired_at=NULL, last_delivery_status=excluded.last_delivery_status, last_error='', updated_at=excluded.updated_at`),
		wake.ID, wake.TaskID, wake.SessionID, wake.Marker, wake.Prompt, wake.CronExpression, wake.Timezone, wake.NextRunAt.UTC(), wake.ExpiresAt.UTC(), wake.LastAttemptAt, wake.LastFiredAt, wake.LastDeliveryStatus, wake.LastError, wake.CreatedAt, wake.UpdatedAt)
	return created, err
}

func (r *Repository) DeleteTaskSessionWake(ctx context.Context, taskID, sessionID, marker string) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_session_wakes WHERE task_id = ? AND session_id = ? AND marker = ?`), taskID, sessionID, marker)
	if err != nil {
		return false, err
	}
	count, _ := result.RowsAffected()
	return count == 1, nil
}

func scanSessionWake(row interface{ Scan(...any) error }) (*models.TaskSessionWake, error) {
	w := &models.TaskSessionWake{}
	err := row.Scan(&w.ID, &w.TaskID, &w.SessionID, &w.Marker, &w.Prompt, &w.CronExpression, &w.Timezone, &w.NextRunAt, &w.ExpiresAt, &w.LastAttemptAt, &w.LastFiredAt, &w.LastDeliveryStatus, &w.LastError, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("session wake not found")
	}
	return w, err
}
