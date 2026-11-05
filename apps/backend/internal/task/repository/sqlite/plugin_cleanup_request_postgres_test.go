package sqlite

import (
	"context"
	"testing"
	"time"
)

func TestCleanupRequestIsRecordableUnderARecoveryClaimPostgres(t *testing.T) {
	repo, _ := newDescriptorPostgresRepos(t)
	exerciseCleanupRequestUnderARecoveryClaim(t, repo, seedDescriptorEnvironmentPostgres)
}

// TestCleanupRequestTakesTheTaskRowBeforeTheExecutionRowPostgres holds the
// owner task row, starts a cleanup request, and requires the request to wait on
// the task row without holding the execution row. A claim checkpoint takes the
// task row first and the execution row later, so a request that held the
// execution row while waiting would deadlock against it.
func TestCleanupRequestTakesTheTaskRowBeforeTheExecutionRowPostgres(t *testing.T) {
	repo, second := newDescriptorPostgresRepos(t)
	seedProvisionedForClaims(t, repo, seedDescriptorEnvironmentPostgres)

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
		done <- second.RecordPluginExecutorCleanupRequested(ctx, "session-descriptor-1", "execution-descriptor-1")
	}()

	waitForTaskRowWaiter(ctx, t, holder)

	if err := holder.QueryRowContext(ctx,
		`SELECT session_id FROM executors_running WHERE session_id = 'session-descriptor-1' FOR UPDATE NOWAIT`).Scan(&locked); err != nil {
		t.Fatalf("a request waiting for the owner task row already holds the execution row: %v", err)
	}
	if err := holder.Rollback(); err != nil {
		t.Fatalf("release holder: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("request after release: %v", err)
	}
}
