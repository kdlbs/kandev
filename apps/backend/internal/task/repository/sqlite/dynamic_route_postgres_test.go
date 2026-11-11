package sqlite

import (
	"context"
	"testing"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresCircuitDefaultsRestore(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init Postgres repository: %v", err)
	}
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newRegistry := func() *dynamicruntime.CircuitRegistry {
		return dynamicruntime.NewCircuitRegistry(
			dynamicruntime.WithCircuitPersistence(repo),
			dynamicruntime.WithCircuitClock(func() time.Time { return now }),
		)
	}
	registry := newRegistry()
	registry.OpenFailure("credential:restore", dynamicruntime.FailureOpen{
		Code: routingerr.CodeRateLimited,
		Now:  now,
	})
	registry.OpenFailure("credential:restore", dynamicruntime.FailureOpen{
		Code: routingerr.CodeRateLimited,
		Now:  now.Add(time.Minute),
	})

	now = now.Add(6 * time.Minute)
	restored := newRegistry()
	if err := restored.Restore(ctx); err != nil {
		t.Fatalf("restore Postgres circuits: %v", err)
	}
	snapshots := restored.Snapshots()
	if len(snapshots) != 1 || snapshots[0].Failures != 2 || !snapshots[0].Until.Equal(now) {
		t.Fatalf("restored Postgres circuit = %+v, want count 2 and original deadline %v", snapshots, now)
	}

	restored.OpenFailure("credential:restore", dynamicruntime.FailureOpen{
		Code: routingerr.CodeRateLimited,
		Now:  now,
	})
	snapshots = restored.Snapshots()
	if len(snapshots) != 1 || snapshots[0].Failures != 3 || !snapshots[0].Until.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("next Postgres failure did not continue ladder: %+v", snapshots)
	}
}
