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
