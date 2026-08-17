package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresClarificationResolutionConcurrentClaimYieldsExactlyOneWinner
// proves the M8 claim on PostgreSQL specifically: two concurrent inserts for
// the same pending_id must yield exactly one winner via the
// ON CONFLICT (pending_id) DO NOTHING race, not the SQLite driver's own
// serialization. The SQLite-backed TestInsertClarificationResolution_SecondCallerLoses
// is not evidence for this path (spec Verification notes: "Schema replay
// alone is insufficient").
func TestPostgresClarificationResolutionConcurrentClaimYieldsExactlyOneWinner(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-pg-race", "sess-pg-race", "turn-pg-race")

	var wg sync.WaitGroup
	start := make(chan struct{})
	claimed := make([]bool, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res := newTestClarificationResolution("pending-pg-race", "sess-pg-race", "task-pg-race")
			if i == 1 {
				res.Status = models.ClarificationResolutionStatusRejected
			}
			claimed[i], _, errs[i] = repo.InsertClarificationResolution(ctx, res)
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("insert[%d]: unexpected error: %v", i, err)
		}
	}

	winners := 0
	for _, c := range claimed {
		if c {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("winners = %d, want exactly 1 (claimed=%v)", winners, claimed)
	}

	got, err := repo.GetClarificationResolution(ctx, "pending-pg-race")
	if err != nil {
		t.Fatalf("get after race: %v", err)
	}
	if got.Status != models.ClarificationResolutionStatusAnswered {
		t.Fatalf("got.Status = %q, want answered (the first-registered status must win, not be overwritten)", got.Status)
	}
}

// TestPostgresClarificationResolutionSessionMissingIsForeignKeyViolation
// proves M8a on the Postgres-specific branch of isForeignKeyViolation: a
// typed pgconn.PgError with code 23503, not the SQLite substring match. This
// branch has zero coverage from the SQLite-only test run.
func TestPostgresClarificationResolutionSessionMissingIsForeignKeyViolation(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	res := newTestClarificationResolution("pending-pg-missing", "sess-does-not-exist", "task-pg-missing")
	claimed, stored, err := repo.InsertClarificationResolution(ctx, res)
	if !errors.Is(err, ErrClarificationSessionMissing) {
		t.Fatalf("InsertClarificationResolution error = %v, want ErrClarificationSessionMissing", err)
	}
	if claimed || stored != nil {
		t.Fatalf("expected no claim and no stored row, got claimed=%v stored=%+v", claimed, stored)
	}

	if _, err := repo.GetClarificationResolution(ctx, "pending-pg-missing"); !errors.Is(err, ErrClarificationResolutionNotFound) {
		t.Fatalf("GetClarificationResolution after failed claim = %v, want ErrClarificationResolutionNotFound", err)
	}
}
