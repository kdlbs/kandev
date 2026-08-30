package sqlite

//revive:disable:file-length-limit // Ownership cutover coverage is intentionally scenario-heavy.

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

// ---------------------------------------------------------------------------
// Seed helpers: build a legacy pre-cutover database by pre-creating the
// legacy-shaped task_environments and task_session_worktrees tables before
// the repository initializer runs (which would otherwise create the final
// shapes).
// ---------------------------------------------------------------------------

const legacyEnvDDL = `
	CREATE TABLE task_environments (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		repository_id TEXT DEFAULT '',
		executor_type TEXT NOT NULL DEFAULT '',
		executor_id TEXT DEFAULT '',
		executor_profile_id TEXT DEFAULT '',
		agent_execution_id TEXT DEFAULT '',
		control_port INTEGER DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'creating',
		worktree_id TEXT DEFAULT '',
		worktree_path TEXT DEFAULT '',
		worktree_branch TEXT DEFAULT '',
		workspace_path TEXT DEFAULT '',
		container_id TEXT DEFAULT '',
		sandbox_id TEXT DEFAULT '',
		task_dir_name TEXT DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	CREATE INDEX idx_task_environments_task_id ON task_environments(task_id);
	CREATE TABLE task_environment_repos (
		id TEXT PRIMARY KEY,
		task_environment_id TEXT NOT NULL,
		repository_id TEXT NOT NULL,
		branch_slug TEXT NOT NULL DEFAULT '',
		worktree_id TEXT DEFAULT '',
		worktree_path TEXT DEFAULT '',
		worktree_branch TEXT DEFAULT '',
		position INTEGER DEFAULT 0,
		error_message TEXT DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	CREATE INDEX idx_task_environment_repos_env_id ON task_environment_repos(task_environment_id);`

const legacySessionWorktreeDDL = `
	CREATE TABLE task_session_worktrees (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		worktree_id TEXT NOT NULL,
		repository_id TEXT NOT NULL,
		branch_slug TEXT NOT NULL DEFAULT '',
		position INTEGER DEFAULT 0,
		worktree_path TEXT DEFAULT '',
		worktree_branch TEXT DEFAULT '',
		status TEXT NOT NULL DEFAULT 'active',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		merged_at TIMESTAMP,
		deleted_at TIMESTAMP
	);
	CREATE INDEX idx_task_session_worktrees_session_id ON task_session_worktrees(session_id);`

// openLegacyDB opens a SQLite database in the pre-cutover schema: it runs
// the current initializer once (final shape for everything else), then
// rewinds task_environments and task_environment_repos to the legacy shape
// and recreates task_session_worktrees.
func openLegacyDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	if _, err := NewWithDB(sqlxDB, sqlxDB, nil); err != nil {
		t.Fatalf("seed baseline schema: %v", err)
	}
	rewindToLegacySchema(t, sqlxDB)
	return sqlxDB
}

func rewindToLegacySchema(t *testing.T, db *sqlx.DB) {
	t.Helper()
	if _, err := db.Exec(`DROP TABLE task_environment_repos`); err != nil {
		t.Fatalf("drop final env repos: %v", err)
	}
	if _, err := db.Exec(`DROP TABLE task_environments`); err != nil {
		t.Fatalf("drop final envs: %v", err)
	}
	for _, ddl := range []string{legacyEnvDDL, legacySessionWorktreeDDL} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("seed legacy schema: %v", err)
		}
	}
}

type legacySeed struct {
	envID, taskID, repoID, sessionID string
}

func seedLegacyTask(t *testing.T, db *sqlx.DB, seed legacySeed, startedAt time.Time) {
	t.Helper()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, 'ws-1', 'legacy task', ?, ?)`), seed.taskID, startedAt, startedAt); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
		VALUES (?, ?, 'COMPLETED', ?, ?)`), seed.sessionID, seed.taskID, startedAt, startedAt); err != nil {
		t.Fatalf("seed session: %v", err)
	}
}

func seedLegacySessionWorktree(t *testing.T, db *sqlx.DB, sessionID, worktreeID, repoID, slug, path, branch, status string, startedAt time.Time) {
	t.Helper()
	var deletedAt interface{}
	if status == "deleted" {
		deletedAt = startedAt
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_worktrees (
			id, session_id, worktree_id, repository_id, branch_slug, position,
			worktree_path, worktree_branch, status, created_at, updated_at, merged_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, NULL, ?)`),
		worktreeID+"-row-"+sessionID, sessionID, worktreeID, repoID, slug, path, branch, status, startedAt, startedAt, deletedAt); err != nil {
		t.Fatalf("seed session worktree: %v", err)
	}
}

func seedLegacyFlatEnv(t *testing.T, db *sqlx.DB, seed legacySeed, worktreeID, path, branch string, startedAt time.Time) {
	t.Helper()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_environments (
			id, task_id, repository_id, executor_type, executor_id, executor_profile_id,
			control_port, status, worktree_id, worktree_path, worktree_branch, workspace_path,
			container_id, sandbox_id, task_dir_name, created_at, updated_at
		) VALUES (?, ?, ?, 'worktree', 'exec-1', '', 0, 'ready', ?, ?, ?, ?, '', '', '', ?, ?)`),
		seed.envID, seed.taskID, seed.repoID, worktreeID, path, branch, path, startedAt, startedAt); err != nil {
		t.Fatalf("seed flat env: %v", err)
	}
}

func seedLegacyEnvRepo(t *testing.T, db *sqlx.DB, id, envID, repoID, worktreeID, path, branch string, startedAt time.Time) {
	t.Helper()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_environment_repos (
			id, task_environment_id, repository_id, branch_slug,
			worktree_id, worktree_path, worktree_branch, position,
			error_message, created_at, updated_at
		) VALUES (?, ?, ?, '', ?, ?, ?, 0, '', ?, ?)`),
		id, envID, repoID, worktreeID, path, branch, startedAt, startedAt); err != nil {
		t.Fatalf("seed env repo: %v", err)
	}
}

func seedCleanupJob(t *testing.T, db *sqlx.DB, id, taskID, trigger, state string, startedAt time.Time) {
	t.Helper()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_resource_cleanup_jobs (
			id, operation_id, task_id, trigger, state, resource_snapshot, attempts, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, '{}', 0, ?, ?)`),
		id, id, taskID, trigger, state, startedAt, startedAt); err != nil {
		t.Fatalf("seed cleanup job: %v", err)
	}
}

func legacyTableExists(t *testing.T, db *sqlx.DB, table string) bool {
	t.Helper()
	var exists bool
	err := db.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type='table' AND name=?)`, table).Scan(&exists)
	if err != nil {
		t.Fatalf("probe table: %v", err)
	}
	return exists
}

// ---------------------------------------------------------------------------
// Fresh-schema shape
// ---------------------------------------------------------------------------

func TestCutover_FreshSchemaHasFinalShape(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fresh.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	if _, err := NewWithDB(sqlxDB, sqlxDB, nil); err != nil {
		t.Fatalf("init fresh schema: %v", err)
	}
	if legacyTableExists(t, sqlxDB, "task_session_worktrees") {
		t.Fatal("fresh database must not contain task_session_worktrees")
	}

	envCols := tableColumnSet(t, sqlxDB, "task_environments")
	for _, gone := range []string{columnRepositoryID, columnWorktreeID, columnWorktreePath, columnWorktreeBranch} {
		if envCols[gone] {
			t.Fatalf("fresh task_environments still has legacy column %s", gone)
		}
	}
	repoCols := tableColumnSet(t, sqlxDB, tableTaskEnvRepos)
	for _, required := range []string{"status", "merged_at", "deleted_at"} {
		if !repoCols[required] {
			t.Fatalf("fresh task_environment_repos missing %s", required)
		}
	}
}

func TestCutover_HybridNormalizedEnvironmentWithLegacySessionWorktrees(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hybrid.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("seed final schema: %v", err)
	}
	seed := seedHybridCutoverState(t, db, "sqlite")

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("upgrade hybrid schema: %v", err)
	}
	assertHybridCutoverResult(t, repo, seed)
}

func seedHybridCutoverState(t *testing.T, db *sqlx.DB, suffix string) legacySeed {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{
		envID: "env-hybrid-" + suffix, taskID: "task-hybrid-" + suffix,
		repoID: "repo-hybrid-" + suffix, sessionID: "sess-hybrid-" + suffix,
	}
	seedLegacyTask(t, db, seed, now)
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_environments (
			id, task_id, executor_type, status, workspace_path, created_at, updated_at
		) VALUES (?, ?, 'worktree', 'ready', '/tasks/hybrid', ?, ?)`),
		seed.envID, seed.taskID, now, now); err != nil {
		t.Fatalf("seed normalized environment: %v", err)
	}
	seedLegacyEnvRepo(t, db, "env-repo-hybrid", seed.envID, seed.repoID,
		"wt-hybrid", "/tasks/hybrid/repo", "feature/hybrid", now)
	if _, err := db.Exec(db.Rebind(`
		UPDATE task_environment_repos
		SET status = 'deleted', merged_at = ?, deleted_at = ?
		WHERE id = 'env-repo-hybrid'`), now, now); err != nil {
		t.Fatalf("seed normalized repository lifecycle: %v", err)
	}
	if _, err := db.Exec(legacySessionWorktreeDDL); err != nil {
		t.Fatalf("recreate stale legacy table: %v", err)
	}
	seedLegacySessionWorktree(t, db, seed.sessionID, "wt-session-only", seed.repoID+"-session-only", "",
		"/tasks/hybrid/session-only", "feature/session-only", "active", now)
	return seed
}

func assertHybridCutoverResult(t *testing.T, repo *Repository, seed legacySeed) {
	t.Helper()
	exists, err := repo.tableExists("task_session_worktrees")
	if err != nil {
		t.Fatalf("probe stale legacy table: %v", err)
	}
	if exists {
		t.Fatal("stale legacy table must be removed")
	}
	env, err := repo.GetTaskEnvironment(context.Background(), seed.envID)
	if err != nil {
		t.Fatalf("get normalized environment: %v", err)
	}
	if len(env.Repos) != 2 {
		t.Fatalf("normalized repositories = %+v", env.Repos)
	}
	got := make(map[string]*models.TaskEnvironmentRepo, len(env.Repos))
	for _, envRepo := range env.Repos {
		got[envRepo.RepositoryID] = envRepo
	}
	assertHybridWorktree(t, got[seed.repoID], "wt-hybrid", "/tasks/hybrid/repo", "feature/hybrid")
	if got[seed.repoID].Status != "deleted" || got[seed.repoID].MergedAt == nil ||
		!got[seed.repoID].MergedAt.Equal(got[seed.repoID].CreatedAt) || got[seed.repoID].DeletedAt == nil ||
		!got[seed.repoID].DeletedAt.Equal(got[seed.repoID].CreatedAt) {
		t.Fatalf("normalized repository lifecycle = %+v", got[seed.repoID])
	}
	assertHybridWorktree(t, got[seed.repoID+"-session-only"], "wt-session-only",
		"/tasks/hybrid/session-only", "feature/session-only")
}

func assertHybridWorktree(t *testing.T, got *models.TaskEnvironmentRepo, worktreeID, path, branch string) {
	t.Helper()
	if got == nil || got.WorktreeID != worktreeID || got.WorktreePath != path || got.WorktreeBranch != branch {
		t.Fatalf("normalized worktree = %+v, want id=%q path=%q branch=%q", got, worktreeID, path, branch)
	}
}

func tableColumnSet(t *testing.T, db *sqlx.DB, table string) map[string]bool {
	t.Helper()
	cols := map[string]bool{}
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		cols[name] = true
	}
	return cols
}

// ---------------------------------------------------------------------------
// Legacy normalization
// ---------------------------------------------------------------------------

// TestCutover_NormalizesLegacyFlatEnvironment seeds a legacy flat
// single-repo environment and proves it lands as an environment-repository
// row while the legacy table and flat columns disappear.
func TestCutover_NormalizesLegacyFlatEnvironment(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{envID: "env-1", taskID: "task-1", repoID: "repo-1", sessionID: "sess-1"}
	seedLegacyTask(t, db, seed, now)
	seedLegacySessionWorktree(t, db, "sess-1", "wt-1", "repo-1", "", "/tasks/t-1/kandev", "feature/x", "active", now)
	seedLegacyFlatEnv(t, db, seed, "wt-1", "/tasks/t-1/kandev", "feature/x", now)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	ctx := context.Background()

	if legacyTableExists(t, db, "task_session_worktrees") {
		t.Fatal("task_session_worktrees must be dropped after cutover")
	}
	env, err := repo.GetTaskEnvironment(ctx, "env-1")
	if err != nil {
		t.Fatalf("GetTaskEnvironment: %v", err)
	}
	if len(env.Repos) != 1 {
		t.Fatalf("env repos = %d, want 1", len(env.Repos))
	}
	row := env.Repos[0]
	if row.RepositoryID != "repo-1" || row.WorktreeID != "wt-1" ||
		row.WorktreePath != "/tasks/t-1/kandev" || row.WorktreeBranch != "feature/x" {
		t.Fatalf("normalized repo row = %+v", row)
	}
	if row.Status != "active" {
		t.Fatalf("repo status = %q, want active", row.Status)
	}
	session, err := repo.GetTaskSession(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.TaskEnvironmentID != "env-1" {
		t.Fatalf("session env = %q, want env-1", session.TaskEnvironmentID)
	}
}

func TestCutover_PreservesLegacyDockerCredentialReferences(t *testing.T) {
	db := openLegacyDB(t)
	for _, column := range []string{
		"container_bootstrap_nonce_secret_id TEXT DEFAULT ''",
		"container_control_auth_token_secret_id TEXT DEFAULT ''",
	} {
		if _, err := db.Exec("ALTER TABLE task_environments ADD COLUMN " + column); err != nil {
			t.Fatalf("add legacy credential column: %v", err)
		}
	}
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{envID: "env-docker", taskID: "task-docker", repoID: "repo-docker", sessionID: "session-docker"}
	seedLegacyTask(t, db, seed, now)
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_environments (
			id, task_id, repository_id, executor_type, executor_id, executor_profile_id,
			control_port, status, workspace_path, container_id,
			container_bootstrap_nonce_secret_id, container_control_auth_token_secret_id,
			sandbox_id, task_dir_name, created_at, updated_at
		) VALUES (?, ?, ?, 'local_docker', 'docker', '', 0, 'ready', '/workspace', 'container-1', ?, ?, '', '', ?, ?)`),
		seed.envID, seed.taskID, seed.repoID, "bootstrap-secret", "control-secret", now, now); err != nil {
		t.Fatalf("seed legacy Docker environment: %v", err)
	}

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("upgrade legacy database: %v", err)
	}
	env, err := repo.GetTaskEnvironment(context.Background(), seed.envID)
	if err != nil {
		t.Fatalf("GetTaskEnvironment: %v", err)
	}
	if env.ContainerBootstrapNonceSecretID != "bootstrap-secret" {
		t.Fatalf("ContainerBootstrapNonceSecretID = %q, want bootstrap-secret", env.ContainerBootstrapNonceSecretID)
	}
	if env.ContainerControlAuthTokenSecretID != "control-secret" {
		t.Fatalf("ContainerControlAuthTokenSecretID = %q, want control-secret", env.ContainerControlAuthTokenSecretID)
	}
}

// TestCutover_NormalizesSessionOnlyOwnership seeds sessions with worktrees
// but no environment, and proves the cutover creates one task-owned
// environment, homes the worktrees, and links the sessions.
func TestCutover_NormalizesSessionOnlyOwnership(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{taskID: "task-2", sessionID: "sess-2"}
	seedLegacyTask(t, db, seed, now)
	seedLegacySessionWorktree(t, db, "sess-2", "wt-2", "repo-2", "", "/tasks/t-2/kandev", "feature/y", "active", now)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	ctx := context.Background()

	env, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-2")
	if err != nil {
		t.Fatalf("GetTaskEnvironmentByTaskID: %v", err)
	}
	if env == nil {
		t.Fatal("expected a normalized task-owned environment")
	}
	if env.ExecutorType != "local_pc" {
		t.Fatalf("backfilled executor_type = %q, want default local_pc", env.ExecutorType)
	}
	if len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-2" {
		t.Fatalf("normalized repos = %+v, want wt-2", env.Repos)
	}
	session, err := repo.GetTaskSession(ctx, "sess-2")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.TaskEnvironmentID != env.ID {
		t.Fatalf("session env = %q, want %q", session.TaskEnvironmentID, env.ID)
	}
}

// TestCutover_PreservesEnvRepoRowsAndSharedSessions seeds an existing
// task_environment_repos row, a session worktree referencing the same
// worktree, and a second session sharing the environment. The cutover must
// keep one canonical row and link both sessions.
func TestCutover_PreservesEnvRepoRowsAndSharedSessions(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := db.Exec(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-3', 'ws-1', 'legacy', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	for _, s := range []string{"sess-3a", "sess-3b"} {
		if _, err := db.Exec(`
			INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
			VALUES (?, 'task-3', 'COMPLETED', ?, ?)`, s, now, now); err != nil {
			t.Fatalf("seed session: %v", err)
		}
	}
	seedLegacySessionWorktree(t, db, "sess-3a", "wt-3", "repo-3", "", "/tasks/t-3/kandev", "feature/z", "active", now)
	seedLegacySessionWorktree(t, db, "sess-3b", "wt-3", "repo-3", "", "/tasks/t-3/kandev", "feature/z", "active", now)
	seedLegacyFlatEnv(t, db, legacySeed{envID: "env-3", taskID: "task-3", repoID: "repo-3", sessionID: "sess-3a"}, "wt-3", "/tasks/t-3/kandev", "feature/z", now)
	if _, err := db.Exec(`
		INSERT INTO task_environment_repos (
			id, task_environment_id, repository_id, branch_slug,
			worktree_id, worktree_path, worktree_branch, position, error_message, created_at, updated_at
		) VALUES ('env-repo-3', 'env-3', 'repo-3', '', 'wt-3', '/tasks/t-3/kandev', 'feature/z', 0, '', ?, ?)`,
		now, now); err != nil {
		t.Fatalf("seed env repo: %v", err)
	}

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	ctx := context.Background()

	env, err := repo.GetTaskEnvironment(ctx, "env-3")
	if err != nil {
		t.Fatalf("GetTaskEnvironment: %v", err)
	}
	if len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-3" {
		t.Fatalf("env repos = %+v, want one wt-3 row", env.Repos)
	}
	for _, s := range []string{"sess-3a", "sess-3b"} {
		session, err := repo.GetTaskSession(ctx, s)
		if err != nil {
			t.Fatalf("GetTaskSession(%s): %v", s, err)
		}
		if session.TaskEnvironmentID != "env-3" {
			t.Fatalf("session %s env = %q, want env-3", s, session.TaskEnvironmentID)
		}
	}
}

// TestCutover_ZeroSessionEnvironmentIsRetained proves a task-owned
// environment with no sessions survives the cutover untouched.
func TestCutover_ZeroSessionEnvironmentIsRetained(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := db.Exec(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-4', 'ws-1', 'legacy', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	seedLegacyFlatEnv(t, db, legacySeed{envID: "env-4", taskID: "task-4", repoID: "repo-4", sessionID: ""}, "wt-4", "/tasks/t-4/kandev", "feature/w", now)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	ctx := context.Background()

	env, err := repo.GetTaskEnvironment(ctx, "env-4")
	if err != nil {
		t.Fatalf("GetTaskEnvironment: %v", err)
	}
	if len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-4" {
		t.Fatalf("zero-session env repos = %+v, want wt-4", env.Repos)
	}
}

// TestCutover_ReplayIsNoOp proves a second boot (already-normalized
// database) skips the cutover without errors.
func TestCutover_ReplayIsNoOp(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{envID: "env-5", taskID: "task-5", repoID: "repo-5", sessionID: "sess-5"}
	seedLegacyTask(t, db, seed, now)
	seedLegacySessionWorktree(t, db, "sess-5", "wt-5", "repo-5", "", "/tasks/t-5/kandev", "feature/v", "active", now)
	seedLegacyFlatEnv(t, db, seed, "wt-5", "/tasks/t-5/kandev", "feature/v", now)

	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("first boot: %v", err)
	}
	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("second boot (replay): %v", err)
	}
}

// TestCutover_PurgesPreviewCleanupJobs removes preview-build session_delete
// cleanup jobs while preserving task-lifecycle jobs.
func TestCutover_PurgesPreviewCleanupJobs(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{envID: "env-6", taskID: "task-6", repoID: "repo-6", sessionID: "sess-6"}
	seedLegacyTask(t, db, seed, now)
	seedLegacySessionWorktree(t, db, "sess-6", "wt-6", "repo-6", "", "/tasks/t-6/kandev", "feature/u", "active", now)
	seedLegacyFlatEnv(t, db, seed, "wt-6", "/tasks/t-6/kandev", "feature/u", now)
	seedCleanupJob(t, db, "job-session-delete", "task-6", "session_delete", "pending", now)
	seedCleanupJob(t, db, "job-archive", "task-6", "archive", "pending", now)
	seedCleanupJob(t, db, "job-archive-done", "task-6", "archive", "succeeded", now)

	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("cutover: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM task_resource_cleanup_jobs`).Scan(&count); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 2 {
		t.Fatalf("cleanup jobs after cutover = %d, want 2 (task-lifecycle jobs preserved)", count)
	}
}

// TestCutover_RollbackAtEveryFailpoint injects a failure at each cutover
// step and proves the transaction restores the complete legacy schema and
// data.
func TestCutover_RollbackAtEveryFailpoint(t *testing.T) {
	for _, step := range []string{
		"create_shadow", "copy_envs", "backfill_repos", "link_sessions",
		"validate", "pre_swap", "drop_legacy", "swap", "post_swap",
	} {
		t.Run(step, func(t *testing.T) {
			db := openLegacyDB(t)
			now := time.Now().UTC().Truncate(time.Second)
			seed := legacySeed{envID: "env-r", taskID: "task-r", repoID: "repo-r", sessionID: "sess-r"}
			seedLegacyTask(t, db, seed, now)
			seedLegacySessionWorktree(t, db, "sess-r", "wt-r", "repo-r", "", "/tasks/t-r/kandev", "feature/r", "active", now)
			seedLegacyFlatEnv(t, db, seed, "wt-r", "/tasks/t-r/kandev", "feature/r", now)
			seedCleanupJob(t, db, "job-archive-r", "task-r", "archive", "pending", now)

			repo := &Repository{db: db, ro: db, migrate: dbutil.NewMigrateLogger(db, nil), failCutoverAfter: step}
			err := repo.initSchema()
			if err == nil {
				t.Fatalf("expected injected failure at %s", step)
			}
			if !legacyTableExists(t, db, "task_session_worktrees") {
				t.Fatalf("task_session_worktrees must survive rollback at %s", step)
			}
			var path, branch string
			if err := db.QueryRow(`
				SELECT worktree_path, worktree_branch FROM task_session_worktrees WHERE worktree_id = 'wt-r'`,
			).Scan(&path, &branch); err != nil {
				t.Fatalf("legacy worktree row lost after rollback at %s: %v", step, err)
			}
			if path != "/tasks/t-r/kandev" || branch != "feature/r" {
				t.Fatalf("legacy worktree data changed after rollback at %s", step)
			}
			envCols := tableColumnSet(t, db, "task_environments")
			if !envCols["worktree_id"] {
				t.Fatalf("flat env columns lost after rollback at %s", step)
			}
			var jobCount int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM task_resource_cleanup_jobs WHERE trigger = 'archive'`).Scan(&jobCount); err != nil {
				t.Fatalf("count jobs after rollback at %s: %v", step, err)
			}
			if jobCount != 1 {
				t.Fatalf("cleanup job lost after rollback at %s", step)
			}
		})
	}
}

// TestCutover_CanonicalRepoWinsOverStaleFlatMetadata proves that canonical
// repository metadata wins when the deprecated flat environment fields drift.
func TestCutover_CanonicalRepoWinsOverStaleFlatMetadata(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{envID: "env-p", taskID: "task-p", repoID: "repo-p", sessionID: "sess-p"}
	seedLegacyTask(t, db, seed, now)
	seedLegacySessionWorktree(t, db, "sess-p", "wt-p", "repo-p", "", "/t/one", "feature/p", "active", now)
	seedLegacyFlatEnv(t, db, seed, "wt-p", "/t/two", "feature/stale", now)
	seedLegacyEnvRepo(t, db, "env-repo-p", "env-p", "repo-p", "wt-p", "/t/one", "feature/p", now)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironment(context.Background(), "env-p")
	if err != nil {
		t.Fatalf("get task environment: %v", err)
	}
	if len(env.Repos) != 1 || env.Repos[0].WorktreePath != "/t/one" ||
		env.Repos[0].WorktreeBranch != "feature/p" {
		t.Fatalf("canonical repository metadata = %+v", env.Repos)
	}
}

// TestCutover_PrefersCanonicalMetadataForResumableSession proves that stale
// session metadata cannot override a canonical row for the same worktree.
func TestCutover_PrefersCanonicalMetadataForResumableSession(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{envID: "env-resumable", taskID: "task-resumable", repoID: "repo-resumable", sessionID: "sess-resumable"}
	seedLegacyTask(t, db, seed, now)
	if _, err := db.Exec(db.Rebind(`UPDATE task_sessions SET state = 'WAITING_FOR_INPUT' WHERE id = ?`), seed.sessionID); err != nil {
		t.Fatalf("set session state: %v", err)
	}
	seedLegacyFlatEnv(t, db, seed, "wt-resumable", "/tasks/resumable/repo", "feature/current", now)
	seedLegacyEnvRepo(t, db, "env-repo-resumable", seed.envID, seed.repoID, "wt-resumable", "/tasks/resumable/repo", "feature/current", now)
	seedLegacySessionWorktree(t, db, seed.sessionID, "wt-resumable", seed.repoID, "", "/tasks/resumable/repo", "feature/stale-session", "active", now)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironment(context.Background(), seed.envID)
	if err != nil {
		t.Fatalf("get task environment: %v", err)
	}
	session, err := repo.GetTaskSession(context.Background(), seed.sessionID)
	if err != nil {
		t.Fatalf("get task session: %v", err)
	}
	if len(env.Repos) != 1 || env.Repos[0].WorktreeBranch != "feature/current" ||
		session.State != "WAITING_FOR_INPUT" {
		t.Fatalf("normalized environment = %+v, session state = %q", env.Repos, session.State)
	}
}

// TestCutover_PrefersFlatMetadataForSameWorktree proves that the surviving
// flat environment beats stale session metadata when no canonical row exists.
func TestCutover_PrefersFlatMetadataForSameWorktree(t *testing.T) {
	for _, state := range []string{"RUNNING", "WAITING_FOR_INPUT", sessionStateCancelled} {
		t.Run(state, func(t *testing.T) {
			db := openLegacyDB(t)
			now := time.Now().UTC().Truncate(time.Second)
			seed := legacySeed{envID: "env-flat", taskID: "task-flat", repoID: "repo-flat", sessionID: "sess-flat"}
			seedLegacyTask(t, db, seed, now)
			if _, err := db.Exec(db.Rebind(`UPDATE task_sessions SET state = ? WHERE id = ?`), state, seed.sessionID); err != nil {
				t.Fatalf("set session state: %v", err)
			}
			seedLegacyFlatEnv(t, db, seed, "wt-flat", "/tasks/flat/repo", "feature/current", now)
			seedLegacySessionWorktree(t, db, seed.sessionID, "wt-flat", seed.repoID, "", "/tasks/flat/repo", "feature/stale-session", "active", now)

			repo, err := NewWithDB(db, db, nil)
			if err != nil {
				t.Fatalf("cutover: %v", err)
			}
			env, err := repo.GetTaskEnvironment(context.Background(), seed.envID)
			if err != nil {
				t.Fatalf("get task environment: %v", err)
			}
			if len(env.Repos) != 1 || env.Repos[0].WorktreeBranch != "feature/current" {
				t.Fatalf("normalized environment = %+v", env.Repos)
			}
		})
	}
}

// TestCutover_IgnoresTerminalWorktreeDifferentFromFlatOwner proves that
// terminal session history cannot replace the surviving flat environment
// owner for the same repository and empty branch slot.
func TestCutover_IgnoresTerminalWorktreeDifferentFromFlatOwner(t *testing.T) {
	for _, state := range []string{"COMPLETED", "FAILED", "CANCELLED"} {
		t.Run(state, func(t *testing.T) {
			db := openLegacyDB(t)
			now := time.Now().UTC().Truncate(time.Second)
			seed := legacySeed{envID: "env-flat-terminal", taskID: "task-flat-terminal", repoID: "repo-flat-terminal", sessionID: "sess-flat-terminal"}
			seedLegacyTask(t, db, seed, now)
			if _, err := db.Exec(db.Rebind(`UPDATE task_sessions SET state = ? WHERE id = ?`), state, seed.sessionID); err != nil {
				t.Fatalf("set session state: %v", err)
			}
			seedLegacyFlatEnv(t, db, seed, "wt-flat-current", "/tasks/flat-terminal/repo", "feature/current", now)
			seedLegacySessionWorktree(t, db, seed.sessionID, "wt-flat-old", seed.repoID, "", "/tasks/flat-terminal/old", "feature/old", "active", now)

			repo, err := NewWithDB(db, db, nil)
			if err != nil {
				t.Fatalf("cutover: %v", err)
			}
			env, err := repo.GetTaskEnvironment(context.Background(), seed.envID)
			if err != nil {
				t.Fatalf("get task environment: %v", err)
			}
			if len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-flat-current" {
				t.Fatalf("flat environment owner = %+v", env.Repos)
			}
		})
	}
}

// TestCutover_PreservesTerminalWorktreeOutsideFlatOwnerSlot proves that flat
// source precedence does not discard a different repository or branch slot.
func TestCutover_PreservesTerminalWorktreeOutsideFlatOwnerSlot(t *testing.T) {
	for _, tc := range []struct {
		name              string
		sessionRepoID     string
		sessionBranchSlug string
	}{
		{name: "different repository", sessionRepoID: "repo-flat-terminal-other"},
		{name: "non-empty branch", sessionRepoID: "repo-flat-terminal", sessionBranchSlug: "feature/old"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openLegacyDB(t)
			now := time.Now().UTC().Truncate(time.Second)
			seed := legacySeed{envID: "env-flat-terminal", taskID: "task-flat-terminal", repoID: "repo-flat-terminal", sessionID: "sess-flat-terminal"}
			seedLegacyTask(t, db, seed, now)
			seedLegacyFlatEnv(t, db, seed, "wt-flat-current", "/tasks/flat-terminal/repo", "feature/current", now)
			seedLegacySessionWorktree(t, db, seed.sessionID, "wt-flat-old", tc.sessionRepoID, tc.sessionBranchSlug, "/tasks/flat-terminal/old", "feature/old", "active", now)

			repo, err := NewWithDB(db, db, nil)
			if err != nil {
				t.Fatalf("cutover: %v", err)
			}
			env, err := repo.GetTaskEnvironment(context.Background(), seed.envID)
			if err != nil {
				t.Fatalf("get task environment: %v", err)
			}

			slots := make(map[string]string, len(env.Repos))
			for _, envRepo := range env.Repos {
				slots[envRepo.RepositoryID+"\x00"+envRepo.BranchSlug] = envRepo.WorktreeID
			}
			flatSlot := seed.repoID + "\x00"
			sessionSlot := tc.sessionRepoID + "\x00" + tc.sessionBranchSlug
			if len(env.Repos) != 2 || slots[flatSlot] != "wt-flat-current" || slots[sessionSlot] != "wt-flat-old" {
				t.Fatalf("preserved repository slots = %+v", env.Repos)
			}
		})
	}
}

// TestCutover_IgnoresDeletedHistoricalSessionConflict proves that a deleted
// session reference cannot override an existing task-owned repository row.
func TestCutover_IgnoresDeletedHistoricalSessionConflict(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{envID: "env-history", taskID: "task-history", repoID: "repo-history", sessionID: "sess-history"}
	seedLegacyTask(t, db, seed, now)
	seedLegacyFlatEnv(t, db, seed, "wt-current", "/tasks/history/repo", "feature/current", now)
	seedLegacyEnvRepo(t, db, "env-repo-history", "env-history", "repo-history", "wt-current", "/tasks/history/repo", "feature/current", now)
	seedLegacySessionWorktree(t, db, "sess-history", "wt-deleted", "repo-history", "", "/tasks/history/old", "feature/old", "deleted", now)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironment(context.Background(), "env-history")
	if err != nil {
		t.Fatalf("get task environment: %v", err)
	}
	if len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-current" {
		t.Fatalf("canonical repository row = %+v", env.Repos)
	}
}

// TestCutover_IgnoresTerminalHistoricalSessionConflict proves that a
// completed, failed, or cancelled session cannot override a canonical owner.
func TestCutover_IgnoresTerminalHistoricalSessionConflict(t *testing.T) {
	for _, state := range []string{sessionStateCompleted, sessionStateFailed, sessionStateCancelled} {
		t.Run(state, func(t *testing.T) {
			db := openLegacyDB(t)
			now := time.Now().UTC().Truncate(time.Second)
			seed := legacySeed{envID: "env-terminal", taskID: "task-terminal", repoID: "repo-terminal", sessionID: "sess-terminal"}
			seedLegacyTask(t, db, seed, now)
			if _, err := db.Exec(db.Rebind(`UPDATE task_sessions SET state = ? WHERE id = ?`), state, seed.sessionID); err != nil {
				t.Fatalf("set session state: %v", err)
			}
			seedLegacyFlatEnv(t, db, seed, "wt-current", "/tasks/terminal/repo", "feature/current", now)
			seedLegacyEnvRepo(t, db, "env-repo-terminal", "env-terminal", "repo-terminal", "wt-current", "/tasks/terminal/repo", "feature/current", now)
			seedLegacySessionWorktree(t, db, seed.sessionID, "wt-old", "repo-terminal", "", "/tasks/terminal/old", "feature/current", "active", now)

			repo, err := NewWithDB(db, db, nil)
			if err != nil {
				t.Fatalf("cutover: %v", err)
			}
			env, err := repo.GetTaskEnvironment(context.Background(), seed.envID)
			if err != nil {
				t.Fatalf("get task environment: %v", err)
			}
			if len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-current" {
				t.Fatalf("canonical repository row = %+v", env.Repos)
			}
		})
	}
}

// TestCutover_PreservesCreatedSessionWorktreeWithoutCanonicalOwner proves
// that an unexpected CREATED row is backfilled instead of silently dropped.
func TestCutover_PreservesCreatedSessionWorktreeWithoutCanonicalOwner(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{envID: "env-created", taskID: "task-created", repoID: "repo-created", sessionID: "sess-created"}
	seedLegacyTask(t, db, seed, now)
	if _, err := db.Exec(db.Rebind(`UPDATE task_sessions SET state = 'CREATED' WHERE id = ?`), seed.sessionID); err != nil {
		t.Fatalf("set session state: %v", err)
	}
	seedLegacyFlatEnv(t, db, seed, "wt-created", "/tasks/created/repo", "feature/created", now)
	seedLegacySessionWorktree(t, db, seed.sessionID, "wt-created", "repo-created", "", "/tasks/created/repo", "feature/created", "active", now)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironment(context.Background(), seed.envID)
	if err != nil {
		t.Fatalf("get task environment: %v", err)
	}
	if len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-created" {
		t.Fatalf("created session worktree = %+v", env.Repos)
	}
}

// TestCutover_SharedEnvironmentAcrossTasks proves that a session bound to
// another task's environment (workspace-group sharing / subtask inheritance)
// normalizes onto the owning task's environment instead of failing the
// cutover or fabricating a duplicate worktree owner.
func TestCutover_SharedEnvironmentAcrossTasks(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	owner := legacySeed{envID: "env-shared", taskID: "task-owner", repoID: "repo-shared", sessionID: "sess-owner"}
	seedLegacyTask(t, db, owner, now)
	seedSharedMemberTask(t, db, "task-member", "sess-member", "env-shared", now)
	linkSessionEnvironment(t, db, "sess-owner", "env-shared")

	seedLegacyFlatEnv(t, db, owner, "wt-shared", "/tasks/shared/kandev", "feature/shared", now)
	seedLegacyEnvRepo(t, db, "env-repo-shared", "env-shared", "repo-shared", "wt-shared", "/tasks/shared/kandev", "feature/shared", now)
	seedLegacySessionWorktree(t, db, "sess-owner", "wt-shared", "repo-shared", "", "/tasks/shared/kandev", "feature/shared", "active", now)
	seedLegacySessionWorktree(t, db, "sess-member", "wt-shared", "repo-shared", "", "/tasks/shared/kandev", "feature/shared", "active", now)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	ctx := context.Background()

	env, err := repo.GetTaskEnvironment(ctx, "env-shared")
	if err != nil {
		t.Fatalf("GetTaskEnvironment: %v", err)
	}
	if len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-shared" {
		t.Fatalf("shared environment repos = %+v, want one wt-shared row", env.Repos)
	}
	for _, sessionID := range []string{"sess-owner", "sess-member"} {
		session, err := repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			t.Fatalf("GetTaskSession(%s): %v", sessionID, err)
		}
		if session.TaskEnvironmentID != "env-shared" {
			t.Fatalf("session %s env = %q, want env-shared", sessionID, session.TaskEnvironmentID)
		}
	}
	// The borrowing task must not gain an environment of its own: the
	// physical worktree has exactly one owner.
	memberEnv, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-member")
	if err != nil {
		t.Fatalf("GetTaskEnvironmentByTaskID(task-member): %v", err)
	}
	if memberEnv != nil {
		t.Fatalf("borrowing task gained environment %s with repos %+v", memberEnv.ID, memberEnv.Repos)
	}
	var owners int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM task_environment_repos WHERE worktree_id = 'wt-shared'`).Scan(&owners); err != nil {
		t.Fatalf("count worktree owners: %v", err)
	}
	if owners != 1 {
		t.Fatalf("worktree owner rows = %d, want 1", owners)
	}
}

// TestCutover_SharedEnvironmentWithoutCanonicalRepoRow covers the same
// borrowed-environment shape when only the member session carries the
// worktree metadata: it must land on the environment's owning task.
func TestCutover_SharedEnvironmentWithoutCanonicalRepoRow(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	owner := legacySeed{envID: "env-borrow", taskID: "task-lender", repoID: "", sessionID: "sess-lender"}
	seedLegacyTask(t, db, owner, now)
	seedSharedMemberTask(t, db, "task-borrower", "sess-borrower", "env-borrow", now)
	linkSessionEnvironment(t, db, "sess-lender", "env-borrow")
	seedLegacyFlatEnv(t, db, owner, "", "", "", now)
	seedLegacySessionWorktree(t, db, "sess-borrower", "wt-borrow", "repo-borrow", "", "/tasks/borrow/kandev", "feature/borrow", "active", now)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironment(context.Background(), "env-borrow")
	if err != nil {
		t.Fatalf("GetTaskEnvironment: %v", err)
	}
	if len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-borrow" ||
		env.Repos[0].WorktreePath != "/tasks/borrow/kandev" {
		t.Fatalf("borrowed worktree row = %+v, want wt-borrow on the lending environment", env.Repos)
	}
}

// TestCutover_SharedEnvironmentIgnoresStaleMemberMetadata proves that stale
// worktree metadata on a borrowing session cannot conflict with the canonical
// row owned by the lending task's environment.
func TestCutover_SharedEnvironmentIgnoresStaleMemberMetadata(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	owner := legacySeed{envID: "env-stale", taskID: "task-stale-owner", repoID: "repo-stale", sessionID: "sess-stale-owner"}
	seedLegacyTask(t, db, owner, now)
	seedSharedMemberTask(t, db, "task-stale-member", "sess-stale-member", "env-stale", now)
	linkSessionEnvironment(t, db, "sess-stale-owner", "env-stale")
	if _, err := db.Exec(db.Rebind(
		`UPDATE task_sessions SET state = 'WAITING_FOR_INPUT' WHERE id = ?`), "sess-stale-member"); err != nil {
		t.Fatalf("set member session state: %v", err)
	}
	seedLegacyFlatEnv(t, db, owner, "wt-stale", "/tasks/stale/kandev", "feature/current", now)
	seedLegacyEnvRepo(t, db, "env-repo-stale", "env-stale", "repo-stale", "wt-stale", "/tasks/stale/kandev", "feature/current", now)
	seedLegacySessionWorktree(t, db, "sess-stale-member", "wt-stale", "repo-stale", "", "/tasks/stale/kandev", "feature/stale", "active", now)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironment(context.Background(), "env-stale")
	if err != nil {
		t.Fatalf("GetTaskEnvironment: %v", err)
	}
	if len(env.Repos) != 1 || env.Repos[0].WorktreeBranch != "feature/current" {
		t.Fatalf("shared environment repos = %+v, want the canonical feature/current row", env.Repos)
	}
}

// seedSharedMemberTask seeds a task whose only session borrows another
// task's environment.
func seedSharedMemberTask(t *testing.T, db *sqlx.DB, taskID, sessionID, envID string, startedAt time.Time) {
	t.Helper()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, 'ws-1', 'shared member', ?, ?)`), taskID, startedAt, startedAt); err != nil {
		t.Fatalf("seed member task: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_sessions (id, task_id, state, task_environment_id, started_at, updated_at)
		VALUES (?, ?, 'COMPLETED', ?, ?, ?)`), sessionID, taskID, envID, startedAt, startedAt); err != nil {
		t.Fatalf("seed member session: %v", err)
	}
}

func linkSessionEnvironment(t *testing.T, db *sqlx.DB, sessionID, envID string) {
	t.Helper()
	if _, err := db.Exec(db.Rebind(
		`UPDATE task_sessions SET task_environment_id = ? WHERE id = ?`), envID, sessionID); err != nil {
		t.Fatalf("link session environment: %v", err)
	}
}
