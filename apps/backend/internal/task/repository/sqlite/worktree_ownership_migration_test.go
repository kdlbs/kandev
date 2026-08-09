package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
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
	for _, gone := range []string{"repository_id", "worktree_id", "worktree_path", "worktree_branch"} {
		if envCols[gone] {
			t.Fatalf("fresh task_environments still has legacy column %s", gone)
		}
	}
	repoCols := tableColumnSet(t, sqlxDB, "task_environment_repos")
	for _, required := range []string{"status", "merged_at", "deleted_at"} {
		if !repoCols[required] {
			t.Fatalf("fresh task_environment_repos missing %s", required)
		}
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

// TestCutover_ConflictingWorktreesFailClosed proves that two sessions of the
// same task with different worktrees for the same repository slot abort the
// cutover and leave the legacy database intact.
func TestCutover_ConflictingWorktreesFailClosed(t *testing.T) {
	db := openLegacyDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := db.Exec(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-conflict', 'ws-1', 'legacy', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	for _, s := range []string{"sess-c1", "sess-c2"} {
		if _, err := db.Exec(`
			INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
			VALUES (?, 'task-conflict', 'RUNNING', ?, ?)`, s, now, now); err != nil {
			t.Fatalf("seed session: %v", err)
		}
	}
	seedLegacySessionWorktree(t, db, "sess-c1", "wt-c1", "repo-c", "", "/t/c1", "feature/c1", "active", now)
	seedLegacySessionWorktree(t, db, "sess-c2", "wt-c2", "repo-c", "", "/t/c2", "feature/c2", "active", now)

	_, err := NewWithDB(db, db, nil)
	if err == nil {
		t.Fatal("expected cutover to fail on conflicting worktree identities")
	}
	if !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("error must describe the conflict, got: %v", err)
	}
	// Pre-upgrade state must be intact.
	if !legacyTableExists(t, db, "task_session_worktrees") {
		t.Fatal("legacy table must survive a failed cutover")
	}
	envCols := tableColumnSet(t, db, "task_environments")
	if !envCols["worktree_id"] || !envCols["repository_id"] {
		t.Fatal("legacy flat columns must survive a failed cutover")
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
	for _, state := range []string{"COMPLETED", "FAILED", "CANCELLED"} {
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

// ---------------------------------------------------------------------------
// PostgreSQL coverage
// ---------------------------------------------------------------------------

func openLegacyPostgres(t *testing.T) *sqlx.DB {
	t.Helper()
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("seed baseline postgres schema: %v", err)
	}
	rewindToLegacySchema(t, db)
	return db
}

func TestCutoverPostgres_NormalizesLegacyFlatEnvironment(t *testing.T) {
	db := openLegacyPostgres(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{envID: "env-pg1", taskID: "task-pg1", repoID: "repo-pg1", sessionID: "sess-pg1"}
	seedLegacyTask(t, db, seed, now)
	seedLegacySessionWorktree(t, db, "sess-pg1", "wt-pg1", "repo-pg1", "", "/tasks/pg1/kandev", "feature/pg", "active", now)
	seedLegacyFlatEnv(t, db, seed, "wt-pg1", "/tasks/pg1/kandev", "feature/pg", now)
	seedCleanupJob(t, db, "job-pg-session-delete", "task-pg1", "session_delete", "pending", now)
	seedCleanupJob(t, db, "job-pg-archive", "task-pg1", "archive", "pending", now)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("postgres cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironment(ctx, "env-pg1")
	if err != nil {
		t.Fatalf("GetTaskEnvironment: %v", err)
	}
	if len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-pg1" || env.Repos[0].Status != "active" {
		t.Fatalf("normalized repos = %+v", env.Repos)
	}
	session, err := repo.GetTaskSession(ctx, "sess-pg1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.TaskEnvironmentID != "env-pg1" {
		t.Fatalf("session env = %q, want env-pg1", session.TaskEnvironmentID)
	}
	var tableCount int
	if err := db.Get(&tableCount, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_name = 'task_session_worktrees' AND table_schema = current_schema()`); err != nil {
		t.Fatalf("count legacy table: %v", err)
	}
	if tableCount != 0 {
		t.Fatal("task_session_worktrees must be dropped on postgres")
	}
	var jobCount int
	if err := db.Get(&jobCount, `SELECT COUNT(*) FROM task_resource_cleanup_jobs`); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if jobCount != 1 {
		t.Fatalf("postgres cleanup jobs after cutover = %d, want 1", jobCount)
	}
}

func TestCutoverPostgres_SessionOnlyNormalization(t *testing.T) {
	db := openLegacyPostgres(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{taskID: "task-pg2", sessionID: "sess-pg2"}
	seedLegacyTask(t, db, seed, now)
	seedLegacySessionWorktree(t, db, "sess-pg2", "wt-pg2", "repo-pg2", "", "/tasks/pg2/kandev", "feature/pg2", "active", now)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("postgres cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironmentByTaskID(ctx, "task-pg2")
	if err != nil {
		t.Fatalf("GetTaskEnvironmentByTaskID: %v", err)
	}
	if env == nil || len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-pg2" {
		t.Fatalf("normalized env = %+v", env)
	}
}

func TestCutoverPostgres_ConflictingWorktreesFailClosed(t *testing.T) {
	db := openLegacyPostgres(t)
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, 'ws-1', 'legacy', ?, ?)`), "task-pg-conflict", now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	for _, s := range []string{"sess-pgc1", "sess-pgc2"} {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
			VALUES (?, 'task-pg-conflict', 'COMPLETED', ?, ?)`), s, now, now); err != nil {
			t.Fatalf("seed session: %v", err)
		}
	}
	seedLegacySessionWorktree(t, db, "sess-pgc1", "wt-pgc1", "repo-pgc", "", "/t/pgc1", "feature/a", "active", now)
	seedLegacySessionWorktree(t, db, "sess-pgc2", "wt-pgc2", "repo-pgc", "", "/t/pgc2", "feature/b", "active", now)

	_, err := NewWithDB(db, db, nil)
	if err == nil {
		t.Fatal("expected postgres cutover to fail on conflicting worktrees")
	}
	var tableCount int
	if err := db.Get(&tableCount, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_name = 'task_session_worktrees' AND table_schema = current_schema()`); err != nil {
		t.Fatalf("count legacy table: %v", err)
	}
	if tableCount != 1 {
		t.Fatal("failed postgres cutover must leave the legacy table intact")
	}
}

// TestCutoverPostgres_ReplayIsNoOp proves a second boot skips the cutover.
func TestCutoverPostgres_ReplayIsNoOp(t *testing.T) {
	db := openLegacyPostgres(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{envID: "env-pg3", taskID: "task-pg3", repoID: "repo-pg3", sessionID: "sess-pg3"}
	seedLegacyTask(t, db, seed, now)
	seedLegacySessionWorktree(t, db, "sess-pg3", "wt-pg3", "repo-pg3", "", "/tasks/pg3/kandev", "feature/pg3", "active", now)
	seedLegacyFlatEnv(t, db, seed, "wt-pg3", "/tasks/pg3/kandev", "feature/pg3", now)

	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("first boot: %v", err)
	}
	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("second boot (replay): %v", err)
	}
}

// TestCutoverPostgres_RollbackAtEveryFailpoint mirrors the SQLite failpoint
// matrix on PostgreSQL.
func TestCutoverPostgres_RollbackAtEveryFailpoint(t *testing.T) {
	for _, step := range []string{
		"create_shadow", "copy_envs", "backfill_repos", "link_sessions",
		"validate", "pre_swap", "drop_legacy", "swap", "post_swap",
	} {
		t.Run(step, func(t *testing.T) {
			db := openLegacyPostgres(t)
			now := time.Now().UTC().Truncate(time.Second)
			seed := legacySeed{envID: "env-pgr", taskID: "task-pgr", repoID: "repo-pgr", sessionID: "sess-pgr"}
			seedLegacyTask(t, db, seed, now)
			seedLegacySessionWorktree(t, db, "sess-pgr", "wt-pgr", "repo-pgr", "", "/tasks/pgr/kandev", "feature/pgr", "active", now)
			seedLegacyFlatEnv(t, db, seed, "wt-pgr", "/tasks/pgr/kandev", "feature/pgr", now)

			repo := &Repository{db: db, ro: db, migrate: dbutil.NewMigrateLogger(db, nil), failCutoverAfter: step}
			err := repo.initSchema()
			if err == nil {
				t.Fatalf("expected injected failure at %s", step)
			}
			var tableCount int
			if err := db.Get(&tableCount, `
				SELECT COUNT(*) FROM information_schema.tables
				WHERE table_name = 'task_session_worktrees' AND table_schema = current_schema()`); err != nil {
				t.Fatalf("count legacy table: %v", err)
			}
			if tableCount != 1 {
				t.Fatalf("legacy table must survive rollback at %s", step)
			}
			var path string
			if err := db.Get(&path, db.Rebind(`
				SELECT worktree_path FROM task_session_worktrees WHERE worktree_id = ?`), "wt-pgr"); err != nil {
				t.Fatalf("legacy worktree row lost after rollback at %s: %v", step, err)
			}
			if path != "/tasks/pgr/kandev" {
				t.Fatalf("legacy worktree data changed after rollback at %s", step)
			}
		})
	}
}

// TestCutoverPostgres_FreshSchemaIsFinal proves a fresh PostgreSQL database
// contains only the final schema.
func TestCutoverPostgres_FreshSchemaIsFinal(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init fresh postgres schema: %v", err)
	}
	var tableCount int
	if err := db.Get(&tableCount, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_name = 'task_session_worktrees' AND table_schema = current_schema()`); err != nil {
		t.Fatalf("count legacy table: %v", err)
	}
	if tableCount != 0 {
		t.Fatal("fresh postgres database must not contain task_session_worktrees")
	}
	// The environment FK must reference tasks and the repos FK must
	// reference task_environments after the swap.
	var fkCount int
	if err := db.Get(&fkCount, `
		SELECT COUNT(*) FROM information_schema.table_constraints tc
		JOIN information_schema.referential_constraints rc
			ON rc.constraint_name = tc.constraint_name AND rc.constraint_schema = tc.constraint_schema
		JOIN information_schema.constraint_column_usage ccu
			ON ccu.constraint_name = rc.unique_constraint_name AND ccu.constraint_schema = rc.unique_constraint_schema
		WHERE tc.table_name = 'task_environment_repos'
		  AND tc.constraint_type = 'FOREIGN KEY'
		  AND ccu.table_name = 'task_environments'
		  AND tc.table_schema = current_schema()`); err != nil {
		t.Fatalf("count env-repo FK: %v", err)
	}
	if fkCount != 1 {
		t.Fatalf("task_environment_repos FK to task_environments missing: %d", fkCount)
	}
}
