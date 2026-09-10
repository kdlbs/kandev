package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// A panicking dispatch does not take the process down. Routes R2-R5 never
// reach the event bus's own recover-and-log wrapper, and this dispatch now
// runs synchronously (after the session guard is released, not on a
// goroutine of its own), so nothing above it on the call stack recovers a
// panicking callback unless dispatchKanbanAgentErrorTriggerRecovered does.
// The panic is injected through a registered callback's Execute, the only
// available seam — not a stub engine.
func TestDispatchKanbanAgentErrorTrigger_RecoversFromPanickingCallback(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
	}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, nil)
	installPanickingAgentErrorCallback(t, svc, engine.ActionClearDecisions)

	// handleAgentFailed is a real guard-holding production entry point, not a
	// direct call to the dispatch function, so this exercises the actual
	// call chain: lock -> handleRecoverableFailureLockedState -> unlock ->
	// dispatchKanbanAgentErrorTriggerRecovered.
	handlerReturned := make(chan struct{})
	go func() {
		svc.handleAgentFailed(ctx, watcher.AgentEventData{
			TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom",
		})
		close(handlerReturned)
	}()

	select {
	case <-handlerReturned:
	case <-time.After(3 * time.Second):
		t.Fatal("handleAgentFailed did not return after a panicking callback: the panic escaped recovery")
	}

	errs := filterLogs(logs, msgAgentErrorDispatchPanicked)
	if len(errs) != 1 {
		t.Fatalf("got %d %q ERROR record(s), want 1 (all: %+v)", len(errs), msgAgentErrorDispatchPanicked, logs.All())
	}
	fields := errs[0].ContextMap()
	if fields["task_id"] != "t1" || fields["session_id"] != "s1" {
		t.Errorf("unexpected identity fields: %+v", fields)
	}
	if got := filterLogs(logs, msgAgentErrorDispatched); len(got) != 0 {
		t.Errorf("got %d dispatch INFO record(s), want 0 (the panic ERROR is not a dispatch record)", len(got))
	}
}

// installPanickingAgentErrorCallback swaps svc's agentErrorDeps for a copy
// whose registry resolves kind to a callback that panics on Execute,
// mirroring agentErrorCapturePayload's swap-the-registry approach. Must run
// after svc.initWorkflowEngine() (newAgentErrorTestService already calls it).
func installPanickingAgentErrorCallback(t *testing.T, svc *Service, kind engine.ActionKind) {
	t.Helper()
	deps := svc.agentErrorDeps.Load()
	if deps == nil {
		t.Fatal("installPanickingAgentErrorCallback: agentErrorDeps not initialized")
	}
	registry := &panickingCallbackRegistry{inner: deps.registry, kind: kind}
	options := append([]engine.Option{engine.WithLogger(svc.logger)}, svc.engineOptions...)
	svc.agentErrorDeps.Store(&agentErrorDispatchDeps{
		engine:   engine.New(deps.store, registry, options...),
		registry: registry,
		store:    deps.store,
	})
}

type panickingCallbackRegistry struct {
	inner engine.CallbackRegistry
	kind  engine.ActionKind
}

func (r *panickingCallbackRegistry) Get(kind engine.ActionKind) (engine.ActionCallback, bool) {
	if kind == r.kind {
		return panickingCallback{}, true
	}
	return r.inner.Get(kind)
}

type panickingCallback struct{}

func (panickingCallback) Execute(context.Context, engine.ActionInput) (engine.ActionResult, error) {
	panic("installPanickingAgentErrorCallback: injected test panic")
}
