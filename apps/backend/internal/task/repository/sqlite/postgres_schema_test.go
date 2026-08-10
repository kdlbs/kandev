package sqlite

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

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

func TestPostgresTaskTitleCASAndStaleUpdate(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()

	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, title, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-pg-title-false", "Provisional", `{"agent_title_pending":false,"keep":"value"}`, now, now); err != nil {
		t.Fatalf("seed false-marker task: %v", err)
	}
	accepted, err := repo.SetTaskTitleIfPending(ctx, "task-pg-title-false", "session-owner", "Agent title")
	if err != nil {
		t.Fatalf("false-marker title CAS: %v", err)
	}
	if accepted {
		t.Fatal("accepted title update with a false pending marker")
	}
	falseTask, err := repo.GetTask(ctx, "task-pg-title-false")
	if err != nil {
		t.Fatalf("reload false-marker task: %v", err)
	}
	if falseTask.Title != "Provisional" || falseTask.Metadata["agent_title_pending"] != false || falseTask.Metadata["keep"] != "value" {
		t.Fatalf("false-marker task changed unexpectedly: title=%q metadata=%#v", falseTask.Title, falseTask.Metadata)
	}

	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, title, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-pg-title-race", "Provisional", `{"agent_title_pending":true}`, now, now); err != nil {
		t.Fatalf("seed stale-update task: %v", err)
	}
	claimed, _, err := repo.ClaimTaskTitleSession(ctx, "task-pg-title-race", "session-owner")
	if err != nil || !claimed {
		t.Fatalf("claim postgres title session: claimed=%v err=%v", claimed, err)
	}
	stale, err := repo.GetTask(ctx, "task-pg-title-race")
	if err != nil {
		t.Fatalf("load stale postgres task: %v", err)
	}
	accepted, err = repo.SetTaskTitleIfPending(ctx, "task-pg-title-race", "session-owner", "Agent chosen title")
	if err != nil || !accepted {
		t.Fatalf("winning title CAS: accepted=%v err=%v", accepted, err)
	}
	stale.Description = "updated concurrently"
	stale.Metadata["stale_change"] = "retained"
	if err := repo.UpdateTask(ctx, stale); err != nil {
		t.Fatalf("stale UpdateTask: %v", err)
	}
	current, err := repo.GetTask(ctx, "task-pg-title-race")
	if err != nil {
		t.Fatalf("reload stale-update task: %v", err)
	}
	if current.Title != "Agent chosen title" || current.Description != "updated concurrently" || current.Metadata["stale_change"] != "retained" {
		t.Fatalf("stale update result = title %q description %q metadata %#v", current.Title, current.Description, current.Metadata)
	}
	if _, pending := current.Metadata[models.MetaKeyAgentTitlePending]; pending {
		t.Fatalf("pending marker restored by stale update: %#v", current.Metadata)
	}
	if _, owner := current.Metadata[models.MetaKeyAgentTitleOwnerSessionID]; owner {
		t.Fatalf("owner marker restored by stale update: %#v", current.Metadata)
	}
}

func TestPostgresImproveKandevWorkflowIndexMigration(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	if _, err := db.Exec(`DROP INDEX IF EXISTS uniq_improve_kandev_workflows`); err != nil {
		t.Fatalf("drop improve kandev index: %v", err)
	}
	now := time.Now().UTC()
	for _, id := range []string{"wf-pg-improve-first", "wf-pg-improve-second"} {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO workflows (id, workspace_id, workflow_template_id, name, hidden, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`), id, "ws-pg-improve-kandev", "improve-kandev", "Improve Kandev", 1, now, now); err != nil {
			t.Fatalf("seed legacy workflow %q: %v", id, err)
		}
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("migrate legacy postgres workflow duplicates: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay postgres improve kandev migration: %v", err)
	}
	var count int
	if err := db.QueryRow(db.Rebind(`
		SELECT COUNT(*) FROM workflows
		WHERE workspace_id = ? AND workflow_template_id = ? AND hidden = ?
	`), "ws-pg-improve-kandev", "improve-kandev", 1).Scan(&count); err != nil {
		t.Fatalf("count reconciled workflows: %v", err)
	}
	if count != 1 {
		t.Fatalf("improve-kandev workflow template rows = %d, want 1", count)
	}
}

// TestPostgresUpdateTaskSessionLastReadMessageIDMonotonic is the Postgres
// counterpart to TestUpdateTaskSessionLastReadMessageID (SQLite): the
// mark-read cursor guard used to compare SQLite's rowid pseudo-column, which
// does not exist on Postgres and would fail this query outright. It now
// compares (created_at, id), which is portable — this test proves the
// forward-advance and stale-rejection behavior against a real Postgres
// backend, not just SQLite. Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresUpdateTaskSessionLastReadMessageIDMonotonic(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	// seedForMsgTest uses SQLite's `INSERT OR IGNORE`, which Postgres
	// rejects outright — seed directly with plain INSERTs instead (safe
	// here since each test gets a freshly created, empty isolated schema).
	seedNow := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, '', 'test task', ?, ?)
	`), "task-pg-read", seedNow, seedNow); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-pg-read", TaskID: "task-pg-read", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), "turn-pg-read", "session-pg-read", "task-pg-read", seedNow, seedNow, seedNow); err != nil {
		t.Fatalf("seed turn: %v", err)
	}

	now := time.Now().UTC()
	insertAgentMsg(t, repo, "msg-pg-1", "session-pg-read", "turn-pg-read", "user", "hi", now)
	insertAgentMsg(t, repo, "msg-pg-2", "session-pg-read", "turn-pg-read", "agent", "hello", now.Add(time.Second))

	if err := repo.UpdateTaskSessionLastReadMessageID(ctx, "session-pg-read", "msg-pg-2"); err != nil {
		t.Fatalf("UpdateTaskSessionLastReadMessageID advance on postgres: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "session-pg-read")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.LastReadMessageID != "msg-pg-2" {
		t.Fatalf("session.LastReadMessageID = %q, want %q", session.LastReadMessageID, "msg-pg-2")
	}

	// A delayed/retried request for the older message must not regress the
	// cursor on Postgres either — silent no-op, not an error.
	if err := repo.UpdateTaskSessionLastReadMessageID(ctx, "session-pg-read", "msg-pg-1"); err != nil {
		t.Fatalf("UpdateTaskSessionLastReadMessageID stale update on postgres: %v", err)
	}
	session, err = repo.GetTaskSession(ctx, "session-pg-read")
	if err != nil {
		t.Fatalf("GetTaskSession after stale update: %v", err)
	}
	if session.LastReadMessageID != "msg-pg-2" {
		t.Fatalf("session.LastReadMessageID = %q, want unchanged %q after stale update", session.LastReadMessageID, "msg-pg-2")
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

func TestPostgresSetSessionMetadataKeyIfAbsentIsWriteOnce(t *testing.T) {
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
	`), "task-baseline-pg", "ws-baseline-pg", "Baseline", now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-baseline-pg", TaskID: "task-baseline-pg", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	stored, err := repo.SetSessionMetadataKeyIfAbsent(ctx, "session-baseline-pg", "baseline", map[string]string{"effort": "high"})
	if err != nil {
		t.Fatalf("first SetSessionMetadataKeyIfAbsent: %v", err)
	}
	if !stored {
		t.Fatal("first SetSessionMetadataKeyIfAbsent should store")
	}
	stored, err = repo.SetSessionMetadataKeyIfAbsent(ctx, "session-baseline-pg", "baseline", map[string]string{"effort": "low"})
	if err != nil {
		t.Fatalf("second SetSessionMetadataKeyIfAbsent: %v", err)
	}
	if stored {
		t.Fatal("second SetSessionMetadataKeyIfAbsent should not overwrite")
	}
	session, err := repo.GetTaskSession(ctx, "session-baseline-pg")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	baseline, ok := session.Metadata["baseline"].(map[string]interface{})
	if !ok || baseline["effort"] != "high" {
		t.Fatalf("baseline = %#v, want effort=high", session.Metadata["baseline"])
	}
}

func TestPostgresUpdateSessionContextWindowCountsStrictUsageDrops(t *testing.T) {
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
	`), "task-context-count-pg", "ws-context-count-pg", "Context count", now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-context-count-pg", TaskID: "task-context-count-pg", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	count, err := repo.UpdateSessionContextWindow(ctx, "session-context-count-pg", map[string]interface{}{
		"size": int64(200000), "used": int64(120000),
	})
	if err != nil || count != 0 {
		t.Fatalf("first context update = (%d, %v), want (0, nil)", count, err)
	}
	count, err = repo.UpdateSessionContextWindow(ctx, "session-context-count-pg", map[string]interface{}{
		"size": int64(200000), "used": int64(80000),
	})
	if err != nil || count != 1 {
		t.Fatalf("decreased context update = (%d, %v), want (1, nil)", count, err)
	}
	count, err = repo.UpdateSessionContextWindow(ctx, "session-context-count-pg", map[string]interface{}{
		"size": int64(200000), "used": int64(80000),
	})
	if err != nil || count != 1 {
		t.Fatalf("duplicate context update = (%d, %v), want (1, nil)", count, err)
	}
}

// TestPostgresNormalizeTaskWorktreeOwnershipIsNoOpOnFreshSchema proves the
// one-time cutover is a no-op on a database that already carries the final
// schema: the legacy table does not exist, so nothing is normalized and the
// orphaned session stays untouched (the startup heal
// healSessionTaskEnvironmentIDs owns that repair on final-schema databases).
func TestPostgresNormalizeTaskWorktreeOwnershipIsNoOpOnFreshSchema(t *testing.T) {
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

	if err := repo.normalizeTaskWorktreeOwnership(); err != nil {
		t.Fatalf("normalize task worktree ownership: %v", err)
	}

	var count int
	if err := db.Get(&count, db.Rebind(`
		SELECT COUNT(*) FROM task_environments WHERE task_id = ?
	`), "task-orphaned"); err != nil {
		t.Fatalf("count task environments: %v", err)
	}
	if count != 0 {
		t.Fatalf("task environment count = %d, want 0 (cutover is a no-op on the final schema)", count)
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

func TestPostgresRepositorySecretBindingsSchemaReplay(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}

	if _, err := db.Exec("DROP TABLE repository_secret_bindings"); err != nil {
		t.Fatalf("drop repository secret bindings table: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay repository secret bindings migration: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay repository secret bindings migration twice: %v", err)
	}

	ctx := context.Background()
	seedWorkspace(t, repo, "ws-postgres-secret-bindings")
	repository := &models.Repository{
		ID:          "repo-postgres-secret-bindings",
		WorkspaceID: "ws-postgres-secret-bindings",
		Name:        "postgres secrets",
	}
	bindings := []models.RepositorySecretBinding{{Key: "NPM_TOKEN", SecretID: "secret-pg-npm"}}
	if err := repo.CreateRepositoryWithSecretBindings(ctx, repository, bindings); err != nil {
		t.Fatalf("create repository with bindings after replay: %v", err)
	}
	got, err := repo.GetRepository(ctx, repository.ID)
	if err != nil {
		t.Fatalf("get repository after replay: %v", err)
	}
	if len(got.SecretBindings) != 1 {
		t.Fatalf("repository bindings = %+v, want one binding", got.SecretBindings)
	}
	gotBinding := got.SecretBindings[0]
	if gotBinding.RepositoryID != repository.ID || gotBinding.Key != bindings[0].Key || gotBinding.SecretID != bindings[0].SecretID {
		t.Fatalf("repository binding = %+v, want repository=%q key=%q secret=%q", gotBinding, repository.ID, bindings[0].Key, bindings[0].SecretID)
	}
	if gotBinding.CreatedAt.IsZero() || gotBinding.UpdatedAt.IsZero() {
		t.Fatalf("repository binding timestamps = %+v, want persisted timestamps", gotBinding)
	}
}

// openIsolatedPostgresMultiConn is like testutil.OpenIsolatedPostgres but
// supports a real multi-connection pool. OpenIsolatedPostgres scopes its
// isolated schema via a session-level `SET search_path` issued on one
// connection — fine for its single-connection pool, but a second pooled
// connection never sees that SET and falls back to the default "public"
// schema. Baking search_path into the DSN's libpq `options` param instead
// makes every new connection resolve unqualified table names against the
// isolated schema, so the pool can be sized for genuine concurrency tests.
func openIsolatedPostgresMultiConn(t *testing.T, dsn string, maxConns int) *sqlx.DB {
	t.Helper()
	schema := "kandev_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")

	setup, err := sqlx.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres (schema setup): %v", err)
	}
	if _, err := setup.Exec("CREATE SCHEMA " + schema); err != nil {
		_ = setup.Close()
		t.Fatalf("create postgres schema %s: %v", schema, err)
	}
	_ = setup.Close()
	// Register schema-drop cleanup immediately: any t.Fatalf between here and
	// the end of this function (e.g. sqlx.Open below failing) must still not
	// leak the schema on the Postgres test instance.
	t.Cleanup(func() {
		if cleanup, cerr := sqlx.Open("pgx", dsn); cerr == nil {
			_, _ = cleanup.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
			_ = cleanup.Close()
		}
	})

	// dsn is either a URL ("postgres://user:pass@host/db?sslmode=...") or a
	// libpq keyword/value string ("host=... port=... sslmode=disable" —
	// what CI's KANDEV_TEST_POSTGRES_DSN uses). Appending "?options=..." to
	// the latter corrupts the last keyword's value instead of adding a new
	// one, so the two forms need different separators.
	var scopedDSN string
	if strings.Contains(dsn, "://") {
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		scopedDSN = dsn + sep + "options=" + url.QueryEscape("-c search_path="+schema)
	} else {
		scopedDSN = dsn + " options='-c search_path=" + schema + "'"
	}
	db, err := sqlx.Open("pgx", scopedDSN)
	if err != nil {
		t.Fatalf("open postgres (scoped, %d conns): %v", maxConns, err)
	}
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestPostgresSetSessionPrimary_ConcurrentPromotionsLeaveExactlyOnePrimary
// exercises the race Greptile flagged on PR #1860: SetSessionPrimary demotes
// every other session for a task and promotes the target in one
// transaction, plus a `SELECT ... FOR UPDATE` row lock on the owning task
// so concurrent promotions on separate Postgres connections serialize
// instead of interleaving their demote/promote pairs. Uses
// openIsolatedPostgresMultiConn (not testutil.OpenIsolatedPostgres, which
// caps the pool at one connection) because genuine cross-connection
// concurrency is the whole point — a one-connection pool would trivially
// serialize even the pre-fix code and prove nothing.
func TestPostgresSetSessionPrimary_ConcurrentPromotionsLeaveExactlyOnePrimary(t *testing.T) {
	const concurrency = 8
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), concurrency)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-primary-race", "ws-primary-race", "Task primary race", now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	sessionIDs := make([]string, concurrency)
	for i := range sessionIDs {
		sessionIDs[i] = fmt.Sprintf("session-primary-race-%d", i)
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionIDs[i], TaskID: "task-primary-race", State: models.TaskSessionStateRunning,
		}); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", sessionIDs[i], err)
		}
	}

	var wg sync.WaitGroup
	errs := make([]error, concurrency)
	for i, sessionID := range sessionIDs {
		wg.Add(1)
		go func(i int, sessionID string) {
			defer wg.Done()
			errs[i] = repo.SetSessionPrimary(ctx, sessionID)
		}(i, sessionID)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("SetSessionPrimary(%s) concurrent call failed: %v", sessionIDs[i], err)
		}
	}

	var primaryCount int
	if err := db.Get(&primaryCount, db.Rebind(
		`SELECT COUNT(*) FROM task_sessions WHERE task_id = ? AND is_primary = 1`,
	), "task-primary-race"); err != nil {
		t.Fatalf("count primary sessions: %v", err)
	}
	if primaryCount != 1 {
		t.Errorf("expected exactly 1 primary session after %d concurrent promotions, got %d — FOR UPDATE lock not serializing across connections", concurrency, primaryCount)
	}
}

// TestPostgresClearRecoveredAgentErrors is the Postgres counterpart to
// TestClearRecoveredAgentErrorsBackfill. clearRecoveredAgentErrors is built from
// three dialect-sensitive helpers (jsonColumn, jsonText/timestamp*,
// jsonRemoveKey), so ADR 0027 asks for env-gated Postgres behavior coverage —
// schema replay would not exercise the jsonb operators at all.
//
// migrate.Apply swallows SQL errors, so the assertions below are the only proof
// the statements ran: a dialect mistake leaves the recovered row untouched
// rather than returning an error. Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresClearRecoveredAgentErrors(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	occurredAt := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-pg-recovered", "ws-pg-recovered", "Task", occurredAt, occurredAt); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	// recovered: agent output after the failure. current: none since.
	// blank/nulled: rows the repository wrote as "no metadata" — both dialects
	// raise on parsing those, and the statements scan every row before
	// filtering, so an unguarded cast aborts the migration for everyone.
	for _, sessionID := range []string{"pg-recovered", "pg-current", "pg-blank", "pg-nulled"} {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionID, TaskID: "task-pg-recovered", State: models.TaskSessionStateWaitingForInput,
		}); err != nil {
			t.Fatalf("CreateTaskSession %s: %v", sessionID, err)
		}
	}
	// Seeded with plain SQL on purpose: SetSessionMetadataKey is SQLite-only
	// (json_set/json()), so using it here would fail on the seed rather than
	// exercise the migration under test.
	lastAgentError := `{"last_agent_error":{"message":"agent crashed","occurred_at":"` +
		occurredAt.Format(time.RFC3339Nano) + `"}}`
	for _, sessionID := range []string{"pg-recovered", "pg-current"} {
		if _, err := db.Exec(db.Rebind(
			`UPDATE task_sessions SET metadata = ? WHERE id = ?`), lastAgentError, sessionID); err != nil {
			t.Fatalf("seed last agent error on %s: %v", sessionID, err)
		}
	}
	for value, sessionID := range map[string]string{"": "pg-blank", "null": "pg-nulled"} {
		if _, err := db.Exec(db.Rebind(
			`UPDATE task_sessions SET metadata = ? WHERE id = ?`), value, sessionID); err != nil {
			t.Fatalf("seed %q metadata: %v", value, err)
		}
	}

	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), "pg-turn", "pg-recovered", "task-pg-recovered", occurredAt, occurredAt, occurredAt); err != nil {
		t.Fatalf("seed turn: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, content, type, metadata, created_at)
		VALUES (?, ?, '', ?, 'agent', 'back on track', 'message', '{}', ?)
	`), "pg-msg", "pg-recovered", "pg-turn", occurredAt.Add(time.Hour)); err != nil {
		t.Fatalf("seed agent message: %v", err)
	}

	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_status_summaries (task_id, workspace_id, revision, summary, updated_at)
		VALUES (?, ?, 4, ?, ?)
	`), "task-pg-recovered", "ws-pg-recovered",
		`{"active_error":{"session_id":"pg-recovered","stamp":"s","preview":"agent crashed"},"pending_action":"permission"}`,
		occurredAt); err != nil {
		t.Fatalf("seed status summary: %v", err)
	}

	if err := repo.clearRecoveredAgentErrors(); err != nil {
		t.Fatalf("clearRecoveredAgentErrors: %v", err)
	}

	hasError := func(sessionID string) bool {
		session, err := repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			t.Fatalf("get session %s: %v", sessionID, err)
		}
		_, ok := models.LoadLastAgentError(session.Metadata)
		return ok
	}
	if hasError("pg-recovered") {
		t.Fatal("a failure the agent recovered from must be cleared on Postgres")
	}
	if !hasError("pg-current") {
		t.Fatal("a failure with no successful work after it must be left alone")
	}

	var summary string
	if err := db.Get(&summary, db.Rebind(
		`SELECT summary FROM task_status_summaries WHERE task_id = ?`), "task-pg-recovered"); err != nil {
		t.Fatalf("read status summary: %v", err)
	}
	if strings.Contains(summary, "active_error") {
		t.Fatalf("summary = %s, want the cached error cleared", summary)
	}
	if !strings.Contains(summary, "permission") {
		t.Fatalf("summary = %s, want the rest of the projection preserved", summary)
	}
}
