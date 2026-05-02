package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func newRepoForHealTests(t *testing.T) *Repository {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := NewWithDB(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlxDB.Close()
	})
	return repo
}

// insertEnv writes a minimal task_environments row directly. Bypasses
// CreateTaskEnvironment so tests can construct rows that violate invariants
// (the very rows the heal step is supposed to repair).
func insertEnv(t *testing.T, db *sqlx.DB, env *models.TaskEnvironment) {
	t.Helper()
	if env.CreatedAt.IsZero() {
		env.CreatedAt = time.Now().UTC()
	}
	if env.UpdatedAt.IsZero() {
		env.UpdatedAt = env.CreatedAt
	}
	if env.Status == "" {
		env.Status = models.TaskEnvironmentStatusReady
	}
	_, err := db.Exec(`
		INSERT INTO task_environments (
			id, task_id, repository_id, executor_type, executor_id, executor_profile_id,
			agent_execution_id, control_port, status,
			worktree_id, worktree_path, worktree_branch, workspace_path,
			container_id, sandbox_id,
			created_at, updated_at
		) VALUES (?, ?, '', ?, '', '', '', 0, ?, '', ?, '', ?, '', '', ?, ?)
	`, env.ID, env.TaskID, env.ExecutorType, string(env.Status),
		env.WorktreePath, env.WorkspacePath, env.CreatedAt, env.UpdatedAt)
	if err != nil {
		t.Fatalf("insert env: %v", err)
	}
}

func insertTask(t *testing.T, db *sqlx.DB, taskID string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := db.Exec(`
		INSERT INTO tasks (id, workspace_id, workflow_id, workflow_step_id, title, description, state, created_at, updated_at)
		VALUES (?, '', '', '', 'test task', '', 'todo', ?, ?)
	`, taskID, now, now)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}
}

// TestHealTaskEnvironmentWorkspacePaths_BackfillsEmpty seeds a worktree-mode
// env with worktree_path set but workspace_path empty (the corrupt state seen
// in the live DB) and asserts the heal step backfills workspace_path from
// worktree_path.
func TestHealTaskEnvironmentWorkspacePaths_BackfillsEmpty(t *testing.T) {
	repo := newRepoForHealTests(t)
	insertTask(t, repo.db, "task-A")
	insertEnv(t, repo.db, &models.TaskEnvironment{
		ID:           "env-A",
		TaskID:       "task-A",
		ExecutorType: "worktree",
		WorktreePath: "/home/user/.kandev/worktrees/foo",
		// WorkspacePath intentionally empty.
	})

	if err := repo.healTaskEnvironmentWorkspacePaths(); err != nil {
		t.Fatalf("heal: %v", err)
	}

	got, err := repo.GetTaskEnvironment(context.Background(), "env-A")
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	if got.WorkspacePath != "/home/user/.kandev/worktrees/foo" {
		t.Errorf("workspace_path = %q, want it backfilled from worktree_path", got.WorkspacePath)
	}
}

// TestHealTaskEnvironmentWorkspacePaths_LeavesPopulatedAlone — a row that
// already has a workspace_path must not be touched. Otherwise the heal could
// stamp on task-dir-mode envs whose workspace_path is the worktree's parent.
func TestHealTaskEnvironmentWorkspacePaths_LeavesPopulatedAlone(t *testing.T) {
	repo := newRepoForHealTests(t)
	insertTask(t, repo.db, "task-B")
	insertEnv(t, repo.db, &models.TaskEnvironment{
		ID:            "env-B",
		TaskID:        "task-B",
		ExecutorType:  "worktree",
		WorktreePath:  "/home/user/.kandev/tasks/foo_abc/repo",
		WorkspacePath: "/home/user/.kandev/tasks/foo_abc",
	})

	if err := repo.healTaskEnvironmentWorkspacePaths(); err != nil {
		t.Fatalf("heal: %v", err)
	}

	got, err := repo.GetTaskEnvironment(context.Background(), "env-B")
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	if got.WorkspacePath != "/home/user/.kandev/tasks/foo_abc" {
		t.Errorf("workspace_path = %q, must not be overwritten", got.WorkspacePath)
	}
}

// TestHealTaskEnvironmentWorkspacePaths_TaskDirMode — task-dir-mode envs
// place the worktree at <root>/.kandev/tasks/<name>/<repo>; workspace_path
// must be the per-task root (Dir of worktree_path), not the repo subdir.
// The naive heal would have stamped worktree_path here and corrupted the
// row in a different way.
func TestHealTaskEnvironmentWorkspacePaths_TaskDirMode(t *testing.T) {
	repo := newRepoForHealTests(t)
	insertTask(t, repo.db, "task-T")
	insertEnv(t, repo.db, &models.TaskEnvironment{
		ID:           "env-T",
		TaskID:       "task-T",
		ExecutorType: "worktree",
		WorktreePath: "/home/u/.kandev/tasks/fix-something_abc/kandev",
	})

	if err := repo.healTaskEnvironmentWorkspacePaths(); err != nil {
		t.Fatalf("heal: %v", err)
	}

	got, err := repo.GetTaskEnvironment(context.Background(), "env-T")
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	if got.WorkspacePath != "/home/u/.kandev/tasks/fix-something_abc" {
		t.Errorf("workspace_path = %q, want task root (parent of worktree_path)", got.WorkspacePath)
	}
}

// TestHealTaskEnvironmentWorkspacePaths_Idempotent — running the heal twice
// must not change anything on the second run.
func TestHealTaskEnvironmentWorkspacePaths_Idempotent(t *testing.T) {
	repo := newRepoForHealTests(t)
	insertTask(t, repo.db, "task-C")
	insertEnv(t, repo.db, &models.TaskEnvironment{
		ID:           "env-C",
		TaskID:       "task-C",
		ExecutorType: "worktree",
		WorktreePath: "/x",
	})

	for i := 0; i < 2; i++ {
		if err := repo.healTaskEnvironmentWorkspacePaths(); err != nil {
			t.Fatalf("heal pass %d: %v", i, err)
		}
	}

	got, err := repo.GetTaskEnvironment(context.Background(), "env-C")
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	if got.WorkspacePath != "/x" {
		t.Errorf("workspace_path = %q, want /x after idempotent run", got.WorkspacePath)
	}
}

// TestHealDuplicateTaskEnvironments_KeepsMostRecent seeds two envs for the
// same task (the race the user hit) with sessions pointing at the older one,
// runs the heal, and asserts: only the newer env remains, sessions have been
// re-pointed at it.
func TestHealDuplicateTaskEnvironments_KeepsMostRecent(t *testing.T) {
	repo := newRepoForHealTests(t)

	// initSchema added the unique-task_id index — it would block our
	// duplicate-row seeding. Drop it for the duration of the test; the heal
	// step will succeed against the duplicate-free DB it leaves behind.
	if _, err := repo.db.Exec(`DROP INDEX IF EXISTS uniq_task_environments_task_id`); err != nil {
		t.Fatalf("drop index: %v", err)
	}

	insertTask(t, repo.db, "task-D")

	older := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	newer := older.Add(5 * time.Second)
	insertEnv(t, repo.db, &models.TaskEnvironment{
		ID: "env-old", TaskID: "task-D", ExecutorType: "worktree",
		WorktreePath: "/old", WorkspacePath: "/old",
		CreatedAt: older, UpdatedAt: older,
	})
	insertEnv(t, repo.db, &models.TaskEnvironment{
		ID: "env-new", TaskID: "task-D", ExecutorType: "worktree",
		WorktreePath: "/new", WorkspacePath: "/new",
		CreatedAt: newer, UpdatedAt: newer,
	})
	// A session created against the loser env — must be re-linked.
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID:                "sess-D",
		TaskID:            "task-D",
		State:             models.TaskSessionStateCreated,
		TaskEnvironmentID: "env-old",
	}); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	if err := repo.healDuplicateTaskEnvironments(); err != nil {
		t.Fatalf("heal: %v", err)
	}

	var remaining int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM task_environments WHERE task_id='task-D'`).Scan(&remaining); err != nil {
		t.Fatalf("count: %v", err)
	}
	if remaining != 1 {
		t.Errorf("expected 1 env after heal, got %d", remaining)
	}
	var winnerID string
	if err := repo.db.QueryRow(`SELECT id FROM task_environments WHERE task_id='task-D'`).Scan(&winnerID); err != nil {
		t.Fatalf("scan winner: %v", err)
	}
	if winnerID != "env-new" {
		t.Errorf("winner = %q, want env-new (most recently updated)", winnerID)
	}
	var sessionEnv string
	if err := repo.db.QueryRow(`SELECT task_environment_id FROM task_sessions WHERE id='sess-D'`).Scan(&sessionEnv); err != nil {
		t.Fatalf("scan session env: %v", err)
	}
	if sessionEnv != "env-new" {
		t.Errorf("session env = %q, want env-new (re-linked from loser)", sessionEnv)
	}
}

// TestHealDuplicateTaskEnvironments_NoOpWhenSingle — single-env tasks must
// not be affected. Also verifies the heal handles multi-task DBs correctly.
func TestHealDuplicateTaskEnvironments_NoOpWhenSingle(t *testing.T) {
	repo := newRepoForHealTests(t)
	insertTask(t, repo.db, "task-E")
	insertEnv(t, repo.db, &models.TaskEnvironment{
		ID: "env-E", TaskID: "task-E", ExecutorType: "worktree",
		WorktreePath: "/e", WorkspacePath: "/e",
	})

	if err := repo.healDuplicateTaskEnvironments(); err != nil {
		t.Fatalf("heal: %v", err)
	}

	got, err := repo.GetTaskEnvironment(context.Background(), "env-E")
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	if got.ID != "env-E" {
		t.Errorf("env-E should still exist")
	}
}

// TestEnsureTaskEnvironmentTaskUniqueIndex_BlocksFutureDuplicates asserts the
// unique index is enforced after the heal so a future regression in the
// orchestrator's create path fails loud instead of silently double-inserting.
func TestEnsureTaskEnvironmentTaskUniqueIndex_BlocksFutureDuplicates(t *testing.T) {
	repo := newRepoForHealTests(t)
	insertTask(t, repo.db, "task-F")
	insertEnv(t, repo.db, &models.TaskEnvironment{
		ID: "env-F1", TaskID: "task-F", ExecutorType: "worktree",
		WorktreePath: "/f", WorkspacePath: "/f",
	})

	// initSchema already added the unique index; a second insert for the same
	// task must fail with a constraint error.
	_, err := repo.db.Exec(`
		INSERT INTO task_environments (
			id, task_id, repository_id, executor_type, executor_id, executor_profile_id,
			agent_execution_id, control_port, status,
			worktree_id, worktree_path, worktree_branch, workspace_path,
			container_id, sandbox_id, created_at, updated_at
		) VALUES (?, 'task-F', '', 'worktree', '', '', '', 0, 'ready',
		          '', '/f2', '', '/f2', '', '', datetime('now'), datetime('now'))
	`, "env-F2")
	if err == nil {
		t.Fatal("expected unique-constraint error inserting second env for same task_id")
	}
	// Confirm it's specifically a UNIQUE constraint failure, not some other DB
	// error masquerading as success.
	if !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("expected UNIQUE constraint error, got: %v", err)
	}
	// Belt-and-suspenders: make sure the second row didn't sneak in.
	var n int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM task_environments WHERE task_id='task-F'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 env for task-F, got %d", n)
	}
}

// TestCreateTaskEnvironment_RejectsEmptyWorkspaceForWorktree asserts that
// inserts of a worktree-mode env with empty workspace_path are refused at
// the repository boundary — a future writer regression must fail loud.
func TestCreateTaskEnvironment_RejectsEmptyWorkspaceForWorktree(t *testing.T) {
	repo := newRepoForHealTests(t)
	insertTask(t, repo.db, "task-G")

	err := repo.CreateTaskEnvironment(context.Background(), &models.TaskEnvironment{
		TaskID:       "task-G",
		ExecutorType: "worktree",
		WorktreePath: "/g",
		// WorkspacePath intentionally empty.
	})
	if err == nil {
		t.Fatal("expected create to fail when workspace_path empty for worktree")
	}
}

// TestCreateTaskEnvironment_AllowsNonWorktreeWithEmptyWorkspace — non-worktree
// executors (e.g. local_pc) may legitimately have no workspace_path; the
// guard must not block them.
func TestCreateTaskEnvironment_AllowsNonWorktreeWithEmptyWorkspace(t *testing.T) {
	repo := newRepoForHealTests(t)
	insertTask(t, repo.db, "task-H")

	err := repo.CreateTaskEnvironment(context.Background(), &models.TaskEnvironment{
		TaskID:       "task-H",
		ExecutorType: "local_pc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestUpdateTaskEnvironment_RejectsClearingWorkspaceForWorktree — symmetric
// guard: a writer must not be able to clear a previously-populated
// workspace_path on a worktree env.
func TestUpdateTaskEnvironment_RejectsClearingWorkspaceForWorktree(t *testing.T) {
	repo := newRepoForHealTests(t)
	insertTask(t, repo.db, "task-I")
	if err := repo.CreateTaskEnvironment(context.Background(), &models.TaskEnvironment{
		ID: "env-I", TaskID: "task-I", ExecutorType: "worktree",
		WorktreePath: "/i", WorkspacePath: "/i",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	err := repo.UpdateTaskEnvironment(context.Background(), &models.TaskEnvironment{
		ID: "env-I", TaskID: "task-I", ExecutorType: "worktree",
		WorktreePath: "/i", WorkspacePath: "",
	})
	if err == nil {
		t.Fatal("expected update to fail when clearing workspace_path on worktree env")
	}
}

// insertSessionWithEnvID writes a task_sessions row directly. Bypasses
// CreateTaskSession so tests can seed legacy rows with empty/null
// task_environment_id (the very rows the heal step is supposed to repair).
func insertSessionWithEnvID(t *testing.T, db *sqlx.DB, sessionID, taskID, envID string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := db.Exec(`
		INSERT INTO task_sessions (
			id, task_id, agent_execution_id, container_id, agent_profile_id, executor_id, executor_profile_id, environment_id,
			repository_id, base_branch, base_commit_sha,
			agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
			state, error_message, metadata, started_at, completed_at, updated_at,
			is_primary, review_status, is_passthrough, task_environment_id
		) VALUES (?, ?, '', '', '', '', '', '', '', '', '',
		          '{}', '{}', '{}', '{}',
		          'created', '', '{}', ?, NULL, ?,
		          0, '', 0, ?)
	`, sessionID, taskID, now, now, envID)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
}

// insertSessionWithNullEnvID writes a task_sessions row with task_environment_id
// stored as SQL NULL (not the empty string). Some legacy paths produced NULL
// rather than ”, so the heal must cover both predicates.
func insertSessionWithNullEnvID(t *testing.T, db *sqlx.DB, sessionID, taskID string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := db.Exec(`
		INSERT INTO task_sessions (
			id, task_id, agent_execution_id, container_id, agent_profile_id, executor_id, executor_profile_id, environment_id,
			repository_id, base_branch, base_commit_sha,
			agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
			state, error_message, metadata, started_at, completed_at, updated_at,
			is_primary, review_status, is_passthrough, task_environment_id
		) VALUES (?, ?, '', '', '', '', '', '', '', '', '',
		          '{}', '{}', '{}', '{}',
		          'created', '', '{}', ?, NULL, ?,
		          0, '', 0, NULL)
	`, sessionID, taskID, now, now)
	if err != nil {
		t.Fatalf("insert session with null env: %v", err)
	}
}

// TestHealSessionTaskEnvironmentIDs_LinksOrphans seeds a task with an existing
// env and a session whose task_environment_id is empty. Asserts the heal step
// links the session to the env. This is the exact bug that caused
// `+ → Terminal` to silently fail with "session has no task environment ID".
func TestHealSessionTaskEnvironmentIDs_LinksOrphans(t *testing.T) {
	repo := newRepoForHealTests(t)
	insertTask(t, repo.db, "task-J")
	insertEnv(t, repo.db, &models.TaskEnvironment{
		ID: "env-J", TaskID: "task-J", ExecutorType: "worktree",
		WorktreePath: "/j", WorkspacePath: "/j",
	})
	insertSessionWithEnvID(t, repo.db, "sess-J-orphan", "task-J", "")

	if err := repo.healSessionTaskEnvironmentIDs(); err != nil {
		t.Fatalf("heal: %v", err)
	}

	var got string
	if err := repo.db.QueryRow(`SELECT task_environment_id FROM task_sessions WHERE id='sess-J-orphan'`).Scan(&got); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got != "env-J" {
		t.Errorf("session task_environment_id = %q, want env-J", got)
	}
}

// TestHealSessionTaskEnvironmentIDs_LeavesLinkedAlone — sessions already
// pointing at an env must not be touched.
func TestHealSessionTaskEnvironmentIDs_LeavesLinkedAlone(t *testing.T) {
	repo := newRepoForHealTests(t)
	insertTask(t, repo.db, "task-K")
	insertEnv(t, repo.db, &models.TaskEnvironment{
		ID: "env-K", TaskID: "task-K", ExecutorType: "worktree",
		WorktreePath: "/k", WorkspacePath: "/k",
	})
	insertSessionWithEnvID(t, repo.db, "sess-K-linked", "task-K", "env-K")

	if err := repo.healSessionTaskEnvironmentIDs(); err != nil {
		t.Fatalf("heal: %v", err)
	}

	var got string
	if err := repo.db.QueryRow(`SELECT task_environment_id FROM task_sessions WHERE id='sess-K-linked'`).Scan(&got); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got != "env-K" {
		t.Errorf("session task_environment_id = %q, want env-K untouched", got)
	}
}

// TestHealSessionTaskEnvironmentIDs_NoEnvSkips — a session whose task has no
// env row at all is left alone (the backfillTaskEnvironments pass earlier in
// the boot sequence is responsible for creating one; this heal only links).
func TestHealSessionTaskEnvironmentIDs_NoEnvSkips(t *testing.T) {
	repo := newRepoForHealTests(t)
	insertTask(t, repo.db, "task-L")
	insertSessionWithEnvID(t, repo.db, "sess-L-orphan", "task-L", "")

	if err := repo.healSessionTaskEnvironmentIDs(); err != nil {
		t.Fatalf("heal: %v", err)
	}

	var got string
	if err := repo.db.QueryRow(`SELECT task_environment_id FROM task_sessions WHERE id='sess-L-orphan'`).Scan(&got); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got != "" {
		t.Errorf("session task_environment_id = %q, want empty (no env to link to)", got)
	}
}

// TestHealSessionTaskEnvironmentIDs_LinksNullOrphans — sessions whose
// task_environment_id is SQL NULL (not the empty string) must also be
// healed. The WHERE clause covers both predicates; both must be tested.
func TestHealSessionTaskEnvironmentIDs_LinksNullOrphans(t *testing.T) {
	repo := newRepoForHealTests(t)
	insertTask(t, repo.db, "task-N")
	insertEnv(t, repo.db, &models.TaskEnvironment{
		ID: "env-N", TaskID: "task-N", ExecutorType: "worktree",
		WorktreePath: "/n", WorkspacePath: "/n",
	})
	insertSessionWithNullEnvID(t, repo.db, "sess-N-null", "task-N")

	if err := repo.healSessionTaskEnvironmentIDs(); err != nil {
		t.Fatalf("heal: %v", err)
	}

	var got string
	if err := repo.db.QueryRow(`SELECT task_environment_id FROM task_sessions WHERE id='sess-N-null'`).Scan(&got); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got != "env-N" {
		t.Errorf("session task_environment_id = %q, want env-N (healed from NULL)", got)
	}
}

// TestHealSessionTaskEnvironmentIDs_Idempotent — running twice produces the
// same result. The heal must be a no-op once the data is consistent.
func TestHealSessionTaskEnvironmentIDs_Idempotent(t *testing.T) {
	repo := newRepoForHealTests(t)
	insertTask(t, repo.db, "task-M")
	insertEnv(t, repo.db, &models.TaskEnvironment{
		ID: "env-M", TaskID: "task-M", ExecutorType: "worktree",
		WorktreePath: "/m", WorkspacePath: "/m",
	})
	insertSessionWithEnvID(t, repo.db, "sess-M", "task-M", "")

	for i := 0; i < 2; i++ {
		if err := repo.healSessionTaskEnvironmentIDs(); err != nil {
			t.Fatalf("heal pass %d: %v", i, err)
		}
	}

	var got string
	if err := repo.db.QueryRow(`SELECT task_environment_id FROM task_sessions WHERE id='sess-M'`).Scan(&got); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got != "env-M" {
		t.Errorf("session task_environment_id = %q, want env-M after idempotent heal", got)
	}
}

// silences "imported and not used" if some future refactor drops a use.
var _ = sql.ErrNoRows
