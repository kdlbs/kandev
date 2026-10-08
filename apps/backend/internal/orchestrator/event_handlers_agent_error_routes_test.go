package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// --- AC-A1 route coverage: all five production fire sites reach the real
// dispatch, with the workflow engine actually wired (agentErrorDeps
// populated). R4/R5 previously only exercised via newTransientTestService,
// which never calls initWorkflowEngine — this closed that gap. Each also
// asserts the payload the engine receives (AC-D1/D2/D3), per this spec's
// Verification text for AC-A1/A2. ---

// TestDispatchKanbanAgentErrorTrigger_R1BusDrivenFailureDispatches verifies that
// bus-delivered agent failures reach workflow recovery after cleanup.
func TestDispatchKanbanAgentErrorTrigger_R1BusDrivenFailureDispatches(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
	}
	decisions := &spyDecisionStore{}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })
	captured := agentErrorCapturePayload(t, svc, engine.ActionClearDecisions)

	svc.handleAgentFailed(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "agent crashed",
	})

	waitForFailureRecovery(t, svc)
	if decisions.clearCalls != 1 {
		t.Fatalf("clearCalls = %d, want 1 (R1's bus-driven entry point must reach the real dispatch)", decisions.clearCalls)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Fatalf("got %d dispatch INFO records, want 1", len(got))
	}
	if len(*captured) != 1 {
		t.Fatalf("got %d payload(s), want 1", len(*captured))
	}
	if got := (*captured)[0]; got.FailedSessionID != "s1" || got.ErrorMessage != "agent crashed" {
		t.Errorf("payload = %+v, want FailedSessionID=s1 ErrorMessage=%q", got, "agent crashed")
	}
}

// TestDispatchKanbanAgentErrorTrigger_ManagedRuntimeStartupFailureDispatches verifies
// workflow recovery for exhausted managed-runtime startup recovery.
func TestDispatchKanbanAgentErrorTrigger_ManagedRuntimeStartupFailureDispatches(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
	}
	decisions := &spyDecisionStore{}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })
	captured := agentErrorCapturePayload(t, svc, engine.ActionClearDecisions)

	startupErr := fmt.Errorf("failed to initialize ACP: %w", &routingerr.ManagedRuntimeStartupError{
		Code:    routingerr.CodeManagedRuntimeStartup,
		Details: "reason=npm_transient attempts=2 npm_code=ECONNRESET",
	})
	handled := svc.handleAgentStartFailed(ctx, "t1", "s1", "exec-1", startupErr, false)

	if !handled {
		t.Fatal("expected handled=true for a managed runtime startup failure")
	}
	waitForFailureRecovery(t, svc)
	if decisions.clearCalls != 1 {
		t.Fatalf("clearCalls = %d, want 1 (managed startup failures must reach the real dispatch)", decisions.clearCalls)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Fatalf("got %d dispatch INFO records, want 1", len(got))
	}
	if len(*captured) != 1 {
		t.Fatalf("got %d payload(s), want 1", len(*captured))
	}
	wantMsg := "managed runtime startup failed"
	if got := (*captured)[0]; got.FailedSessionID != "s1" || got.ErrorMessage != wantMsg {
		t.Errorf("payload = %+v, want FailedSessionID=s1 ErrorMessage=%q", got, wantMsg)
	}
}

// TestDispatchKanbanAgentErrorTrigger_R3AuthErrorDispatches verifies workflow
// recovery for authentication failures during agent startup.
func TestDispatchKanbanAgentErrorTrigger_R3AuthErrorDispatches(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
	}
	decisions := &spyDecisionStore{}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })
	captured := agentErrorCapturePayload(t, svc, engine.ActionClearDecisions)

	handled := svc.handleAgentStartFailed(
		ctx, "t1", "s1", "exec-1", errors.New("authentication required: please log in"), false,
	)

	if !handled {
		t.Fatal("expected handled=true for an auth-error start failure")
	}
	waitForFailureRecovery(t, svc)
	if decisions.clearCalls != 1 {
		t.Fatalf("clearCalls = %d, want 1 (R3's auth-error branch must reach the real dispatch)", decisions.clearCalls)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Fatalf("got %d dispatch INFO records, want 1", len(got))
	}
	if len(*captured) != 1 {
		t.Fatalf("got %d payload(s), want 1", len(*captured))
	}
	wantMsg := "authentication required: please log in"
	if got := (*captured)[0]; got.FailedSessionID != "s1" || got.ErrorMessage != wantMsg {
		t.Errorf("payload = %+v, want FailedSessionID=s1 ErrorMessage=%q", got, wantMsg)
	}
}

func TestDispatchKanbanAgentErrorTrigger_ManagedStartupCausePreservesAuthRecovery(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
	}
	decisions := &spyDecisionStore{}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })
	captured := agentErrorCapturePayload(t, svc, engine.ActionClearDecisions)
	startupErr := &routingerr.ManagedRuntimeStartupError{
		Code:     routingerr.CodeManagedRuntimeStartup,
		Details:  "reason=retry_initialize_failed attempts=2",
		Reason:   "retry_initialize_failed",
		Attempts: 2,
		Cause:    errors.New("Authentication required: please log in"),
	}

	if !svc.handleAgentStartFailed(ctx, "t1", "s1", "exec-1", startupErr, false) {
		t.Fatal("expected managed startup error with an auth cause to be handled")
	}
	waitForFailureRecovery(t, svc)
	if decisions.clearCalls != 1 || len(*captured) != 1 {
		t.Fatalf("auth recovery dispatches = %d decisions, %d payloads; want one each", decisions.clearCalls, len(*captured))
	}
	if got := filterLogs(logs, "agent start failure is auth error, treating as recoverable"); len(got) != 1 {
		t.Fatalf("auth recovery log count = %d, want 1", len(got))
	}
	if got := filterLogs(logs, "managed runtime startup failure is recoverable"); len(got) != 0 {
		t.Fatalf("auth cause entered generic managed-runtime recovery: %#v", got)
	}
	if got := (*captured)[0].ErrorMessage; !strings.Contains(got, "Authentication required") {
		t.Fatalf("recovery dispatch lost the final auth diagnostic: %q", got)
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("load recovered session: %v", err)
	}
	lastErr, ok := models.LoadLastAgentError(session.Metadata)
	if !ok || lastErr.Code != string(routingerr.CodeAuthRequired) ||
		lastErr.StartupReason != "retry_initialize_failed" || lastErr.StartupAttempts != 2 {
		t.Fatalf("persisted auth startup failure = %#v, want auth code, final reason, and two attempts", lastErr)
	}
}

// TestDispatchKanbanAgentErrorTrigger_R4NoCachedPromptDispatches verifies that
// a retry without a cached prompt falls back to workflow recovery.
func TestDispatchKanbanAgentErrorTrigger_R4NoCachedPromptDispatches(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
	}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	decisions := &spyDecisionStore{}
	svc, logs := newAgentErrorTransientTestService(t, repo, stepGetter, agentMgr, func(s *Service) {
		s.engineDecisions = decisions
	})
	t.Cleanup(svc.cancelAllTransientRetries)
	captured := agentErrorCapturePayload(t, svc, engine.ActionClearDecisions)

	svc.scheduleTransientRetry("t1", "s1", "", 1, time.Hour)
	svc.retryTransientPrompt(ctx, "t1", "s1", "")

	waitForFailureRecovery(t, svc)
	if decisions.clearCalls != 1 {
		t.Fatalf("clearCalls = %d, want 1 (R4's no-cached-prompt path must reach the real dispatch)", decisions.clearCalls)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Fatalf("got %d dispatch INFO records, want 1", len(got))
	}
	if len(*captured) != 1 {
		t.Fatalf("got %d payload(s), want 1", len(*captured))
	}
	wantMsg := "Automatic provider retry was not possible. Resume or start fresh to continue."
	if got := (*captured)[0]; got.FailedSessionID != "s1" || got.ErrorMessage != wantMsg {
		t.Errorf("payload = %+v, want FailedSessionID=s1 ErrorMessage=%q", got, wantMsg)
	}
}

// TestDispatchKanbanAgentErrorTrigger_R5SynchronousPromptErrorDispatches verifies
// that a failed replacement prompt dispatches recovery only after completed teardown.
func TestDispatchKanbanAgentErrorTrigger_R5SynchronousPromptErrorDispatches(t *testing.T) {
	for _, tc := range []struct {
		name         string
		stopErr      error
		wantDispatch int
	}{
		{"stopped", nil, 1},
		{"already absent", lifecycle.ErrExecutionNotFound, 1},
		{"stop failed", errors.New("runtime stop failed"), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testSynchronousPromptFailureDispatch(t, tc.stopErr, tc.wantDispatch)
		})
	}
}

// testSynchronousPromptFailureDispatch exercises workflow recovery after the
// transient retry owns teardown and its replacement prompt fails synchronously.
func testSynchronousPromptFailureDispatch(t *testing.T, stopErr error, wantDispatch int) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
	}
	agentMgr := &mockAgentManager{
		repoForExecutionLookup:  repo,
		promptErr:               errors.New("session rejected prompt synchronously"),
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error { return stopErr },
	}
	decisions := &spyDecisionStore{}
	svc, logs := newAgentErrorTransientTestService(t, repo, stepGetter, agentMgr, func(s *Service) {
		s.engineDecisions = decisions
	})
	t.Cleanup(svc.cancelAllTransientRetries)
	captured := agentErrorCapturePayload(t, svc, engine.ActionClearDecisions)

	svc.rememberTurnPrompt("s1", "hello", "", false, nil)
	svc.transientRetries.Store("s1", &transientRetryEntry{attempt: 1, cancel: func() {}})

	svc.retryTransientPrompt(ctx, "t1", "s1", "exec-1")

	waitForFailureRecovery(t, svc)
	if decisions.clearCalls != wantDispatch {
		t.Fatalf("clearCalls = %d, want %d after transient teardown", decisions.clearCalls, wantDispatch)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != wantDispatch {
		t.Fatalf("got %d dispatch INFO records, want %d", len(got), wantDispatch)
	}
	if len(*captured) != wantDispatch {
		t.Fatalf("got %d payload(s), want %d", len(*captured), wantDispatch)
	}
	if wantDispatch == 0 {
		return
	}
	wantMsg := "Automatic provider retry could not be started. Resume or start fresh to continue."
	if got := (*captured)[0]; got.FailedSessionID != "s1" || got.ErrorMessage != wantMsg {
		t.Errorf("payload = %+v, want FailedSessionID=s1 ErrorMessage=%q", got, wantMsg)
	}
}
