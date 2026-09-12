package dashboard_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/office/dashboard"
)

// The retained-capacity determination is unexported, so these tests drive it
// through the participant-removal guarded path: removal always reaches the
// termination step, so whether the terminator was called is a direct readout
// of what the determination decided.

// seedSeat writes a participant seat directly. A taskID of "" makes it a
// template-level row, which the effective slate projects onto every task at
// that step.
func seedSeat(t *testing.T, db *sqlx.DB, id, stepID, taskID, role, agentID string, decisionRequired int) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, provenance)
		VALUES (?, ?, ?, ?, ?, ?, 0, 'manual')
	`, id, stepID, taskID, role, agentID, decisionRequired); err != nil {
		t.Fatalf("seed seat %s: %v", id, err)
	}
}

// moveTaskToStep repoints the task at a different step, leaving every seat
// written at the old step in place. That is what production does: seats are
// keyed (step, task, role, agent) and are not cleaned up on a step change.
func moveTaskToStep(t *testing.T, db *sqlx.DB, taskID, stepID string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE tasks SET workflow_step_id = ? WHERE id = ?`, stepID, taskID); err != nil {
		t.Fatalf("move task %s to %s: %v", taskID, stepID, err)
	}
}

// removeReviewerExpectingTermination runs the removal path and reports
// whether the session terminator fired.
func removeReviewer(t *testing.T, deps *testDeps, rt *recordingTerminator, taskID, agentID string) int {
	t.Helper()
	before := len(rt.calls)
	if err := deps.svc.RemoveTaskReviewer(context.Background(), "", taskID, agentID); err != nil {
		t.Fatalf("remove reviewer %s: %v", agentID, err)
	}
	return len(rt.calls) - before
}

func newCapacityDeps(t *testing.T, taskID string) (*testDeps, *recordingTerminator) {
	t.Helper()
	deps := newTestDeps(t)
	rt := &recordingTerminator{}
	deps.svc.SetSessionTerminator(rt)
	insertTestTask(t, deps.db, taskID, "ws-cap", "Capacity", "todo", 2)
	return deps, rt
}

// AC-OFFICE-SESSION-TERM-002.1 (runner alternative), -001.3: an agent that is
// still the task's runner keeps its session when a role is removed.
func TestRemoveParticipant_SuppressesWhenAgentIsRunner(t *testing.T) {
	deps, rt := newCapacityDeps(t, "task-runner")
	ctx := context.Background()

	if err := deps.svc.AddTaskReviewer(ctx, "", "task-runner", "agent-dual"); err != nil {
		t.Fatalf("add reviewer: %v", err)
	}
	if err := deps.repo.UpdateTaskAssignee(ctx, "task-runner", "agent-dual"); err != nil {
		t.Fatalf("set runner: %v", err)
	}

	if got := removeReviewer(t, deps, rt, "task-runner", "agent-dual"); got != 0 {
		t.Errorf("runner still seated on the task: want 0 terminations, got %d", got)
	}
}

// AC-OFFICE-SESSION-TERM-002.1 (seat alternative), -002.5, -002.6: an agent
// holding a second seat keeps its session, including when that seat carries
// no decision obligation.
func TestRemoveParticipant_SuppressesWhenAgentHoldsAnotherSeat(t *testing.T) {
	deps, rt := newCapacityDeps(t, "task-two-seats")
	ctx := context.Background()

	if err := deps.svc.AddTaskReviewer(ctx, "", "task-two-seats", "agent-both"); err != nil {
		t.Fatalf("add reviewer: %v", err)
	}
	// A decision-free approver seat still addresses the agent on the task.
	seedSeat(t, deps.db, "seat-approver", "step-task-two-seats", "task-two-seats",
		"approver", "agent-both", 0)

	if got := removeReviewer(t, deps, rt, "task-two-seats", "agent-both"); got != 0 {
		t.Errorf("agent still holds an approver seat: want 0 terminations, got %d", got)
	}
}

// AC-OFFICE-SESSION-TERM-001.4: losing the last capacity still ends the
// session, carrying the removal path's own reason.
func TestRemoveParticipant_TerminatesWhenNoCapacityRemains(t *testing.T) {
	deps, rt := newCapacityDeps(t, "task-last")
	ctx := context.Background()

	if err := deps.svc.AddTaskReviewer(ctx, "", "task-last", "agent-only"); err != nil {
		t.Fatalf("add reviewer: %v", err)
	}
	// Someone else runs the task, so the removed agent retains nothing.
	if err := deps.repo.UpdateTaskAssignee(ctx, "task-last", "agent-runner"); err != nil {
		t.Fatalf("set runner: %v", err)
	}

	if got := removeReviewer(t, deps, rt, "task-last", "agent-only"); got != 1 {
		t.Fatalf("want 1 termination, got %d", got)
	}
	if call := rt.calls[len(rt.calls)-1]; call.reason != "participant_removed" {
		t.Errorf("reason: got %q, want participant_removed", call.reason)
	}
}

// AC-OFFICE-SESSION-TERM-002.4: a template-level seat counts exactly as a
// per-task seat does.
func TestRemoveParticipant_SuppressesForTemplateLevelSeat(t *testing.T) {
	deps, rt := newCapacityDeps(t, "task-template")
	ctx := context.Background()

	if err := deps.svc.AddTaskReviewer(ctx, "", "task-template", "agent-tmpl"); err != nil {
		t.Fatalf("add reviewer: %v", err)
	}
	seedSeat(t, deps.db, "seat-tmpl", "step-task-template", "", "approver", "agent-tmpl", 1)

	if got := removeReviewer(t, deps, rt, "task-template", "agent-tmpl"); got != 0 {
		t.Errorf("template-level approver seat addresses the agent: want 0 terminations, got %d", got)
	}
}

// AC-OFFICE-SESSION-TERM-002.3: a seat recorded at a step the task has left
// must not, on its own, suppress a termination. Without the step bound the
// stale row would suppress every future termination for this pair.
func TestRemoveParticipant_StaleSeatAtLeftStepDoesNotSuppress(t *testing.T) {
	deps, rt := newCapacityDeps(t, "task-moved")
	ctx := context.Background()

	if err := deps.svc.AddTaskReviewer(ctx, "", "task-moved", "agent-stale"); err != nil {
		t.Fatalf("add reviewer: %v", err)
	}
	if err := deps.repo.UpdateTaskAssignee(ctx, "task-moved", "agent-runner"); err != nil {
		t.Fatalf("set runner: %v", err)
	}
	// The task advances. The reviewer seat written at the previous step
	// stays readable, but no longer addresses the agent on this task.
	moveTaskToStep(t, deps.db, "task-moved", "step-later")

	if got := removeReviewer(t, deps, rt, "task-moved", "agent-stale"); got != 1 {
		t.Errorf("stale seat at a left step must not suppress: want 1 termination, got %d", got)
	}
}

// AC-OFFICE-SESSION-TERM-002.9: the determination compares profile
// identifiers and does not require the profile to resolve to a live agent.
func TestRemoveParticipant_SeatNamingUnresolvableProfileStillSuppresses(t *testing.T) {
	deps, rt := newCapacityDeps(t, "task-ghost")

	seedSeat(t, deps.db, "seat-ghost-rev", "step-task-ghost", "task-ghost",
		"reviewer", "agent-ghost", 1)
	seedSeat(t, deps.db, "seat-ghost-app", "step-task-ghost", "task-ghost",
		"approver", "agent-ghost", 1)

	if got := removeReviewer(t, deps, rt, "task-ghost", "agent-ghost"); got != 0 {
		t.Errorf("seat naming an unresolvable profile still names this agent: want 0, got %d", got)
	}
}

// AC-OFFICE-SESSION-TERM-002.8 (first sentence): with no current step the
// effective slate is empty, so only the runner capacity can be retained.
func TestRemoveParticipant_NoCurrentStepLeavesOnlyRunnerCapacity(t *testing.T) {
	deps, rt := newCapacityDeps(t, "task-nostep")
	ctx := context.Background()

	if err := deps.svc.AddTaskReviewer(ctx, "", "task-nostep", "agent-seated"); err != nil {
		t.Fatalf("add reviewer: %v", err)
	}
	if err := deps.repo.UpdateTaskAssignee(ctx, "task-nostep", "agent-runner"); err != nil {
		t.Fatalf("set runner: %v", err)
	}
	moveTaskToStep(t, deps.db, "task-nostep", "")

	if got := removeReviewer(t, deps, rt, "task-nostep", "agent-seated"); got != 1 {
		t.Errorf("empty slate retains no seat capacity: want 1 termination, got %d", got)
	}
	// The runner is still retained through the projection, which reads
	// runner rows across steps.
	if got := removeReviewer(t, deps, rt, "task-nostep", "agent-runner"); got != 0 {
		t.Errorf("runner capacity survives an empty slate: want 0 terminations, got %d", got)
	}
}

// AC-OFFICE-SESSION-TERM-001.7, -002.8 (second sentence): a task that does not
// resolve is a failed determination, not an absence of capacity. The guarded
// path still reports success.
func TestRemoveParticipant_UnresolvableTaskFailsClosed(t *testing.T) {
	deps, rt := newCapacityDeps(t, "task-present")

	if err := deps.svc.RemoveTaskReviewer(context.Background(), "", "task-absent", "agent-x"); err != nil {
		t.Fatalf("removal must report success when the determination fails: %v", err)
	}
	if len(rt.calls) != 0 {
		t.Errorf("failed determination must not terminate: got %d calls", len(rt.calls))
	}
}

// AC-OFFICE-SESSION-TERM-001.8: neither identifier names a pair, so nothing is
// decided and nothing is ended.
func TestRemoveParticipant_EmptyIdentifiersReportSuccess(t *testing.T) {
	deps, rt := newCapacityDeps(t, "task-empty")

	if err := deps.svc.RemoveTaskReviewer(context.Background(), "", "task-empty", ""); err != nil {
		t.Fatalf("empty agent id must report success: %v", err)
	}
	if err := deps.svc.RemoveTaskReviewer(context.Background(), "", "", "agent-x"); err != nil {
		t.Fatalf("empty task id must report success: %v", err)
	}
	if len(rt.calls) != 0 {
		t.Errorf("empty identifiers must not terminate: got %d calls", len(rt.calls))
	}
}

// AC-OFFICE-SESSION-TERM-002.12: a capacity granted between the guarded
// path's commit and the determination's read is observed, because the read
// asks about the present. The seat is seeded from inside
// SetSessionCapacityReadHook, which fires after RemoveTaskReviewer's own
// removal has committed and immediately before the determination reads
// capacity — the exact window the AC names — so this only passes for an
// implementation whose read genuinely happens after that window, not one
// that decided from state captured earlier.
func TestRemoveParticipant_CapacityRegainedBeforeReadSuppresses(t *testing.T) {
	deps, rt := newCapacityDeps(t, "task-regained")
	ctx := context.Background()

	if err := deps.svc.AddTaskReviewer(ctx, "", "task-regained", "agent-back"); err != nil {
		t.Fatalf("add reviewer: %v", err)
	}
	if err := deps.repo.UpdateTaskAssignee(ctx, "task-regained", "agent-runner"); err != nil {
		t.Fatalf("set runner: %v", err)
	}

	restore := dashboard.SetSessionCapacityReadHook(func(taskID, agentProfileID string) {
		if taskID == "task-regained" && agentProfileID == "agent-back" {
			seedSeat(t, deps.db, "seat-back", "step-task-regained", "task-regained",
				"approver", "agent-back", 1)
		}
	})
	t.Cleanup(restore)

	if got := removeReviewer(t, deps, rt, "task-regained", "agent-back"); got != 0 {
		t.Errorf("re-seated agent is addressed on the task: want 0 terminations, got %d", got)
	}
}
