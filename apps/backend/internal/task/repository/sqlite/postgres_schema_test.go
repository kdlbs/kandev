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
