package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

// ListAdmittedSessionIDs returns the ids of every session in the admitted
// population — state STARTING or RUNNING — across the entire instance: every
// workspace, every workflow, every board, Office and non-Office alike.
//
// The query deliberately has no joins and no filters. A session counts regardless
// of whether its task is archived, ephemeral or automation-origin, and regardless
// of config_mode or IsPassthrough on the session itself: each one runs an agent
// process and consumes a core. Adding any of the filters the neighbouring task
// queries carry would under-count exactly the work the ceiling exists to bound.
//
// The state literals here are written out rather than derived from
// models.IsAdmittedSessionState on purpose: deriving them would make the
// drift-guard test compare the predicate against itself. Keeping the two sides
// independent is what lets TestAdmittedSessionIDsMatchSQLFilter catch an edit to
// one that was not made to the other.
func (r *Repository) ListAdmittedSessionIDs(ctx context.Context) ([]string, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT s.id
		FROM task_sessions s
		WHERE s.state IN (?, ?)
		ORDER BY s.id ASC
	`), string(models.TaskSessionStateStarting), string(models.TaskSessionStateRunning))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}
