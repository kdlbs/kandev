package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// ListStepSignalWatchdogSessions returns observations for the periodic signal
// scan. Recovery must re-read the full session under its existing guards.
func (r *Repository) ListStepSignalWatchdogSessions(ctx context.Context) ([]*models.TaskSession, error) {
	rows, err := r.ro.QueryContext(ctx, stepSignalWatchdogQuery(r.ro.DriverName()))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var sessions []*models.TaskSession
	for rows.Next() {
		var session models.TaskSession
		var metadata string
		if err := rows.Scan(&session.ID, &session.TaskID, &session.State, &session.IsPassthrough, &metadata); err != nil {
			return nil, err
		}
		if err := unmarshalSessionJSON(metadata, &session.Metadata, "watchdog session metadata"); err != nil {
			return nil, err
		}
		sessions = append(sessions, &session)
	}
	return sessions, rows.Err()
}

func stepSignalWatchdogQuery(driver string) string {
	metadata := "(CASE WHEN metadata IS NULL OR metadata = '' THEN '{}' ELSE metadata END)"
	key := models.SessionMetaKeyPendingStepCompletion
	projection := "CASE WHEN json_type(" + metadata + ") = 'object' THEN json_object('" + key + "', json_extract(" + metadata + ", '$." + key + "')) ELSE " + metadata + " END"
	if dialect.IsPostgres(driver) {
		metadata += "::jsonb"
		projection = "(CASE WHEN jsonb_typeof(" + metadata + ") = 'object' THEN jsonb_build_object('" + key + "', " + metadata + "->'" + key + "') ELSE " + metadata + " END)::text"
	}
	return `SELECT id, task_id, state, is_passthrough, ` + projection + `
		FROM task_sessions
		WHERE state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
		ORDER BY started_at DESC`
}
