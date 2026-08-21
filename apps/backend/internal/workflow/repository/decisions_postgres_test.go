package repository

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/testutil"
	"github.com/kandev/kandev/internal/workflow/models"
)

// openIsolatedPostgresMultiConn is like testutil.OpenIsolatedPostgres but
// supports a real multi-connection pool. OpenIsolatedPostgres scopes its
// isolated schema via a session-level `SET search_path` issued on one
// connection — fine for its single-connection pool, but a second pooled
// connection never sees that SET and falls back to the default "public"
// schema. Baking search_path into the DSN's libpq `options` param instead
// makes every new connection resolve unqualified table names against the
// isolated schema, so the pool can be sized for genuine concurrency tests.
// Mirrors internal/task/repository/sqlite's helper of the same name.
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
	t.Cleanup(func() {
		if cleanup, cerr := sqlx.Open("pgx", dsn); cerr == nil {
			_, _ = cleanup.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
			_ = cleanup.Close()
		}
	})

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

// setupPostgresDecisionTestRepo opens an isolated multi-connection Postgres
// pool, fakes the workflows/task_sessions tables normally owned by
// internal/task/repository/sqlite (as sqlite_test.go's setupTestRepoWithDB
// does for SQLite), and seeds the "wf-test" workflow row newPhase2TestStep
// expects.
func setupPostgresDecisionTestRepo(t *testing.T, dsn string, maxConns int) *Repository {
	t.Helper()
	db := openIsolatedPostgresMultiConn(t, dsn, maxConns)

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS workflows (
		id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '',
		workflow_template_id TEXT DEFAULT '', name TEXT NOT NULL,
		description TEXT DEFAULT '', created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL
	)`); err != nil {
		t.Fatalf("create workflows table: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS task_sessions (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create task_sessions table: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO workflows (id, workspace_id, name, created_at, updated_at) VALUES (?, '', 'Test', ?, ?)
	`), "wf-test", now, now); err != nil {
		t.Fatalf("seed test workflow: %v", err)
	}

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	return repo
}

// TestPostgresRecordStepDecision_ConcurrentSameDeciderRace is the AC-27/29
// PostgreSQL counterpart proving decisionActiveDeciderIndexName actually
// closes the race: under READ COMMITTED, several concurrent
// RecordStepDecision calls for the same (task, step, decider, role) can each
// run the "supersede prior" UPDATE and see zero matching rows before any of
// them commits, then all INSERT — without the unique index this leaves more
// than one active row for the same decider identity. Uses
// openIsolatedPostgresMultiConn (not testutil.OpenIsolatedPostgres, capped
// at one connection) because genuine cross-connection concurrency is the
// whole point. Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresRecordStepDecision_ConcurrentSameDeciderRace(t *testing.T) {
	const concurrency = 8
	repo := setupPostgresDecisionTestRepo(t, testutil.PostgresDSNFromEnv(t), concurrency)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Review")

	const taskID = "task-decision-race"
	start := make(chan struct{})
	results := make(chan error, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- repo.RecordStepDecision(ctx, &models.WorkflowStepDecision{
				TaskID: taskID, StepID: step.ID, ParticipantID: "p1",
				Decision: "approved", DeciderType: "agent", DeciderID: "alice", Role: "reviewer",
			})
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	for err := range results {
		if err != nil {
			t.Fatalf("RecordStepDecision: %v", err)
		}
	}

	all, err := repo.ListStepDecisions(ctx, taskID, step.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != concurrency {
		t.Fatalf("expected %d total rows (every concurrent write landed), got %d", concurrency, len(all))
	}
	activeCount := 0
	for _, d := range all {
		if d.SupersededAt == nil {
			activeCount++
		}
	}
	if activeCount != 1 {
		t.Fatalf("expected exactly 1 active decision for decider alice/reviewer after the race, got %d", activeCount)
	}
}
