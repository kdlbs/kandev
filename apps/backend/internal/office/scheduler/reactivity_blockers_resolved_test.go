package scheduler

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/events/bus"
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

func TestCommittedTaskCompletionWakesForFailedAndCancelledBlockersOnce(t *testing.T) {
	for _, state := range []string{"FAILED", "CANCELLED"} {
		t.Run(state, func(t *testing.T) {
			repo := newReactivityTestRepo(t)
			ss := newChildrenCompletedTestScheduler(t, repo)
			createChildrenCompletedAgent(t, repo, "agent-1")
			ctx := context.Background()
			if _, err := repo.ExecRaw(ctx, `CREATE TABLE IF NOT EXISTS workflow_steps (id TEXT PRIMARY KEY, agent_profile_id TEXT DEFAULT '')`); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.ExecRaw(ctx, `CREATE TABLE IF NOT EXISTS workspaces (id TEXT PRIMARY KEY, office_workflow_id TEXT DEFAULT '')`); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.ExecRaw(ctx, `CREATE TABLE IF NOT EXISTS workflows (id TEXT PRIMARY KEY, workspace_id TEXT DEFAULT '', is_system INTEGER DEFAULT 0, workflow_template_id TEXT DEFAULT '')`); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.ExecRaw(ctx, `INSERT INTO workspaces (id, office_workflow_id) VALUES ('ws-1', 'wf-office')`); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.ExecRaw(ctx, `INSERT INTO workflows (id, workspace_id) VALUES ('wf-office', 'ws-1')`); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.ExecRaw(ctx, `INSERT INTO tasks (id, workspace_id, workflow_id, state) VALUES ('blocker', 'ws-1', 'wf-office', ?), ('blocked', 'ws-1', 'wf-office', 'TODO')`, state); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.ExecRaw(ctx, `INSERT INTO task_blockers (task_id, blocker_task_id, created_at) VALUES ('blocked', 'blocker', CURRENT_TIMESTAMP)`); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.ExecRaw(ctx, `INSERT INTO workflow_step_participants (id, step_id, task_id, role, agent_profile_id, decision_required, position) VALUES ('p-runner', 'step-x', 'blocked', 'runner', 'agent-1', 0, 0)`); err != nil {
				t.Fatal(err)
			}
			event := &bus.Event{Data: map[string]any{
				"task_id": "blocker", "old_state": "IN_PROGRESS", "new_state": state,
			}}
			for i := 0; i < 2; i++ {
				if err := ss.handleCommittedTaskCompletion(ctx, event); err != nil {
					t.Fatalf("delivery %d: %v", i+1, err)
				}
			}
			if got := runsCountForReason(t, ss, RunReasonTaskBlockersResolved); got != 1 {
				t.Fatalf("blocker runs = %d, want one after duplicate %s delivery", got, state)
			}
		})
	}
}
