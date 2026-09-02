package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
)

const (
	columnCreatedAt      = "created_at"
	columnRepositoryID   = "repository_id"
	columnStatus         = "status"
	columnUpdatedAt      = "updated_at"
	columnWorktreeBranch = "worktree_branch"
	columnWorktreeID     = "worktree_id"
	columnWorktreePath   = "worktree_path"
	tableTaskEnvRepos    = "task_environment_repos"
)

// envRepoKey is the normalized repository-slot key: one row per
// (repository, branch slug) pair within a task environment.
func envRepoKey(repositoryID, branchSlug string) string {
	return repositoryID + "\x00" + branchSlug
}

// rowForKey returns (creating if needed) the normalized repo target for a
// (repository, branch slug) key in this task.
func (t *taskWorktreeTargets) rowForKey(repositoryID, branchSlug string) *envRepoTarget {
	key := envRepoKey(repositoryID, branchSlug)
	if target, ok := t.byKey[key]; ok {
		return target
	}
	target := &envRepoTarget{
		key:          key,
		repositoryID: repositoryID,
		branchSlug:   branchSlug,
	}
	t.byKey[key] = target
	t.ordering = append(t.ordering, key)
	return target
}

// targetForWorktree finds the keyed target already carrying a physical
// worktree identity, or nil.
func (t *taskWorktreeTargets) targetForWorktree(worktreeID string) *envRepoTarget {
	if worktreeID == "" {
		return nil
	}
	if target, ok := t.byWtID[worktreeID]; ok {
		return target
	}
	for _, key := range t.ordering {
		if t.byKey[key].worktreeID == worktreeID {
			return t.byKey[key]
		}
	}
	return nil
}

// mergeLegacyEnvRepo merges a pre-existing environment-repository row. These
// rows are the canonical source and win field conflicts (except for the
// physical-worktree identity, which must agree everywhere).
func (t *taskWorktreeTargets) mergeLegacyEnvRepo(row legacyEnvRepo) error {
	target := t.rowForKey(row.repositoryID, row.branchSlug)
	target.canonical = true
	if err := target.claimWorktree(row.worktreeID, row.worktreePath, row.worktreeBranch, row.position, row.errorMessage, row.createdAt, row.updatedAt, row.status); err != nil {
		return err
	}
	target.mergedAt = row.mergedAt
	target.deletedAt = row.deletedAt
	return target.mergeCreation(row.createdAt, row.updatedAt)
}

// canonicalOwnerForSlot returns the active canonical owner for a normalized
// repository slot. Rows from collapsed environments are already re-homed into
// this task target, so their original environment ID is not part of ownership.
func (t *taskWorktreeTargets) canonicalOwnerForSlot(repositoryID, branchSlug string) *envRepoTarget {
	target, ok := t.byKey[envRepoKey(repositoryID, branchSlug)]
	if !ok || !target.canonical || target.worktreeID == "" ||
		target.status == worktreeRepoStatusDeleted || target.deletedAt != nil {
		return nil
	}
	return target
}

// mergeFlatEnv merges the deprecated flat worktree columns of the surviving
// environment.
func (t *taskWorktreeTargets) mergeFlatEnv(env *legacyEnv) error {
	if existing := t.targetForWorktree(env.worktreeID); existing != nil {
		// task_environment_repos is the canonical source. The deprecated flat
		// fields can retain the old task-root path or branch after a repository
		// row has been updated, so never let them overwrite canonical metadata.
		if existing.status == "" {
			existing.status = worktreeRepoStatusActive
		}
		return nil
	}
	if env.repositoryID == "" && env.worktreeID == "" {
		return nil
	}
	target := t.rowForKey(env.repositoryID, "")
	return target.claimWorktree(env.worktreeID, env.worktreePath, env.worktreeBranch, 0, "", env.createdAt, env.updatedAt, worktreeRepoStatusActive)
}

// mergeSessionWorktree folds a legacy session-worktree row into the task's
// targets. Matching is by worktree identity first (legacy rows could carry an
// empty repository_id), then by (repository, branch slug).
func (t *taskWorktreeTargets) mergeSessionWorktree(wt legacySessionWorktree, historical bool) error {
	if wt.worktreeID == "" {
		return nil
	}
	if existing := t.targetForWorktree(wt.worktreeID); existing != nil {
		if historical {
			return nil
		}
		return existing.verifyWorktree(wt.worktreeID, wt.worktreePath, wt.worktreeBranch)
	}
	target := t.rowForKey(wt.repositoryID, wt.branchSlug)
	if historical && target.worktreeID != "" {
		return nil
	}
	status := wt.status
	if status == "" {
		status = worktreeRepoStatusActive
	}
	if err := target.claimWorktree(wt.worktreeID, wt.worktreePath, wt.worktreeBranch, wt.position, "", wt.createdAt, wt.updatedAt, status); err != nil {
		return err
	}
	target.mergedAt = wt.mergedAt
	target.deletedAt = wt.deletedAt
	return nil
}

// claimWorktree sets or verifies the physical worktree identity and its
// path/branch on the target. Later sources keep their timestamps but must
// agree on identity, path, and branch.
func (t *envRepoTarget) claimWorktree(worktreeID, path, branch string, position int, errorMessage string, createdAt, updatedAt time.Time, status string) error {
	if t.worktreeID == "" {
		t.fillWorktree(worktreeID, path, branch, position, errorMessage, createdAt, updatedAt, status)
		return nil
	}
	if worktreeID != "" && worktreeID != t.worktreeID {
		return fmt.Errorf("worktree %s conflicts with %s", worktreeID, t.worktreeID)
	}
	return t.verifyWorktree(worktreeID, path, branch)
}

// fillWorktree stamps a freshly claimed environment-repository target.
func (t *envRepoTarget) fillWorktree(worktreeID, path, branch string, position int, errorMessage string, createdAt, updatedAt time.Time, status string) {
	t.worktreeID = worktreeID
	t.worktreePath = path
	t.worktreeBranch = branch
	if position != 0 || t.position == 0 {
		t.position = position
	}
	if errorMessage != "" {
		t.errorMessage = errorMessage
	}
	if t.status == "" {
		t.status = status
	}
	if t.createdAt.IsZero() {
		t.createdAt = createdAt
	}
	if t.updatedAt.IsZero() {
		t.updatedAt = updatedAt
	}
}

func (t *envRepoTarget) mergeCreation(createdAt, updatedAt time.Time) error {
	if t.createdAt.IsZero() {
		t.createdAt = createdAt
	}
	if t.updatedAt.IsZero() {
		t.updatedAt = updatedAt
	}
	return nil
}

// verifyWorktree asserts a second source agrees on the physical worktree
// identity, path, and branch.
func (t *envRepoTarget) verifyWorktree(worktreeID, path, branch string) error {
	if worktreeID != "" && worktreeID != t.worktreeID {
		return fmt.Errorf("worktree %s conflicts with %s", worktreeID, t.worktreeID)
	}
	if path != "" && t.worktreePath != "" && path != t.worktreePath {
		return fmt.Errorf("path %q conflicts with %q", path, t.worktreePath)
	}
	if branch != "" && t.worktreeBranch != "" && branch != t.worktreeBranch {
		return fmt.Errorf("branch %q conflicts with %q", branch, t.worktreeBranch)
	}
	return nil
}

// cutoverBuildShadows creates the final-shape shadow tables.
func (r *Repository) cutoverBuildShadows(tx *sqlx.Tx) error {
	if _, err := tx.Exec(finalTaskEnvironmentsDDL("task_environments_shadow")); err != nil {
		return fmt.Errorf("cutover: create task_environments_shadow: %w", err)
	}
	if _, err := tx.Exec(finalTaskEnvironmentReposDDL("task_environment_repos_shadow", "task_environments_shadow")); err != nil {
		return fmt.Errorf("cutover: create task_environment_repos_shadow: %w", err)
	}
	return r.maybeFailCutover("create_shadow")
}

// insertNormalized copies the surviving environments into the shadow table
// and inserts every normalized repository row and session link.
func (r *Repository) insertNormalized(c *worktreeCutover, tx *sqlx.Tx) error {
	for taskID, envID := range c.taskEnvIDs {
		env, ok := c.envs[envID]
		if !ok {
			return fmt.Errorf("cutover: internal: environment %s for task %s missing", envID, taskID)
		}
		if _, err := tx.Exec(tx.Rebind(`
			INSERT INTO task_environments_shadow (
				id, task_id, executor_type, executor_id, executor_profile_id,
				control_port, status, materialization_session_id, workspace_path, container_id,
				container_bootstrap_nonce_secret_id, container_control_auth_token_secret_id,
				sandbox_id, agentctl_auth_secret_id, agentctl_bootstrap_secret_id,
				task_dir_name, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			env.id, taskID, env.executorType, env.executorID, env.executorProfileID,
			env.controlPort, env.status, "", env.workspacePath, env.containerID,
			env.containerBootstrapNonceSecretID, env.containerControlAuthTokenSecretID,
			env.sandboxID, env.authSecretID, env.bootstrapSecretID,
			env.taskDirName, env.createdAt, env.updatedAt); err != nil {
			return fmt.Errorf("cutover: insert shadow environment %s: %w", env.id, err)
		}
	}
	if err := r.maybeFailCutover("copy_envs"); err != nil {
		return err
	}

	for taskID, targets := range c.tasks {
		for _, key := range targets.ordering {
			target := targets.byKey[key]
			if err := c.insertRepoRow(tx, taskID, targets.envID, target); err != nil {
				return err
			}
		}
	}
	if err := r.maybeFailCutover("backfill_repos"); err != nil {
		return err
	}

	for sessionID, envID := range c.sessionEnvIDs {
		if envID == "" {
			continue
		}
		if _, err := tx.Exec(tx.Rebind(
			`UPDATE task_sessions SET task_environment_id = ? WHERE id = ?`),
			envID, sessionID); err != nil {
			return fmt.Errorf("cutover: link session %s to environment %s: %w", sessionID, envID, err)
		}
	}
	if err := r.maybeFailCutover("link_sessions"); err != nil {
		return err
	}
	return nil
}

func (c *worktreeCutover) insertRepoRow(tx *sqlx.Tx, taskID, envID string, target *envRepoTarget) error {
	if envID == "" {
		return fmt.Errorf("cutover: internal: no environment resolved for task %s (repo %q)", taskID, target.repositoryID)
	}
	var mergedAt, deletedAt interface{}
	if target.mergedAt != nil {
		mergedAt = *target.mergedAt
	}
	if target.deletedAt != nil {
		deletedAt = *target.deletedAt
	}
	if _, err := tx.Exec(tx.Rebind(`
		INSERT INTO task_environment_repos_shadow (
			id, task_environment_id, repository_id, branch_slug,
			worktree_id, worktree_path, worktree_branch, position,
			error_message, status, created_at, updated_at, merged_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		uuid.New().String(), envID, target.repositoryID, target.branchSlug,
		target.worktreeID, target.worktreePath, target.worktreeBranch, target.position,
		target.errorMessage, target.status, target.createdAt, target.updatedAt,
		mergedAt, deletedAt); err != nil {
		return fmt.Errorf("cutover: insert shadow repo row for task %s: %w", taskID, err)
	}
	return nil
}

// validateCutover proves the normalized graph is complete and consistent
// before the schema swap: the full legacy worktree inventory (identity, path,
// branch) matches the shadow inventory, every session resolves to a surviving
// environment, and the shadow schema is exactly the final shape.
func (r *Repository) validateCutover(c *worktreeCutover, tx *sqlx.Tx) error {
	if mismatch := c.worktreeInventoryMismatch(); mismatch != "" {
		return fmt.Errorf("cutover: worktree inventory mismatch: %s", mismatch)
	}

	// Every session that referenced a worktree must resolve to an
	// environment that owns that worktree. The owning environment can belong
	// to another task when the session borrows a shared workspace.
	for _, wt := range c.sessionWts {
		if c.isSupersededSessionWorktree(wt) {
			continue
		}
		envID := c.sessionEnvIDs[wt.sessionID]
		if envID == "" {
			return fmt.Errorf("cutover: session %s lost its environment", wt.sessionID)
		}
		ownerTaskID := c.sessionOwnerTaskID(wt.sessionID)
		targets, ok := c.tasks[ownerTaskID]
		if !ok || targets.envID != envID || !targets.ownsWorktree(wt.worktreeID) {
			return fmt.Errorf(
				"cutover: session %s worktree %s has no environment-repository owner "+
					"(session task %s, environment %s owned by task %s)",
				wt.sessionID, wt.worktreeID, c.sessionTasks[wt.sessionID], envID, ownerTaskID)
		}
	}

	if err := r.maybeFailCutover("validate"); err != nil {
		return err
	}
	return r.checkShadowFinalSchema(tx)
}

// worktreeInventoryMismatch compares the legacy and normalized worktree
// identity/path/branch sets, returning a diagnostic when they differ.
func (c *worktreeCutover) worktreeInventoryMismatch() string {
	oldInventory := c.legacyWorktreeInventory()
	newInventory := make(map[string]bool)
	for _, targets := range c.tasks {
		for _, key := range targets.ordering {
			target := targets.byKey[key]
			if target.worktreeID == "" || target.status == worktreeRepoStatusDeleted || target.deletedAt != nil {
				continue
			}
			newInventory[worktreeInventoryKey(target.worktreeID, target.worktreePath, target.worktreeBranch)] = true
		}
	}

	var missing, extra []string
	for key := range oldInventory {
		if !newInventory[key] {
			missing = append(missing, key)
		}
	}
	for key := range newInventory {
		if !oldInventory[key] {
			extra = append(extra, key)
		}
	}
	if len(missing) == 0 && len(extra) == 0 {
		return ""
	}
	return fmt.Sprintf("missing %d, extra %d:\n  missing: %s\n  extra: %s",
		len(missing), len(extra), strings.Join(missing, ", "), strings.Join(extra, ", "))
}

// legacyWorktreeInventory returns the complete pre-cutover worktree
// identity/path/branch set across session worktrees, environment-repository
// rows, and flat environment columns.
func (c *worktreeCutover) legacyWorktreeInventory() map[string]bool {
	inventory := make(map[string]bool)
	canonicalIDs := make(map[string]bool)
	for _, row := range c.envRepos {
		if row.worktreeID != "" {
			canonicalIDs[row.worktreeID] = true
		}
	}
	for _, wt := range c.sessionWts {
		if wt.worktreeID == "" || c.isSupersededSessionWorktree(wt) {
			continue
		}
		inventory[worktreeInventoryKey(wt.worktreeID, wt.worktreePath, wt.worktreeBranch)] = true
	}
	for _, row := range c.envRepos {
		if row.worktreeID == "" || row.status == worktreeRepoStatusDeleted || row.deletedAt != nil {
			continue
		}
		inventory[worktreeInventoryKey(row.worktreeID, row.worktreePath, row.worktreeBranch)] = true
	}
	for _, env := range c.envs {
		if env.worktreeID == "" || c.loserEnvIDs[env.id] || c.demotedFlatEnvironments[env.id] || canonicalIDs[env.worktreeID] {
			continue
		}
		inventory[worktreeInventoryKey(env.worktreeID, env.worktreePath, env.worktreeBranch)] = true
	}
	return inventory
}

// ownsWorktree reports whether one of the task's repository targets carries
// the physical worktree identity.
func (t *taskWorktreeTargets) ownsWorktree(worktreeID string) bool {
	for _, key := range t.ordering {
		if t.byKey[key].worktreeID == worktreeID {
			return true
		}
	}
	return false
}

func worktreeInventoryKey(id, path, branch string) string {
	return id + "\x00" + path + "\x00" + branch
}

// checkShadowFinalSchema asserts the shadow tables expose exactly the final
// column sets (no flat worktree columns, lifecycle columns present).
func (r *Repository) checkShadowFinalSchema(tx *sqlx.Tx) error {
	envColumns, err := r.tableColumns(tx, "task_environments_shadow")
	if err != nil {
		return err
	}
	for _, legacy := range []string{columnRepositoryID, columnWorktreeID, columnWorktreePath, columnWorktreeBranch} {
		if envColumns[legacy] {
			return fmt.Errorf("cutover: task_environments_shadow still carries legacy column %s", legacy)
		}
	}
	for _, required := range []string{"id", "task_id", "executor_type", columnStatus, "workspace_path",
		"agentctl_auth_secret_id", "agentctl_bootstrap_secret_id", columnCreatedAt, columnUpdatedAt} {
		if !envColumns[required] {
			return fmt.Errorf("cutover: task_environments_shadow missing final column %s", required)
		}
	}

	repoColumns, err := r.tableColumns(tx, "task_environment_repos_shadow")
	if err != nil {
		return err
	}
	for _, required := range []string{"id", "task_environment_id", columnRepositoryID, "branch_slug",
		columnWorktreeID, columnWorktreePath, columnWorktreeBranch, "position", "error_message",
		columnStatus, columnCreatedAt, columnUpdatedAt, "merged_at", "deleted_at"} {
		if !repoColumns[required] {
			return fmt.Errorf("cutover: task_environment_repos_shadow missing final column %s", required)
		}
	}
	return nil
}

// tableColumns returns the column-name set of a table for either engine.
func (r *Repository) tableColumns(tx *sqlx.Tx, table string) (map[string]bool, error) {
	columns := make(map[string]bool)
	var rows *sql.Rows
	var err error
	if dialect.IsPostgres(r.db.DriverName()) {
		rows, err = tx.Query(`
			SELECT column_name FROM information_schema.columns
			WHERE table_name = $1 AND table_schema = current_schema()`, table)
	} else {
		rows, err = tx.Query(`SELECT name FROM pragma_table_info(?)`, table)
	}
	if err != nil {
		return nil, fmt.Errorf("cutover: read columns of %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

// cutoverSwap drops the legacy schema and renames the shadow tables into
// place. The environment-repository table is dropped before the environment
// table because PostgreSQL refuses to drop a table referenced by a foreign
// key; the environment-owned snapshot FK is rebound to the environment
// shadow before the parent is dropped. SQLite has FK enforcement disabled for
// the swap. Postgres constraint names are restored to their canonical form
// afterwards.
func (r *Repository) cutoverSwap(c *worktreeCutover, tx *sqlx.Tx) error {
	if _, err := tx.Exec(`DROP TABLE task_session_worktrees`); err != nil {
		return fmt.Errorf("cutover: drop task_session_worktrees: %w", err)
	}
	// Preview-build session-delete cleanup jobs are invalid under the
	// task-owned model; purge them before any cleanup worker can claim them.
	if _, err := tx.Exec(tx.Rebind(`DELETE FROM task_resource_cleanup_jobs WHERE trigger = ?`), "session_delete"); err != nil {
		return fmt.Errorf("cutover: purge preview session_delete cleanup jobs: %w", err)
	}
	if err := r.maybeFailCutover("drop_legacy"); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE task_environment_repos`); err != nil {
		return fmt.Errorf("cutover: drop legacy task_environment_repos: %w", err)
	}
	if err := r.rebindGitSnapshotEnvironmentForeignKey(c, tx); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE task_environments`); err != nil {
		return fmt.Errorf("cutover: drop legacy task_environments: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE task_environments_shadow RENAME TO task_environments`); err != nil {
		return fmt.Errorf("cutover: rename task_environments_shadow: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE task_environment_repos_shadow RENAME TO task_environment_repos`); err != nil {
		return fmt.Errorf("cutover: rename task_environment_repos_shadow: %w", err)
	}
	if err := r.maybeFailCutover("swap"); err != nil {
		return err
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		if err := r.cutoverRenamePostgresConstraints(tx); err != nil {
			return err
		}
	}
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_task_environments_task_id ON task_environments(task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_task_environments_status ON task_environments(status)`,
		`CREATE INDEX IF NOT EXISTS idx_task_environment_repos_env_id ON task_environment_repos(task_environment_id)`,
		`CREATE INDEX IF NOT EXISTS idx_task_environment_repos_repository_id ON task_environment_repos(repository_id)`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("cutover: recreate index: %w", err)
		}
	}
	return nil
}

// cutoverRenamePostgresConstraints restores the canonical constraint names
// after the shadow tables take the real names. SQLite does not persist
// constraint names, so this is Postgres-only. The shadow names are looked up
// dynamically because PostgreSQL truncates over-long identifiers to 63 bytes.
func (r *Repository) cutoverRenamePostgresConstraints(tx *sqlx.Tx) error {
	renames := []struct {
		table, canonical string
		matchShadow      string
		constraintType   string
	}{
		{"task_environments", "task_environments_task_id_fkey", "task_environments_shadow_task_id_fkey", "f"},
		{tableTaskEnvRepos, "task_environment_repos_task_environment_id_fkey", "task_environment_repos_shadow_task_environment_id_fkey", "f"},
		{tableTaskEnvRepos, "task_environment_repos_env_repo_branch_key", "task_environment_repos_shadow_task_environment_id_", "u"},
	}
	for _, rename := range renames {
		var constraintName string
		err := tx.QueryRow(tx.Rebind(`
			SELECT conname FROM pg_constraint
			WHERE conrelid = (SELECT oid FROM pg_class WHERE relname = ? AND relnamespace = (SELECT oid FROM pg_namespace WHERE nspname = current_schema()))
			  AND conname LIKE ?
			  AND contype = ?`), rename.table, rename.matchShadow+"%", rename.constraintType).Scan(&constraintName)
		if err != nil {
			return fmt.Errorf("cutover: find postgres constraint on %s: %w", rename.table, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(
			`ALTER TABLE %s RENAME CONSTRAINT %s TO %s`, rename.table, constraintName, rename.canonical)); err != nil {
			return fmt.Errorf("cutover: rename postgres constraint %s: %w", constraintName, err)
		}
	}
	return nil
}

// maybeFailCutover is a test-only failpoint: when the repository's
// failCutoverAfter step matches, the migration aborts so tests can prove the
// transaction restores the complete pre-upgrade state.
func (r *Repository) maybeFailCutover(step string) error {
	if r.failCutoverAfter == "" {
		return nil
	}
	if r.failCutoverAfter == step {
		r.failCutoverAfter = ""
		return errors.New("injected cutover failure at " + step)
	}
	return nil
}
