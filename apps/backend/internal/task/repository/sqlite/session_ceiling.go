package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/db/dialect"
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

// ListTasksWithCeilingDeferral returns every task carrying a ceiling deferral, in
// the order the sweep drains them.
//
// This is deliberately not ListTasksWithMetadataKey. That function's transaction
// shape, JSON predicate and driver split are the right shapes and are reused here,
// but its WHERE and its ORDER BY are both wrong for this caller:
//
//   - it adds t.archived_at IS NULL, which hides exactly the archived tasks whose
//     stranded records the sweep exists to drop;
//   - it adds t.is_ephemeral = 0, which would exempt ephemeral tasks that count
//     toward the population on the same terms as any other;
//   - it adds andNotAutomationOriginT, which would exclude Office work.
//
// and it ends ORDER BY t.updated_at ASC, t.created_at ASC, t.id ASC. This method
// orders by t.id ascending alone, and that IS the drain order — no in-memory sort
// is applied to it. id is unique, so the order is total and needs no tiebreak, and
// it is stable across ticks. A leading updated_at term would not be: updated_at is
// not append-only, so any unrelated write to a deferred task would move it in the
// drain order between ticks.
func (r *Repository) ListTasksWithCeilingDeferral(ctx context.Context) ([]*models.Task, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+taskSelectColumns("t")+`
		FROM tasks t
		WHERE `+ceilingDeferredPredicate(r.ro.DriverName())+`
		ORDER BY t.id ASC
	`))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return r.scanTasks(rows)
}

// ceilingDeferredPredicate matches tasks whose shared deferred_launch record
// carries ceiling_deferred set to true. The nested path is what discriminates
// this sweep's candidates from the other two meanings that share the record; a
// mere key-presence test would also match a record left behind with the flag
// explicitly false.
func ceilingDeferredPredicate(driver string) string {
	if dialect.IsPostgres(driver) {
		return "jsonb_extract_path(CASE WHEN t.metadata IS NULL OR t.metadata = 'null' OR t.metadata = '' THEN '{}'::jsonb ELSE t.metadata::jsonb END, '" +
			models.MetaKeyDeferredLaunch + "', '" + models.CeilingDeferredKey + "') = 'true'::jsonb"
	}
	return "json_type(CASE WHEN t.metadata IS NULL OR t.metadata = 'null' OR t.metadata = '' THEN '{}' ELSE t.metadata END, '$." +
		models.MetaKeyDeferredLaunch + "." + models.CeilingDeferredKey + "') = 'true'"
}
