package service_test

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/runs/commentkeys"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// fakeDispatcher records every HandleTrigger call so tests can pin the
// exact trigger + payload + operation id the office service emits.
type fakeDispatcher struct {
	mu    sync.Mutex
	calls []dispatcherCall
	// nextErr lets a test simulate engine.HandleTrigger returning a
	// specific error.
	nextErr error
}

type dispatcherCall struct {
	taskID  string
	trigger engine.Trigger
	payload any
	opID    string
}

func (f *fakeDispatcher) HandleTrigger(
	_ context.Context, taskID string, trigger engine.Trigger, payload any, opID string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, dispatcherCall{taskID, trigger, payload, opID})
	return f.nextErr
}

func (f *fakeDispatcher) Calls() []dispatcherCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]dispatcherCall, len(f.calls))
	copy(out, f.calls)
	return out
}

// TestEngineDispatcher_NoDispatcher_DropsTrigger pins the contract that
// when no dispatcher is wired (e.g. a test that only exercises the
// office service in isolation) comment events do not produce any
// engine calls and do not error.
func TestEngineDispatcher_NoDispatcher_DropsTrigger(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	// Deliberately do NOT call SetWorkflowEngineDispatcher.

	ctx := context.Background()
	createTestAgent(t, svc, "ws-1", "agent-1")
	insertTestTask(t, svc, "task-1", "ws-1")
	setTestTaskAssignee(t, svc, "task-1", "agent-1")

	comment := &models.TaskComment{
		TaskID:     "task-1",
		AuthorType: "user",
		AuthorID:   "user-x",
		Body:       "fix this",
	}
	if err := svc.CreateComment(ctx, comment); err != nil {
		t.Fatalf("create comment: %v", err)
	}
}

// TestEngineDispatcher_RoutesToDispatcher pins that a comment fires
// engine.HandleTrigger with TriggerOnComment + a typed
// OnCommentPayload when the dispatcher is wired.
func TestEngineDispatcher_RoutesToDispatcher(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)

	disp := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(disp)

	ctx := context.Background()
	createTestAgent(t, svc, "ws-1", "agent-1")
	insertTestTask(t, svc, "task-1", "ws-1")
	setTestTaskAssignee(t, svc, "task-1", "agent-1")

	comment := &models.TaskComment{
		TaskID:     "task-1",
		AuthorType: "user",
		AuthorID:   "user-x",
		Body:       "fix this",
	}
	if err := svc.CreateComment(ctx, comment); err != nil {
		t.Fatalf("create comment: %v", err)
	}

	calls := disp.Calls()
	if len(calls) != 1 {
		t.Fatalf("want 1 dispatcher call, got %d", len(calls))
	}
	got := calls[0]
	if got.taskID != "task-1" {
		t.Errorf("taskID = %q, want task-1", got.taskID)
	}
	if got.trigger != engine.TriggerOnComment {
		t.Errorf("trigger = %q, want %q", got.trigger, engine.TriggerOnComment)
	}
	payload, ok := got.payload.(engine.OnCommentPayload)
	if !ok {
		t.Fatalf("payload type = %T, want engine.OnCommentPayload", got.payload)
	}
	if payload.CommentID != comment.ID {
		t.Errorf("payload.CommentID = %q, want %q", payload.CommentID, comment.ID)
	}
	if payload.AuthorID != "user-x" {
		t.Errorf("payload.AuthorID = %q, want user-x", payload.AuthorID)
	}
	wantOp := "task_comment:" + comment.ID
	if got.opID != wantOp {
		t.Errorf("operationID = %q, want %q", got.opID, wantOp)
	}
}

func TestEngineDispatcher_SkipsAlreadyDispatchedCommentEvent(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)

	disp := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(disp)

	ctx := context.Background()
	createTestAgent(t, svc, "ws-1", "agent-1")
	insertTestTask(t, svc, "task-1", "ws-1")
	setTestTaskAssignee(t, svc, "task-1", "agent-1")

	event := bus.NewEvent(events.OfficeCommentCreated, "test", map[string]string{
		"task_id":           "task-1",
		"comment_id":        "comment-1",
		"author_type":       "user",
		"author_id":         "user-x",
		"engine_dispatched": commentkeys.EngineDispatchedValue,
	})
	if err := eb.Publish(ctx, events.OfficeCommentCreated, event); err != nil {
		t.Fatalf("publish comment event: %v", err)
	}
	if calls := disp.Calls(); len(calls) != 0 {
		t.Fatalf("dispatcher calls = %d, want 0", len(calls))
	}
}

func TestEngineDispatcher_SkipsDoneStepSelfComment(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)

	disp := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(disp)

	ctx := context.Background()
	createTestAgent(t, svc, "ws-1", "runner-on-review")
	insertTestTask(t, svc, "task-done", "ws-1")
	svc.ExecSQL(t, `
		INSERT INTO workflow_steps (id, agent_profile_id)
		VALUES ('step-work', ''), ('step-review', ''), ('step-done', '')
	`)
	svc.ExecSQL(t, `UPDATE tasks SET workflow_step_id = 'step-done' WHERE id = 'task-done'`)
	svc.ExecSQL(t, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES
			('p-work', 'step-work', 'task-done', 'runner', 'runner-on-work', 0, 0),
			('p-review', 'step-review', 'task-done', 'runner', 'runner-on-review', 0, 0)
	`)

	comment := &models.TaskComment{
		TaskID:     "task-done",
		AuthorType: "agent",
		AuthorID:   "runner-on-review",
		Body:       "Done",
	}
	if err := svc.CreateComment(ctx, comment); err != nil {
		t.Fatalf("create comment: %v", err)
	}
	if calls := disp.Calls(); len(calls) != 0 {
		t.Fatalf("dispatcher calls = %d, want 0", len(calls))
	}
}

func TestEngineDispatcher_DispatchesOlderDoneStepRunnerComment(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)

	disp := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(disp)

	ctx := context.Background()
	createTestAgent(t, svc, "ws-1", "runner-on-work")
	createTestAgent(t, svc, "ws-1", "runner-on-review")
	insertTestTask(t, svc, "task-done", "ws-1")
	svc.ExecSQL(t, `
		INSERT INTO workflow_steps (id, agent_profile_id)
		VALUES ('step-work', ''), ('step-review', ''), ('step-done', '')
	`)
	svc.ExecSQL(t, `UPDATE tasks SET workflow_step_id = 'step-done' WHERE id = 'task-done'`)
	svc.ExecSQL(t, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES
			('p-work', 'step-work', 'task-done', 'runner', 'runner-on-work', 0, 0),
			('p-review', 'step-review', 'task-done', 'runner', 'runner-on-review', 0, 0)
	`)

	comment := &models.TaskComment{
		TaskID:     "task-done",
		AuthorType: "agent",
		AuthorID:   "runner-on-work",
		Body:       "Older runner reply",
	}
	if err := svc.CreateComment(ctx, comment); err != nil {
		t.Fatalf("create comment: %v", err)
	}
	if calls := disp.Calls(); len(calls) != 1 {
		t.Fatalf("dispatcher calls = %d, want 1", len(calls))
	}
}

// TestEngineDispatcher_NoSession_DropsTrigger pins that when the
// dispatcher returns ErrEngineNoSession the subscriber drops the
// trigger silently — there is no legacy fallback after Phase 4.
func TestEngineDispatcher_NoSession_DropsTrigger(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)

	disp := &fakeDispatcher{nextErr: shared.ErrEngineNoSession}
	svc.SetWorkflowEngineDispatcher(disp)

	ctx := context.Background()
	createTestAgent(t, svc, "ws-1", "agent-1")
	insertTestTask(t, svc, "task-1", "ws-1")
	setTestTaskAssignee(t, svc, "task-1", "agent-1")

	comment := &models.TaskComment{
		TaskID:     "task-1",
		AuthorType: "user",
		AuthorID:   "user-x",
		Body:       "fix this",
	}
	if err := svc.CreateComment(ctx, comment); err != nil {
		t.Fatalf("create comment: %v", err)
	}

	// Dispatcher was tried.
	if calls := disp.Calls(); len(calls) != 1 {
		t.Fatalf("dispatcher calls = %d, want 1", len(calls))
	}
	// No legacy fallback — runs table stays empty.
	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("no-session: want 0 runs (no legacy fallback), got %d", len(runs))
	}
}

// fallbackHandledRoutingDispatcher is a service.RoutingDispatcher fake whose
// HandlePostStartFailure always reports handled=true, letting tests pin the
// "routing already requeued the run" branch of handleAgentFailed.
type fallbackHandledRoutingDispatcher struct{}

func (fallbackHandledRoutingDispatcher) DispatchWithRouting(
	context.Context, *models.Run, *models.AgentInstance, service.LaunchContext,
) (bool, bool, error) {
	return false, false, nil
}

func (fallbackHandledRoutingDispatcher) HandlePostStartFailure(
	context.Context, *models.Run, *models.AgentInstance, string, *streams.ProviderError,
) (bool, error) {
	return true, nil
}

func (fallbackHandledRoutingDispatcher) MarkRunSuccessHealth(
	context.Context, *models.Run, *models.AgentInstance,
) {
}

// queueTaskAssignedRunForAgentFailedTests queues + claims a run for taskID
// so an AgentFailed event resolves it via resolveLifecycleRun's
// task+agent fallback (GetClaimedRunByTaskAndAgent).
func queueTaskAssignedRunForAgentFailedTests(
	t *testing.T, svc *service.Service, agentID, taskID string,
) *models.Run {
	t.Helper()
	ctx := context.Background()
	if err := svc.QueueRun(
		ctx, agentID, service.RunReasonTaskAssigned, `{"task_id":"`+taskID+`"}`, "",
	); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim run: %v (run=%v)", err, run)
	}
	return run
}

// publishAgentFailed publishes an AgentFailed lifecycle event for the
// given task/agent/session, mirroring what the lifecycle manager emits
// on a terminal agent-session failure (see TestRunLifecycle_ErrorEventEmitted).
func publishAgentFailed(
	t *testing.T, eb bus.EventBus, taskID, agentID, sessionID, errMsg string,
) {
	t.Helper()
	evt := bus.NewEvent(events.AgentFailed, "test", map[string]string{
		"task_id":          taskID,
		"session_id":       sessionID,
		"error_message":    errMsg,
		"agent_profile_id": agentID,
	})
	if err := eb.Publish(context.Background(), events.AgentFailed, evt); err != nil {
		t.Fatalf("publish agent failed: %v", err)
	}
}

// TestEngineDispatcher_AgentFailed_RoutesToDispatcher pins WO-05: a
// terminal agent-session failure dispatches TriggerOnAgentError with a
// typed OnAgentErrorPayload so the workflow engine can queue_run the
// workspace CEO agent. Before this wiring, TriggerOnAgentError had zero
// production dispatchers and the CEO never learned about the failure.
func TestEngineDispatcher_AgentFailed_RoutesToDispatcher(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	disp := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(disp)

	ctx := context.Background()
	createTestAgent(t, svc, "ws-1", "worker-1")
	insertTestTask(t, svc, "task-1", "ws-1")
	setTestTaskAssignee(t, svc, "task-1", "worker-1")

	run := queueTaskAssignedRunForAgentFailedTests(t, svc, "worker-1", "task-1")

	publishAgentFailed(t, eb, "task-1", "worker-1", "sess-err", "boom")

	calls := disp.Calls()
	if len(calls) != 1 {
		t.Fatalf("dispatcher calls = %d, want 1: %+v", len(calls), calls)
	}
	got := calls[0]
	if got.taskID != "task-1" {
		t.Errorf("taskID = %q, want task-1", got.taskID)
	}
	if got.trigger != engine.TriggerOnAgentError {
		t.Errorf("trigger = %q, want %q", got.trigger, engine.TriggerOnAgentError)
	}
	payload, ok := got.payload.(engine.OnAgentErrorPayload)
	if !ok {
		t.Fatalf("payload type = %T, want engine.OnAgentErrorPayload", got.payload)
	}
	if payload.FailedAgentID != "worker-1" {
		t.Errorf("payload.FailedAgentID = %q, want worker-1", payload.FailedAgentID)
	}
	if payload.FailedSessionID != "sess-err" {
		t.Errorf("payload.FailedSessionID = %q, want sess-err", payload.FailedSessionID)
	}
	if payload.ErrorMessage != "boom" {
		t.Errorf("payload.ErrorMessage = %q, want boom", payload.ErrorMessage)
	}
	wantOp := "agent_error:" + run.ID
	if got.opID != wantOp {
		t.Errorf("operationID = %q, want %q", got.opID, wantOp)
	}

	// Failure bookkeeping still ran: the run itself is marked failed.
	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	var found bool
	for _, r := range runs {
		if r.ID == run.ID {
			found = true
			if r.Status != service.RunStatusFailed {
				t.Errorf("run status = %q, want failed", r.Status)
			}
		}
	}
	if !found {
		t.Fatalf("run %s not found in ws-1 runs", run.ID)
	}
}

// TestEngineDispatcher_AgentFailed_SameRunTwice_OnlyOneCall pins that a
// duplicate AgentFailed event for the same run does not double-dispatch.
// The run is no longer claimed after the first failure, so
// resolveLifecycleRun's task+agent fallback returns sql.ErrNoRows for the
// replay and handleAgentFailed returns early before reaching dispatch.
func TestEngineDispatcher_AgentFailed_SameRunTwice_OnlyOneCall(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	disp := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(disp)

	createTestAgent(t, svc, "ws-1", "worker-1")
	insertTestTask(t, svc, "task-1", "ws-1")
	setTestTaskAssignee(t, svc, "task-1", "worker-1")
	queueTaskAssignedRunForAgentFailedTests(t, svc, "worker-1", "task-1")

	publishAgentFailed(t, eb, "task-1", "worker-1", "sess-err", "boom")
	publishAgentFailed(t, eb, "task-1", "worker-1", "sess-err", "boom")

	if calls := disp.Calls(); len(calls) != 1 {
		t.Fatalf("dispatcher calls = %d, want 1 (replay guard)", len(calls))
	}
}

// TestEngineDispatcher_AgentFailed_PostStartFallbackHandled_SkipsDispatch
// pins that when routing already requeued the run (tryPostStartFallback
// returns true), handleAgentFailed returns before reaching either the
// failure bookkeeping or the engine dispatch — the failure was not
// terminal, so there is nothing to escalate.
func TestEngineDispatcher_AgentFailed_PostStartFallbackHandled_SkipsDispatch(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	disp := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(disp)
	svc.SetRoutingDispatcher(fallbackHandledRoutingDispatcher{})

	createTestAgent(t, svc, "ws-1", "worker-1")
	insertTestTask(t, svc, "task-1", "ws-1")
	setTestTaskAssignee(t, svc, "task-1", "worker-1")
	run := queueTaskAssignedRunForAgentFailedTests(t, svc, "worker-1", "task-1")
	svc.ExecSQL(t, `UPDATE runs SET resolved_provider_id = 'test-provider' WHERE id = ?`, run.ID)

	publishAgentFailed(t, eb, "task-1", "worker-1", "sess-err", "boom")

	if calls := disp.Calls(); len(calls) != 0 {
		t.Fatalf("dispatcher calls = %d, want 0 (post-start fallback handled)", len(calls))
	}
}

// TestEngineDispatcher_PathBEscalation_DoesNotFireAgentErrorTrigger pins
// that the pre-launch retry-exhaustion escalation path (HandleRunFailure
// -> escalateFailure -> queueCEOAgentError, "Path B" in the WO-05 task
// plan) queues its CEO run directly and never touches the engine
// dispatcher — TriggerOnAgentError is exclusively a Path A (failed agent
// session) concern, so the two mechanisms cannot double-fire for one
// failure.
func TestEngineDispatcher_PathBEscalation_DoesNotFireAgentErrorTrigger(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	disp := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(disp)

	ctx := context.Background()
	ceo := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "ceo-pathb",
		Role:        models.AgentRoleCEO,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, ceo); err != nil {
		t.Fatalf("create ceo: %v", err)
	}
	createTestAgent(t, svc, "ws-1", "worker-pathb")

	if err := svc.QueueRun(
		ctx, "worker-pathb", service.RunReasonTaskAssigned, `{"task_id":"t1"}`, "",
	); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v (run=%v)", err, run)
	}
	run.RetryCount = service.MaxRetryCount

	if err := svc.HandleRunFailure(ctx, run, errForTest("boom")); err != nil {
		t.Fatalf("handle run failure: %v", err)
	}

	if calls := disp.Calls(); len(calls) != 0 {
		t.Fatalf("dispatcher calls = %d, want 0 (Path B does not fire the engine trigger)", len(calls))
	}

	next, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim ceo run: %v", err)
	}
	if next == nil {
		t.Fatal("expected a queued CEO agent_error run")
	}
	if next.AgentProfileID != ceo.ID {
		t.Errorf("agent = %q, want CEO %q", next.AgentProfileID, ceo.ID)
	}
	if next.Reason != service.RunReasonAgentError {
		t.Errorf("reason = %q, want agent_error", next.Reason)
	}
}
