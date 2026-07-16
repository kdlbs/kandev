package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresExecutorRunningLocalPIDMigration is the Postgres counterpart to
// TestExecutorRunningLocalPIDMigrationOnLegacyDB (SQLite): local_pid is on the
// shared migration path, so ADR 0027 asks for env-gated Postgres replay coverage
// too. It rewinds to a pre-local_pid schema, re-runs migrations, and asserts the
// ADD COLUMN re-adds the column with its default while an existing row survives.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresExecutorRunningLocalPIDMigration(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-pg", "ws-task-pg", "Task pg", now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-pg", TaskID: "task-pg", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-pg", SessionID: "session-pg", TaskID: "task-pg", ExecutorID: "exec-pg",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusStarting,
		Resumable: true, ResumeToken: "pg-resume-token", LocalPID: 321,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}

	// Rewind to a pre-local_pid schema, then re-migrate as a new-binary boot would.
	if _, err := db.Exec(`ALTER TABLE executors_running DROP COLUMN local_pid`); err != nil {
		t.Fatalf("simulate legacy schema (drop local_pid): %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations on legacy postgres DB: %v", err)
	}

	got, err := repo.GetExecutorRunningBySessionID(ctx, "session-pg")
	if err != nil {
		t.Fatalf("legacy row must survive the migration: %v", err)
	}
	if got.LocalPID != 0 {
		t.Errorf("legacy row local_pid = %d, want 0 (column default after ADD COLUMN)", got.LocalPID)
	}
	if got.ResumeToken != "pg-resume-token" {
		t.Errorf("resume_token lost across migration: got %q", got.ResumeToken)
	}
	if got.Status != models.ExecutorRunningStatusStarting {
		t.Errorf("status lost across migration: got %q", got.Status)
	}
}

func TestPostgresExecutionProfileMigration(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-pg-execution-profile", "ws-pg-execution-profile", "Task", now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-pg-execution-profile", TaskID: "task-pg-execution-profile",
		AgentProfileID: "office-agent", ExecutionProfileID: "codex-profile",
		State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-pg-execution-profile", SessionID: "session-pg-execution-profile",
		TaskID: "task-pg-execution-profile", ExecutionProfileID: "codex-profile",
		ExecutorID: "exec-pg", Runtime: agentruntime.RuntimeStandalone,
		Status: models.ExecutorRunningStatusStarting, ResumeToken: "pg-token",
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}

	for _, table := range []string{"task_sessions", "executors_running"} {
		if _, err := db.Exec(`ALTER TABLE ` + table + ` DROP COLUMN execution_profile_id`); err != nil {
			t.Fatalf("drop %s.execution_profile_id: %v", table, err)
		}
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations on legacy postgres schema: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay postgres migrations: %v", err)
	}

	gotSession, err := repo.GetTaskSession(ctx, "session-pg-execution-profile")
	if err != nil {
		t.Fatalf("read migrated session: %v", err)
	}
	if gotSession.ExecutionProfileID != "" || gotSession.AgentProfileID != "office-agent" {
		t.Fatalf("migrated session identities = (%q, %q), want (office-agent, empty)",
			gotSession.AgentProfileID, gotSession.ExecutionProfileID)
	}
	gotRunning, err := repo.GetExecutorRunningBySessionID(ctx, "session-pg-execution-profile")
	if err != nil {
		t.Fatalf("read migrated running executor: %v", err)
	}
	if gotRunning.ExecutionProfileID != "" || gotRunning.ResumeToken != "pg-token" {
		t.Fatalf("migrated executor = (%q, %q), want (empty, pg-token)",
			gotRunning.ExecutionProfileID, gotRunning.ResumeToken)
	}
}

func TestPostgresSchemaReinitializes(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))

	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("first postgres schema init: %v", err)
	}
	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("second postgres schema init: %v", err)
	}
}

func TestPostgresSkipsLegacyTaskEnvironmentBackfill(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init fresh postgres schema: %v", err)
	}

	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`), "task-orphaned", "Orphaned task", now, now); err != nil {
		t.Fatalf("insert orphaned task: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "session-orphaned", "task-orphaned", "CREATED", now, now); err != nil {
		t.Fatalf("insert orphaned session: %v", err)
	}

	if err := repo.backfillTaskEnvironments(); err != nil {
		t.Fatalf("backfill task environments: %v", err)
	}

	var count int
	if err := db.Get(&count, db.Rebind(`
		SELECT COUNT(*) FROM task_environments WHERE task_id = ?
	`), "task-orphaned"); err != nil {
		t.Fatalf("count task environments: %v", err)
	}
	if count != 0 {
		t.Fatalf("task environment count = %d, want 0", count)
	}
}

func TestPostgresWorkflowHiddenRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init fresh postgres schema: %v", err)
	}
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-postgres", Name: "Postgres"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	visible := &models.Workflow{ID: "wf-visible", WorkspaceID: "ws-postgres", Name: "Visible"}
	if err := repo.CreateWorkflow(ctx, visible); err != nil {
		t.Fatalf("create visible workflow: %v", err)
	}
	retrieved, err := repo.GetWorkflow(ctx, visible.ID)
	if err != nil {
		t.Fatalf("get visible workflow: %v", err)
	}
	if retrieved.Hidden {
		t.Fatalf("visible workflow Hidden = true, want false")
	}

	hidden := &models.Workflow{ID: "wf-hidden", WorkspaceID: "ws-postgres", Name: "Hidden", Hidden: true}
	if err := repo.CreateWorkflow(ctx, hidden); err != nil {
		t.Fatalf("create hidden workflow: %v", err)
	}
	retrieved, err = repo.GetWorkflow(ctx, hidden.ID)
	if err != nil {
		t.Fatalf("get hidden workflow: %v", err)
	}
	if !retrieved.Hidden {
		t.Fatalf("hidden workflow Hidden = false, want true")
	}

	hidden.Hidden = false
	if err := repo.UpdateWorkflow(ctx, hidden); err != nil {
		t.Fatalf("update hidden workflow to visible: %v", err)
	}
	retrieved, err = repo.GetWorkflow(ctx, hidden.ID)
	if err != nil {
		t.Fatalf("get updated workflow: %v", err)
	}
	if retrieved.Hidden {
		t.Fatalf("updated workflow Hidden = true, want false")
	}
}

func TestPostgresTaskEnvironmentReposMultiBranchMigration(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := db.Exec(`
		CREATE TABLE task_environment_repos (
			id TEXT PRIMARY KEY,
			task_environment_id TEXT NOT NULL,
			repository_id TEXT NOT NULL,
			worktree_id TEXT DEFAULT '',
			worktree_path TEXT DEFAULT '',
			worktree_branch TEXT DEFAULT '',
			position INTEGER DEFAULT 0,
			error_message TEXT DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			UNIQUE(task_environment_id, repository_id)
		)
	`); err != nil {
		t.Fatalf("create legacy task_environment_repos: %v", err)
	}

	repo := &Repository{db: db}
	if err := repo.migrateTaskEnvironmentReposAllowMultiBranch(); err != nil {
		t.Fatalf("migrate task_environment_repos: %v", err)
	}
	if err := repo.migrateTaskEnvironmentReposAllowMultiBranch(); err != nil {
		t.Fatalf("rerun migration: %v", err)
	}

	now := time.Now().UTC()
	if _, err := db.Exec(`
		INSERT INTO task_environment_repos (
			id, task_environment_id, repository_id, branch_slug,
			worktree_id, created_at, updated_at
		) VALUES
			('ter-main', 'env-1', 'repo-1', '', 'wt-main', $1, $1),
			('ter-branch', 'env-1', 'repo-1', 'branch-5hn', 'wt-branch', $1, $1)
	`, now); err != nil {
		t.Fatalf("insert same repo multi-branch rows: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO task_environment_repos (
			id, task_environment_id, repository_id, branch_slug,
			worktree_id, created_at, updated_at
		) VALUES ('ter-dupe', 'env-1', 'repo-1', '', 'wt-dupe', $1, $1)
	`, now); err == nil {
		t.Fatal("expected duplicate env/repo/branch insert to fail")
	}
}
