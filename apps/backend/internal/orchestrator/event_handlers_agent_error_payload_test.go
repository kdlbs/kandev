package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// agentErrorCapturePayload swaps svc's agentErrorDeps for a copy whose
// registry wraps kind's real callback so the test can observe the
// engine.OnAgentErrorPayload the engine actually delivers, rather than the
// value the dispatch computed before calling it (AC-D1/D2/D3). Must run
// after svc.initWorkflowEngine() (or newAgentErrorTestService /
// newAgentErrorTransientTestService, which call it). The wrapped callback
// still forwards to the real one, so its side effects (e.g. clearCalls)
// are unaffected.
func agentErrorCapturePayload(t *testing.T, svc *Service, kind engine.ActionKind) *[]engine.OnAgentErrorPayload {
	t.Helper()
	deps := svc.agentErrorDeps.Load()
	if deps == nil {
		t.Fatal("agentErrorCapturePayload: agentErrorDeps not initialized")
	}
	captured := &[]engine.OnAgentErrorPayload{}
	registry := &payloadCapturingRegistry{inner: deps.registry, kind: kind, captured: captured}
	options := append([]engine.Option{engine.WithLogger(svc.logger)}, svc.engineOptions...)
	svc.agentErrorDeps.Store(&agentErrorDispatchDeps{
		engine:   engine.New(deps.store, registry, options...),
		registry: registry,
		store:    deps.store,
	})
	return captured
}

type payloadCapturingRegistry struct {
	inner    engine.CallbackRegistry
	kind     engine.ActionKind
	captured *[]engine.OnAgentErrorPayload
}

func (r *payloadCapturingRegistry) Get(kind engine.ActionKind) (engine.ActionCallback, bool) {
	cb, ok := r.inner.Get(kind)
	if !ok || kind != r.kind {
		return cb, ok
	}
	return payloadCapturingCallback{inner: cb, captured: r.captured}, true
}

type payloadCapturingCallback struct {
	inner    engine.ActionCallback
	captured *[]engine.OnAgentErrorPayload
}

func (c payloadCapturingCallback) Execute(ctx context.Context, in engine.ActionInput) (engine.ActionResult, error) {
	if p, ok := in.Payload.(engine.OnAgentErrorPayload); ok {
		*c.captured = append(*c.captured, p)
	}
	return c.inner.Execute(ctx, in)
}

// --- AC-D1/D2/D3: the payload the engine actually receives. No test
// anywhere previously observed OnAgentErrorPayload's fields. ---

func TestDispatchKanbanAgentErrorTrigger_PayloadFields(t *testing.T) {
	ctx := context.Background()

	t.Run("AC-D1/D3: FailedSessionID and an explicit ErrorMessage pass through unchanged", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
		}
		decisions := &spyDecisionStore{}
		svc, _ := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })
		captured := agentErrorCapturePayload(t, svc, engine.ActionClearDecisions)

		svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{
			TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom",
		})

		if len(*captured) != 1 {
			t.Fatalf("got %d payload(s), want 1", len(*captured))
		}
		got := (*captured)[0]
		if got.FailedSessionID != "s1" {
			t.Errorf("FailedSessionID = %q, want s1", got.FailedSessionID)
		}
		if got.ErrorMessage != "boom" {
			t.Errorf("ErrorMessage = %q, want boom", got.ErrorMessage)
		}
	})

	t.Run("AC-D3: an empty ErrorMessage defaults to the literal agent failed", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
		}
		decisions := &spyDecisionStore{}
		svc, _ := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })
		captured := agentErrorCapturePayload(t, svc, engine.ActionClearDecisions)

		svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{
			TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1",
		})

		if len(*captured) != 1 {
			t.Fatalf("got %d payload(s), want 1", len(*captured))
		}
		if got := (*captured)[0].ErrorMessage; got != "agent failed" {
			t.Errorf("ErrorMessage = %q, want the default %q", got, "agent failed")
		}
	})

	t.Run("AC-D2: an explicit event AgentProfileID is used directly", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
		}
		decisions := &spyDecisionStore{}
		svc, _ := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })
		captured := agentErrorCapturePayload(t, svc, engine.ActionClearDecisions)

		svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{
			TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", AgentProfileID: "profile-event",
		})

		if len(*captured) != 1 {
			t.Fatalf("got %d payload(s), want 1", len(*captured))
		}
		if got := (*captured)[0].FailedAgentID; got != "profile-event" {
			t.Errorf("FailedAgentID = %q, want the event's own profile-event", got)
		}
	})

	t.Run("AC-D2: an empty event AgentProfileID falls back to the session's own", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		session, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("get session: %v", err)
		}
		session.AgentProfileID = "profile-session"
		if err := repo.UpdateTaskSession(ctx, session); err != nil {
			t.Fatalf("update session: %v", err)
		}
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
		}
		decisions := &spyDecisionStore{}
		svc, _ := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })
		captured := agentErrorCapturePayload(t, svc, engine.ActionClearDecisions)

		svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{
			TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1",
		})

		if len(*captured) != 1 {
			t.Fatalf("got %d payload(s), want 1", len(*captured))
		}
		if got := (*captured)[0].FailedAgentID; got != "profile-session" {
			t.Errorf("FailedAgentID = %q, want the session's fallback profile-session", got)
		}
	})

	t.Run("AC-D2: both empty yields an empty FailedAgentID", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
		}
		decisions := &spyDecisionStore{}
		svc, _ := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })
		captured := agentErrorCapturePayload(t, svc, engine.ActionClearDecisions)

		svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{
			TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1",
		})

		if len(*captured) != 1 {
			t.Fatalf("got %d payload(s), want 1", len(*captured))
		}
		if got := (*captured)[0].FailedAgentID; got != "" {
			t.Errorf("FailedAgentID = %q, want empty (neither event nor session set one)", got)
		}
	})
}
