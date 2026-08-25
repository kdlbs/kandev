package sqlite

import (
	"github.com/kandev/kandev/internal/db/dialect"
)

// migrateTaskSessionsRollupColumnsToBigint widens
// task_sessions.cost_subcents, .tokens_in and .tokens_out from INTEGER to
// BIGINT (AC-28, docs/specs/task-cost-ledger/spec.md), so all four rollup
// columns - these three plus the already-BIGINT tokens_cached_in - satisfy
// the same 64-bit rule as task_usage_events. Idempotent and dialect-aware in
// the established runMigrations() shape: on SQLite this is a no-op, because
// SQLite's INTEGER is already 64-bit; on Postgres, widening INTEGER (or
// SMALLINT) to BIGINT is a lossless type change that Postgres accepts with no
// USING clause, and re-running ALTER COLUMN ... TYPE BIGINT against a column
// that is already BIGINT is itself a no-op, so no pre-check is needed to make
// replay safe.
func (r *Repository) migrateTaskSessionsRollupColumnsToBigint() {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return
	}
	r.migrate.Apply("task_sessions.cost_subcents.bigint",
		`ALTER TABLE task_sessions ALTER COLUMN cost_subcents TYPE BIGINT`)
	r.migrate.Apply("task_sessions.tokens_in.bigint",
		`ALTER TABLE task_sessions ALTER COLUMN tokens_in TYPE BIGINT`)
	r.migrate.Apply("task_sessions.tokens_out.bigint",
		`ALTER TABLE task_sessions ALTER COLUMN tokens_out TYPE BIGINT`)
}
