package sqlite

import (
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
)

// gitSnapshotCutoverLockID serializes concurrent PostgreSQL initializers while
// the session-owned snapshot table is replaced by its environment-owned shape.
const gitSnapshotCutoverLockID int64 = 0x4B44_4749_5453 // "KDGITS" marker

const gitSnapshotCutoverShadowTable = "task_session_git_snapshots_environment_shadow"

// migrateGitSnapshotOwnership performs the one-time shadow-table cutover from
// session-owned snapshots to environment-owned snapshots. The migration is
// deliberately separate from the best-effort additive migrations: dropping
// the old table and renaming the shadow table must commit or roll back as one
// operation.
func (r *Repository) migrateGitSnapshotOwnership() error {
	exists, err := r.tableExists("task_session_git_snapshots")
	if err != nil {
		return fmt.Errorf("git snapshot cutover: probe table: %w", err)
	}
	if !exists {
		return nil
	}

	tx, err := r.db.Beginx()
	if err != nil {
		return fmt.Errorf("git snapshot cutover: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := r.acquireGitSnapshotCutoverLocks(tx); err != nil {
		return err
	}

	finalShape, err := r.columnExists(tx, "task_session_git_snapshots", "task_environment_id")
	if err != nil {
		return fmt.Errorf("git snapshot cutover: inspect final column: %w", err)
	}
	if finalShape {
		return r.replayGitSnapshotFinalShape(tx)
	}

	if err := r.prepareGitSnapshotCutover(tx); err != nil {
		return err
	}
	if err := r.swapGitSnapshotCutover(tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("git snapshot cutover: commit: %w", err)
	}
	return nil
}

func (r *Repository) replayGitSnapshotFinalShape(tx *sqlx.Tx) error {
	if err := r.ensureGitSnapshotIndexes(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("git snapshot cutover: commit final-shape replay: %w", err)
	}
	return nil
}

func (r *Repository) prepareGitSnapshotCutover(tx *sqlx.Tx) error {
	if _, err := tx.Exec("DROP TABLE IF EXISTS " + gitSnapshotCutoverShadowTable); err != nil {
		return fmt.Errorf("git snapshot cutover: remove stale shadow: %w", err)
	}
	if _, err := tx.Exec(finalGitSnapshotDDL(gitSnapshotCutoverShadowTable)); err != nil {
		return fmt.Errorf("git snapshot cutover: create shadow: %w", err)
	}
	if err := r.maybeFailGitSnapshotCutover("create_shadow"); err != nil {
		return err
	}

	if err := r.copyGitSnapshotWinners(tx); err != nil {
		return err
	}
	if err := r.maybeFailGitSnapshotCutover("copy_snapshots"); err != nil {
		return err
	}
	if err := r.validateGitSnapshotCutover(tx); err != nil {
		return err
	}
	if err := r.maybeFailGitSnapshotCutover("validate"); err != nil {
		return err
	}
	if err := r.maybeFailGitSnapshotCutover("pre_swap"); err != nil {
		return err
	}
	return nil
}

func (r *Repository) swapGitSnapshotCutover(tx *sqlx.Tx) error {
	if _, err := tx.Exec("DROP TABLE task_session_git_snapshots"); err != nil {
		return fmt.Errorf("git snapshot cutover: drop legacy table: %w", err)
	}
	if _, err := tx.Exec("ALTER TABLE " + gitSnapshotCutoverShadowTable + " RENAME TO task_session_git_snapshots"); err != nil {
		return fmt.Errorf("git snapshot cutover: rename shadow: %w", err)
	}
	if err := r.maybeFailGitSnapshotCutover("swap"); err != nil {
		return err
	}
	if err := r.ensureGitSnapshotIndexes(tx); err != nil {
		return err
	}
	if err := r.maybeFailGitSnapshotCutover("post_swap"); err != nil {
		return err
	}
	return nil
}

func finalGitSnapshotDDL(tableName string) string {
	return fmt.Sprintf(`
	CREATE TABLE %s (
		id TEXT PRIMARY KEY,
		task_environment_id TEXT NOT NULL,
		session_id TEXT,
		snapshot_type TEXT NOT NULL,
		branch TEXT NOT NULL,
		remote_branch TEXT DEFAULT '',
		head_commit TEXT DEFAULT '',
		base_commit TEXT DEFAULT '',
		ahead INTEGER DEFAULT 0,
		behind INTEGER DEFAULT 0,
		files TEXT DEFAULT '{}',
		triggered_by TEXT DEFAULT '',
		metadata TEXT DEFAULT '{}',
		created_at TIMESTAMP NOT NULL,
		FOREIGN KEY (task_environment_id) REFERENCES task_environments(id) ON DELETE CASCADE,
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE SET NULL
	)`, tableName)
}

func (r *Repository) acquireGitSnapshotCutoverLocks(tx *sqlx.Tx) error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return nil
	}
	if _, err := tx.Exec("SET LOCAL lock_timeout = '30s'"); err != nil {
		return fmt.Errorf("git snapshot cutover: set lock timeout: %w", err)
	}
	if _, err := tx.Exec("SELECT pg_advisory_xact_lock($1)", gitSnapshotCutoverLockID); err != nil {
		return fmt.Errorf("git snapshot cutover: acquire migration advisory lock: %w", err)
	}
	if _, err := tx.Exec(`
		LOCK TABLE task_session_git_snapshots, task_sessions, task_environments
		IN ACCESS EXCLUSIVE MODE`); err != nil {
		return fmt.Errorf("git snapshot cutover: lock ownership tables: %w", err)
	}
	return nil
}

func (r *Repository) copyGitSnapshotWinners(tx *sqlx.Tx) error {
	repositoryExpr := snapshotRepositoryExpr(r.db.DriverName(), "legacy")
	query := `
		INSERT INTO ` + gitSnapshotCutoverShadowTable + ` (
			id, task_environment_id, session_id, snapshot_type, branch, remote_branch,
			head_commit, base_commit, ahead, behind, files, triggered_by, metadata, created_at
		)
		SELECT id, task_environment_id, session_id, snapshot_type, branch, remote_branch,
			head_commit, base_commit, ahead, behind, files, triggered_by, metadata, created_at
		FROM (
			SELECT
				legacy.id,
				environment.id AS task_environment_id,
				legacy.session_id,
				COALESCE(legacy.snapshot_type, '') AS snapshot_type,
				COALESCE(legacy.branch, '') AS branch,
				COALESCE(legacy.remote_branch, '') AS remote_branch,
				COALESCE(legacy.head_commit, '') AS head_commit,
				COALESCE(legacy.base_commit, '') AS base_commit,
				COALESCE(legacy.ahead, 0) AS ahead,
				COALESCE(legacy.behind, 0) AS behind,
				COALESCE(legacy.files, '{}') AS files,
				COALESCE(legacy.triggered_by, '') AS triggered_by,
				COALESCE(legacy.metadata, '{}') AS metadata,
				legacy.created_at,
				ROW_NUMBER() OVER (
					PARTITION BY environment.id, ` + repositoryExpr + `
					ORDER BY legacy.created_at DESC,
						CASE WHEN COALESCE(TRIM(legacy.files), '') NOT IN ('', '{}') THEN 1 ELSE 0 END DESC,
						legacy.id DESC
				) AS row_number
			FROM task_session_git_snapshots legacy
			INNER JOIN task_sessions session ON session.id = legacy.session_id
			INNER JOIN task_environments environment ON environment.id = session.task_environment_id
			WHERE NULLIF(TRIM(COALESCE(session.task_environment_id, '')), '') IS NOT NULL
		) ranked
		WHERE row_number = 1
	`
	if _, err := tx.Exec(r.db.Rebind(query)); err != nil {
		return fmt.Errorf("git snapshot cutover: copy current winners: %w", err)
	}
	return nil
}

func (r *Repository) validateGitSnapshotCutover(tx *sqlx.Tx) error {
	columns, err := r.tableColumns(tx, gitSnapshotCutoverShadowTable)
	if err != nil {
		return fmt.Errorf("git snapshot cutover: inspect shadow: %w", err)
	}
	for _, column := range []string{"id", "task_environment_id", "session_id", "snapshot_type", "files", "metadata", "created_at"} {
		if !columns[column] {
			return fmt.Errorf("git snapshot cutover: shadow is missing column %s", column)
		}
	}
	return nil
}

func (r *Repository) ensureGitSnapshotIndexes(tx *sqlx.Tx) error {
	for _, query := range []string{
		`CREATE INDEX IF NOT EXISTS idx_git_snapshots_environment ON task_session_git_snapshots(task_environment_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_git_snapshots_environment_type ON task_session_git_snapshots(task_environment_id, snapshot_type, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_git_snapshots_session ON task_session_git_snapshots(session_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_git_snapshots_type ON task_session_git_snapshots(session_id, snapshot_type)`,
	} {
		if _, err := tx.Exec(query); err != nil {
			return fmt.Errorf("git snapshot cutover: create index: %w", err)
		}
	}
	return nil
}

func (r *Repository) maybeFailGitSnapshotCutover(step string) error {
	if r.failGitSnapshotCutoverAfter == step {
		return fmt.Errorf("git snapshot cutover: injected failure after %s", step)
	}
	return nil
}
