package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
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
	// sql driver returns a sqlite-specific error; just confirm it's a real DB error.
	if _, ok := err.(interface{ Error() string }); !ok {
		t.Fatalf("unexpected error type: %T", err)
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

// silences "imported and not used" if some future refactor drops a use.
var _ = sql.ErrNoRows
