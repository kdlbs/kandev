package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

// TestPluginEnvelopeWriteTakesTheTaskRowBeforeTheInventoryLockPostgres holds
// the owner task row, starts a claim checkpoint, and requires the checkpoint to
// wait on the task row without holding the environment inventory lock. Ready
// publication takes the task row first, so a checkpoint that took the inventory
// lock first would deadlock against it.
func TestPluginEnvelopeWriteTakesTheTaskRowBeforeTheInventoryLockPostgres(t *testing.T) {
	repo, second := newDescriptorPostgresRepos(t)
	seedProvisionedForClaims(t, repo, seedDescriptorEnvironmentPostgres)
	claim := acquireClaimForDescriptorEnvironment(t, repo, "op-1:cleanup")
	_, revision := storedEnvelope(t, repo)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	holder, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin holder: %v", err)
	}
	defer func() { _ = holder.Rollback() }()
	var locked string
	if err := holder.QueryRowContext(ctx, `SELECT id FROM tasks WHERE id = 'task-descriptor-1' FOR UPDATE`).Scan(&locked); err != nil {
		t.Fatalf("lock owner task row: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		running := descriptorFixture(1, "install-a", "cleanup_progress", "/srv/work", "worker-1")
		running.ExpectedPluginExecutorRevision = revision
		_, err := second.CheckpointPluginExecutorClaimInventory(ctx, running, models.PluginExecutorClaimCheckpoint{
			Claim: claim, Kind: models.PluginClaimKindCleanup, Sequence: 1, Digest: "d",
		})
		done <- err
	}()

	waitForTaskRowWaiter(ctx, t, holder)

	var free bool
	if err := holder.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1, 0))`,
		pluginInventoryLockNamespace+"environment-descriptor-1").Scan(&free); err != nil {
		t.Fatalf("probe inventory lock: %v", err)
	}
	if !free {
		t.Fatal("a checkpoint waiting for the owner task row already holds the environment inventory lock")
	}
	if err := holder.Rollback(); err != nil {
		t.Fatalf("release holder: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("checkpoint after release: %v", err)
	}
}

// waitForTaskRowWaiter blocks until another backend is waiting on a
// tasks-row FOR UPDATE, so the probe that follows cannot pass vacuously.
func waitForTaskRowWaiter(ctx context.Context, t *testing.T, probe *sqlx.Tx) {
	t.Helper()
	for {
		var waiting int
		if err := probe.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE wait_event_type = 'Lock' AND query LIKE '%FROM tasks WHERE id%FOR UPDATE%'`).Scan(&waiting); err != nil {
			t.Fatalf("inspect lock waiters: %v", err)
		}
		if waiting > 0 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("the checkpoint never waited on the owner task row")
		case <-time.After(20 * time.Millisecond):
		}
	}
}
