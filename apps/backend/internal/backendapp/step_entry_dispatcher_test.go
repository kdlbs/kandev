package backendapp

import (
	"context"
	"testing"

	workflowengine "github.com/kandev/kandev/internal/workflow/engine"
)

// fakeWorkflowEngineProvider lets a test swap which *engine.Engine
// WorkflowEngine() returns between calls, simulating
// orchestrator.Service.reinitWorkflowEngine replacing s.workflowEngine
// in place after later boot wiring (e.g. SetReviewRunner) registers a
// callback the earlier-constructed engine never had.
type fakeWorkflowEngineProvider struct {
	eng *workflowengine.Engine
}

func (p *fakeWorkflowEngineProvider) WorkflowEngine() *workflowengine.Engine {
	return p.eng
}

// countingCallback records every invocation so the test can assert whether
// DispatchStepEntry reached it.
type countingCallback struct {
	calls int
}

func (c *countingCallback) Execute(_ context.Context, _ workflowengine.ActionInput) (workflowengine.ActionResult, error) {
	c.calls++
	return workflowengine.ActionResult{}, nil
}

// fixedStepStore returns a single fixed StepSpec for any LoadStep call, and
// panics on every other TransitionStore method, since DispatchStepEntry only
// calls LoadStep.
type fixedStepStore struct {
	step workflowengine.StepSpec
}

func (s *fixedStepStore) LoadStep(_ context.Context, _, _ string) (workflowengine.StepSpec, error) {
	return s.step, nil
}

func (s *fixedStepStore) LoadState(context.Context, string, string) (workflowengine.MachineState, error) {
	panic("not implemented")
}

func (s *fixedStepStore) LoadNextStep(context.Context, string, int) (workflowengine.StepSpec, error) {
	panic("not implemented")
}

func (s *fixedStepStore) LoadPreviousStep(context.Context, string, int) (workflowengine.StepSpec, error) {
	panic("not implemented")
}

func (s *fixedStepStore) ApplyTransition(context.Context, string, string, string, string, workflowengine.Trigger) error {
	panic("not implemented")
}

func (s *fixedStepStore) ApplyTransitionIfAtStep(context.Context, string, string, string, string, workflowengine.Trigger) (bool, error) {
	panic("not implemented")
}

func (s *fixedStepStore) PersistData(context.Context, string, map[string]any) error {
	panic("not implemented")
}

func (s *fixedStepStore) IsOperationApplied(context.Context, string) (bool, error) {
	panic("not implemented")
}

func (s *fixedStepStore) MarkOperationApplied(context.Context, string) error {
	panic("not implemented")
}

func newRunCodeReviewStep() workflowengine.StepSpec {
	return workflowengine.StepSpec{
		ID:         "step-build",
		WorkflowID: "wf-1",
		Events: map[workflowengine.Trigger][]workflowengine.Action{
			workflowengine.TriggerOnEnter: {
				{Kind: workflowengine.ActionRunCodeReview},
			},
		},
	}
}

// TestEngineStepEntryDispatcherAdapterReadsEngineLazily proves
// engineStepEntryDispatcherAdapter.DispatchStepEntry resolves the engine
// fresh from its provider on every call rather than capturing a pointer at
// construction time. It reproduces the exact shape of the CRITICAL bug this
// fix addresses: an engine built before SetReviewRunner registers
// ActionRunCodeReview (no callback, so DispatchStepEntry would silently
// no-op) is later replaced — via reinitWorkflowEngine's full-replacement
// semantics, not mutation — by an engine that does have the callback
// registered. A stale captured *engine.Engine would keep reaching the first,
// callback-less engine forever.
func TestEngineStepEntryDispatcherAdapterReadsEngineLazily(t *testing.T) {
	store := &fixedStepStore{step: newRunCodeReviewStep()}

	// Engine #1: built as if before SetReviewRunner ever ran — no
	// ActionRunCodeReview callback registered, matching
	// buildWorkflowCallbacks' `if svc.reviewRunner != nil` guard.
	engineWithoutCallback := workflowengine.New(store, workflowengine.MapRegistry{})

	// Engine #2: built as if after SetReviewRunner ran — the callback is
	// now registered, matching a completed reinitWorkflowEngine() call.
	callback := &countingCallback{}
	engineWithCallback := workflowengine.New(store, workflowengine.MapRegistry{
		workflowengine.ActionRunCodeReview: callback,
	})

	provider := &fakeWorkflowEngineProvider{eng: engineWithoutCallback}
	adapter := &engineStepEntryDispatcherAdapter{engineProvider: provider, log: newTestLogger()}

	adapter.DispatchStepEntry(context.Background(), "task-1", "wf-1", "step-build", "entry-1")
	if callback.calls != 0 {
		t.Fatalf("callback should not have been reachable through the pre-reinit engine, got %d calls", callback.calls)
	}

	// Simulate reinitWorkflowEngine replacing s.workflowEngine wholesale.
	provider.eng = engineWithCallback

	adapter.DispatchStepEntry(context.Background(), "task-1", "wf-1", "step-build", "entry-2")
	if callback.calls != 1 {
		t.Fatalf("expected DispatchStepEntry to reach the current (post-reinit) engine's callback exactly once, got %d calls", callback.calls)
	}
}

// TestEngineStepEntryDispatcherAdapterNilEngineIsNoop proves DispatchStepEntry
// tolerates a provider that has not initialised an engine yet, matching
// switchWorkflowDispatcher's existing nil-engine handling.
func TestEngineStepEntryDispatcherAdapterNilEngineIsNoop(t *testing.T) {
	provider := &fakeWorkflowEngineProvider{eng: nil}
	adapter := &engineStepEntryDispatcherAdapter{engineProvider: provider, log: newTestLogger()}

	adapter.DispatchStepEntry(context.Background(), "task-1", "wf-1", "step-build", "entry-1")
}
