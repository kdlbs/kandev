package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresExecutorReachabilityCheckedAtOrdering guards the SQLite-only
// risk the design calls out: TIMESTAMP takes NUMERIC affinity on SQLite and
// compares the driver's text encoding lexically, so a whole-second value can
// sort after a later sub-second one unless writes are normalized to UTC and
// bind time.Time rather than a pre-formatted string. Postgres compares
// temporally and is unaffected either way, so this test alone would pass
// even with the bug — it exists to prove the fix doesn't regress on
// Postgres, not to catch the bug (that's the SQLite test's job).
func TestPostgresExecutorReachabilityCheckedAtOrdering(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	executor := &models.Executor{ID: "exec-reachability-pg", Name: "ssh-host", Type: models.ExecutorTypeSSH, Status: models.ExecutorStatusActive}
	if err := repo.CreateExecutor(ctx, executor); err != nil {
		t.Fatalf("create executor: %v", err)
	}

	older := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 1, 1, 12, 0, 0, 500_000_000, time.UTC)

	if err := repo.UpsertExecutorReachability(ctx, models.ExecutorReachabilityObservation{
		ExecutorID: executor.ID, SeenUpdatedAt: executor.UpdatedAt,
		InitialState: models.ExecutorReachabilityStateReachable,
		Reason:       "", Host: "10.0.0.2", CheckedAt: newer, FailureThreshold: 3,
	}); err != nil {
		t.Fatalf("upsert newer: %v", err)
	}
	if err := repo.UpsertExecutorReachability(ctx, models.ExecutorReachabilityObservation{
		ExecutorID: executor.ID, SeenUpdatedAt: executor.UpdatedAt,
		InitialState: models.ExecutorReachabilityStateUnreachable, InitialFailures: 1,
		Reason: models.ExecutorReachabilityReasonNetwork, Message: "connection refused",
		Host: "10.0.0.1", CheckedAt: older, FailureThreshold: 3,
	}); err != nil {
		t.Fatalf("upsert older: %v", err)
	}

	got, err := repo.GetExecutorReachability(ctx, executor.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Host != "10.0.0.2" || got.State != models.ExecutorReachabilityStateReachable {
		t.Fatalf("record = %+v, want the newer write to survive", got)
	}
}
