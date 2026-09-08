package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/office/models"
)

// migrateRoutineCatchUp applies every schema and data migration for the
// routine catch-up capability (docs/specs/office/requirements/routine-catch-up.md):
// the three gap-summary columns on office_routine_runs, the deprecated
// catch_up_policy alias value on existing rows, the catch_up_max bound
// clamps, and the office_routines table rebuild that fixes the column's
// stored DEFAULT for upgraded installs (AC-003.3).
func (r *Repository) migrateRoutineCatchUp() {
	r.migrate.Apply("office_routine_runs.catch_up_missed_ticks",
		`ALTER TABLE office_routine_runs ADD COLUMN catch_up_missed_ticks INTEGER`)
	r.migrate.Apply("office_routine_runs.catch_up_first_missed_at",
		`ALTER TABLE office_routine_runs ADD COLUMN catch_up_first_missed_at TIMESTAMP`)
	r.migrate.Apply("office_routine_runs.catch_up_truncated",
		`ALTER TABLE office_routine_runs ADD COLUMN catch_up_truncated INTEGER NOT NULL DEFAULT 0`)

	r.migrate.Apply("office_routines.catch_up_policy_rename",
		fmt.Sprintf(
			`UPDATE office_routines SET catch_up_policy = '%s' WHERE catch_up_policy = '%s'`,
			models.CatchUpPolicySummarizeMissed, models.CatchUpPolicyEnqueueMissedWithCap,
		))
	r.migrate.Apply("office_routines.catch_up_max_floor",
		fmt.Sprintf(`UPDATE office_routines SET catch_up_max = %d WHERE catch_up_max < 1`, models.CatchUpMaxDefault))
	r.migrate.Apply("office_routines.catch_up_max_ceiling",
		fmt.Sprintf(`UPDATE office_routines SET catch_up_max = %d WHERE catch_up_max > %d`,
			models.CatchUpMaxCeiling, models.CatchUpMaxCeiling))

	if err := r.migrateRoutineDefaultPolicyRebuild(); err != nil {
		if r.log != nil {
			r.log.Warn("routine catch-up default-policy rebuild failed", zap.Error(err))
		} else {
			fmt.Println("office sqlite migrate routine catch-up default policy:", err)
		}
		return
	}
	if r.log != nil {
		r.log.Info("migration applied", zap.String("name", "office_routines.catch_up_policy_default_rebuild"))
	}
}

// routineTableExists returns true when office_routines is present. Mirrors
// tasksTableExists's shape for the same reason: a minimal single-domain
// test repository may not have created it yet.
func (r *Repository) routineTableExists() bool {
	var exists int
	err := r.db.QueryRow(
		"SELECT 1 FROM sqlite_master WHERE type='table' AND name='office_routines'",
	).Scan(&exists)
	return err == nil && exists == 1
}

// routineCatchUpPolicyDefaultIsStale reports whether office_routines'
// catch_up_policy column still carries the retired literal as its stored
// DEFAULT. SQLite has no ALTER COLUMN ... SET DEFAULT, so this is the guard
// that makes migrateRoutineDefaultPolicyRebuild's table-recreate idempotent
// — it only runs while the live schema disagrees.
func (r *Repository) routineCatchUpPolicyDefaultIsStale() bool {
	rows, err := r.db.Queryx(`PRAGMA table_info(office_routines)`)
	if err != nil {
		return false
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false
		}
		if name != "catch_up_policy" {
			continue
		}
		return strings.Contains(dflt.String, string(models.CatchUpPolicyEnqueueMissedWithCap))
	}
	return false
}

// migrateRoutineDefaultPolicyRebuild recreates office_routines with a
// corrected catch_up_policy DEFAULT ('summarize_missed'), following the
// table-rebuild pattern runTaskPriorityRecreate already establishes in this
// package: acquire a dedicated connection so PRAGMA and the DDL share one
// SQLite session, disable foreign keys for the duration (office_routine_runs
// and office_routine_triggers both declare
// ON DELETE CASCADE REFERENCES office_routines(id), and dropping the old
// table with enforcement on would cascade every routine run and trigger out
// of existence), then create/copy/drop/rename. Every column is mirrored
// verbatim; only the DEFAULT clause changes.
func (r *Repository) migrateRoutineDefaultPolicyRebuild() error {
	if !r.routineTableExists() {
		return nil
	}
	if !r.routineCatchUpPolicyDefaultIsStale() {
		return nil
	}

	ctx := context.Background()
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}
	defer func() { _, _ = conn.ExecContext(ctx, `PRAGMA foreign_keys=ON`) }()

	for _, stmt := range routineDefaultPolicyMigrationStatements() {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("routine default-policy migration step failed: %w", err)
		}
	}
	return nil
}

// routineDefaultPolicyMigrationStatements returns the ordered SQL that
// rebuilds office_routines with the corrected catch_up_policy DEFAULT. The
// column list mirrors createRoutineTables' office_routines shape exactly;
// row values (including catch_up_policy itself, already normalized by the
// value-level UPDATE that runs immediately before this in migrateRoutineCatchUp)
// are copied verbatim, so ordering relative to that UPDATE does not matter.
func routineDefaultPolicyMigrationStatements() []string {
	return []string{
		`CREATE TABLE office_routines_new (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			task_template TEXT NOT NULL DEFAULT '{}',
			assignee_agent_profile_id TEXT DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			concurrency_policy TEXT DEFAULT 'skip_if_active',
			catch_up_policy TEXT NOT NULL DEFAULT 'summarize_missed',
			catch_up_max INTEGER NOT NULL DEFAULT 25,
			variables TEXT DEFAULT '{}',
			last_run_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`,
		`INSERT INTO office_routines_new (
			id, workspace_id, name, description, task_template,
			assignee_agent_profile_id, status, concurrency_policy,
			catch_up_policy, catch_up_max, variables, last_run_at,
			created_at, updated_at
		) SELECT
			id, workspace_id, name, COALESCE(description,''),
			COALESCE(task_template,'{}'), COALESCE(assignee_agent_profile_id,''),
			COALESCE(status,'active'), COALESCE(concurrency_policy,'skip_if_active'),
			catch_up_policy, catch_up_max, COALESCE(variables,'{}'), last_run_at,
			created_at, updated_at
		FROM office_routines`,
		`DROP TABLE office_routines`,
		`ALTER TABLE office_routines_new RENAME TO office_routines`,
	}
}
