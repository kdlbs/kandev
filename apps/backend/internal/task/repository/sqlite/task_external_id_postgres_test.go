package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresExternalIDConcurrentInsertYieldsExactlyOneWinner is the
// PostgreSQL twin of the SQLite concurrency coverage
// (docs/specs/tasks/external-id-idempotency/spec.md, "Uniqueness and
// concurrency"): the SQLite path passing is not evidence for Postgres, since
// the two drivers classify unique-violation errors completely differently
// (typed pgconn.PgError vs. SQLite driver string-matching). Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
//
// KNOWN CI FAILURE, pre-existing and out of scope for this feature: both
// tests in this file call repo.CreateTask, which — via
// prepareTaskForCreate's Priority default — always inserts a non-empty
// Priority *string*. On a real Postgres install tasks.priority is still
// INTEGER: migrateTaskPriorityToText (internal/office/repository/sqlite)
// converts it to TEXT on SQLite only, and its detection query silently
// no-ops on Postgres (sqlite_master doesn't exist there), so the migration
// never runs. Any non-numeric Priority string then fails with
// "invalid input syntax for type integer". Reproduced independently of
// external_id and of this branch with a minimal repo.CreateTask call
// against origin/main. Every other Postgres-gated test in this repo avoids
// this by seeding tasks with raw SQL instead of repo.CreateTask; these two
// can't do that without defeating what they test. Not fixed here — a real
// fix is a Postgres priority-column migration, a different subsystem and a
// schema change, out of scope for task external_id idempotency. The
// "Backend Postgres" (postgres-boot) CI job is not part of this repo's
// required merge gate (.github/workflows/backend-tests.yml: the `test`
// aggregate job depends only on `static_checks` and `test_shards`).
func TestPostgresExternalIDConcurrentInsertYieldsExactlyOneWinner(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-pg-race", Name: "ws-pg-race"}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			task := &models.Task{WorkspaceID: "ws-pg-race", Title: "Race", ExternalID: "ext-pg-race"}
			errs[i] = repo.CreateTask(ctx, task)
		}()
	}
	close(start)
	wg.Wait()

	winners, conflicts := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, ErrExternalIDConflict):
			conflicts++
		default:
			t.Fatalf("unexpected error (want nil or ErrExternalIDConflict): %v", err)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("winners=%d conflicts=%d, want exactly one of each", winners, conflicts)
	}

	byExternal, err := repo.GetTaskByExternalID(ctx, "ws-pg-race", "ext-pg-race")
	if err != nil {
		t.Fatalf("get task by external id: %v", err)
	}
	if byExternal == nil {
		t.Fatal("expected the winner's task to be findable by external_id")
	}
}

// TestPostgresExternalIDColumnUsesDeterministicCollation covers the
// column-level COLLATE "C" requirement directly: the migration
// (base_migrations.go: `ALTER TABLE tasks ADD COLUMN external_id TEXT
// COLLATE "C"`) must actually take effect, independent of whatever the test
// database's own default collation happens to be. A behavioral
// upper-vs-lower-case probe alone is not diagnostic here — a typical
// Postgres install already treats "ext-1" and "EXT-1" as distinct even on an
// unqualified TEXT column, since collation affects sorting/equality-class
// membership under ICU-style rules, not plain byte comparison — so that
// probe would pass identically whether or not the override was ever applied.
// Asserting the catalog directly is the only way to pin that the override
// took effect.
func TestPostgresExternalIDColumnUsesDeterministicCollation(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	var collation string
	err = db.QueryRowContext(ctx, `
		SELECT co.collname
		FROM pg_attribute a
		JOIN pg_collation co ON a.attcollation = co.oid
		WHERE a.attrelid = 'tasks'::regclass AND a.attname = 'external_id'
	`).Scan(&collation)
	if err != nil {
		t.Fatalf("query external_id column collation: %v", err)
	}
	if collation != "C" {
		t.Fatalf("tasks.external_id collation = %q, want \"C\" — an unqualified TEXT column would silently inherit the database default, which may be case-insensitive or nondeterministic", collation)
	}

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-pg-case", Name: "ws-pg-case"}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	lower := &models.Task{WorkspaceID: "ws-pg-case", Title: "Lower", ExternalID: "ext-1"}
	if err := repo.CreateTask(ctx, lower); err != nil {
		t.Fatalf("create lower-case task: %v", err)
	}
	upper := &models.Task{WorkspaceID: "ws-pg-case", Title: "Upper", ExternalID: "EXT-1"}
	if err := repo.CreateTask(ctx, upper); err != nil {
		t.Fatalf("create upper-case task: %v, want success — case-sensitive identity", err)
	}
	if lower.ID == upper.ID {
		t.Fatal("expected two distinct tasks for ext-1 and EXT-1")
	}

	if _, err := repo.GetTaskByExternalID(ctx, "ws-pg-case", "ext-1"); err != nil {
		t.Fatalf("lookup ext-1: %v", err)
	}
	if _, err := repo.GetTaskByExternalID(ctx, "ws-pg-case", "EXT-1"); err != nil {
		t.Fatalf("lookup EXT-1: %v", err)
	}
}
