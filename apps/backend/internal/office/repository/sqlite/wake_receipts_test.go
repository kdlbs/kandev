package sqlite_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// seedWakeCandidate creates a parent task with one non-archived, terminal
// (COMPLETED) child, so it satisfies ListStuckParents' predicate on its
// own. Returns the child_set_key ListStuckParents will compute for it.
func seedWakeCandidate(t *testing.T, repo *sqlite.Repository, ctx context.Context, wsID, parentID string) string {
	t.Helper()
	insertTask(t, repo, ctx, parentID, wsID, "Parent", "", "")
	childID := parentID + "-child-0"
	insertTask(t, repo, ctx, childID, wsID, "Child", "", "")
	if _, err := repo.ExecRaw(ctx,
		`UPDATE tasks SET parent_id = ?, state = 'COMPLETED' WHERE id = ?`, parentID, childID,
	); err != nil {
		t.Fatalf("set child state: %v", err)
	}
	return childID + ":COMPLETED"
}

// seedWakeReceipt records a receipt for parentID as already delivered for
// childSetKey, the same shape UpsertWakeReceiptTx writes.
func seedWakeReceipt(t *testing.T, repo *sqlite.Repository, ctx context.Context, parentID, childSetKey string) {
	t.Helper()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO parent_child_wake_receipts (parent_task_id, child_set_key, delivered_run_id, delivered_at)
		VALUES (?, ?, 'prior-run', datetime('now'))
	`, parentID, childSetKey); err != nil {
		t.Fatalf("seed receipt: %v", err)
	}
}

// seedWakeRun inserts a runs row for parentID the way CreateRunTx would,
// so tests can simulate a wake already queued/claimed/finished/failed for
// this parent without going through the full reconciler.
func seedWakeRun(t *testing.T, repo *sqlite.Repository, ctx context.Context, runID, parentID, reason, status string) {
	t.Helper()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO runs (id, agent_profile_id, reason, payload, status, requested_at)
		VALUES (?, 'agent-x', ?, ?, ?, datetime('now'))
	`, runID, reason, fmt.Sprintf(`{"task_id":%q}`, parentID), status); err != nil {
		t.Fatalf("seed run: %v", err)
	}
}

// TestListStuckParents_LimitDoesNotStarveLaterCandidates is R1-A's
// regression test: candidates with an already-current receipt (nothing to
// do) must not consume LIMIT slots ahead of candidates that genuinely need
// a wake. Before the fix, the receipt comparison ran in Go after the SQL
// LIMIT, so the first 5 "resting" parents in id order permanently starved
// every parent past them.
func TestListStuckParents_LimitDoesNotStarveLaterCandidates(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	// p1..p5: already delivered for their current (unchanged) child set —
	// not actionable.
	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("p%d", i)
		key := seedWakeCandidate(t, repo, ctx, "ws-1", id)
		seedWakeReceipt(t, repo, ctx, id, key)
	}
	// p6..p8: no receipt at all — genuinely stuck, need a wake.
	want := map[string]bool{}
	for i := 6; i <= 8; i++ {
		id := fmt.Sprintf("p%d", i)
		seedWakeCandidate(t, repo, ctx, "ws-1", id)
		want[id] = true
	}

	candidates, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents: %v", err)
	}
	if len(candidates) != 3 {
		t.Fatalf("candidates = %d, want 3 (p6,p7,p8): %#v", len(candidates), candidates)
	}
	got := map[string]bool{}
	for _, c := range candidates {
		got[c.ParentTaskID] = true
	}
	for id := range want {
		if !got[id] {
			t.Errorf("expected %s among candidates, got %v", id, got)
		}
	}
}

// TestListStuckParents_ExcludesParentWithActiveOrFinishedRun is R1-B's
// regression test: a parent whose wake was already delivered via the
// edge-triggered path (queueChildrenCompletedRun / cascadeChildrenCompleted)
// never gets a receipt written for it — those paths don't write one — so
// only a runs-backed check, not the receipt, can tell the sweep the wake
// was already delivered.
func TestListStuckParents_ExcludesParentWithActiveOrFinishedRun(t *testing.T) {
	const reason = "task_children_completed"

	blocking := []string{"queued", "claimed", "finished"}
	for _, status := range blocking {
		t.Run(status, func(t *testing.T) {
			repo := newSearchTestRepo(t)
			ctx := context.Background()

			seedWakeCandidate(t, repo, ctx, "ws-1", "parent-1")
			seedWakeRun(t, repo, ctx, "run-1", "parent-1", reason, status)

			candidates, err := repo.ListStuckParents(ctx, reason, 5)
			if err != nil {
				t.Fatalf("ListStuckParents: %v", err)
			}
			if len(candidates) != 0 {
				t.Fatalf("status %q: expected the already-delivered parent to be excluded, got %#v", status, candidates)
			}
		})
	}

	nonBlocking := []string{"failed", "cancelled"}
	for _, status := range nonBlocking {
		t.Run(status, func(t *testing.T) {
			repo := newSearchTestRepo(t)
			ctx := context.Background()

			seedWakeCandidate(t, repo, ctx, "ws-1", "parent-1")
			seedWakeRun(t, repo, ctx, "run-1", "parent-1", reason, status)

			candidates, err := repo.ListStuckParents(ctx, reason, 5)
			if err != nil {
				t.Fatalf("ListStuckParents: %v", err)
			}
			if len(candidates) != 1 {
				t.Fatalf("status %q: expected the parent to still be a candidate (no successful delivery on record), got %#v", status, candidates)
			}
		})
	}
}

// TestListStuckParents_ExcludesArchivedOnlyChildren is R1-C's regression
// test: a parent whose only children are archived (including one that
// never reached a terminal state) must not be swept with a spurious empty
// child-set key.
func TestListStuckParents_ExcludesArchivedOnlyChildren(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "parent-archived-only", "ws-1", "Parent", "", "")
	insertTask(t, repo, ctx, "parent-archived-only-child-0", "ws-1", "Child", "", "")
	if _, err := repo.ExecRaw(ctx, `
		UPDATE tasks SET parent_id = 'parent-archived-only', state = 'IN_PROGRESS', archived_at = datetime('now')
		WHERE id = 'parent-archived-only-child-0'
	`); err != nil {
		t.Fatalf("archive child: %v", err)
	}

	// Control: a normal candidate, so an empty result can't be mistaken
	// for a broken query.
	seedWakeCandidate(t, repo, ctx, "ws-1", "parent-normal")

	candidates, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ParentTaskID != "parent-normal" {
		t.Fatalf("candidates = %#v, want exactly [parent-normal]", candidates)
	}
}
