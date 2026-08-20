package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// TestPauseForClarificationInput_DrainsPeerQueueAfterPause pins the T1
// contract with the real detaching canceller and a deterministic dispatch
// barrier. Pause must detach the clarification, reserve the queued peer, and
// leave the detached question pending for a late answer.
func TestPauseForClarificationInput_DrainsPeerQueueAfterPause(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	setSessionExecID(t, repo, "s1", "exec-1")
	seedPendingClarificationMessage(t, repo, "t1", "s1")

	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
	}
	store := clarification.NewStore(time.Minute)
	store.CreateRequest(&clarification.Request{PendingID: "pending-s1", SessionID: "s1"})
	canceller := clarification.NewCanceller(store, repo, nil, testLogger())
	svc := createEngineService(t, repo, newMockStepGetter(), agentMgr)
	svc.SetClarificationCanceller(canceller)
	svc.turnService = &repoBackedTurnService{repo: repo}
	workerDone := make(chan struct{})
	svc.onQueuedMessageExecutionComplete = func() { close(workerDone) }

	// Seed a peer message. Distinct QueuedBy to avoid the queue's
	// admission-time auto-merge collapsing it.
	_, err := svc.messageQueue.QueueMessageWithMetadata(ctx, "s1", "t1", "peer-1", "", "user-1", false, nil, map[string]interface{}{})
	if err != nil {
		t.Fatalf("queue peer: %v", err)
	}
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 1 {
		t.Fatalf("pre-pause queue count = %d, want 1", got)
	}

	detached, err := svc.PauseForClarificationInput(ctx, "s1")
	if err != nil {
		t.Fatalf("PauseForClarificationInput: %v", err)
	}
	if detached != 1 {
		t.Fatalf("expected one detached bundle, got %d", detached)
	}
	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for queued peer worker")
	}
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Fatalf("post-pause queue count = %d, want 0 after reservation", got)
	}
	clarificationMessage, err := repo.GetMessage(ctx, "clarification-s1")
	if err != nil {
		t.Fatalf("load detached clarification: %v", err)
	}
	if clarificationMessage.Metadata["status"] != "pending" || clarificationMessage.Metadata["agent_disconnected"] != true {
		t.Fatalf("detached clarification metadata = %#v, want pending and disconnected", clarificationMessage.Metadata)
	}
}

// TestPauseForClarificationInput_SameTurnBarrierUnchanged pins the
// contract that BEFORE T1 fires (i.e., on a turn that's still in the
// same turn the clarification was asked), the workflow barrier still
// blocks. The detached bundle's turn_id stays equal to the current
// turn until the drain starts a new turn. The barrier must hold for
// the current turn regardless of T1.
func TestPauseForClarificationInput_SameTurnBarrierUnchanged(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateRunning
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session state: %v", err)
	}
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	setSessionExecID(t, repo, "s1", "exec-1")
	seedPendingClarificationMessage(t, repo, "t1", "s1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Plan", Position: 0,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToNext},
			},
		},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Implement", Position: 1,
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, stepGetter, agentMgr)
	svc.turnService = &repoBackedTurnService{repo: repo}

	// Manually call handleAgentReady without ever going through
	// PauseForClarificationInput — the same-turn barrier must hold.
	svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("same-turn barrier broken: workflow step = %q, want step1", task.WorkflowStepID)
	}
}

// TestPauseForClarificationInput_NewTurnAfterDrainAdvancesWorkflow verifies
// that the dispatched successor can complete its turn and advance the
// workflow while the detached clarification remains answerable.
func TestPauseForClarificationInput_NewTurnAfterDrainAdvancesWorkflow(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	setSessionExecID(t, repo, "s1", "exec-1")
	seedPendingClarificationMessage(t, repo, "t1", "s1")
	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Plan", Position: 0,
		Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToNext}}},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Name: "Implement", Position: 1}
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	store := clarification.NewStore(time.Minute)
	store.CreateRequest(&clarification.Request{PendingID: "pending-s1", SessionID: "s1"})
	svc := createEngineService(t, repo, stepGetter, agentMgr)
	svc.SetClarificationCanceller(clarification.NewCanceller(store, repo, nil, testLogger()))
	svc.turnService = &repoBackedTurnService{repo: repo}
	workerDone := make(chan struct{})
	svc.onQueuedMessageExecutionComplete = func() { close(workerDone) }
	_, err := svc.messageQueue.QueueMessageWithMetadata(ctx, "s1", "t1", "peer-1", "", "user-1", false, nil, map[string]interface{}{})
	if err != nil {
		t.Fatalf("queue peer: %v", err)
	}
	if _, err := svc.PauseForClarificationInput(ctx, "s1"); err != nil {
		t.Fatalf("PauseForClarificationInput: %v", err)
	}
	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for successor worker")
	}
	turn, err := repo.GetActiveTurnBySessionID(ctx, "s1")
	if err != nil || turn == nil {
		t.Fatalf("successor turn = %#v, err=%v", turn, err)
	}
	svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"})
	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("load task after successor completion: %v", err)
	}
	if task.WorkflowStepID != "step2" {
		t.Fatalf("successor completion did not advance workflow: step=%q, want step2", task.WorkflowStepID)
	}
	clarificationMessage, err := repo.GetMessage(ctx, "clarification-s1")
	if err != nil {
		t.Fatalf("load detached clarification: %v", err)
	}
	if clarificationMessage.Metadata["status"] != "pending" || clarificationMessage.Metadata["agent_disconnected"] != true {
		t.Fatalf("detached clarification metadata = %#v, want pending and disconnected", clarificationMessage.Metadata)
	}
}

// TestPauseForClarificationInput_CancelledRunSkipsDrain pins the safety
// contract: when PauseForClarificationInput early-returns because the
// turn is terminal (e.g., already Cancelled), T1's drain must not run.
// The terminal early-return path (line 566-568) is before the
// cancelAgentSilentExpectedWithGuard call and therefore before line 597
// drain.
func TestPauseForClarificationInput_CancelledRunSkipsDrain(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateCancelled)
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	setSessionExecID(t, repo, "s1", "exec-1")
	seedPendingClarificationMessage(t, repo, "t1", "s1")

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, newMockStepGetter(), agentMgr)
	svc.turnService = &repoBackedTurnService{repo: repo}

	_, err := svc.messageQueue.QueueMessageWithMetadata(ctx, "s1", "t1", "peer-1", "user-4", "user-4", false, nil, map[string]interface{}{})
	if err != nil {
		t.Fatalf("queue peer: %v", err)
	}

	detached, err := svc.PauseForClarificationInput(ctx, "s1")
	if err != nil {
		t.Fatalf("PauseForClarificationInput: %v", err)
	}
	if detached != 0 {
		t.Fatalf("terminal session should not detach, got %d", detached)
	}

	// Queue must still hold the peer message — drain was bypassed.
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 1 {
		t.Fatalf("terminal early-return drained queue (count=%d), want 1", got)
	}
}
