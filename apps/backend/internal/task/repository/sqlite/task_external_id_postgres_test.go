package sqlite

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresTaskPriorityMigrationConvertsLegacyInteger verifies that a
// PostgreSQL repository can create tasks after the legacy INTEGER priority
// column is migrated to the string-valued task model. The migration runs from
// the task repository itself, so tests that use NewWithDB do not depend on the
// office repository being initialized first.
func TestPostgresTaskPriorityMigrationConvertsLegacyInteger(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}

	var dataType, defaultValue, nullable string
	err := db.QueryRowContext(context.Background(), `
		SELECT data_type, column_default, is_nullable
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'tasks'
		  AND column_name = 'priority'
	`).Scan(&dataType, &defaultValue, &nullable)
	if err != nil {
		t.Fatalf("inspect tasks.priority: %v", err)
	}
	if dataType != "text" {
		t.Fatalf("tasks.priority data type = %q, want text", dataType)
	}
	if defaultValue == "" || !strings.Contains(defaultValue, "medium") {
		t.Fatalf("tasks.priority default = %q, want medium", defaultValue)
	}
	if nullable != "NO" {
		t.Fatalf("tasks.priority nullable = %q, want NO", nullable)
	}
}

// TestPostgresExternalIDConcurrentInsertYieldsExactlyOneWinner is the
// PostgreSQL twin of the SQLite concurrency coverage
// (docs/specs/tasks/external-id-idempotency/spec.md, "Uniqueness and
// concurrency"): the SQLite path passing is not evidence for Postgres, since
// the two drivers classify unique-violation errors completely differently
// (typed pgconn.PgError vs. SQLite driver string-matching). Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
//
// The task repository migrates the legacy PostgreSQL INTEGER priority column
// to the string-valued task model before these tests create any rows.
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
