package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// TestReclaimStuckSignalSessionsOnce_ReclaimsStuckRunningSession covers facet
// (a) of the WO-38 regression: a session accepted a step_complete_kandev
// signal and then never reached turn-end or a failure event at all — neither
// the in-process stall ticker nor the idle-session reaper catches this (see
// stuck_signal_watchdog.go's package doc). The watchdog must reclaim the
// session and apply the transition the agent asked for, rather than leaving
// it silently RUNNING forever.
func TestReclaimStuckSignalSessionsOnce_ReclaimsStuckRunningSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1") // seeds session RUNNING

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		AutoAdvanceRequiresSignal: true,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToNext},
			},
		},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
	}
	svc := createTestService(repo, stepGetter, newMockTaskRepo())

	// The agent called step_complete_kandev well over the watchdog's
	// threshold ago, but the turn never reached turn-end and never failed —
	// no bus event will ever fire onStepCompletionSignaled for this session.
	signal := models.PendingStepCompletionSignal{
		StepID:     "step1",
		Source:     models.StepCompletionSourceAgent,
		Summary:    "all done",
		SignaledAt: time.Now().UTC().Add(-30 * time.Minute),
	}
	if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
		t.Fatalf("seed pending signal: %v", err)
	}

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Fatalf("precondition: session should start RUNNING, got %q", session.State)
	}

	svc.reclaimStuckSignalSessionsOnce(ctx)

	updatedTask, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updatedTask.WorkflowStepID != "step2" {
		t.Fatalf("expected watchdog to apply the pending transition to step2, got %q", updatedTask.WorkflowStepID)
	}
	updatedSession, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if updatedSession.State == models.TaskSessionStateRunning {
		t.Fatal("session is still silently RUNNING after the watchdog ran — the exact regression WO-38 fixes")
	}
	if _, hasSignal := models.LoadPendingStepSignal(updatedSession.Metadata); hasSignal {
		t.Error("expected the applied signal to be cleared from the session bag")
	}
}

// TestReclaimStuckSignalSessionsOnce_BelowThreshold pins the watchdog's
// threshold gate. Without this, a positive-only suite couldn't distinguish
// "the watchdog reclaims because the signal is overdue" from "the watchdog
// reclaims any RUNNING session holding a signal" — the latter would race an
// agent turn that is about to end normally.
func TestReclaimStuckSignalSessionsOnce_BelowThreshold(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		AutoAdvanceRequiresSignal: true,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToNext},
			},
		},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
	}
	svc := createTestService(repo, stepGetter, newMockTaskRepo())

	signal := models.PendingStepCompletionSignal{
		StepID:     "step1",
		Source:     models.StepCompletionSourceAgent,
		Summary:    "all done",
		SignaledAt: time.Now().UTC().Add(-1 * time.Minute), // well under threshold
	}
	if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
		t.Fatalf("seed pending signal: %v", err)
	}

	svc.reclaimStuckSignalSessionsOnce(ctx)

	updatedTask, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updatedTask.WorkflowStepID != "step1" {
		t.Fatalf("expected no transition below threshold, got %q", updatedTask.WorkflowStepID)
	}
	updatedSession, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if updatedSession.State != models.TaskSessionStateRunning {
		t.Fatalf("expected session to remain RUNNING below threshold, got %q", updatedSession.State)
	}
}

// TestReclaimStuckSignalSessionsOnce_UnblocksAutoStart covers facet (b): the
// regression that actually matters, not just that the watchdog notices the
// stuck session. Before this watchdog existed, a card in this state could
// not be rescued even by an ordinary board move — flipStaleRunningToWaiting
// declines while activeTurns still holds the stuck turn, and
// queueAutoStartPromptIfRunning queues the auto-start prompt into a turn-end
// that will never arrive instead of sending it (see stuck_signal_watchdog.go's
// package doc). This test proves that once the watchdog reclaims the
// session, the target step's auto_start_agent on_enter action actually sends
// the prompt via PromptAgent — not queues it.
func TestReclaimStuckSignalSessionsOnce_UnblocksAutoStart(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1") // seeds session RUNNING
	setSessionExecID(t, repo, "s1", "exec-1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		AutoAdvanceRequiresSignal: true,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToNext},
			},
		},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{
				{Type: wfmodels.OnEnterAutoStartAgent},
			},
		},
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	agentMgr.promptDone = make(chan struct{})
	svc := createEngineService(t, repo, stepGetter, agentMgr)

	signal := models.PendingStepCompletionSignal{
		StepID:     "step1",
		Source:     models.StepCompletionSourceAgent,
		Summary:    "all done",
		SignaledAt: time.Now().UTC().Add(-30 * time.Minute),
	}
	if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
		t.Fatalf("seed pending signal: %v", err)
	}

	svc.reclaimStuckSignalSessionsOnce(ctx)

	select {
	case <-agentMgr.promptDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the auto-start prompt to be sent — " +
			"regression: prompt queued behind a turn-end that never arrives")
	}

	agentMgr.mu.Lock()
	prompted := len(agentMgr.capturedPrompts) > 0
	agentMgr.mu.Unlock()
	if !prompted {
		t.Fatal("expected the auto-start prompt to be sent via PromptAgent, not queued")
	}

	status := svc.messageQueue.GetStatus(ctx, "s1")
	if status.Count > 0 {
		t.Fatalf("expected no queued prompt (it should have been sent directly), got count=%d", status.Count)
	}
}
