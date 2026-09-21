package orchestrator

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

// TestPendingMove_BackwardMoveDispatchesOnEnterWithStaleRoute reproduces the
// production bug: a deferred move_task_kandev applied while the task already
// carries a workflow_session_route naming a step OTHER than the move's
// destination — the state any task is in after at least one prior workflow
// entry — commits the step change, but processStepExitAndEnterForDeferredMove
// passes a hardcoded transitionID of 0 into processOnEnter. The staleness
// guard's "may replace a stale route" escape hatch requires a positive entry
// ID to recognize a legitimate callback, so with 0 it falls through to the
// plain route-destination comparison, which fails, and on_enter
// (auto_start_agent) is silently skipped even though the step change itself
// commits.
//
// A route is deliberately seeded here rather than reached by two prior real
// transitions: without an established route, the guard's legacy no-route
// compatibility path lets any entry ID — including the buggy 0 — through,
// which is why a task's very first deferred move always "accidentally"
// worked and the existing scenario tests never caught this.
func TestPendingMove_BackwardMoveDispatchesOnEnterWithStaleRoute(t *testing.T) {
	sc := buildPendingMoveScenario(t)

	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.Metadata = map[string]interface{}{
		models.MetaKeyWorkflowSessionRoute: models.WorkflowSessionRoute{
			OperationID:       "seed-in-review-route",
			DestinationStepID: stepInReviewID,
			TargetKind:        workflowSessionRouteTargetProfile,
			DestinationID:     sc.reviewSessionID,
			Phase:             workflowSessionRouteCommitted,
		},
	}
	if err := sc.repo.UpdateTask(sc.ctx, task); err != nil {
		t.Fatalf("seed workflow session route: %v", err)
	}

	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load review session: %v", err)
	}

	// The backward move: Review bounces the task back to In Progress, exactly
	// the production reproducer (a step whose position precedes the current
	// one).
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, &messagequeue.PendingMove{
		TaskID:         "task-1",
		WorkflowID:     "wf1",
		WorkflowStepID: stepInProgressID,
	})

	task = sc.waitForTaskStep(t, stepInProgressID, 2*time.Second)
	if task.WorkflowStepID != stepInProgressID {
		t.Fatalf("workflow_step_id = %q, want %q — step change alone is not the bug under test",
			task.WorkflowStepID, stepInProgressID)
	}

	// THE ACTUAL ASSERTION: on_enter's auto_start_agent action must dispatch,
	// not just the step change. Before the fix, processOnEnter's staleness
	// guard rejects this callback (the hardcoded transitionID=0 can't
	// identify itself against the seeded stale route) and no session is ever
	// started.
	if !sc.waitForAgentStart(2 * time.Second) {
		t.Fatal("StartAgentProcess was never called — on_enter auto_start_agent did not dispatch after the backward deferred move")
	}

	sessions, err := sc.repo.ListTaskSessions(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	var freshImpl *models.TaskSession
	for _, s := range sessions {
		if s.AgentProfileID == profileImpl && s.ID != sc.implSessionID {
			freshImpl = s
		}
	}
	if freshImpl == nil {
		t.Fatal("expected a fresh impl-profile session after the backward deferred move dispatched on_enter")
	}
	if !freshImpl.IsPrimary {
		t.Error("fresh impl session must be primary after on_enter dispatches auto_start_agent")
	}
}

// waitForTaskStep polls the task's workflow_step_id until it reaches want or
// the deadline elapses, returning the last-read task either way.
func (sc *pendingMoveScenario) waitForTaskStep(t *testing.T, want string, timeout time.Duration) *models.Task {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var task *models.Task
	for {
		var err error
		task, err = sc.repo.GetTask(sc.ctx, "task-1")
		if err != nil {
			t.Fatalf("load task: %v", err)
		}
		if task.WorkflowStepID == want || time.Now().After(deadline) {
			return task
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// waitForAgentStart polls the mock agent manager until StartAgentProcess has
// been called at least once or the deadline elapses.
func (sc *pendingMoveScenario) waitForAgentStart(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		sc.agentMgr.mu.Lock()
		started := len(sc.agentMgr.startAgentProcessCalls) > 0
		sc.agentMgr.mu.Unlock()
		if started || time.Now().After(deadline) {
			return started
		}
		time.Sleep(10 * time.Millisecond)
	}
}
