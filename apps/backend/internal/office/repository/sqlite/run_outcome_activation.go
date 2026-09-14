package sqlite

import (
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/db"
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
// positive probe confirms runs.outcome exists. The required office migration
// runner fails startup for unexpected migration errors, so this probe runs
// only after the schema contract is complete. WriteMetaKeyIfAbsent makes the write
// replay-safe: the instant is never overwritten once set. Any failure here
// is logged and swallowed — activation is a best-effort published fact,
// never a boot-blocking requirement, and the sweep-equivalent writer path
// (Office's own FinishRun calls) does not gate on this key.
func (r *Repository) activateRunOutcome() {
	if err := persistence.EnsureMetaTable(r.db); err != nil {
		r.logActivationWarn("ensure kandev_meta table failed", err)
		return
	}
	exists, err := db.ColumnExists(r.db, "runs", "outcome")
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
