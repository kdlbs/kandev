package sqlite

import (
	"context"
	"github.com/kandev/kandev/internal/runs/models"
)

// LatestRunForSession returns the current run identity, including terminal runs.
func (r *Repository) LatestRunForSession(ctx context.Context, sessionID string) (*models.Run, error) {
	var run models.Run
	err := r.ro.GetContext(ctx, &run, r.ro.Rebind(`SELECT * FROM runs WHERE session_id=? ORDER BY requested_at DESC,id DESC LIMIT 1`), sessionID)
	return &run, err
}

// RecordFailure retains the diagnostic used by both run inspection and chat badges.
func (r *Repository) RecordFailure(ctx context.Context, id, message string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE runs SET failure_reason=?,error_message=? WHERE id=?`), message, message, id)
	return err
}
