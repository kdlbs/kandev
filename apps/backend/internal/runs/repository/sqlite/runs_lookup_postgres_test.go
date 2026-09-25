package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresGetClaimedRunByTaskID is the PostgreSQL twin of
// TestGetClaimedRunByTaskID_MatchesThePayloadTaskAndPrefersTheLatestClaim
// (Review round 16, RR16-F1): GetClaimedRunByTaskID read task_id out of the
// runs payload with a literal json_extract(...), a SQLite-only function
// that is a syntax error on Postgres. The two office/service.Service
// callers introduced by this branch (TaskBoundaryCarrierMetadata,
// TaskBoundaryCarrierForRunQueue) both swallow that error and silently
// fall back to the task's frozen carrier, which cannot advance causation
// depth — so the depth ceiling this whole card exists to add was inert on
// Postgres. This exercises the same payload-match/claimed-at-ordering/
// unknown-task cases this method's SQLite test covers, against a real
// Postgres backend, so a dialect regression here fails loudly instead of
// silently defeating the depth ceiling. Skips unless KANDEV_TEST_POSTGRES_DSN
// is set.
func TestPostgresGetClaimedRunByTaskID(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	// runs is created by the task repository's schema init, mirroring
	// production boot order (see failure_postgres_test.go).
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	officeRepo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}
	repo := officeRepo.RunsRepository()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	older := seedTaskRun(t, repo, "pg-older-claim", "a1", "t1", "queued")
	setStatus(t, repo, older.ID, "claimed", timePtr(base), nil)
	newer := seedTaskRun(t, repo, "pg-newer-claim", "a2", "t1", "queued")
	setStatus(t, repo, newer.ID, "claimed", timePtr(base.Add(time.Hour)), nil)
	// Same task but still queued, and another task's claimed run — neither
	// should match.
	seedTaskRun(t, repo, "pg-queued-same-task", "a3", "t1", "queued")
	otherTask := seedTaskRun(t, repo, "pg-other-task", "a4", "t2", "queued")
	setStatus(t, repo, otherTask.ID, "claimed", timePtr(base.Add(2*time.Hour)), nil)

	got, err := repo.GetClaimedRunByTaskID(ctx, "t1")
	if err != nil {
		t.Fatalf("get claimed run: %v", err)
	}
	if got.ID != newer.ID {
		t.Errorf("claimed run = %q, want %q (most recent claim for t1)", got.ID, newer.ID)
	}

	if _, err := repo.GetClaimedRunByTaskID(ctx, "t-unknown"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("unknown task err = %v, want sql.ErrNoRows", err)
	}
}
