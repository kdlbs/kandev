package repository

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
)

// Phase 0 tests for the multi-repo TaskEnvironment / TaskEnvironmentRepo schema.

func newEnv(t *testing.T, repo *sqlite.Repository, taskID, envID string) *models.TaskEnvironment {
	t.Helper()
	env := &models.TaskEnvironment{
		ID:           envID,
		TaskID:       taskID,
		ExecutorType: "local_pc",
		Status:       models.TaskEnvironmentStatusReady,
		TaskDirName:  "fix-bug_ab12",
	}
	if err := repo.CreateTaskEnvironment(context.Background(), env); err != nil {
		t.Fatalf("create env: %v", err)
	}
	return env
}

func newTaskWithRepo(t *testing.T, repo *sqlite.Repository, taskID string) {
	t.Helper()
	ctx := context.Background()
	wf := &models.Workflow{ID: "wf-env", Name: "Env Workflow"}
	_ = repo.CreateWorkflow(ctx, wf)
	task := &models.Task{ID: taskID, WorkflowID: wf.ID, Title: "Env Task"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
}

func TestTaskEnvironment_TaskDirNameRoundTrip(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()

	newTaskWithRepo(t, repo, "task-env-1")
	env := newEnv(t, repo, "task-env-1", "env-1")

	got, err := repo.GetTaskEnvironment(context.Background(), env.ID)
	if err != nil {
		t.Fatalf("get env: %v", err)
	}
	if got.TaskDirName != "fix-bug_ab12" {
		t.Errorf("expected task_dir_name persisted; got %q", got.TaskDirName)
	}
	if len(got.Repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(got.Repos))
	}
}

func TestTaskEnvironmentRepo_CRUD(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	newTaskWithRepo(t, repo, "task-env-2")
	env := newEnv(t, repo, "task-env-2", "env-2")

	er := &models.TaskEnvironmentRepo{
		TaskEnvironmentID: env.ID,
		RepositoryID:      "repo-frontend",
		WorktreeID:        "wt-1",
		WorktreePath:      "/tmp/tasks/x/frontend",
		WorktreeBranch:    "feature/x",
		Position:          0,
	}
	if err := repo.CreateTaskEnvironmentRepo(ctx, er); err != nil {
		t.Fatalf("create env repo: %v", err)
	}
	if er.ID == "" {
		t.Fatal("expected ID to be assigned")
	}

	list, err := repo.ListTaskEnvironmentRepos(ctx, env.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].RepositoryID != "repo-frontend" {
		t.Fatalf("unexpected list: %+v", list)
	}

	er.WorktreeBranch = "feature/x-renamed"
	er.ErrorMessage = "fetch failed"
	if err := repo.UpdateTaskEnvironmentRepo(ctx, er); err != nil {
		t.Fatalf("update: %v", err)
	}
	list, _ = repo.ListTaskEnvironmentRepos(ctx, env.ID)
	if list[0].WorktreeBranch != "feature/x-renamed" || list[0].ErrorMessage != "fetch failed" {
		t.Errorf("update did not persist: %+v", list[0])
	}

	if err := repo.DeleteTaskEnvironmentRepo(ctx, er.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ = repo.ListTaskEnvironmentRepos(ctx, env.ID)
	if len(list) != 0 {
		t.Errorf("expected empty list after delete, got %d", len(list))
	}
}

func TestTaskEnvironmentRepo_PreloadOnGet(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	newTaskWithRepo(t, repo, "task-env-3")
	env := newEnv(t, repo, "task-env-3", "env-3")

	rows := []*models.TaskEnvironmentRepo{
		{TaskEnvironmentID: env.ID, RepositoryID: "repo-backend", Position: 1, WorktreePath: "/b"},
		{TaskEnvironmentID: env.ID, RepositoryID: "repo-frontend", Position: 0, WorktreePath: "/f"},
	}
	for _, r := range rows {
		if err := repo.CreateTaskEnvironmentRepo(ctx, r); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	got, err := repo.GetTaskEnvironment(ctx, env.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(got.Repos))
	}
	if got.Repos[0].RepositoryID != "repo-frontend" {
		t.Errorf("expected position-ordered list, frontend (pos=0) first; got %s", got.Repos[0].RepositoryID)
	}
	if got.RepoFor("repo-backend") == nil {
		t.Error("expected RepoFor lookup to find repo-backend")
	}
	if got.RepoFor("missing") != nil {
		t.Error("expected RepoFor to return nil for unknown repo")
	}
}

func TestTaskEnvironmentRepo_CreateWithEmbeddedRepos(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	newTaskWithRepo(t, repo, "task-env-4")
	env := &models.TaskEnvironment{
		ID:           "env-4",
		TaskID:       "task-env-4",
		ExecutorType: "local_pc",
		Status:       models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{
			{RepositoryID: "repo-a", Position: 0, WorktreePath: "/a"},
			{RepositoryID: "repo-b", Position: 1, WorktreePath: "/b"},
		},
	}
	if err := repo.CreateTaskEnvironment(ctx, env); err != nil {
		t.Fatalf("create env with repos: %v", err)
	}
	got, err := repo.GetTaskEnvironment(ctx, env.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Repos) != 2 {
		t.Fatalf("expected 2 repos persisted, got %d", len(got.Repos))
	}
	for _, r := range got.Repos {
		if r.TaskEnvironmentID != env.ID {
			t.Errorf("expected env id wired; got %q", r.TaskEnvironmentID)
		}
	}
}

func TestTaskEnvironmentRepo_CascadeDeleteOnEnvDelete(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	newTaskWithRepo(t, repo, "task-env-5")
	env := newEnv(t, repo, "task-env-5", "env-5")
	er := &models.TaskEnvironmentRepo{TaskEnvironmentID: env.ID, RepositoryID: "repo-x"}
	if err := repo.CreateTaskEnvironmentRepo(ctx, er); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.DeleteTaskEnvironment(ctx, env.ID); err != nil {
		t.Fatalf("delete env: %v", err)
	}
	list, err := repo.ListTaskEnvironmentRepos(ctx, env.ID)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected cascade delete to remove per-repo rows, got %d", len(list))
	}
}

func TestTaskEnvironmentRepo_BackfillFromLegacyEnv(t *testing.T) {
	// Backfill is run by initSchema. Simulate a "legacy" environment by inserting
	// directly into task_environments with repository_id set, then re-opening the
	// repository so initSchema re-runs the backfill on the existing data.
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "backfill.db")

	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := sqlite.NewWithDB(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	if _, err := worktree.NewSQLiteStore(sqlxDB, sqlxDB); err != nil {
		t.Fatalf("worktree store: %v", err)
	}

	ctx := context.Background()
	newTaskWithRepo(t, repo, "task-env-6")

	// Insert a legacy-shaped env (no per-repo row exists).
	if _, err := sqlxDB.Exec(`
		INSERT INTO task_environments (
			id, task_id, repository_id, executor_type, executor_id, executor_profile_id,
			agent_execution_id, control_port, status,
			worktree_id, worktree_path, worktree_branch, workspace_path,
			container_id, sandbox_id, task_dir_name,
			created_at, updated_at
		) VALUES ('legacy-env', 'task-env-6', 'repo-legacy', 'local_pc', '', '',
			'', 0, 'ready',
			'wt-legacy', '/wt/legacy', 'main', '',
			'', '', '',
			datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("insert legacy env: %v", err)
	}

	// Verify no per-repo row yet.
	list, err := repo.ListTaskEnvironmentRepos(ctx, "legacy-env")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 per-repo rows pre-backfill, got %d", len(list))
	}

	// Re-open the repository to retrigger initSchema → backfill.
	if err := repo.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	dbConn2, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	sqlxDB2 := sqlx.NewDb(dbConn2, "sqlite3")
	repo2, err := sqlite.NewWithDB(sqlxDB2, sqlxDB2)
	if err != nil {
		t.Fatalf("reopen repo: %v", err)
	}
	defer func() {
		_ = repo2.Close()
		_ = sqlxDB2.Close()
	}()

	list, err = repo2.ListTaskEnvironmentRepos(ctx, "legacy-env")
	if err != nil {
		t.Fatalf("list after reopen: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 backfilled row, got %d", len(list))
	}
	got := list[0]
	if got.RepositoryID != "repo-legacy" || got.WorktreeID != "wt-legacy" || got.WorktreeBranch != "main" {
		t.Errorf("backfilled row mismatch: %+v", got)
	}

	// Idempotency: a second backfill must not duplicate rows.
	list, _ = repo2.ListTaskEnvironmentRepos(ctx, "legacy-env")
	if len(list) != 1 {
		t.Errorf("expected backfill to be idempotent, got %d rows", len(list))
	}
}
