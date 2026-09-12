package scheduler

import (
	"context"
	"fmt"
	"testing"

	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/runs/dedupkeys"
)

// insertBlockerRelationship records that taskID is blocked by
// blockerTaskID, and gives blockerTaskID the given (already-terminal or
// not) state.
func insertBlockerRelationship(
	t *testing.T, repo *officesqlite.Repository, taskID, blockerTaskID, blockerState string,
) {
	t.Helper()
	ctx := context.Background()
	if _, err := repo.ExecRaw(ctx,
		`INSERT INTO tasks (id, workspace_id, state) VALUES (?, 'ws-1', ?)`,
		blockerTaskID, blockerState,
	); err != nil {
		t.Fatalf("insert blocker task %s: %v", blockerTaskID, err)
	}
	if _, err := repo.ExecRaw(ctx,
		`INSERT INTO task_blockers (task_id, blocker_task_id, created_at) VALUES (?, ?, CURRENT_TIMESTAMP)`,
		taskID, blockerTaskID,
	); err != nil {
		t.Fatalf("insert task_blockers row (%s blocked by %s): %v", taskID, blockerTaskID, err)
	}
}

// TestCascadeBlockersResolved_QueuesRunWithSharedDigestKey is the
// regression test for both blocker-resolution producers converging on the
// same key shape (AC-OFFICE-RUN-DEDUP-002.1, SR-59's resolution: only the
// digest need be byte-identical, not the whole key/opID — see
// docs/specs/office/system-design/run-dedup-generation-03.md#the-shared-key-builders).
// Before this test, office/scheduler.cascadeBlockersResolved and
// allBlockersResolvedExcept had zero coverage: nothing exercised the
// blocked task's wake at all, let alone the specific
// dedupkeys.BlockerDigest call.
func TestCascadeBlockersResolved_QueuesRunWithSharedDigestKey(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	queue := newChildrenCompletedQueue(t, ss)
	ctx := context.Background()

	// blocked-1 is blocked by blocker-a (already done) and blocker-b (the
	// one resolving now). setupChildrenCompletedParent gives blocked-1 a
	// runner participant row so GetTaskAssignee resolves it to agent-1 —
	// reused here even though it's named for the children-completed tests,
	// because it does exactly what a blocked task's assignee wiring needs.
	setupChildrenCompletedParent(t, ss, "blocked-1", "agent-1")
	insertBlockerRelationship(t, repo, "blocked-1", "blocker-a", "COMPLETED")
	insertBlockerRelationship(t, repo, "blocked-1", "blocker-b", "IN_PROGRESS")

	// blocker-b finishes: it's the second and last outstanding blocker.
	if _, err := repo.ExecRaw(ctx, `UPDATE tasks SET state = 'COMPLETED' WHERE id = 'blocker-b'`); err != nil {
		t.Fatalf("complete blocker-b: %v", err)
	}
	ss.cascadeBlockersResolved(ctx, &TaskSnapshot{ID: "blocker-b", WorkspaceID: "ws-1"}, queue)

	runs, err := repo.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	var got *string
	for _, run := range runs {
		if run.Reason == RunReasonTaskBlockersResolved && run.AgentProfileID == "agent-1" {
			got = run.IdempotencyKey
			break
		}
	}
	if got == nil {
		t.Fatalf("no %s run persisted for agent-1: runs=%#v", RunReasonTaskBlockersResolved, runs)
	}

	// The key's digest segment must be exactly what the OTHER blocker
	// producer (office/service.resolveAndWakeIfUnblocked) would derive
	// for the identical blocker set, in either read order — that's the
	// structural convergence AC-002.1 requires.
	digestAscending := dedupkeys.BlockerDigest([]string{"blocker-a", "blocker-b"})
	digestDescending := dedupkeys.BlockerDigest([]string{"blocker-b", "blocker-a"})
	if digestAscending != digestDescending {
		t.Fatalf("BlockerDigest is not order-independent: %q vs %q", digestAscending, digestDescending)
	}
	want := fmt.Sprintf("%s:blocked-1:agent-1:%s", RunReasonTaskBlockersResolved, digestAscending)
	if *got != want {
		t.Fatalf("idempotency key = %q, want %q", *got, want)
	}
}

// TestCascadeBlockersResolved_StillBlocked_DoesNotQueue proves the
// negative: cascadeBlockersResolved must not wake the blocked task while
// another blocker is still outstanding.
func TestCascadeBlockersResolved_StillBlocked_DoesNotQueue(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	queue := newChildrenCompletedQueue(t, ss)
	ctx := context.Background()

	setupChildrenCompletedParent(t, ss, "blocked-2", "agent-1")
	insertBlockerRelationship(t, repo, "blocked-2", "blocker-c", "IN_PROGRESS") // never finishes
	insertBlockerRelationship(t, repo, "blocked-2", "blocker-d", "IN_PROGRESS")

	if _, err := repo.ExecRaw(ctx, `UPDATE tasks SET state = 'COMPLETED' WHERE id = 'blocker-d'`); err != nil {
		t.Fatalf("complete blocker-d: %v", err)
	}
	ss.cascadeBlockersResolved(ctx, &TaskSnapshot{ID: "blocker-d", WorkspaceID: "ws-1"}, queue)

	if got := runsCountForReason(t, ss, RunReasonTaskBlockersResolved); got != 0 {
		t.Fatalf("persisted runs = %d, want 0 (blocker-c is still unresolved)", got)
	}
}
