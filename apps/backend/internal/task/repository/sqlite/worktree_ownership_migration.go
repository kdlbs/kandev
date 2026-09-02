package sqlite

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/db/dialect"
)

// The one-time task-worktree ownership cutover (ADR-2026-08-08): backfill
// task_environment_repos from the legacy flat task_environments worktree
// columns and the task_session_worktrees table, validate the complete
// worktree inventory, then drop both legacy artifacts in the same
// transaction. Runtime code must never reference the legacy schema; only this
// migration knows it.
//
// This migration returns every error directly. It never uses the
// best-effort MigrateLogger path, whose contract swallows unexpected
// failures.

const (
	executorTypeLocalPC          = "local_pc"
	taskEnvironmentStatusStopped = "stopped"
)

// taskWorktreeCutoverLockID serializes concurrent PostgreSQL initializers.
// All instances must derive the same bigint so a second boot waits for the
// first to finish the cutover instead of racing it.
const taskWorktreeCutoverLockID int64 = 0x4B44_5457_4354 // "KDTWC" marker

// finalTaskEnvironmentsDDL is the task_environments shape without the
// deprecated flat repository_id/worktree_id/worktree_path/worktree_branch
// columns. tableName must be a literal table name (shadow tables reuse it).
func finalTaskEnvironmentsDDL(tableName string) string {
	return fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			executor_type TEXT NOT NULL DEFAULT '',
			executor_id TEXT DEFAULT '',
			executor_profile_id TEXT DEFAULT '',
			control_port INTEGER DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'creating',
			materialization_session_id TEXT DEFAULT '',
			workspace_path TEXT DEFAULT '',
			container_id TEXT DEFAULT '',
			container_bootstrap_nonce_secret_id TEXT DEFAULT '',
			container_control_auth_token_secret_id TEXT DEFAULT '',
			sandbox_id TEXT DEFAULT '',
			agentctl_auth_secret_id TEXT DEFAULT '',
			agentctl_bootstrap_secret_id TEXT DEFAULT '',
			task_dir_name TEXT DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
		)`, tableName)
}

// finalTaskEnvironmentReposDDL is the task_environment_repos shape with the
// physical-worktree lifecycle columns (status, merged_at, deleted_at).
// refTable is the environment table the FK targets (the shadow name during
// the cutover). SQLite rewrites FK references when the parent table is
// renamed; PostgreSQL binds by OID, so the shadow constraint keeps its
// shadow-scoped name and is renamed after the swap.
func finalTaskEnvironmentReposDDL(tableName, refTable string) string {
	return fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			task_environment_id TEXT NOT NULL,
			repository_id TEXT NOT NULL,
			branch_slug TEXT NOT NULL DEFAULT '',
			worktree_id TEXT DEFAULT '',
			worktree_path TEXT DEFAULT '',
			worktree_branch TEXT DEFAULT '',
			position INTEGER DEFAULT 0,
			error_message TEXT DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			merged_at TIMESTAMP,
			deleted_at TIMESTAMP,
			FOREIGN KEY (task_environment_id) REFERENCES %s(id) ON DELETE CASCADE,
			UNIQUE(task_environment_id, repository_id, branch_slug)
		)`, tableName, refTable)
}

// normalizeTaskWorktreeOwnership performs the one-time cutover to the
// task-owned worktree model. It is a no-op on fresh databases and on
// databases that already completed the cutover (no task_session_worktrees
// table). On legacy databases it backfills task_environment_repos from every
// legacy source, links sessions to their task environment, validates the
// complete worktree inventory, drops task_session_worktrees and the
// deprecated task_environments columns, and purges preview-build
// session_delete cleanup jobs — all in one transaction.
func (r *Repository) normalizeTaskWorktreeOwnership() error {
	exists, err := r.tableExists("task_session_worktrees")
	if err != nil {
		return fmt.Errorf("cutover: probe legacy table: %w", err)
	}
	if !exists {
		return nil
	}
	restoreForeignKeys, err := r.prepareTaskWorktreeCutoverForeignKeys()
	if err != nil {
		return err
	}
	defer restoreForeignKeys()

	tx, err := r.db.Beginx()
	if err != nil {
		return fmt.Errorf("cutover: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := r.cutoverAcquireLocks(tx); err != nil {
		return err
	}

	if err := r.cutoverEnsureLegacyBranchSlug(tx); err != nil {
		return err
	}
	legacyEnvColumns, err := r.cutoverLegacyEnvironmentColumns(tx)
	if err != nil {
		return err
	}
	legacyRepoColumns, err := r.tableColumns(tx, "task_environment_repos")
	if err != nil {
		return err
	}
	cut := &worktreeCutover{
		envs:                      make(map[string]*legacyEnv),
		taskEnvs:                  make(map[string][]*legacyEnv),
		sessions:                  make(map[string]*legacySession),
		sessionTasks:              make(map[string]string),
		sessionEnvIDs:             make(map[string]string),
		sessionWorktreeSuperseded: make(map[string]bool),
		demotedWorktrees:          make(map[string]bool),
		demotedFlatEnvironments:   make(map[string]bool),
		authoritativeWorktreeIDs:  make(map[string]bool),
		tasks:                     make(map[string]*taskWorktreeTargets),
		taskEnvIDs:                make(map[string]string),
		loserEnvIDs:               make(map[string]bool),
		executorTypes:             make(map[string]string),
	}
	if err := cut.loadLegacy(tx, legacyEnvColumns, legacyRepoColumns); err != nil {
		return err
	}
	if err := cut.normalize(tx); err != nil {
		return err
	}
	if err := r.cutoverBuildShadows(tx); err != nil {
		return err
	}
	if err := r.insertNormalized(cut, tx); err != nil {
		return err
	}
	if err := r.validateCutover(cut, tx); err != nil {
		return err
	}
	if err := r.maybeFailCutover("pre_swap"); err != nil {
		return err
	}
	if err := r.cutoverSwap(cut, tx); err != nil {
		return err
	}
	if err := r.maybeFailCutover("post_swap"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cutover: commit: %w", err)
	}
	r.logCutoverDemotions(cut)
	return nil
}

func (r *Repository) cutoverLegacyEnvironmentColumns(tx *sqlx.Tx) (map[string]bool, error) {
	columns := make(map[string]bool, 6)
	for _, column := range []string{
		"repository_id", "worktree_id", "worktree_path", "worktree_branch",
		"container_bootstrap_nonce_secret_id", "container_control_auth_token_secret_id",
	} {
		has, err := r.columnExists(tx, "task_environments", column)
		if err != nil {
			return nil, err
		}
		columns[column] = has
	}
	return columns, nil
}

// logCutoverDemotions reports every legacy worktree that lost a contested
// repository slot. The migration performs no filesystem operation, so each
// listed directory and branch is still on disk and recoverable by hand.
func (r *Repository) logCutoverDemotions(c *worktreeCutover) {
	if len(c.demotions) == 0 || r.log == nil {
		return
	}
	r.log.Warn("cutover: duplicate legacy worktrees demoted to history",
		zap.Int("count", len(c.demotions)),
		zap.Strings("worktrees", c.demotions))
}

// cutoverAcquireLocks serializes the cutover for the active database engine:
// PostgreSQL takes a migration advisory lock plus exclusive locks on every
// affected ownership table (aborting on lock timeout); SQLite relies on the
// transaction becoming the writer lock. SQLite FK enforcement is disabled for
// the duration of the cutover because SQLite cannot rebind an existing FK to a
// shadow parent table in place.
func (r *Repository) cutoverAcquireLocks(tx *sqlx.Tx) error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return nil
	}
	if _, err := tx.Exec(`SET LOCAL lock_timeout = '30s'`); err != nil {
		return fmt.Errorf("cutover: set lock timeout: %w", err)
	}
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock($1)`, taskWorktreeCutoverLockID); err != nil {
		return fmt.Errorf("cutover: acquire migration advisory lock: %w", err)
	}
	if _, err := tx.Exec(`
		LOCK TABLE task_session_git_snapshots, task_session_worktrees,
			task_environments, task_environment_repos, task_sessions,
			task_resource_cleanup_jobs
		IN ACCESS EXCLUSIVE MODE`); err != nil {
		return fmt.Errorf("cutover: lock ownership tables: %w", err)
	}
	return nil
}

// tableExists reports whether a table exists on either database engine.
func (r *Repository) tableExists(name string) (bool, error) {
	var exists bool
	var err error
	if dialect.IsPostgres(r.db.DriverName()) {
		err = r.db.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_name = $1 AND table_schema = current_schema())`,
			name).Scan(&exists)
	} else {
		err = r.db.QueryRow(
			`SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?)`,
			name).Scan(&exists)
	}
	if err != nil {
		return false, fmt.Errorf("probe table %s: %w", name, err)
	}
	return exists, nil
}

// cutoverEnsureLegacyBranchSlug adds task_session_worktrees.branch_slug when
// a very old database predates the multi-branch column. The legacy migration
// that used to do this (via MigrateLogger) was removed with the legacy
// schema; the cutover must ensure the column before reading it.
func (r *Repository) cutoverEnsureLegacyBranchSlug(tx *sqlx.Tx) error {
	has, err := r.columnExists(tx, "task_session_worktrees", "branch_slug")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	if _, err := tx.Exec(`ALTER TABLE task_session_worktrees ADD COLUMN branch_slug TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("cutover: add legacy branch_slug: %w", err)
	}
	return nil
}

// columnExists reports whether a column exists on either database engine.
func (r *Repository) columnExists(tx *sqlx.Tx, table, column string) (bool, error) {
	var exists bool
	var err error
	if dialect.IsPostgres(r.db.DriverName()) {
		err = tx.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = $1 AND column_name = $2 AND table_schema = current_schema())`,
			table, column).Scan(&exists)
	} else {
		err = tx.QueryRow(`SELECT EXISTS (
			SELECT 1 FROM pragma_table_info(?) WHERE name = ?)`,
			table, column).Scan(&exists)
	}
	if err != nil {
		return false, fmt.Errorf("probe column %s.%s: %w", table, column, err)
	}
	return exists, nil
}
