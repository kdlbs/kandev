package sqlite

import (
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
)

// prepareTaskWorktreeCutoverForeignKeys makes the SQLite table swap safe for
// child tables whose foreign keys point at the parent being replaced. SQLite
// cannot alter a foreign key target in place, so the swap runs with FK checks
// disabled on the single writer connection and restores them before startup
// continues. PostgreSQL rebinds the snapshot FK to the shadow table instead.
func (r *Repository) prepareTaskWorktreeCutoverForeignKeys() (func(), error) {
	if dialect.IsPostgres(r.db.DriverName()) {
		return func() {}, nil
	}
	if _, err := r.db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		return nil, fmt.Errorf("cutover: disable sqlite foreign keys: %w", err)
	}
	return func() { _, _ = r.db.Exec(`PRAGMA foreign_keys=ON`) }, nil
}

// rebindGitSnapshotEnvironmentForeignKey preserves an already environment-
// owned snapshot table while PostgreSQL replaces task_environments with its
// shadow table. Snapshot rows belonging to an environment that loses the
// worktree election are first re-homed to that task's surviving environment.
func (r *Repository) rebindGitSnapshotEnvironmentForeignKey(c *worktreeCutover, tx *sqlx.Tx) error {
	hasEnvironmentColumn, err := r.columnExists(tx, "task_session_git_snapshots", "task_environment_id")
	if err != nil {
		return fmt.Errorf("cutover: inspect git snapshot ownership: %w", err)
	}
	if !hasEnvironmentColumn {
		return nil
	}
	if err := r.rehomeGitSnapshotsForCutover(c, tx); err != nil {
		return err
	}
	if !dialect.IsPostgres(r.db.DriverName()) {
		return nil
	}
	if _, err := tx.Exec(postgresGitSnapshotShadowForeignKeyDDL); err != nil {
		return fmt.Errorf("cutover: rebind git snapshot environment foreign key: %w", err)
	}
	return nil
}

func (r *Repository) rehomeGitSnapshotsForCutover(c *worktreeCutover, tx *sqlx.Tx) error {
	for environmentID, environment := range c.envs {
		survivingID := c.taskEnvIDs[environment.taskID]
		if survivingID == "" || survivingID == environmentID {
			continue
		}
		if _, err := tx.Exec(tx.Rebind(`
			UPDATE task_session_git_snapshots
			SET task_environment_id = ?
			WHERE task_environment_id = ?
		`), survivingID, environmentID); err != nil {
			return fmt.Errorf("cutover: rehome git snapshots from environment %s to %s: %w", environmentID, survivingID, err)
		}
	}
	return nil
}

const postgresGitSnapshotShadowForeignKeyDDL = `
DO $$
DECLARE
	old_constraint_name text;
BEGIN
	FOR old_constraint_name IN
		SELECT DISTINCT tc.constraint_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON kcu.constraint_name = tc.constraint_name
			AND kcu.constraint_schema = tc.constraint_schema
			AND kcu.table_schema = tc.table_schema
			AND kcu.table_name = tc.table_name
		WHERE tc.table_schema = current_schema()
			AND tc.table_name = 'task_session_git_snapshots'
			AND tc.constraint_type = 'FOREIGN KEY'
			AND kcu.column_name = 'task_environment_id'
	LOOP
		EXECUTE format('ALTER TABLE task_session_git_snapshots DROP CONSTRAINT %I', old_constraint_name);
	END LOOP;

	ALTER TABLE task_session_git_snapshots
		ADD CONSTRAINT task_session_git_snapshots_task_environment_id_fkey
		FOREIGN KEY (task_environment_id) REFERENCES task_environments_shadow(id) ON DELETE CASCADE;
END $$;
`
