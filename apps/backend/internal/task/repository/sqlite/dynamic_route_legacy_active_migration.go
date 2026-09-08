package sqlite

import (
	"fmt"

	"github.com/kandev/kandev/internal/db"
)

// dynamicRouteLegacyActiveBackfillColumn marks that
// backfillLegacyActiveDynamicRoutes has already run against this database.
// Its presence is the marker: a fresh install's dynamic_route_states DDL
// declares the column inline (see initDynamicRoutingSchema), so a new
// database never runs the backfill. On an existing database the column is
// absent until this migration adds it in the same transaction as the
// backfill, so a partial apply cannot leave the marker present without the
// backfill having actually run.
const dynamicRouteLegacyActiveBackfillColumn = "legacy_active_backfill_applied"

// backfillLegacyActiveDynamicRoutes is a one-time migration for databases
// created before the durable "active" route status existed. Before that
// status was introduced, a successfully-launched dynamic route was persisted
// "starting" and never transitioned further, so on such a database every
// currently-IDLE Office dynamic session's route is durably "starting" only
// because the marking mechanism did not exist yet - not because anything is
// stuck. The startup orphan sweep (isOrphanableDynamicSessionState,
// internal/orchestrator) treats IDLE as orphanable, which is correct for a
// route that becomes stranded after an upgrade, so without this backfill it
// would also flip every one of these healthy legacy routes to
// action_required on the first restart after upgrading.
//
// The backfill runs once, gated on the marker column rather than on the
// "starting"/IDLE row shape itself, because that shape recurs legitimately:
// a route claimed after this migration has already run can also fail before
// reaching "active" and be left "starting" against an IDLE session, and that
// is exactly the orphan the startup sweep exists to catch. Re-running a
// value-based backfill on every boot would silently swallow it instead.
func (r *Repository) backfillLegacyActiveDynamicRoutes() error {
	exists, err := db.ColumnExists(r.db, "dynamic_route_states", dynamicRouteLegacyActiveBackfillColumn)
	if err != nil {
		return fmt.Errorf("dynamic route legacy backfill: probe marker column: %w", err)
	}
	if exists {
		return nil
	}
	tx, err := r.db.Beginx()
	if err != nil {
		return fmt.Errorf("dynamic route legacy backfill: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`
		UPDATE dynamic_route_states
		SET state = 'active'
		WHERE state = 'starting'
			AND session_id IN (SELECT id FROM task_sessions WHERE state = 'IDLE')
	`); err != nil {
		return fmt.Errorf("dynamic route legacy backfill: backfill dynamic_route_states: %w", err)
	}
	if _, err := tx.Exec(`
		UPDATE task_sessions
		SET route_state = 'active'
		WHERE state = 'IDLE' AND route_state = 'starting'
	`); err != nil {
		return fmt.Errorf("dynamic route legacy backfill: backfill task_sessions: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE dynamic_route_states ADD COLUMN ` +
		dynamicRouteLegacyActiveBackfillColumn + ` INTEGER NOT NULL DEFAULT 1`); err != nil {
		return fmt.Errorf("dynamic route legacy backfill: add marker column: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("dynamic route legacy backfill: commit: %w", err)
	}
	return nil
}
