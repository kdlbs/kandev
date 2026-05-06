package dashboard_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/dashboard"
)

// recordingTerminator captures TerminateOfficeSession calls so tests can
// verify the dashboard service forwards reassignments / participant removals.
type recordingTerminator struct {
	calls []termCall
}

type termCall struct {
	taskID, agentID, reason string
}

func (r *recordingTerminator) TerminateOfficeSession(_ context.Context, taskID, agentID, reason string) error {
	r.calls = append(r.calls, termCall{taskID: taskID, agentID: agentID, reason: reason})
	return nil
}

func TestRemoveTaskReviewer_TerminatesSession(t *testing.T) {
	deps := newTestDeps(t)
	rt := &recordingTerminator{}
	deps.svc.SetSessionTerminator(rt)

	insertTestTask(t, deps.db, "task-rev", "ws-1", "Review", "todo", 2)

	if err := deps.svc.AddTaskReviewer(context.Background(), "", "task-rev", "agent-rev"); err != nil {
		t.Fatalf("add reviewer: %v", err)
	}
	if len(rt.calls) != 0 {
		t.Errorf("add must not terminate, got %d calls", len(rt.calls))
	}

	if err := deps.svc.RemoveTaskReviewer(context.Background(), "", "task-rev", "agent-rev"); err != nil {
		t.Fatalf("remove reviewer: %v", err)
	}
	if len(rt.calls) != 1 {
		t.Fatalf("expected 1 terminate call, got %d", len(rt.calls))
	}
	got := rt.calls[0]
	if got.taskID != "task-rev" || got.agentID != "agent-rev" {
		t.Errorf("term call: got %+v", got)
	}
}

// TestSetSessionTerminator_NilTolerated keeps the wiring optional: when no
// terminator is registered, removal still succeeds (dashboard tests run
// without the orchestrator wired in).
func TestSetSessionTerminator_NilTolerated(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "task-nil", "ws-1", "Nil", "todo", 2)
	if err := deps.svc.AddTaskReviewer(context.Background(), "", "task-nil", "agent-x"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := deps.svc.RemoveTaskReviewer(context.Background(), "", "task-nil", "agent-x"); err != nil {
		t.Fatalf("remove without terminator: %v", err)
	}
}

// staticDashboardSessionTerminator is the test-side type alias used to
// cross-check the interface satisfaction at compile time.
var _ dashboard.SessionTerminator = (*recordingTerminator)(nil)

// recordingReactivity is a stub ReactivityApplier that returns a fixed result
// so the dashboard's runReactivityForAssigneeChange path runs end-to-end in
// tests without bringing the scheduler into the picture.
type recordingReactivity struct {
	result *dashboard.TaskReactivityResult
	err    error
}

func (r *recordingReactivity) ApplyTaskMutation(_ context.Context, _ string, _ string, _ dashboard.TaskReactivityChange) (*dashboard.TaskReactivityResult, error) {
	return r.result, r.err
}

// TestSetTaskAssignee_TerminatesPrevSession exercises B5a: changing the
// assignee fires the terminator on the previous agent's (task, agent) pair.
func TestSetTaskAssignee_TerminatesPrevSession(t *testing.T) {
	deps := newTestDeps(t)
	rt := &recordingTerminator{}
	deps.svc.SetSessionTerminator(rt)
	deps.svc.SetReactivityApplier(&recordingReactivity{result: &dashboard.TaskReactivityResult{}})

	insertTestTask(t, deps.db, "task-r", "ws-r", "Reassign", "todo", 2)
	// Seed prev assignee directly via the underlying repo update.
	if err := deps.repo.UpdateTaskAssignee(context.Background(), "task-r", "agent-prev"); err != nil {
		t.Fatalf("seed prev assignee: %v", err)
	}

	if err := deps.svc.SetTaskAssigneeAsAgent(context.Background(), "", "task-r", "agent-new"); err != nil {
		t.Fatalf("set assignee: %v", err)
	}

	if len(rt.calls) != 1 {
		t.Fatalf("expected 1 terminate call, got %d (%+v)", len(rt.calls), rt.calls)
	}
	got := rt.calls[0]
	if got.taskID != "task-r" || got.agentID != "agent-prev" {
		t.Errorf("term call: got %+v", got)
	}
}
