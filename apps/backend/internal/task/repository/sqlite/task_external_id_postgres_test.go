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

// TestPostgresExternalIDCaseSensitiveDespiteDatabaseCollation covers the
// column-level COLLATE "C" requirement: even when the database's default
// collation is case-insensitive (as configured for this test schema),
// "ext-1" and "EXT-1" must remain distinct identities. An unqualified TEXT
// column would silently inherit the database default and let this
// scenario collide.
func TestPostgresExternalIDCaseSensitiveDespiteDatabaseCollation(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
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
