package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// seedStuckSignalWorkflow wires the same two-step workflow every test in this
// file reclaims through: step1 requires a completion signal and moves to
// step2 on turn-complete.
func seedStuckSignalWorkflow(sg *mockStepGetter) {
	sg.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		AutoAdvanceRequiresSignal: true,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToNext},
			},
		},
	}
	sg.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
	}
}

func seedOverdueStuckSignal(t *testing.T, repo *sqliterepo.Repository, sessionID string) {
	t.Helper()
	signal := models.PendingStepCompletionSignal{
		StepID:     "step1",
		Source:     models.StepCompletionSourceAgent,
		Summary:    "all done",
		SignaledAt: time.Now().UTC().Add(-30 * time.Minute),
	}
	if err := repo.SetSessionMetadataKey(context.Background(), sessionID, models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
		t.Fatalf("seed pending signal: %v", err)
	}
}

// TestReclaimStuckSignalSessionIfDue_PreservesNewTurnStartedDuringCancelWindow
// is the COR-001 regression test. cancelAgentWhileUnlocked briefly unlocks the
// per-session cancelInFlight guard while the agent manager's CancelAgent call
// is in flight (stuck_signal_watchdog.go). If a new turn starts during that
// window -- e.g. the user retried the session, or a queued prompt was
// dispatched -- the watchdog must not blindly complete "the active turn" once
// it regains the guard, because GetActiveTurnBySessionID resolves to the
// newest open turn and that is now the new one, not the stuck one the
// watchdog set out to settle. Before COR-001, completeTurnForSession(ctx,
// session.ID) did exactly that: it looked up whatever turn was active *after*
// the unlock window and completed it, silently closing out a turn the
// watchdog never intended to touch. The fix captures the turn ID before
// unlocking and only completes it if it is still the active turn afterward
// (completeTurnForTaskSessionCheckedOwned's fail-closed
// verifyExpectedTurnOwnership).
func TestReclaimStuckSignalSessionIfDue_PreservesNewTurnStartedDuringCancelWindow(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	setSessionExecID(t, repo, "s1", "exec-1")

	stepGetter := newMockStepGetter()
	seedStuckSignalWorkflow(stepGetter)

	turns := &repoTurnService{repo: repo}
	stuckTurn, err := turns.StartTurn(ctx, "s1")
	if err != nil {
		t.Fatalf("start stuck turn: %v", err)
	}

	var newTurn *models.Turn
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	agentMgr.cancelAgentFunc = func(context.Context, string) error {
		// Simulate a new turn starting for this session while the watchdog's
		// CancelAgent call is in flight and the guard is unlocked -- exactly
		// the window cancelAgentWhileUnlocked opens.
		started, startErr := turns.StartTurn(ctx, "s1")
		if startErr != nil {
			return startErr
		}
		newTurn = started
		return lifecycle.ErrCancelEscalated
	}

	svc := createEngineService(t, repo, stepGetter, agentMgr)
	svc.turnService = turns

	seedOverdueStuckSignal(t, repo, "s1")

	svc.reclaimStuckSignalSessionsOnce(ctx)

	gotStuck, err := turns.GetTurn(ctx, stuckTurn.ID)
	if err != nil {
		t.Fatalf("get stuck turn: %v", err)
	}
	if gotStuck.CompletedAt != nil {
		t.Error("the pre-existing stuck turn must not be completed once a new turn has superseded it")
	}
	if newTurn == nil {
		t.Fatal("test setup bug: cancelAgentFunc did not run")
	}
	gotNew, err := turns.GetTurn(ctx, newTurn.ID)
	if err != nil {
		t.Fatalf("get new turn: %v", err)
	}
	if gotNew.CompletedAt != nil {
		t.Error("the new turn started during the unlock window must not be completed by the watchdog either -- COR-001 regression")
	}

	updatedTask, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updatedTask.WorkflowStepID != "step1" {
		t.Fatalf("expected no transition once the captured turn was superseded, got %q", updatedTask.WorkflowStepID)
	}
	updatedSession, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if updatedSession.State != models.TaskSessionStateRunning {
		t.Fatalf("expected session to remain RUNNING when the reclaim is skipped, got %q", updatedSession.State)
	}
	if _, hasSignal := models.LoadPendingStepSignal(updatedSession.Metadata); !hasSignal {
		t.Error("expected the pending signal to remain unapplied when the reclaim is skipped")
	}
}

// TestReclaimStuckSignalSessionIfDue_SkipsOnGenericCancelError is the
// TEST-001 regression test. cancelAgentWhileUnlocked tolerates only
// ErrNoExecutionForSession and ErrCancelEscalated; any other error from the
// agent manager's CancelAgent must abort the reclaim entirely rather than
// proceeding to complete a turn the agent manager could not actually settle.
func TestReclaimStuckSignalSessionIfDue_SkipsOnGenericCancelError(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	setSessionExecID(t, repo, "s1", "exec-1")

	stepGetter := newMockStepGetter()
	seedStuckSignalWorkflow(stepGetter)

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	agentMgr.cancelAgentErr = errors.New("provider unreachable")

	svc := createEngineService(t, repo, stepGetter, agentMgr)
	svc.turnService = &repoTurnService{repo: repo}

	seedOverdueStuckSignal(t, repo, "s1")

	svc.reclaimStuckSignalSessionsOnce(ctx)

	updatedTask, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updatedTask.WorkflowStepID != "step1" {
		t.Fatalf("expected no transition when CancelAgent returns a non-tolerated error, got %q", updatedTask.WorkflowStepID)
	}
	updatedSession, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if updatedSession.State != models.TaskSessionStateRunning {
		t.Fatalf("expected session to remain RUNNING when the reclaim is skipped, got %q", updatedSession.State)
	}
	if _, hasSignal := models.LoadPendingStepSignal(updatedSession.Metadata); !hasSignal {
		t.Error("expected the pending signal to remain unapplied when the reclaim is skipped")
	}
}

// TestReclaimStuckSignalSessionIfDue_ConcurrentExplicitCancelJoinsInsteadOfDoubleInvoking
// is the SEC-001 regression test. Before SEC-001, the watchdog's own
// cancellation was not registered in s.cancellationOperations, so a
// concurrent explicit Service.CancelAgent call for the same session would
// independently invoke the agent manager's CancelAgent a second time instead
// of joining the watchdog's already-in-flight operation. This also serves as
// the first live proof that reclaimStuckSignalSessionIfDue's split into an
// outer claim function plus reclaimStuckSignalSessionOwned does not
// self-deadlock: finishCancellationWithActions (invoked from the outer
// function only after the owned call has released its guard) re-acquires the
// same per-session cancelInFlight mutex to run the joined action below.
func TestReclaimStuckSignalSessionIfDue_ConcurrentExplicitCancelJoinsInsteadOfDoubleInvoking(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	setSessionExecID(t, repo, "s1", "exec-1")

	stepGetter := newMockStepGetter()
	seedStuckSignalWorkflow(stepGetter)

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	agentMgr.cancelAgentEntered = make(chan struct{}, 1)
	agentMgr.cancelAgentBlock = make(chan struct{})

	svc := createEngineService(t, repo, stepGetter, agentMgr)
	svc.turnService = &repoTurnService{repo: repo}

	seedOverdueStuckSignal(t, repo, "s1")

	watchdogDone := make(chan struct{})
	go func() {
		defer close(watchdogDone)
		svc.reclaimStuckSignalSessionsOnce(ctx)
	}()

	select {
	case <-agentMgr.cancelAgentEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the watchdog's own CancelAgent call to start")
	}

	// By the time CancelAgent has been entered, claimCancellationWithActionExclusive
	// has already registered the watchdog's operation (it runs before
	// cancelAgentWhileUnlocked, which is what unblocked CancelAgent above).
	operation := svc.currentCancellation("s1")
	if operation == nil {
		t.Fatal("test setup bug: no cancellation operation registered for s1")
	}

	explicitDone := make(chan error, 1)
	go func() {
		explicitDone <- svc.CancelAgent(ctx, "s1")
	}()

	select {
	case <-operation.joined:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the concurrent explicit cancel to join the watchdog's operation -- SEC-001 regression: it may have raced to become owner and independently invoked CancelAgent instead")
	}

	close(agentMgr.cancelAgentBlock)

	select {
	case <-watchdogDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the watchdog's reclaim to finish")
	}
	select {
	case err := <-explicitDone:
		if err != nil {
			t.Fatalf("expected the joined explicit cancel to return without error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the concurrent explicit CancelAgent to return -- possible self-deadlock in finishCancellationWithActions")
	}

	if got := agentMgr.cancelAgentCalls.Load(); got != 1 {
		t.Fatalf("expected exactly one underlying CancelAgent call (the watchdog's own), got %d -- "+
			"a concurrent explicit cancel independently re-invoked CancelAgent instead of joining the watchdog's claimed operation (SEC-001 regression)", got)
	}
}

// TestReclaimStuckSignalSessionIfDue_CancelsCapturedPromptIdentity prevents a
// session lookup from retargeting cancellation at a replacement execution.
// The watchdog must pass the execution, generation, and activity epoch that
// made the reclaim eligible to the lifecycle manager.
func TestReclaimStuckSignalSessionIfDue_CancelsCapturedPromptIdentity(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	setSessionExecID(t, repo, "s1", "exec-1")

	stepGetter := newMockStepGetter()
	seedStuckSignalWorkflow(stepGetter)
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	agentMgr.currentPromptExecutionID = "exec-1"
	agentMgr.currentPromptGeneration.Store(7)
	agentMgr.currentPromptActivityEpoch.Store(11)
	agentMgr.currentPromptLastActivityAt = time.Now().UTC().Add(-15 * time.Minute)
	var gotSession, gotExecution string
	var gotGeneration, gotEpoch uint64
	agentMgr.cancelAgentForPromptFunc = func(_ context.Context, sessionID, executionID string, generation, activityEpoch uint64) error {
		gotSession, gotExecution = sessionID, executionID
		gotGeneration, gotEpoch = generation, activityEpoch
		return lifecycle.ErrCancelEscalated
	}

	svc := createEngineService(t, repo, stepGetter, agentMgr)
	svc.turnService = &repoTurnService{repo: repo}
	seedOverdueStuckSignal(t, repo, "s1")

	svc.reclaimStuckSignalSessionsOnce(ctx)

	if gotSession != "s1" || gotExecution != "exec-1" || gotGeneration != 7 || gotEpoch != 11 {
		t.Fatalf("captured prompt identity = session %q execution %q generation %d epoch %d, want s1/exec-1/7/11",
			gotSession, gotExecution, gotGeneration, gotEpoch)
	}
	if got := agentMgr.cancelAgentForPromptCalls.Load(); got != 1 {
		t.Fatalf("identity-aware cancellation calls = %d, want 1", got)
	}
}

func TestReclaimStuckSignalSessionIfDue_DoesNotSweepTurnWhenNoneWasCaptured(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	setSessionExecID(t, repo, "s1", "exec-1")

	stepGetter := newMockStepGetter()
	seedStuckSignalWorkflow(stepGetter)
	turns := &repoTurnService{repo: repo}
	var successor *models.Turn
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	agentMgr.currentPromptExecutionID = "exec-1"
	agentMgr.currentPromptGeneration.Store(1)
	agentMgr.currentPromptActivityEpoch.Store(1)
	agentMgr.currentPromptLastActivityAt = time.Now().UTC().Add(-15 * time.Minute)
	agentMgr.cancelAgentFunc = func(ctx context.Context, sessionID string) error {
		var err error
		successor, err = turns.StartTurn(ctx, sessionID)
		if err != nil {
			t.Errorf("start successor turn in cancel hook: %v", err)
		}
		return err
	}

	svc := createEngineService(t, repo, stepGetter, agentMgr)
	svc.turnService = turns
	seedOverdueStuckSignal(t, repo, "s1")

	svc.reclaimStuckSignalSessionsOnce(ctx)

	if got := agentMgr.cancelAgentCalls.Load(); got != 1 {
		t.Fatalf("cancel calls = %d, want 1", got)
	}
	if successor == nil {
		t.Fatal("cancel hook did not create a successor turn")
	}
	completed, err := turns.GetTurn(ctx, successor.ID)
	if err != nil {
		t.Fatalf("get successor turn: %v", err)
	}
	if completed.CompletedAt != nil {
		t.Fatal("a successor turn must not be completed when no turn was captured before cancellation")
	}
	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("task advanced to %q after an unscoped turn capture, want step1", task.WorkflowStepID)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Fatalf("session state = %q, want RUNNING after fail-closed successor detection", session.State)
	}
}

func TestReclaimStuckSignalSessionIfDue_JoinedCancelDoesNotCompleteSuccessorStep(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	setSessionExecID(t, repo, "s1", "exec-1")

	stepGetter := newMockStepGetter()
	seedStuckSignalWorkflow(stepGetter)
	stepGetter.steps["step2"].CancelTriggersTurnComplete = true
	stepGetter.steps["step2"].Events = wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{
		Type: wfmodels.OnTurnCompleteMoveToNext,
	}}}
	stepGetter.steps["step3"] = &wfmodels.WorkflowStep{ID: "step3", WorkflowID: "wf1", Name: "Step 3", Position: 2}

	turns := &repoTurnService{repo: repo}
	if _, err := turns.StartTurn(ctx, "s1"); err != nil {
		t.Fatalf("start stuck turn: %v", err)
	}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	agentMgr.cancelAgentEntered = make(chan struct{}, 1)
	agentMgr.cancelAgentBlock = make(chan struct{})
	agentMgr.currentPromptExecutionID = "exec-1"
	agentMgr.currentPromptGeneration.Store(1)
	agentMgr.currentPromptActivityEpoch.Store(1)
	agentMgr.currentPromptLastActivityAt = time.Now().UTC().Add(-15 * time.Minute)
	svc := createEngineService(t, repo, stepGetter, agentMgr)
	svc.turnService = turns
	seedOverdueStuckSignal(t, repo, "s1")

	watchdogDone := make(chan struct{})
	go func() {
		defer close(watchdogDone)
		svc.reclaimStuckSignalSessionsOnce(ctx)
	}()
	select {
	case <-agentMgr.cancelAgentEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watchdog cancellation")
	}
	explicitDone := make(chan error, 1)
	go func() { explicitDone <- svc.CancelAgent(ctx, "s1") }()
	operation := svc.currentCancellation("s1")
	if operation == nil {
		t.Fatal("no shared cancellation operation registered")
	}
	select {
	case <-operation.joined:
	case <-time.After(2 * time.Second):
		t.Fatal("explicit cancellation did not join watchdog operation")
	}
	close(agentMgr.cancelAgentBlock)
	select {
	case <-watchdogDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for watchdog")
	}
	select {
	case err := <-explicitDone:
		if err != nil {
			t.Fatalf("joined explicit cancellation failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for joined cancellation")
	}

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.WorkflowStepID != "step2" {
		t.Fatalf("task step = %q, want step2 after one completion", task.WorkflowStepID)
	}
}
