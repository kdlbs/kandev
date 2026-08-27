package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
)

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

// TestCutoverPostgres_OrphanedSessionWorktreeUsesFlatOwner proves PostgreSQL
// uses the shared recoverable-orphan classification during cutover.
func TestCutoverPostgres_OrphanedSessionWorktreeUsesFlatOwner(t *testing.T) {
	db := openLegacyPostgres(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{envID: "env-pg-orphan", taskID: "task-pg-orphan", repoID: "repo-pg-orphan", sessionID: "sess-pg-orphan"}
	seedLegacyTask(t, db, seed, now)
	seedLegacyFlatEnv(t, db, seed, "wt-pg-orphan", "/tasks/pg-orphan/current", "feature/current", now)
	seedLegacySessionWorktree(t, db, seed.sessionID, "wt-pg-orphan", seed.repoID, "",
		"/tasks/pg-orphan/stale", "feature/stale", "active", now)
	deleteLegacySession(t, db, seed.sessionID)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("postgres cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironment(context.Background(), seed.envID)
	if err != nil {
		t.Fatalf("GetTaskEnvironment: %v", err)
	}
	if len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-pg-orphan" ||
		env.Repos[0].WorktreePath != "/tasks/pg-orphan/current" ||
		env.Repos[0].WorktreeBranch != "feature/current" {
		t.Fatalf("normalized postgres repos = %+v", env.Repos)
	}
}

func TestCutoverPostgres_ActiveOrphanedSessionWorktreeFailsClosed(t *testing.T) {
	db := openLegacyPostgres(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{taskID: "task-pg-orphan-unique", sessionID: "sess-pg-orphan-unique"}
	seedLegacyTask(t, db, seed, now)
	seedLegacySessionWorktree(t, db, seed.sessionID, "wt-pg-orphan-unique", "repo-pg-orphan-unique", "",
		"/tasks/pg-orphan-unique/repo", "feature/unique", "active", now)
	deleteLegacySession(t, db, seed.sessionID)

	if _, err := NewWithDB(db, db, nil); err == nil ||
		!strings.Contains(err.Error(), "references missing session "+seed.sessionID) {
		t.Fatalf("postgres cutover error = %v, want missing-session conflict", err)
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

func TestCutoverPostgres_CanonicalRepoSupersedesDivergentFlatOwner(t *testing.T) {
	db := openLegacyPostgres(t)
	now := time.Now().UTC().Truncate(time.Second)
	seed := legacySeed{
		envID:     "env-pg-flat-conflict",
		taskID:    "task-pg-flat-conflict",
		repoID:    "repo-pg-flat-conflict",
		sessionID: "sess-pg-flat-conflict",
	}
	seedLegacyTask(t, db, seed, now)
	seedLegacyFlatEnv(t, db, seed, "wt-pg-flat", "/tasks/pg-flat", "feature/flat", now)
	seedLegacyEnvRepo(t, db, "env-pg-repo-canonical", seed.envID, seed.repoID,
		"wt-pg-canonical", "/tasks/pg-canonical", "feature/canonical", now)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("postgres cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironment(context.Background(), seed.envID)
	if err != nil {
		t.Fatalf("GetTaskEnvironment: %v", err)
	}
	if len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-pg-canonical" {
		t.Fatalf("normalized postgres repos = %+v, want canonical wt-pg-canonical", env.Repos)
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

// TestCutoverPostgres_ElectsSingleSlotOwner mirrors the SQLite election: two
// legacy worktrees contesting one repository slot resolve to a single owner
// instead of aborting the upgrade.
func TestCutoverPostgres_ElectsSingleSlotOwner(t *testing.T) {
	db := openLegacyPostgres(t)
	now := time.Now().UTC().Truncate(time.Second)
	older := now.Add(-2 * time.Hour)
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, 'ws-1', 'legacy', ?, ?)`), "task-pg-conflict", older, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	for _, s := range []string{"sess-pgc1", "sess-pgc2"} {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
			VALUES (?, 'task-pg-conflict', 'COMPLETED', ?, ?)`), s, older, now); err != nil {
			t.Fatalf("seed session: %v", err)
		}
	}
	seedLegacySessionWorktree(t, db, "sess-pgc1", "wt-pgc1", "repo-pgc", "", "/t/pgc1", "feature/a", "active", older)
	seedLegacySessionWorktree(t, db, "sess-pgc2", "wt-pgc2", "repo-pgc", "", "/t/pgc2", "feature/b", "active", now)

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("postgres cutover: %v", err)
	}
	env, err := repo.GetTaskEnvironmentByTaskID(context.Background(), "task-pg-conflict")
	if err != nil {
		t.Fatalf("GetTaskEnvironmentByTaskID: %v", err)
	}
	if env == nil || len(env.Repos) != 1 || env.Repos[0].WorktreeID != "wt-pgc2" {
		t.Fatalf("elected postgres env = %+v, want one wt-pgc2 row", env)
	}
	var tableCount int
	if err := db.Get(&tableCount, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_name = 'task_session_worktrees' AND table_schema = current_schema()`); err != nil {
		t.Fatalf("count legacy table: %v", err)
	}
	if tableCount != 0 {
		t.Fatal("completed postgres cutover must drop the legacy table")
	}
}

// TestCutoverPostgres_IrreconcilableMetadataFailsClosed proves postgres still
// rolls back when two live sessions disagree about one worktree's path.
func TestCutoverPostgres_IrreconcilableMetadataFailsClosed(t *testing.T) {
	db := openLegacyPostgres(t)
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, 'ws-1', 'legacy', ?, ?)`), "task-pg-metadata", now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	for _, s := range []string{"sess-pgm1", "sess-pgm2"} {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
			VALUES (?, 'task-pg-metadata', 'RUNNING', ?, ?)`), s, now, now); err != nil {
			t.Fatalf("seed session: %v", err)
		}
	}
	seedLegacySessionWorktree(t, db, "sess-pgm1", "wt-pgm", "repo-pgm", "", "/t/one", "feature/m", "active", now)
	seedLegacySessionWorktree(t, db, "sess-pgm2", "wt-pgm", "repo-pgm", "", "/t/two", "feature/m", "active", now)

	if _, err := NewWithDB(db, db, nil); err == nil {
		t.Fatal("expected postgres cutover to fail on irreconcilable metadata")
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
