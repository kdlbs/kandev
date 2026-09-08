package dashboard_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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
	calls  []dashboard.TaskReactivityChange
}

func (r *recordingReactivity) ApplyTaskMutation(_ context.Context, _ string, _ string, change dashboard.TaskReactivityChange) (*dashboard.TaskReactivityResult, error) {
	r.calls = append(r.calls, change)
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

// countingCanceller records hard-cancel calls so the reassignment path's two
// halves can be asserted independently.
type countingCanceller struct {
	mu     sync.Mutex
	called []string
}

func (c *countingCanceller) CancelTaskExecution(_ context.Context, taskID, _ string, _ bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.called = append(c.called, taskID)
	return nil
}

// waitForCancel blocks briefly for the hard cancel, which the reassignment
// path dispatches on its own goroutine.
func (c *countingCanceller) waitForCancel(t *testing.T) int {
	t.Helper()
	for i := 0; i < 200; i++ {
		c.mu.Lock()
		n := len(c.called)
		c.mu.Unlock()
		if n > 0 {
			return n
		}
		time.Sleep(5 * time.Millisecond)
	}
	return 0
}

// seedSelfReviewTask builds the shipped fallback shape: one agent is both the
// task's runner and a seated reviewer on it.
func seedSelfReviewTask(t *testing.T, deps *testDeps, taskID, agentID string) {
	t.Helper()
	ctx := context.Background()
	insertTestTask(t, deps.db, taskID, "ws-self", "Self review", "todo", 2)
	if err := deps.svc.AddTaskReviewer(ctx, "", taskID, agentID); err != nil {
		t.Fatalf("seat %s as reviewer: %v", agentID, err)
	}
	if err := deps.repo.UpdateTaskAssignee(ctx, taskID, agentID); err != nil {
		t.Fatalf("make %s the runner: %v", agentID, err)
	}
}

// TestSetTaskAssignee_KeepsSessionOfPrevRunnerStillSeated is
// AC-OFFICE-SESSION-TERM-003.2. Reassignment removes the runner capacity and
// leaves the reviewer seat, so the running execution must still be cancelled
// while the session row is left alone. Asserting only the session row would
// pass for an implementation that wrongly suppressed both.
func TestSetTaskAssignee_KeepsSessionOfPrevRunnerStillSeated(t *testing.T) {
	deps := newTestDeps(t)
	rt := &recordingTerminator{}
	cancels := &countingCanceller{}
	deps.svc.SetSessionTerminator(rt)
	deps.svc.SetTaskCanceller(cancels)
	deps.svc.SetReactivityApplier(&recordingReactivity{
		result: &dashboard.TaskReactivityResult{InterruptSessionID: "task-self"},
	})

	seedSelfReviewTask(t, deps, "task-self", "agent-dual")

	if err := deps.svc.SetTaskAssigneeAsAgent(context.Background(), "", "task-self", "agent-new"); err != nil {
		t.Fatalf("reassign: %v", err)
	}

	if len(rt.calls) != 0 {
		t.Errorf("prev runner is still a seated reviewer: want 0 terminations, got %d (%+v)",
			len(rt.calls), rt.calls)
	}
	if got := cancels.waitForCancel(t); got != 1 {
		t.Errorf("the execution it no longer runs must still be cancelled: got %d cancels", got)
	}
}

// TestSetTaskAssignee_TerminatesWhenPrevRunnerKeepsNothing keeps the other
// half of AC-OFFICE-SESSION-TERM-001.4 honest: with no seat left behind, the
// reassignment path still ends the session.
func TestSetTaskAssignee_TerminatesWhenPrevRunnerKeepsNothing(t *testing.T) {
	deps := newTestDeps(t)
	rt := &recordingTerminator{}
	deps.svc.SetSessionTerminator(rt)
	deps.svc.SetReactivityApplier(&recordingReactivity{result: &dashboard.TaskReactivityResult{}})

	insertTestTask(t, deps.db, "task-plain", "ws-self", "Plain", "todo", 2)
	if err := deps.repo.UpdateTaskAssignee(context.Background(), "task-plain", "agent-prev"); err != nil {
		t.Fatalf("seed runner: %v", err)
	}

	if err := deps.svc.SetTaskAssigneeAsAgent(context.Background(), "", "task-plain", "agent-new"); err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if len(rt.calls) != 1 {
		t.Fatalf("want 1 termination, got %d (%+v)", len(rt.calls), rt.calls)
	}
	if rt.calls[0].reason != "task_reassigned" {
		t.Errorf("reason: got %q, want task_reassigned", rt.calls[0].reason)
	}
}

// TestAddTaskReviewer_ClaimKeepsSessionOfDisplacedRunner is the claim path's
// half of AC-OFFICE-SESSION-TERM-001.6: displacing the auto-cast reviewer
// seat from the agent that also runs the task must not stop the task.
func TestAddTaskReviewer_ClaimKeepsSessionOfDisplacedRunner(t *testing.T) {
	deps := newTestDeps(t)
	rt := &recordingTerminator{}
	deps.svc.SetSessionTerminator(rt)
	insertTestTask(t, deps.db, "claim-dual", "ws-claim-dual", "C", "todo", 2)

	if _, err := deps.db.Exec(`
		INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
		VALUES ('agent-claiming', '', 'agent-claiming', 'agent-claiming', 'ws-claim-dual', '', datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed claiming agent profile: %v", err)
	}
	// The auto-cast reviewer is the task's runner: the shipped fallback when
	// no candidate is neither runner nor already seated.
	if _, err := deps.db.Exec(`
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, provenance)
		VALUES ('auto-seat-claim-dual', 'step-claim-dual', 'claim-dual', 'reviewer', 'agent-auto', 1, 0, 'auto')
	`); err != nil {
		t.Fatalf("seed auto seat: %v", err)
	}
	if err := deps.repo.UpdateTaskAssignee(context.Background(), "claim-dual", "agent-auto"); err != nil {
		t.Fatalf("make agent-auto the runner: %v", err)
	}

	body := `{"agent_profile_id":"agent-claiming"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/office/tasks/claim-dual/reviewers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
	if len(rt.calls) != 0 {
		t.Errorf("displaced agent still runs the task: want 0 terminations, got %d (%+v)",
			len(rt.calls), rt.calls)
	}
}

// TestSelfReviewSessionSurvivesRemoval_BothIdentityFlagStates is
// AC-OFFICE-SESSION-TERM-003.5. The defect is reachable in the shipped
// configuration (flag off), where a task with a runner routes every office run
// through the runner's session, so the flag must not change the outcome.
func TestSelfReviewSessionSurvivesRemoval_BothIdentityFlagStates(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "identity_off", true: "identity_on"}[enabled], func(t *testing.T) {
			deps := newTestDeps(t)
			rt := &recordingTerminator{}
			deps.svc.SetSessionTerminator(rt)
			deps.svc.SetOfficeSessionIdentity(enabled)

			seedSelfReviewTask(t, deps, "task-flag", "agent-dual")

			if err := deps.svc.RemoveTaskReviewer(
				context.Background(), "", "task-flag", "agent-dual"); err != nil {
				t.Fatalf("remove reviewer: %v", err)
			}
			if len(rt.calls) != 0 {
				t.Errorf("runner keeps its session regardless of the flag: got %d terminations",
					len(rt.calls))
			}
		})
	}
}
