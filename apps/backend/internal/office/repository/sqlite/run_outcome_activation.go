package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/persistence"
)

// runOutcomeActivationKey is the kandev_meta key published once a boot has
// verified runs.outcome is present. See docs/specs/task-delivery-ledger/
// spec.md, "Activation points".
const runOutcomeActivationKey = "telemetry.run_outcome.activated_at"

// migrateRunOutcome adds the nullable runs.outcome column. Legacy rows and
// every row written before activation carry NULL, never a guessed value.
func (r *Repository) migrateRunOutcome() {
	r.migrate.Apply("runs.outcome", "ALTER TABLE runs ADD COLUMN outcome TEXT")
}

// activateRunOutcome writes telemetry.run_outcome.activated_at only after a
// positive probe confirms runs.outcome exists. The migration runner
// (MigrateLogger.Apply) swallows failures at WARN, so this probe is the
// only thing standing between a failed migration and a consumer wrongly
// believing the mechanism is live. WriteMetaKeyIfAbsent makes the write
// replay-safe: the instant is never overwritten once set. Any failure here
// is logged and swallowed — activation is a best-effort published fact,
// never a boot-blocking requirement, and the sweep-equivalent writer path
// (Office's own FinishRun calls) does not gate on this key.
func (r *Repository) activateRunOutcome() {
	if err := persistence.EnsureMetaTable(r.db); err != nil {
		r.logActivationWarn("ensure kandev_meta table failed", err)
		return
	}
	exists, err := columnExists(r.db, "runs", "outcome")
	if err != nil {
		r.logActivationWarn("runs.outcome schema probe failed", err)
		return
	}
	if !exists {
		return
	}
	if _, err := persistence.WriteMetaKeyIfAbsent(
		r.db, runOutcomeActivationKey, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		r.logActivationWarn("write run outcome activation key failed", err)
	}
}

func (r *Repository) logActivationWarn(msg string, err error) {
	if r.log != nil {
		r.log.Warn(msg, zap.Error(err))
	}
}

// columnExists reports whether table declares column, on either dialect.
// SQLite has no information_schema, so it falls back to PRAGMA table_info;
// Postgres is queried through the standard catalog view scoped to the
// current schema so callers never see another schema's same-named table.
func columnExists(db interface {
	DriverName() string
	Query(string, ...interface{}) (*sql.Rows, error)
	QueryRow(string, ...interface{}) *sql.Row
	Rebind(string) string
}, table, column string) (bool, error) {
	if dialect.IsPostgres(db.DriverName()) {
		var exists bool
		err := db.QueryRow(db.Rebind(`
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = current_schema()
				  AND table_name = ?
				  AND column_name = ?
			)
		`), table, column).Scan(&exists)
		return exists, err
	}
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
