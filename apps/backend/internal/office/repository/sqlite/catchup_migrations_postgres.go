package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/office/models"
)

// migrateRoutineDefaultPolicyRebuildPostgres is the PostgreSQL companion to
// migrateRoutineDefaultPolicyRebuild: that function detects staleness via
// sqlite_master and PRAGMA table_info, neither of which exist on Postgres,
// so without this companion an upgraded Postgres install's
// office_routines.catch_up_policy column keeps the retired
// 'enqueue_missed_with_cap' DEFAULT forever, violating
// AC-OFFICE-ROUTINE-CATCHUP-003.3. Unlike SQLite, Postgres supports
// ALTER COLUMN ... SET DEFAULT directly, so no table rebuild is needed here
// at all (mirrors migrateTaskPriorityToTextPostgres's dialect split for the
// same class of migration in internal/task/repository/sqlite).
func (r *Repository) migrateRoutineDefaultPolicyRebuildPostgres() error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return nil
	}

	var dflt sql.NullString
	err := r.db.Get(&dflt, `
		SELECT column_default
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'office_routines'
		  AND column_name = 'catch_up_policy'
	`)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect office_routines.catch_up_policy default: %w", err)
	}
	if !dflt.Valid || !strings.Contains(dflt.String, string(models.CatchUpPolicyEnqueueMissedWithCap)) {
		return nil
	}

	if _, err := r.db.Exec(fmt.Sprintf(
		`ALTER TABLE office_routines ALTER COLUMN catch_up_policy SET DEFAULT '%s'`,
		models.CatchUpPolicySummarizeMissed,
	)); err != nil {
		return fmt.Errorf("set office_routines.catch_up_policy default: %w", err)
	}
	return nil
}

// applyRoutineDefaultPolicyRebuildPostgres runs the Postgres rebuild and
// logs its own outcome, matching migrateRoutineCatchUp's existing
// warn-and-continue handling for the SQLite path: a failed schema-hygiene
// fix must never block boot.
func (r *Repository) applyRoutineDefaultPolicyRebuildPostgres() {
	if err := r.migrateRoutineDefaultPolicyRebuildPostgres(); err != nil {
		if r.log != nil {
			r.log.Warn("routine catch-up default-policy postgres rebuild failed", zap.Error(err))
		} else {
			fmt.Println("office sqlite migrate routine catch-up default policy (postgres):", err)
		}
	}
}
