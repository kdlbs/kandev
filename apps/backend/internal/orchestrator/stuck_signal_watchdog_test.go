package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
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
	// createEngineService leaves turnService nil, which would make
	// completeTurnForSession's activeTurns.Delete unreachable (see
	// completeTurnForTaskSessionCheckedOwned's turnService==nil early
	// return) and silently defeat the activeTurns.Store assertion below.
	// Wire a real turn service so that reclaiming the stuck turn is what
	// actually clears activeTurns, not an artifact of the guard never
	// firing.
	svc.turnService = &repoTurnService{repo: repo}

	signal := models.PendingStepCompletionSignal{
		StepID:     "step1",
		Source:     models.StepCompletionSourceAgent,
		Summary:    "all done",
		SignaledAt: time.Now().UTC().Add(-30 * time.Minute),
	}
	if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
		t.Fatalf("seed pending signal: %v", err)
	}

	// Simulate the exact stale bookkeeping the watchdog exists to clear: an
	// activeTurns entry left over from the turn that never reached turn-end.
	// Note this does NOT gate the prompt-sent-vs-queued assertion below —
	// flipStaleRunningToWaiting (the only production reader of activeTurns on
	// this path) short-circuits on session.State before it ever reaches the
	// activeTurns check, because the watchdog's own updateTaskSessionState
	// call (stuck_signal_watchdog.go) already flips state to
	// WAITING_FOR_INPUT first. What this entry lets us assert is the
	// watchdog's own cleanup of that stale bookkeeping (below), independent
	// of the prompt-delivery assertion.
	svc.activeTurns.Store("s1", "turn-1")

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

	// The auto-start prompt sent above dispatches a fresh turn, which
	// re-populates activeTurns with a new turn ID — so "cleared" isn't the
	// right postcondition. What proves the watchdog actually closed the
	// stale bookkeeping (rather than the new turn silently overwriting it)
	// is that the stale turn ID from before the reclaim is gone.
	if turnIDVal, tracked := svc.activeTurns.Load("s1"); tracked && turnIDVal == "turn-1" {
		t.Error("expected the watchdog to close the stale turn-1 activeTurns entry, but it is still present")
	}

	status := svc.messageQueue.GetStatus(ctx, "s1")
	if status.Count > 0 {
		t.Fatalf("expected no queued prompt (it should have been sent directly), got count=%d", status.Count)
	}
}

// TestReclaimStuckSignalSessionsOnce_SkipsWhenStillActive is the negative
// control both Review round 2 (Finding 3) and round 3 (Finding 2) require:
// signal age alone is not inactivity. A RUNNING session with a signal well
// past stuckSignalWatchdogThreshold (30 minutes old) whose agent's tracked
// prompt activity is recent (30 seconds old) must not be reclaimed. This is
// the case an epoch-equality check across the watchdog's own sub-millisecond
// scan window cannot see: a live agent producing events every few seconds
// sails straight through that window unchanged, so only gating on real
// elapsed inactivity (time.Since(lastActivityAt) >=
// stuckSignalWatchdogThreshold) catches it. Without this guard, the watchdog
// could force-close a turn that is still genuinely in flight (a long tool
// call, a provider retry/backoff that outlasts the signal-age threshold) and
// dispatch a second prompt into the same live session — the exact "quiet
// corruption" this watchdog exists to prevent, now caused by the watchdog
// itself.
func TestReclaimStuckSignalSessionsOnce_SkipsWhenStillActive(t *testing.T) {
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
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	agentMgr.currentPromptExecutionID = "exec-1"
	agentMgr.currentPromptGeneration.Store(1)
	agentMgr.currentPromptActivityEpoch.Store(1)
	agentMgr.currentPromptLastActivityAt = time.Now().UTC().Add(-30 * time.Second)
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

	updatedTask, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updatedTask.WorkflowStepID != "step1" {
		t.Fatalf("expected no transition while the agent is still active, got %q", updatedTask.WorkflowStepID)
	}
	updatedSession, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if updatedSession.State != models.TaskSessionStateRunning {
		t.Fatalf("expected session to remain RUNNING while still active, got %q", updatedSession.State)
	}
	if _, hasSignal := models.LoadPendingStepSignal(updatedSession.Metadata); !hasSignal {
		t.Error("expected the pending signal to remain untouched — the watchdog must not have reclaimed")
	}
}

// TestReclaimStuckSignalSessionsOnce_ExcludesPassthrough is Review round 4's
// BLOCKING finding: passthrough (PTY) sessions manage their own RUNNING/idle
// transitions (MarkPassthroughRunning) and never write lastActivityAt — every
// writer of that field (armPromptActivity, markAgentActivity,
// recordSteerActivity, recordActivity) sits on the ACP path only. A tracked
// passthrough execution therefore reports a zero-value lastActivityAt, which
// a naive now.Sub(lastActivityAt) >= threshold check reads as "infinitely
// inactive" and reclaims unconditionally. Reclaiming a passthrough session
// force-closes its turn and, per processOnEnter's PTY prompt path
// (event_handlers_workflow.go), writes the next step's auto-start prompt
// directly into the live agent's stdin — corrupting a healthy, still-running
// PTY agent. Sibling guards (flipStaleRunningToWaiting, markIdleAfterReset)
// already exclude passthrough for exactly this reason; this watchdog's
// candidate filter must too.
func TestReclaimStuckSignalSessionsOnce_ExcludesPassthrough(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1") // seeds session RUNNING
	setSessionExecID(t, repo, "s1", "exec-1")

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.IsPassthrough = true
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("seed passthrough session: %v", err)
	}

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

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isPassthrough: true, isAgentRunning: true}
	// A tracked execution whose lastActivityAt was never written — the exact
	// shape GetPromptActivityForSession reports in production for a
	// passthrough session (see manager_interaction.go: only ACP-path callers
	// write promptActivitySnapshot). currentPromptLastActivityAt is left at
	// its zero value deliberately.
	agentMgr.currentPromptExecutionID = "exec-1"
	agentMgr.currentPromptGeneration.Store(1)
	agentMgr.currentPromptActivityEpoch.Store(1)
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

	updatedTask, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updatedTask.WorkflowStepID != "step1" {
		t.Fatalf("expected no transition for a passthrough session, got %q — "+
			"a healthy PTY agent's turn was force-closed and the next step's "+
			"prompt would be written straight into its live stdin", updatedTask.WorkflowStepID)
	}
	updatedSession, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if updatedSession.State != models.TaskSessionStateRunning {
		t.Fatalf("expected passthrough session to remain RUNNING, got %q", updatedSession.State)
	}
	if _, hasSignal := models.LoadPendingStepSignal(updatedSession.Metadata); !hasSignal {
		t.Error("expected the pending signal to remain untouched on a passthrough session — the watchdog must not have reclaimed")
	}
}

// TestReclaimStuckSignalSessionsOnce_ReclaimsOnGenuineInactivity is the
// positive test Review round 4 found missing: every existing reclaim test
// (_ReclaimsStuckRunningSession, _UnblocksAutoStart) passes through
// GetPromptActivityForSession's fail-open ErrNoExecutionForSession branch —
// "no execution tracked at all" — never the branch that actually reads a
// tracked execution's elapsed activity and finds it genuinely stale. Round
// 5's fail-closed rewrite of stuckSignalInactiveLongEnough moves those two
// tests further onto that same no-execution branch, so without this test the
// watchdog's actual "no activity for N minutes" purpose would still have
// zero coverage proving it fires. Here the execution IS tracked, is not
// passthrough, and lastActivityAt is a real non-zero timestamp older than
// stuckSignalWatchdogThreshold — the one shape that must reclaim.
func TestReclaimStuckSignalSessionsOnce_ReclaimsOnGenuineInactivity(t *testing.T) {
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
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	agentMgr.currentPromptExecutionID = "exec-1"
	agentMgr.currentPromptGeneration.Store(1)
	agentMgr.currentPromptActivityEpoch.Store(1)
	// Genuinely stale: well past stuckSignalWatchdogThreshold (10m), and a
	// real non-zero timestamp — not the zero value a never-initialized
	// tracked execution would report.
	agentMgr.currentPromptLastActivityAt = time.Now().UTC().Add(-15 * time.Minute)
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

	updatedTask, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updatedTask.WorkflowStepID != "step2" {
		t.Fatalf("expected the watchdog to reclaim a session with genuinely stale tracked "+
			"activity and apply the pending transition to step2, got %q", updatedTask.WorkflowStepID)
	}
	updatedSession, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if updatedSession.State == models.TaskSessionStateRunning {
		t.Fatal("expected the session to no longer be RUNNING after a genuine-inactivity reclaim")
	}
	if _, hasSignal := models.LoadPendingStepSignal(updatedSession.Metadata); hasSignal {
		t.Error("expected the applied signal to be cleared from the session bag")
	}
}

// TestReclaimStuckSignalSessionsOnce_SettlesStuckExecutionBeforeReclaiming is
// the regression test for PR #2975 review Thread 2: the reclaim path
// (completeTurnForSession -> updateTaskSessionState ->
// reconcileStepCompletionSignalLocked) mutates task/session state but never
// settles the stuck execution's lifecycle-level prompt wait
// (promptMu/promptDoneCh/promptFinished in
// agent/runtime/lifecycle/manager_interaction.go). Without settling it first,
// a subsequent auto-start re-prompt resolves to the SAME still-stuck
// execution (GetExecutionBySession) and deadlocks behind it — the reclaim
// looks successful in the DB but the card never actually recovers. The fix
// must call the existing CancelAgent/escalateStuckCancel primitive (via
// cancelAgentWhileUnlocked) to settle the execution BEFORE applying any of
// the reclaim's state mutations, not just before returning from the
// function.
func TestReclaimStuckSignalSessionsOnce_SettlesStuckExecutionBeforeReclaiming(t *testing.T) {
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
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	agentMgr.currentPromptExecutionID = "exec-1"
	agentMgr.currentPromptGeneration.Store(1)
	agentMgr.currentPromptActivityEpoch.Store(1)
	// Genuinely stale, exactly like _ReclaimsOnGenuineInactivity — this
	// session must clear the inactivity gate and reach the reclaim.
	agentMgr.currentPromptLastActivityAt = time.Now().UTC().Add(-15 * time.Minute)

	var cancelSawSessionStillRunning, cancelSawStepStillOne bool
	agentMgr.cancelAgentFunc = func(_ context.Context, sessionID string) error {
		if sessionID != "s1" {
			t.Errorf("CancelAgent called with unexpected session ID %q", sessionID)
		}
		if session, err := repo.GetTaskSession(ctx, sessionID); err == nil && session.State == models.TaskSessionStateRunning {
			cancelSawSessionStillRunning = true
		}
		if task, err := repo.GetTask(ctx, "t1"); err == nil && task.WorkflowStepID == "step1" {
			cancelSawStepStillOne = true
		}
		// Mirrors the real lifecycle manager's behavior when a stuck agent
		// never acknowledges cancel: escalation runs and this sentinel is
		// returned. cancelAgentWhileUnlocked already tolerates it.
		return lifecycle.ErrCancelEscalated
	}

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

	if got := agentMgr.cancelAgentCalls.Load(); got != 1 {
		t.Fatalf("expected the watchdog to call CancelAgent exactly once to settle the stuck execution before reclaiming, got %d calls", got)
	}
	if !cancelSawSessionStillRunning {
		t.Error("expected CancelAgent to be called before the reclaim flips the session out of RUNNING — settling the execution must happen first, not after")
	}
	if !cancelSawStepStillOne {
		t.Error("expected CancelAgent to be called before the reclaim applies the pending step transition — settling the execution must happen first, not after")
	}

	updatedTask, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updatedTask.WorkflowStepID != "step2" {
		t.Fatalf("expected the watchdog to still apply the pending transition to step2 after settling the execution, got %q", updatedTask.WorkflowStepID)
	}
}

func TestReclaimStuckSignalSessionsOnce_RetriesWaitingSignalAfterReconcileFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	seedStuckSignalWorkflow(stepGetter)
	firstLookup := true
	stepGetter.getStepFunc = func(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
		if firstLookup {
			firstLookup = false
			return nil, errors.New("transient workflow step lookup failure")
		}
		if step, ok := stepGetter.steps[stepID]; ok {
			return step, nil
		}
		return nil, nil
	}
	// There is no tracked execution, so the lifecycle inactivity gate has
	// positive evidence that the process is gone and the watchdog can reclaim.
	svc := createEngineService(t, repo, stepGetter, &mockAgentManager{})
	svc.turnService = &repoTurnService{repo: repo}
	seedOverdueStuckSignal(t, repo, "s1")

	svc.reclaimStuckSignalSessionsOnce(ctx)
	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task after first tick: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("task advanced after failed reconciliation, got %q", task.WorkflowStepID)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session after first tick: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state after first tick = %q, want WAITING_FOR_INPUT", session.State)
	}
	if _, ok := models.LoadPendingStepSignal(session.Metadata); !ok {
		t.Fatal("pending signal was lost after failed reconciliation")
	}

	svc.reclaimStuckSignalSessionsOnce(ctx)
	task, err = repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task after retry: %v", err)
	}
	if task.WorkflowStepID != "step2" {
		t.Fatalf("task step after retry = %q, want step2", task.WorkflowStepID)
	}
	session, err = repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session after retry: %v", err)
	}
	if _, ok := models.LoadPendingStepSignal(session.Metadata); ok {
		t.Fatal("pending signal remained after successful retry")
	}
}
