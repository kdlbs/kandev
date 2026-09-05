package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

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

func TestDispatchKanbanAgentErrorTrigger_R2ManagedRuntimeNpmFailureDispatches(t *testing.T) {
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

	npmErr := fmt.Errorf("failed to initialize ACP: %w", &routingerr.ManagedRuntimeStartupError{
		Code:    routingerr.CodeManagedRuntimeNpmResolution,
		Details: "npm error code ETARGET",
	})
	handled := svc.handleAgentStartFailed(ctx, "t1", "s1", "exec-1", npmErr, false)

	if !handled {
		t.Fatal("expected handled=true for a managed npm runtime startup failure")
	}
	if decisions.clearCalls != 1 {
		t.Fatalf("clearCalls = %d, want 1 (R2's npm-resolution branch must reach the real dispatch)", decisions.clearCalls)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Fatalf("got %d dispatch INFO records, want 1", len(got))
	}
	if len(*captured) != 1 {
		t.Fatalf("got %d payload(s), want 1", len(*captured))
	}
	wantMsg := "managed npm runtime failed to prepare"
	if got := (*captured)[0]; got.FailedSessionID != "s1" || got.ErrorMessage != wantMsg {
		t.Errorf("payload = %+v, want FailedSessionID=s1 ErrorMessage=%q", got, wantMsg)
	}
}

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

func TestDispatchKanbanAgentErrorTrigger_R5SynchronousPromptErrorDispatches(t *testing.T) {
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
		repoForExecutionLookup: repo,
		promptErr:              errors.New("session rejected prompt synchronously"),
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

	if decisions.clearCalls != 1 {
		t.Fatalf("clearCalls = %d, want 1 (R5's synchronous PromptTask failure must reach the real dispatch)", decisions.clearCalls)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 1 {
		t.Fatalf("got %d dispatch INFO records, want 1", len(got))
	}
	if len(*captured) != 1 {
		t.Fatalf("got %d payload(s), want 1", len(*captured))
	}
	wantMsg := "Automatic provider retry could not be started. Resume or start fresh to continue."
	if got := (*captured)[0]; got.FailedSessionID != "s1" || got.ErrorMessage != wantMsg {
		t.Errorf("payload = %+v, want FailedSessionID=s1 ErrorMessage=%q", got, wantMsg)
	}
}
