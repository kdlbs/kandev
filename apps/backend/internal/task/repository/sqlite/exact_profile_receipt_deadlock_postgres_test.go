package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestPostgresConcurrentExactProfileReceiptLookupsDoNotDeadlock(t *testing.T) {
	repoA, repoB, observer := newTaskPostgresRepoPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	assignment := seedPostgresExactProfileAttempt(t, repoA, "concurrent-receipt-read")
	binding := &models.ExactProfileLaunchAttemptBinding{
		TaskID: assignment.TaskID, SessionID: "exact-pg-concurrent-read-session",
		ExecutionID: "exact-pg-concurrent-read-execution", AttemptID: "exact-pg-concurrent-read-attempt",
		SessionIncarnationID: "exact-pg-concurrent-read-incarnation",
		AgentProfileID:       assignment.AgentProfileID, Model: "model-exact",
		ProfileRevision: assignment.ProfileRevision, Generation: assignment.Generation,
	}
	if err := repoA.CreateTaskSession(ctx, &models.TaskSession{ID: binding.SessionID, TaskID: binding.TaskID, QueueIncarnationID: binding.SessionIncarnationID, State: models.TaskSessionStateCreated}); err != nil {
		t.Fatal(err)
	}
	if changed, err := repoA.BindExactProfileLaunchAttempt(ctx, binding); err != nil || !changed {
		t.Fatalf("bind = (%v, %v)", changed, err)
	}
	receipt := &models.ExactProfileLaunchReceipt{
		TaskID: binding.TaskID, SessionID: binding.SessionID, AgentProfileID: binding.AgentProfileID,
		ProfileRevision: binding.ProfileRevision, Generation: binding.Generation,
		Model: binding.Model, Outcome: models.ExactProfileLaunchOutcomeApplied,
	}
	if changed, err := repoA.RecordExactProfileLaunchReceiptForAttempt(ctx, binding, receipt); err != nil || !changed {
		t.Fatalf("record receipt = (%v, %v)", changed, err)
	}

	// Before the lock-order fix, both lookups acquired a share lock on the
	// assignment before waiting on this session row. Releasing it made them
	// request an update lock on the assignment concurrently.
	observer.SetMaxOpenConns(2)
	blocker, err := observer.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback() }()
	var lockedSession string
	if err := blocker.GetContext(ctx, &lockedSession, `SELECT id FROM task_sessions WHERE id = $1 FOR UPDATE`, binding.SessionID); err != nil {
		t.Fatal(err)
	}

	pidA := pgBackendPID(t, repoA.db)
	pidB := pgBackendPID(t, repoB.db)
	type result struct {
		receipt *models.ExactProfileLaunchReceipt
		err     error
	}
	results := []chan result{make(chan result, 1), make(chan result, 1)}
	dones := []chan struct{}{make(chan struct{}), make(chan struct{})}
	for i, repo := range []*Repository{repoA, repoB} {
		go func(i int, repo *Repository) {
			defer close(dones[i])
			loaded, err := repo.GetExactProfileLaunchReceipt(ctx, binding.TaskID, binding.SessionID)
			results[i] <- result{receipt: loaded, err: err}
		}(i, repo)
	}
	if err := waitForPostgresLock(ctx, observer, pidA, dones[0]); err != nil {
		t.Fatal(err)
	}
	if err := waitForPostgresLock(ctx, observer, pidB, dones[1]); err != nil {
		t.Fatal(err)
	}
	if err := blocker.Commit(); err != nil {
		t.Fatal(err)
	}

	loaded := make([]result, len(results))
	for i := range results {
		select {
		case loaded[i] = <-results[i]:
		case <-ctx.Done():
			t.Fatalf("lookup %d did not finish: %v", i, ctx.Err())
		}
	}
	for i, got := range loaded {
		if got.err != nil || got.receipt == nil || got.receipt.Outcome != receipt.Outcome {
			t.Errorf("lookup %d = %#v, %v", i, got.receipt, got.err)
		}
	}
}
