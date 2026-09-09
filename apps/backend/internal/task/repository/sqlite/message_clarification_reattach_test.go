package sqlite

import (
	"context"
	"testing"
	"time"
)

// TestReattachActiveClarificationBundleClearsDetachedFlagOnlyForCurrentPendingRows
// covers the exact-retry adoption path: a detached, still-pending bundle on the
// session's current turn regains a live waiter, so its rows must stop
// advertising agent_disconnected. Superseded, terminal, and never-detached rows
// are left alone, and a repeated call changes nothing.
func TestReattachActiveClarificationBundleClearsDetachedFlagOnlyForCurrentPendingRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.September, 5, 6, 0, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-reattach", "session-reattach")
	createPendingActionTurn(t, repo, "task-reattach", "session-reattach", "turn-old", base, base)
	createClarificationBundleMessage(
		t, repo, "message-old", "task-reattach", "session-reattach", "turn-old",
		"pending-old", "q-old", base,
	)
	setClarificationMessageMetadata(t, repo, "message-old", func(metadata map[string]interface{}) {
		metadata["agent_disconnected"] = true
	})
	createPendingActionTurn(
		t, repo, "task-reattach", "session-reattach", "turn-current",
		base.Add(time.Minute), base.Add(time.Minute),
	)
	createClarificationBundleMessage(
		t, repo, "message-current-1", "task-reattach", "session-reattach", "turn-current",
		"pending-current", "q1", base.Add(time.Minute),
	)
	createClarificationBundleMessage(
		t, repo, "message-current-2", "task-reattach", "session-reattach", "turn-current",
		"pending-current", "q2", base.Add(time.Minute+time.Second),
	)
	for _, id := range []string{"message-current-1", "message-current-2"} {
		setClarificationMessageMetadata(t, repo, id, func(metadata map[string]interface{}) {
			metadata["agent_disconnected"] = true
		})
	}
	createClarificationBundleMessage(
		t, repo, "message-answered", "task-reattach", "session-reattach", "turn-current",
		"pending-answered", "q-answered", base.Add(time.Minute+2*time.Second),
	)
	setClarificationMessageMetadata(t, repo, "message-answered", func(metadata map[string]interface{}) {
		metadata["status"] = "answered"
		metadata["agent_disconnected"] = true
	})
	mutationTime := base.Add(time.Hour)
	repo.clockNow = func() time.Time { return mutationTime }

	updated, active, err := repo.ReattachActiveClarificationBundle(ctx, "session-reattach", "pending-current")
	if err != nil {
		t.Fatalf("ReattachActiveClarificationBundle: %v", err)
	}
	if !active {
		t.Fatal("current pending bundle was not reported active")
	}
	if ids := messageIDs(updated); len(ids) != 2 || ids[0] != "message-current-1" || ids[1] != "message-current-2" {
		t.Fatalf("reattached message IDs = %v, want both current pending rows", ids)
	}
	for _, message := range updated {
		if _, detached := message.Metadata["agent_disconnected"]; detached {
			t.Fatalf("returned row %s still carries agent_disconnected: %#v", message.ID, message.Metadata)
		}
		if status, _ := message.Metadata["status"].(string); status != "pending" {
			t.Fatalf("returned row %s status = %q, want pending", message.ID, status)
		}
		if !message.UpdatedAt.Equal(mutationTime) {
			t.Fatalf("reattached updated_at = %v, want injected mutation time %v", message.UpdatedAt, mutationTime)
		}
	}
	current, err := repo.GetMessage(ctx, "message-current-1")
	if err != nil {
		t.Fatalf("GetMessage(current): %v", err)
	}
	if _, detached := current.Metadata["agent_disconnected"]; detached {
		t.Fatalf("persisted current row still detached: %#v", current.Metadata)
	}
	old, err := repo.GetMessage(ctx, "message-old")
	if err != nil {
		t.Fatalf("GetMessage(old): %v", err)
	}
	if detached, _ := old.Metadata["agent_disconnected"].(bool); !detached {
		t.Fatalf("superseded-turn row was reattached: %#v", old.Metadata)
	}
	answered, err := repo.GetMessage(ctx, "message-answered")
	if err != nil {
		t.Fatalf("GetMessage(answered): %v", err)
	}
	if detached, _ := answered.Metadata["agent_disconnected"].(bool); !detached {
		t.Fatalf("terminal row was reattached: %#v", answered.Metadata)
	}
	repeated, active, err := repo.ReattachActiveClarificationBundle(ctx, "session-reattach", "pending-current")
	if err != nil {
		t.Fatalf("repeated reattach: %v", err)
	}
	if !active {
		t.Fatal("already-attached current pending bundle was not reported active")
	}
	if len(repeated) != 0 {
		t.Fatalf("repeated reattach changed rows: %v", messageIDs(repeated))
	}
}

// TestReattachActiveClarificationBundleIgnoresNeverDetachedBundle proves the
// call is a no-op for a bundle that still has its original waiter.
func TestReattachActiveClarificationBundleIgnoresNeverDetachedBundle(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.September, 5, 6, 0, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-live", "session-live")
	createPendingActionTurn(t, repo, "task-live", "session-live", "turn-live", base, base)
	createClarificationBundleMessage(
		t, repo, "message-live", "task-live", "session-live", "turn-live",
		"pending-live", "q-live", base,
	)

	updated, active, err := repo.ReattachActiveClarificationBundle(ctx, "session-live", "pending-live")
	if err != nil {
		t.Fatalf("ReattachActiveClarificationBundle: %v", err)
	}
	if !active {
		t.Fatal("never-detached current pending bundle was not reported active")
	}
	if len(updated) != 0 {
		t.Fatalf("live bundle was rewritten: %v", messageIDs(updated))
	}
}
