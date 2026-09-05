package orchestrator

// Tests for docs/specs/workflow-evaluate-only-operation-marking/spec.md
// (AC-EO-9 through AC-EO-17): dispatchKanbanAgentErrorTrigger must mark an
// operation applied if and only if it has itself fully determined that
// operation's transition outcome. In EvaluateOnly mode the engine defers the
// mark for a deferred transition (HandleResult.OperationMarkDeferred), and
// this dispatch only marks the operation once its own commit has actually
// succeeded — never before, and never on a declined or errored commit.

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// --- AC-EO-10: a successful transition is marked applied by the caller (not
// the engine) after its own commit succeeds, and a redelivery of the exact
// same failure is then idempotent. ---

func TestDispatchKanbanAgentErrorTrigger_SuccessfulTransitionMarksAppliedAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{
			{Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{"step_id": "step2"}},
		}},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, nil)

	data := watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom"}
	svc.dispatchKanbanAgentErrorTrigger(ctx, data)

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.WorkflowStepID != "step2" {
		t.Fatalf("WorkflowStepID = %q, want step2 (the transition must have committed)", task.WorkflowStepID)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Fatalf("got %d dispatch INFO records, want 1", len(got))
	}

	operationID := agentErrorOperationID("s1", "exec-1")
	applied, err := svc.workflowStore.IsOperationApplied(ctx, operationID)
	if err != nil || !applied {
		t.Fatalf("IsOperationApplied = %v, %v, want true, nil (AC-EO-10: the caller must mark applied itself after its own commit succeeds)", applied, err)
	}

	logs.TakeAll()
	svc.dispatchKanbanAgentErrorTrigger(ctx, data)

	task, err = repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.WorkflowStepID != "step2" {
		t.Fatalf("WorkflowStepID changed to %q on redelivery, want unchanged step2", task.WorkflowStepID)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 0 {
		t.Errorf("redelivery emitted %d dispatch record(s), want 0 (AC-EO-10: idempotent short-circuit)", len(got))
	}
}

// --- AC-EO-11 case 2: a credential preflight failure declines the
// transition, and the operation must stay unmarked so a redelivery retries. ---

// agentErrorListSessionsFailsAfterFirstCallRepo fails every other
// ListTaskSessions call: each delivery makes exactly two calls in sequence
// (otherWorkingSessionID's own guard check, then
// findReusableSessionForProfile's credential-preflight lookup), so an
// odd/even split lets the guard check keep succeeding on every redelivery
// while the later preflight call keeps failing.
type agentErrorListSessionsFailsAfterFirstCallRepo struct {
	sessionExecutorStore
	calls int32
	err   error
}

func (r *agentErrorListSessionsFailsAfterFirstCallRepo) ListTaskSessions(
	ctx context.Context, taskID string,
) ([]*models.TaskSession, error) {
	if atomic.AddInt32(&r.calls, 1)%2 == 0 {
		return nil, r.err
	}
	return r.sessionExecutorStore.ListTaskSessions(ctx, taskID)
}

func TestDispatchKanbanAgentErrorTrigger_CredentialPreflightFailureLeavesOperationUnmarked(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{
			{Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{"step_id": "step2"}},
		}},
	}
	// A different AgentProfileID than the current session's (empty) forces
	// preflightWorkflowStepCredentials past its early-return and into
	// findReusableSessionForProfile's ListTaskSessions call.
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1, AgentProfileID: "profile-x",
	}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, nil)
	// The first ListTaskSessions call is otherWorkingSessionID's own guard
	// check (which must still succeed); only the later credential-preflight
	// call fails.
	svc.repo = &agentErrorListSessionsFailsAfterFirstCallRepo{sessionExecutorStore: svc.repo, err: errors.New("list sessions unavailable")}

	data := watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom"}
	svc.dispatchKanbanAgentErrorTrigger(ctx, data)

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("WorkflowStepID = %q, want step1 (a credential preflight failure must decline the transition)", task.WorkflowStepID)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Fatalf("got %d dispatch INFO records, want 1 (dispatch still reports success)", len(got))
	}

	operationID := agentErrorOperationID("s1", "exec-1")
	applied, err := svc.workflowStore.IsOperationApplied(ctx, operationID)
	if err != nil || applied {
		t.Fatalf("IsOperationApplied = %v, %v, want false, nil (a declined transition must leave the operation unmarked)", applied, err)
	}

	logs.TakeAll()
	svc.dispatchKanbanAgentErrorTrigger(ctx, data)

	task, err = repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("WorkflowStepID = %q after redelivery, want step1 (still declined the same way)", task.WorkflowStepID)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Errorf("redelivery emitted %d dispatch record(s), want 1 (AC-EO-11: an unmarked declined transition retries)", len(got))
	}
}

// --- AC-EO-11 case 3: a source (from-step) load failure declines the
// transition, and the operation must stay unmarked so a redelivery retries. ---

func TestDispatchKanbanAgentErrorTrigger_SourceStepLoadFailureLeavesOperationUnmarked(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{
			{Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{"step_id": "step2"}},
		}},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1}

	// The pre-engine walk's own LoadStep call (through workflowStore's
	// stepCache) populates and permanently caches step1's compiled spec on
	// its first, successful fetch. applyEngineTransitionWithCommitMode's
	// loadWorkflowStepForLifecycle("transition source") call bypasses that
	// cache and calls GetStep directly every time — so failing step1 from its
	// second raw GetStep call onward fails only that call, on every delivery.
	var mu sync.Mutex
	callsPerID := map[string]int{}
	stepGetter.getStepFunc = func(_ context.Context, id string) (*wfmodels.WorkflowStep, error) {
		mu.Lock()
		callsPerID[id]++
		n := callsPerID[id]
		mu.Unlock()
		if id == "step1" && n >= 2 {
			return nil, errors.New("source step store unavailable")
		}
		return stepGetter.steps[id], nil
	}

	svc, logs := newAgentErrorTestService(t, repo, stepGetter, nil)

	data := watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom"}
	svc.dispatchKanbanAgentErrorTrigger(ctx, data)

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("WorkflowStepID = %q, want step1 (a from-step load failure must decline the transition)", task.WorkflowStepID)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Fatalf("got %d dispatch INFO records, want 1 (dispatch still reports success)", len(got))
	}

	operationID := agentErrorOperationID("s1", "exec-1")
	applied, err := svc.workflowStore.IsOperationApplied(ctx, operationID)
	if err != nil || applied {
		t.Fatalf("IsOperationApplied = %v, %v, want false, nil (a declined transition must leave the operation unmarked)", applied, err)
	}

	logs.TakeAll()
	svc.dispatchKanbanAgentErrorTrigger(ctx, data)

	task, err = repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("WorkflowStepID = %q after redelivery, want step1 (still declined the same way)", task.WorkflowStepID)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Errorf("redelivery emitted %d dispatch record(s), want 1 (AC-EO-11: an unmarked declined transition retries)", len(got))
	}
}

// --- AC-EO-11 case 4: a commit error declines the transition, and the
// operation must stay unmarked so a redelivery retries. ---

type agentErrorCommitErrorRepo struct {
	sessionExecutorStore
	err error
}

func (r *agentErrorCommitErrorRepo) UpdateTaskWithWorkflowStepAdmission(
	_ context.Context, _ *models.Task, _ string, _ int,
) (bool, error) {
	return false, r.err
}

func TestDispatchKanbanAgentErrorTrigger_CommitErrorLeavesOperationUnmarked(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{
			{Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{"step_id": "step2"}},
		}},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, nil)
	// workflowStore captured svc.repo at initWorkflowEngine time; reassigning
	// svc.repo alone would leave the commit path on the old repo, so this
	// case (unlike the credential-preflight one) must reinit after wrapping.
	svc.repo = &agentErrorCommitErrorRepo{sessionExecutorStore: svc.repo, err: errors.New("db write failed")}
	svc.initWorkflowEngine()

	data := watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom"}
	svc.dispatchKanbanAgentErrorTrigger(ctx, data)

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("WorkflowStepID = %q, want step1 (a commit error must leave the task on its source step)", task.WorkflowStepID)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Fatalf("got %d dispatch INFO records, want 1 (dispatch still reports success)", len(got))
	}

	operationID := agentErrorOperationID("s1", "exec-1")
	applied, err := svc.workflowStore.IsOperationApplied(ctx, operationID)
	if err != nil || applied {
		t.Fatalf("IsOperationApplied = %v, %v, want false, nil (a declined transition must leave the operation unmarked)", applied, err)
	}

	logs.TakeAll()
	svc.dispatchKanbanAgentErrorTrigger(ctx, data)

	task, err = repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("WorkflowStepID = %q after redelivery, want step1 (still declined the same way)", task.WorkflowStepID)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Errorf("redelivery emitted %d dispatch record(s), want 1 (AC-EO-11: an unmarked declined transition retries)", len(got))
	}
}

// --- AC-EO-14: an unmarked operation is retried at least once, so a
// non-transition action from the same trigger (e.g. clear_decisions) must
// replay on redelivery rather than being skipped as already-applied. ---

func TestDispatchKanbanAgentErrorTrigger_CommitErrorReplaysNonTransitionAction(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{
			{Type: wfmodels.GenericActionClearDecisions},
			{Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{"step_id": "step2"}},
		}},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1}

	decisions := &spyDecisionStore{}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })
	// workflowStore captured svc.repo at initWorkflowEngine time; reassigning
	// svc.repo alone would leave the commit path on the old repo, so this
	// case (like the sibling CommitErrorLeavesOperationUnmarked test) must
	// reinit after wrapping.
	svc.repo = &agentErrorCommitErrorRepo{sessionExecutorStore: svc.repo, err: errors.New("db write failed")}
	svc.initWorkflowEngine()

	data := watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom"}
	svc.dispatchKanbanAgentErrorTrigger(ctx, data)

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("WorkflowStepID = %q, want step1 (a commit error must leave the task on its source step)", task.WorkflowStepID)
	}

	operationID := agentErrorOperationID("s1", "exec-1")
	applied, err := svc.workflowStore.IsOperationApplied(ctx, operationID)
	if err != nil || applied {
		t.Fatalf("IsOperationApplied = %v, %v, want false, nil (a declined transition must leave the operation unmarked)", applied, err)
	}
	if decisions.clearCalls != 1 {
		t.Fatalf("clearCalls = %d, want 1 after the first (failed-commit) delivery", decisions.clearCalls)
	}

	logs.TakeAll()
	svc.dispatchKanbanAgentErrorTrigger(ctx, data)

	task, err = repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("WorkflowStepID = %q after redelivery, want step1 (still declined the same way)", task.WorkflowStepID)
	}
	if decisions.clearCalls != 2 {
		t.Fatalf("clearCalls = %d, want 2 (AC-EO-14: an unmarked operation's non-transition "+
			"action must replay on redelivery, not be skipped as already-applied)", decisions.clearCalls)
	}
}

// --- AC-EO-13: the operation-id lock spans load -> evaluate -> commit ->
// mark, so two concurrent deliveries of the same failure commit exactly
// once. ---

// blockingAgentErrorDecisionStore is engine.DecisionStore's clear_decisions
// action, wired to block the first caller inside ClearStepDecisions until the
// test releases it — used to hold a dispatch goroutine mid-evaluation (after
// its operation-id lock is already acquired) so a second, concurrent
// delivery of the same operation id can be proven to block on that lock
// rather than racing ahead.
type blockingAgentErrorDecisionStore struct {
	mu         sync.Mutex
	clearCalls int
	entered    chan struct{}
	release    chan struct{}
}

func (b *blockingAgentErrorDecisionStore) ListStepDecisions(context.Context, string, string) ([]engine.DecisionInfo, error) {
	return nil, nil
}

func (b *blockingAgentErrorDecisionStore) RecordStepDecision(context.Context, engine.DecisionInfo) error {
	return nil
}

func (b *blockingAgentErrorDecisionStore) ClearStepDecisions(context.Context, string, string) (int64, error) {
	b.mu.Lock()
	b.clearCalls++
	first := b.clearCalls == 1
	b.mu.Unlock()
	if first {
		close(b.entered)
		<-b.release
	}
	return 0, nil
}

// agentErrorCountingGetTaskSessionRepo counts GetTaskSession calls so a test
// can prove a blocked dispatch's own state load has not yet started —
// distinct from the lock's ref count, which only proves a goroutine is
// registered as a waiter, not that it has been kept from reaching
// resolveAgentErrorDispatchTarget. It embeds the concrete repository (rather
// than the narrower sessionExecutorStore interface) so every other
// type-asserted repo capability the commit path relies on — e.g.
// workflowMoveAdmissionRepository in updateTransitionTask — stays promoted
// through the wrapper; embedding the interface would silently drop those.
type agentErrorCountingGetTaskSessionRepo struct {
	*sqliterepo.Repository
	mu    sync.Mutex
	calls int
}

func (r *agentErrorCountingGetTaskSessionRepo) GetTaskSession(
	ctx context.Context, sessionID string,
) (*models.TaskSession, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	return r.Repository.GetTaskSession(ctx, sessionID)
}

func (r *agentErrorCountingGetTaskSessionRepo) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func waitForAgentErrorLockRefs(t *testing.T, svc *Service, operationID string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.agentErrorOperationLocksMu.Lock()
		got := 0
		if entry := svc.agentErrorOperationLocks[operationID]; entry != nil {
			got = entry.refs
		}
		svc.agentErrorOperationLocksMu.Unlock()
		if got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for agent-error lock refs = %d", want)
}

func TestDispatchKanbanAgentErrorTrigger_ConcurrentSameOperationExactlyOneCommits(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{
			{Type: wfmodels.GenericActionClearDecisions},
			{Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{"step_id": "step2"}},
		}},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1}

	decisions := &blockingAgentErrorDecisionStore{entered: make(chan struct{}), release: make(chan struct{})}
	countingRepo := &agentErrorCountingGetTaskSessionRepo{Repository: repo}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) {
		s.engineDecisions = decisions
		s.repo = countingRepo
	})

	data := watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom"}
	operationID := agentErrorOperationID("s1", "exec-1")

	firstDone := make(chan struct{})
	go func() {
		svc.dispatchKanbanAgentErrorTrigger(ctx, data)
		close(firstDone)
	}()

	select {
	case <-decisions.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the first dispatch to enter clear_decisions")
	}

	// AC-EO-13: the first dispatch's own state load must have already
	// happened (it is blocked past that point, inside clear_decisions), and
	// nothing else has touched GetTaskSession yet.
	if got := countingRepo.callCount(); got != 1 {
		t.Fatalf("GetTaskSession calls = %d, want 1 before the second dispatch starts", got)
	}

	secondDone := make(chan struct{})
	go func() {
		svc.dispatchKanbanAgentErrorTrigger(ctx, data)
		close(secondDone)
	}()

	waitForAgentErrorLockRefs(t, svc, operationID, 2)
	select {
	case <-secondDone:
		t.Fatal("second dispatch returned before the first released the operation-id lock")
	case <-time.After(100 * time.Millisecond):
	}

	// AC-EO-13: the operation-id lock must be held across the entire
	// load -> evaluate -> commit -> mark sequence, not just around the engine
	// call. If a regression narrowed the lock's span to start later, the
	// second dispatch's own GetTaskSession call could run here, ahead of the
	// first dispatch's commit — this assertion is the one that would catch
	// that narrowing, which waitForAgentErrorLockRefs alone cannot (it only
	// proves the second goroutine registered as a waiter, not that it was
	// kept from reaching resolveAgentErrorDispatchTarget).
	if got := countingRepo.callCount(); got != 1 {
		t.Fatalf("GetTaskSession calls = %d, want 1 while the first dispatch still holds the operation-id lock", got)
	}

	close(decisions.release)

	select {
	case <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the first dispatch to finish")
	}
	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the second dispatch to finish")
	}

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.WorkflowStepID != "step2" {
		t.Fatalf("WorkflowStepID = %q, want step2", task.WorkflowStepID)
	}

	decisions.mu.Lock()
	clearCalls := decisions.clearCalls
	decisions.mu.Unlock()
	if clearCalls != 1 {
		t.Fatalf("clearCalls = %d, want 1 (the lock must serialize the two deliveries so the second sees the operation already applied)", clearCalls)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Fatalf("got %d dispatch INFO records, want 1 (the serialized second delivery must be idempotent)", len(got))
	}
}

// agentErrorBlockingCommitRepo blocks the first
// UpdateTaskWithWorkflowStepAdmission call (the transition commit) until the
// test releases it, and counts GetTaskSession calls the same way
// agentErrorCountingGetTaskSessionRepo does. It embeds the concrete
// *sqliterepo.Repository, not the narrower sessionExecutorStore interface,
// for the same reason agentErrorCountingGetTaskSessionRepo does: the commit
// path type-asserts s.repo against workflowMoveAdmissionRepository, and an
// interface embed would silently drop that capability.
type agentErrorBlockingCommitRepo struct {
	*sqliterepo.Repository

	sessionMu    sync.Mutex
	sessionCalls int

	commitMu    sync.Mutex
	commitCalls int
	entered     chan struct{}
	release     chan struct{}
}

func (r *agentErrorBlockingCommitRepo) GetTaskSession(
	ctx context.Context, sessionID string,
) (*models.TaskSession, error) {
	r.sessionMu.Lock()
	r.sessionCalls++
	r.sessionMu.Unlock()
	return r.Repository.GetTaskSession(ctx, sessionID)
}

func (r *agentErrorBlockingCommitRepo) sessionCallCount() int {
	r.sessionMu.Lock()
	defer r.sessionMu.Unlock()
	return r.sessionCalls
}

func (r *agentErrorBlockingCommitRepo) UpdateTaskWithWorkflowStepAdmission(
	ctx context.Context, task *models.Task, targetStepID string, limit int,
) (bool, error) {
	r.commitMu.Lock()
	r.commitCalls++
	first := r.commitCalls == 1
	r.commitMu.Unlock()
	if first {
		close(r.entered)
		<-r.release
	}
	return r.Repository.UpdateTaskWithWorkflowStepAdmission(ctx, task, targetStepID, limit)
}

// TestDispatchKanbanAgentErrorTrigger_ConcurrentSameOperationLockSpansThroughCommit
// is the AC-EO-13 lock-SPAN case the sibling
// ConcurrentSameOperationExactlyOneCommits test cannot see: that test blocks
// the first dispatch inside engine evaluation (clear_decisions), which is
// *before* the caller's own commit call, so it proves the lock covers
// load -> engine-call but says nothing about whether the lock is still held
// once HandleTrigger has returned and the caller is committing and marking.
// This test blocks the first dispatch inside its own commit call instead — a
// strictly later point in the sequence — and re-asserts that the second
// dispatch's state load still has not run. A lock released as soon as
// HandleTrigger returns (before the caller's commit and mark) would let the
// second dispatch's GetTaskSession call run here, which this test's
// sessionCallCount assertion below would catch.
func TestDispatchKanbanAgentErrorTrigger_ConcurrentSameOperationLockSpansThroughCommit(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{
			{Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{"step_id": "step2"}},
		}},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1}

	blockingRepo := &agentErrorBlockingCommitRepo{Repository: repo, entered: make(chan struct{}), release: make(chan struct{})}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.repo = blockingRepo })

	data := watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom"}
	operationID := agentErrorOperationID("s1", "exec-1")

	firstDone := make(chan struct{})
	go func() {
		svc.dispatchKanbanAgentErrorTrigger(ctx, data)
		close(firstDone)
	}()

	select {
	case <-blockingRepo.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the first dispatch to enter its commit call")
	}

	secondDone := make(chan struct{})
	go func() {
		svc.dispatchKanbanAgentErrorTrigger(ctx, data)
		close(secondDone)
	}()

	waitForAgentErrorLockRefs(t, svc, operationID, 2)
	select {
	case <-secondDone:
		t.Fatal("second dispatch returned before the first released the operation-id lock")
	case <-time.After(100 * time.Millisecond):
	}

	// AC-EO-13: the lock must still be held here — the first dispatch is
	// blocked *inside its own commit call*, strictly after HandleTrigger has
	// already returned. If the lock's span were narrowed to end as soon as
	// HandleTrigger returns (i.e. released before the commit and mark), the
	// second dispatch's own state load could run concurrently with the
	// first's pending commit, which this assertion would catch.
	if got := blockingRepo.sessionCallCount(); got != 1 {
		t.Fatalf("GetTaskSession calls = %d, want 1 while the first dispatch is still inside its own commit call", got)
	}
	operationApplied, err := svc.workflowStore.IsOperationApplied(ctx, operationID)
	if err != nil || operationApplied {
		t.Fatalf("IsOperationApplied = %v, %v, want false, nil (the first dispatch has not committed or marked yet)", operationApplied, err)
	}

	close(blockingRepo.release)

	select {
	case <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the first dispatch to finish")
	}
	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the second dispatch to finish")
	}

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.WorkflowStepID != "step2" {
		t.Fatalf("WorkflowStepID = %q, want step2", task.WorkflowStepID)
	}

	blockingRepo.commitMu.Lock()
	commitCalls := blockingRepo.commitCalls
	blockingRepo.commitMu.Unlock()
	if commitCalls != 1 {
		t.Fatalf("commit calls = %d, want 1 (the lock held through commit must serialize the second delivery into an idempotent no-op)", commitCalls)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Fatalf("got %d dispatch INFO records, want 1 (the serialized second delivery must be idempotent)", len(got))
	}
}

// TestLockAgentErrorOperationKeepsEntryUntilWaitersExit is the direct
// ref-counting unit test for the lock helper itself, mirroring
// TestLockChildCompletionOperationKeepsEntryUntilWaitersExit.
func TestLockAgentErrorOperationKeepsEntryUntilWaitersExit(t *testing.T) {
	svc := &Service{}
	unlockFirst := svc.lockAgentErrorOperation("op")

	secondAcquired := make(chan struct{})
	releaseSecond := make(chan struct{})
	done := make(chan struct{})
	go func() {
		unlockSecond := svc.lockAgentErrorOperation("op")
		close(secondAcquired)
		<-releaseSecond
		unlockSecond()
		close(done)
	}()

	waitForAgentErrorLockRefs(t, svc, "op", 2)
	unlockFirst()
	select {
	case <-secondAcquired:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second lock holder")
	}
	waitForAgentErrorLockRefs(t, svc, "op", 1)
	close(releaseSecond)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second lock release")
	}

	svc.agentErrorOperationLocksMu.Lock()
	_, exists := svc.agentErrorOperationLocks["op"]
	svc.agentErrorOperationLocksMu.Unlock()
	if exists {
		t.Fatal("expected lock entry to be deleted after all holders exit")
	}
}

// --- AC-EO-17: a marker lost after a successful commit (the process-local
// map is not durable across a restart) must not replay the stale
// transition — a redelivery evaluates the task's current (target) step. ---

func TestDispatchKanbanAgentErrorTrigger_MarkerLostAfterCommitRedeliveryEvaluatesFreshState(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{
			{Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{"step_id": "step2"}},
		}},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
	}
	decisions := &spyDecisionStore{}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

	data := watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom"}
	svc.dispatchKanbanAgentErrorTrigger(ctx, data)

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.WorkflowStepID != "step2" {
		t.Fatalf("WorkflowStepID = %q, want step2 after the first delivery's transition", task.WorkflowStepID)
	}

	operationID := agentErrorOperationID("s1", "exec-1")
	applied, err := svc.workflowStore.IsOperationApplied(ctx, operationID)
	if err != nil || !applied {
		t.Fatalf("IsOperationApplied = %v, %v, want true, nil after a successful commit", applied, err)
	}

	// Simulate the marker being lost (a process restart clears the
	// process-local appliedOps map) without touching persisted task/step
	// state.
	svc.workflowStore.appliedOps.Delete(operationID)
	logs.TakeAll()

	svc.dispatchKanbanAgentErrorTrigger(ctx, data)

	task, err = repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.WorkflowStepID != "step2" {
		t.Fatalf("WorkflowStepID = %q after redelivery, want step2 (unchanged; step2's own action must fire, not step1's stale transition)", task.WorkflowStepID)
	}
	if decisions.clearCalls != 1 {
		t.Fatalf("clearCalls = %d, want 1 (redelivery must evaluate step2's declared action, proving it re-read current state rather than replaying step1's stale transition)", decisions.clearCalls)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Fatalf("got %d dispatch INFO records on redelivery, want 1", len(got))
	}

	reapplied, err := svc.workflowStore.IsOperationApplied(ctx, operationID)
	if err != nil || !reapplied {
		t.Fatalf("IsOperationApplied after redelivery = %v, %v, want true, nil (a further delivery must be idempotent again)", reapplied, err)
	}
}
