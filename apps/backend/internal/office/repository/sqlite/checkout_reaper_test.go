package sqlite_test

import (
	"context"
	"testing"
	"time"
)

// TestReapStaleCheckouts_ReapsWithNoInFlightRun is the regression test for
// the checkout-leak backstop: a task whose checkout is older than the
// threshold and has no queued/claimed run behind it must be cleared, since
// nothing else will ever release it (the run that acquired it may have
// crashed before its terminal event fired, or the release call itself may
// have failed).
func TestReapStaleCheckouts_ReapsWithNoInFlightRun(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "task-reap-1", "ws-1", "Stale checkout", "", "")
	if _, err := repo.ExecRaw(ctx, `
		UPDATE tasks SET checkout_agent_id = 'agent-gone', checkout_at = datetime('now', '-1 hour')
		WHERE id = 'task-reap-1'
	`); err != nil {
		t.Fatalf("seed stale checkout: %v", err)
	}

	count, err := repo.ReapStaleCheckouts(ctx, time.Now().UTC().Add(-30*time.Minute))
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if count != 1 {
		t.Fatalf("reaped count = %d, want 1", count)
	}

	// Cleared checkouts accept a new agent.
	acquired, err := repo.CheckoutTask(ctx, "task-reap-1", "agent-new")
	if err != nil {
		t.Fatalf("checkout after reap: %v", err)
	}
	if !acquired {
		t.Fatal("expected checkout to be reaped, but a new agent still cannot acquire it")
	}
}

// TestReapStaleCheckouts_SkipsTaskWithInFlightRun proves the reaper does
// not clear a checkout that a queued/claimed run still legitimately holds
// — only a checkout with nothing behind it is a leak.
func TestReapStaleCheckouts_SkipsTaskWithInFlightRun(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "task-reap-2", "ws-1", "Stale checkout, live run", "", "")
	if _, err := repo.ExecRaw(ctx, `
		UPDATE tasks SET checkout_agent_id = 'agent-active', checkout_at = datetime('now', '-1 hour')
		WHERE id = 'task-reap-2'
	`); err != nil {
		t.Fatalf("seed stale checkout: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, requested_at
		) VALUES (
			'run-reap-2', 'agent-active', 'task_assigned', '{"task_id":"task-reap-2"}',
			'claimed', 1, '{}', CURRENT_TIMESTAMP
		)
	`); err != nil {
		t.Fatalf("seed claimed run: %v", err)
	}

	count, err := repo.ReapStaleCheckouts(ctx, time.Now().UTC().Add(-30*time.Minute))
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if count != 0 {
		t.Fatalf("reaped count = %d, want 0 (task has an in-flight run)", count)
	}

	// The checkout must still be held by agent-active — a different agent
	// cannot acquire it.
	acquired, err := repo.CheckoutTask(ctx, "task-reap-2", "agent-new")
	if err != nil {
		t.Fatalf("checkout after reap: %v", err)
	}
	if acquired {
		t.Fatal("expected checkout to remain held (in-flight run), but a new agent acquired it")
	}
}

// TestReapStaleCheckouts_UsesExactCheckoutRun proves that a queued run for
// the same agent cannot keep a stale checkout alive after the run that owns
// the checkout has finished. Reaping follows checkout_run_id, not the
// holder's agent identity.
func TestReapStaleCheckouts_UsesExactCheckoutRun(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "task-reap-run-id", "ws-1", "Run-owned stale checkout", "", "")
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, requested_at
		) VALUES
			('run-finished-owner', 'agent-owner', 'task_assigned', '{"task_id":"task-reap-run-id"}',
			 'finished', 1, '{}', CURRENT_TIMESTAMP),
			('run-queued-successor', 'agent-owner', 'task_assigned', '{"task_id":"task-reap-run-id"}',
			 'queued', 1, '{}', CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed runs: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		UPDATE tasks
		SET checkout_agent_id = 'agent-owner',
		    checkout_run_id = 'run-finished-owner',
		    checkout_at = datetime('now', '-1 hour')
		WHERE id = 'task-reap-run-id'
	`); err != nil {
		t.Fatalf("seed stale run-owned checkout: %v", err)
	}

	count, err := repo.ReapStaleCheckouts(ctx, time.Now().UTC().Add(-30*time.Minute))
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if count != 1 {
		t.Fatalf("reaped count = %d, want 1", count)
	}
}

// TestReapStaleCheckouts_ReapsWhenOnlyRunBelongsToAnotherAgent is the
// regression test for the compound scenario this card was filed about: the
// checkout holder's own run finished (leaking the checkout), and a
// *different* agent's run is queued against the same task — precisely
// because the leaked checkout is blocking it (see
// requeueContendedCheckout in scheduler_integration.go). A queued/claimed
// run belonging to a different agent than the checkout holder is not
// evidence the checkout is still legitimately held, so it must not
// suppress the reap. Without holder-scoping this run is exactly what
// disarms the backstop the leak scenario depends on.
func TestReapStaleCheckouts_ReapsWhenOnlyRunBelongsToAnotherAgent(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "task-reap-4", "ws-1", "Leaked checkout, blocked waiter", "", "")
	if _, err := repo.ExecRaw(ctx, `
		UPDATE tasks SET checkout_agent_id = 'agent-runner-leaked', checkout_at = datetime('now', '-1 hour')
		WHERE id = 'task-reap-4'
	`); err != nil {
		t.Fatalf("seed stale checkout: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, requested_at
		) VALUES (
			'run-reap-4', 'agent-reviewer-waiting', 'task_review_requested', '{"task_id":"task-reap-4"}',
			'queued', 1, '{}', CURRENT_TIMESTAMP
		)
	`); err != nil {
		t.Fatalf("seed queued run for a different agent: %v", err)
	}

	count, err := repo.ReapStaleCheckouts(ctx, time.Now().UTC().Add(-30*time.Minute))
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if count != 1 {
		t.Fatalf("reaped count = %d, want 1 (the only run belongs to a different agent than the holder)", count)
	}

	// Cleared checkout accepts a new agent.
	acquired, err := repo.CheckoutTask(ctx, "task-reap-4", "agent-new")
	if err != nil {
		t.Fatalf("checkout after reap: %v", err)
	}
	if !acquired {
		t.Fatal("expected checkout to be reaped, but a new agent still cannot acquire it")
	}
}

// TestReapStaleCheckouts_SkipsRecentCheckout proves a checkout younger
// than the threshold is left alone even with no in-flight run — it may
// simply not have started its run row yet.
func TestReapStaleCheckouts_SkipsRecentCheckout(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "task-reap-3", "ws-1", "Fresh checkout", "", "")
	if _, err := repo.ExecRaw(ctx, `
		UPDATE tasks SET checkout_agent_id = 'agent-fresh', checkout_at = datetime('now')
		WHERE id = 'task-reap-3'
	`); err != nil {
		t.Fatalf("seed fresh checkout: %v", err)
	}

	count, err := repo.ReapStaleCheckouts(ctx, time.Now().UTC().Add(-30*time.Minute))
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if count != 0 {
		t.Fatalf("reaped count = %d, want 0 (checkout is fresh)", count)
	}
}
