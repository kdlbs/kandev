package engine

import (
	"context"
	"errors"
	"testing"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type fakeStore struct {
	state          MachineState
	stepsByID      map[string]StepSpec
	nextSteps      map[int]StepSpec // keyed by currentPosition
	prevSteps      map[int]StepSpec // keyed by currentPosition
	persistedData  map[string]any
	applied        map[string]bool
	transitionFrom string
	transitionTo   string
	// callLog records ApplyTransition and MarkOperationApplied invocations in
	// order, so tests can assert both whether a call happened and, for
	// AC-EO-3, that the commit precedes the mark.
	callLog []string
}

func (s *fakeStore) LoadState(_ context.Context, _, _ string) (MachineState, error) {
	return s.state, nil
}

func (s *fakeStore) LoadStep(_ context.Context, _, stepID string) (StepSpec, error) {
	step, ok := s.stepsByID[stepID]
	if !ok {
		return StepSpec{}, errors.New("step not found")
	}
	return step, nil
}

func (s *fakeStore) LoadNextStep(_ context.Context, _ string, currentPosition int) (StepSpec, error) {
	step, ok := s.nextSteps[currentPosition]
	if !ok {
		return StepSpec{}, errors.New("no next step")
	}
	return step, nil
}

func (s *fakeStore) LoadPreviousStep(_ context.Context, _ string, currentPosition int) (StepSpec, error) {
	step, ok := s.prevSteps[currentPosition]
	if !ok {
		return StepSpec{}, errors.New("no previous step")
	}
	return step, nil
}

func (s *fakeStore) ApplyTransition(_ context.Context, _, _, fromStepID, toStepID string, _ Trigger) error {
	s.transitionFrom = fromStepID
	s.transitionTo = toStepID
	s.callLog = append(s.callLog, "ApplyTransition")
	return nil
}

func (s *fakeStore) ApplyTransitionIfAtStep(
	_ context.Context, _, _, expectedStepID, toStepID string, _ Trigger,
) (bool, error) {
	if s.state.CurrentStepID != expectedStepID {
		return false, nil
	}
	s.transitionFrom = expectedStepID
	s.transitionTo = toStepID
	s.state.CurrentStepID = toStepID
	return true, nil
}

func (s *fakeStore) PersistData(_ context.Context, _ string, data map[string]any) error {
	s.persistedData = data
	return nil
}

func (s *fakeStore) IsOperationApplied(_ context.Context, operationID string) (bool, error) {
	// Logged unconditionally (even for an empty operationID) so the AC-EO-7
	// "no store calls at all" tests actually prove this method was never
	// reached, rather than passing vacuously because this method never logs.
	s.callLog = append(s.callLog, "IsOperationApplied")
	if operationID == "" {
		return false, nil
	}
	return s.applied[operationID], nil
}

func (s *fakeStore) MarkOperationApplied(_ context.Context, operationID string) error {
	if operationID == "" {
		return nil
	}
	s.applied[operationID] = true
	s.callLog = append(s.callLog, "MarkOperationApplied")
	return nil
}

// erroringCallback always fails, for AC-EO-5: a processActions error must
// short-circuit HandleTrigger before the mark is ever attempted.
type erroringCallback struct{}

func (c *erroringCallback) Execute(_ context.Context, _ ActionInput) (ActionResult, error) {
	return ActionResult{}, errors.New("callback failed")
}

type fakeCallback struct {
	result   ActionResult
	executed bool
}

func (c *fakeCallback) Execute(_ context.Context, _ ActionInput) (ActionResult, error) {
	c.executed = true
	return c.result, nil
}

// recordingCallback captures the ActionInput it was invoked with so tests can
// assert the engine dispatched the right typed action to the callback.
type recordingCallback struct {
	calls []ActionInput
}

func (c *recordingCallback) Execute(_ context.Context, in ActionInput) (ActionResult, error) {
	c.calls = append(c.calls, in)
	return ActionResult{}, nil
}

// TestHandleTrigger_SetSessionMode_InvokesCallback exercises the full engine path
// for the set_session_mode action (issue #1183): a step compiled from a
// set_session_mode on_enter action dispatches to the registered callback with the
// typed mode carried through. This is the engine-level end-to-end of the feature.
func TestHandleTrigger_SetSessionMode_InvokesCallback(t *testing.T) {
	compiled := CompileStep(&wfmodels.WorkflowStep{
		ID: "step-1", WorkflowID: "wf1",
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{
				{Type: wfmodels.OnEnterSetSessionMode, Config: map[string]any{"mode": "acceptEdits"}},
			},
		},
	})
	store := &fakeStore{
		state:     MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
		stepsByID: map[string]StepSpec{"step-1": compiled},
		applied:   map[string]bool{},
	}

	cb := &recordingCallback{}
	eng := New(store, MapRegistry{ActionSetSessionMode: cb})

	if _, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnEnter,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cb.calls) != 1 {
		t.Fatalf("expected set_session_mode callback to fire once, got %d", len(cb.calls))
	}
	got := cb.calls[0].Action
	if got.Kind != ActionSetSessionMode {
		t.Fatalf("unexpected action kind dispatched: %s", got.Kind)
	}
	if got.SetSessionMode == nil || got.SetSessionMode.Mode != "acceptEdits" {
		t.Fatalf("expected dispatched mode acceptEdits, got %+v", got.SetSessionMode)
	}
}

// TestHandleTriggerSessionShapedOnly_ExecutesSessionShapedRunsSessionIndependent
// covers the workflow-switch route's use of HandleTriggerSessionShapedOnly:
// it must still execute the session-shaped kinds (auto_start_agent here) —
// that is this route's actual production path for them — while skipping the
// session-independent kinds DispatchStepEntry now owns exclusively.
func TestHandleTriggerSessionShapedOnly_ExecutesSessionShapedRunsSessionIndependent(t *testing.T) {
	compiled := CompileStep(&wfmodels.WorkflowStep{
		ID: "step-1", WorkflowID: "wf1",
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{
				{Type: wfmodels.OnEnterAutoStartAgent},
				{Type: wfmodels.OnEnterClearDecisions},
			},
		},
	})
	store := &fakeStore{
		state:     MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
		stepsByID: map[string]StepSpec{"step-1": compiled},
		applied:   map[string]bool{},
	}

	shaped := &recordingCallback{}
	independent := &recordingCallback{}
	eng := New(store, MapRegistry{
		ActionAutoStartAgent: shaped,
		ActionClearDecisions: independent,
	})

	if _, err := eng.HandleTriggerSessionShapedOnly(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnEnter,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(shaped.calls) != 1 {
		t.Fatalf("expected auto_start_agent (session-shaped) to fire once, got %d", len(shaped.calls))
	}
	if len(independent.calls) != 0 {
		t.Fatalf("expected clear_decisions (session-independent) not to fire, got %d calls", len(independent.calls))
	}
}

func TestHandleTrigger_FirstTransitionWins(t *testing.T) {
	store := &fakeStore{
		state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
		stepsByID: map[string]StepSpec{
			"step-1": {
				ID:       "step-1",
				Position: 1,
				Events: map[Trigger][]Action{
					TriggerOnTurnComplete: {
						{Kind: ActionMoveToNext},
						{Kind: ActionMoveToStep, MoveToStep: &MoveToStepAction{StepID: "manual-target"}},
					},
				},
			},
		},
		nextSteps: map[int]StepSpec{1: {ID: "step-2", Position: 2}},
		applied:   map[string]bool{},
	}

	eng := New(store, MapRegistry{})
	result, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnTurnComplete,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Transitioned {
		t.Fatalf("expected transition")
	}
	if result.ToStepID != "step-2" {
		t.Fatalf("expected first transition target step-2, got %q", result.ToStepID)
	}
}

func TestHandleTrigger_PersistsDataPatchFromCallbacks(t *testing.T) {
	store := &fakeStore{
		state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
		stepsByID: map[string]StepSpec{
			"step-1": {
				ID: "step-1",
				Events: map[Trigger][]Action{
					TriggerOnEnter: {
						{Kind: ActionSetWorkflowData},
					},
				},
			},
		},
		nextSteps: map[int]StepSpec{},
		applied:   map[string]bool{},
	}
	registry := MapRegistry{
		ActionSetWorkflowData: &fakeCallback{result: ActionResult{DataPatch: map[string]any{"k": "v"}}},
	}

	eng := New(store, registry)
	result, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnEnter,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Transitioned {
		t.Fatalf("did not expect transition")
	}
	if got, ok := store.persistedData["k"]; !ok || got != "v" {
		t.Fatalf("expected persisted data patch, got %+v", store.persistedData)
	}
}

func TestHandleTrigger_IdempotentByOperationID(t *testing.T) {
	store := &fakeStore{
		state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
		stepsByID: map[string]StepSpec{
			"step-1": {
				ID:       "step-1",
				Position: 1,
				Events: map[Trigger][]Action{
					TriggerOnTurnComplete: {{Kind: ActionMoveToNext}},
				},
			},
		},
		nextSteps: map[int]StepSpec{1: {ID: "step-2", Position: 2}},
		applied:   map[string]bool{},
	}

	eng := New(store, MapRegistry{})
	first, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnTurnComplete, OperationID: "op-1",
	})
	if err != nil {
		t.Fatalf("unexpected first error: %v", err)
	}
	if !first.Transitioned {
		t.Fatalf("expected first call to transition")
	}

	second, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnTurnComplete, OperationID: "op-1",
	})
	if err != nil {
		t.Fatalf("unexpected second error: %v", err)
	}
	if !second.Idempotent {
		t.Fatalf("expected idempotent result on second call")
	}
}

func TestHandleTrigger_EvaluateOnlySkipsPersistence(t *testing.T) {
	store := &fakeStore{
		state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
		stepsByID: map[string]StepSpec{
			"step-1": {
				ID:       "step-1",
				Position: 1,
				Events: map[Trigger][]Action{
					TriggerOnTurnComplete: {
						{Kind: ActionSetWorkflowData},
						{Kind: ActionMoveToNext},
					},
				},
			},
		},
		nextSteps: map[int]StepSpec{1: {ID: "step-2", Position: 2}},
		applied:   map[string]bool{},
	}
	cb := &fakeCallback{result: ActionResult{DataPatch: map[string]any{"k": "v"}}}
	registry := MapRegistry{ActionSetWorkflowData: cb}

	eng := New(store, registry)
	result, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnTurnComplete, EvaluateOnly: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Transitioned {
		t.Fatalf("expected transition")
	}
	if result.ToStepID != "step-2" {
		t.Fatalf("expected target step-2, got %q", result.ToStepID)
	}
	// Callbacks should still execute
	if !cb.executed {
		t.Fatalf("expected callback to execute in evaluate-only mode")
	}
	// But persistence should NOT happen
	if store.persistedData != nil {
		t.Fatalf("expected no persisted data in evaluate-only mode, got %+v", store.persistedData)
	}
	if store.transitionTo != "" {
		t.Fatalf("expected no applied transition in evaluate-only mode, got %q", store.transitionTo)
	}
}

func TestHandleTrigger_RequiresApprovalSkipsTransition(t *testing.T) {
	store := &fakeStore{
		state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
		stepsByID: map[string]StepSpec{
			"step-1": {
				ID:       "step-1",
				Position: 1,
				Events: map[Trigger][]Action{
					TriggerOnTurnComplete: {
						{Kind: ActionMoveToNext, RequiresApproval: true},
					},
				},
			},
		},
		nextSteps: map[int]StepSpec{1: {ID: "step-2", Position: 2}},
		applied:   map[string]bool{},
	}

	eng := New(store, MapRegistry{})
	result, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnTurnComplete,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Transitioned {
		t.Fatalf("expected no transition when requires_approval is true")
	}
}

func TestHandleTrigger_RequiresApprovalFallsToNextAction(t *testing.T) {
	store := &fakeStore{
		state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
		stepsByID: map[string]StepSpec{
			"step-1": {
				ID:       "step-1",
				Position: 1,
				Events: map[Trigger][]Action{
					TriggerOnTurnComplete: {
						{Kind: ActionMoveToNext, RequiresApproval: true},
						{Kind: ActionMoveToStep, MoveToStep: &MoveToStepAction{StepID: "step-3"}},
					},
				},
			},
		},
		nextSteps: map[int]StepSpec{1: {ID: "step-2", Position: 2}},
		applied:   map[string]bool{},
	}

	eng := New(store, MapRegistry{})
	result, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnTurnComplete,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Transitioned {
		t.Fatalf("expected transition to fallback action")
	}
	if result.ToStepID != "step-3" {
		t.Fatalf("expected transition to step-3, got %q", result.ToStepID)
	}
}

func TestHandleTrigger_LoadPreviousStep(t *testing.T) {
	store := &fakeStore{
		state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-2"},
		stepsByID: map[string]StepSpec{
			"step-2": {
				ID:       "step-2",
				Position: 2,
				Events: map[Trigger][]Action{
					TriggerOnTurnComplete: {{Kind: ActionMoveToPrevious}},
				},
			},
		},
		prevSteps: map[int]StepSpec{2: {ID: "step-1", Position: 1}},
		applied:   map[string]bool{},
	}

	eng := New(store, MapRegistry{})
	result, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnTurnComplete,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Transitioned {
		t.Fatalf("expected transition")
	}
	if result.ToStepID != "step-1" {
		t.Fatalf("expected target step-1, got %q", result.ToStepID)
	}
}

// --- AC-EO-1/AC-EO-4: EvaluateOnly with a deferred transition and a
// non-empty OperationID must not mark the operation applied, and must report
// OperationMarkDeferred so the caller knows it now owns the marker. ---

func TestHandleTrigger_EvaluateOnlyDeferredTransitionSkipsMarkAndReportsDeferred(t *testing.T) {
	store := &fakeStore{
		state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
		stepsByID: map[string]StepSpec{
			"step-1": {
				ID:       "step-1",
				Position: 1,
				Events: map[Trigger][]Action{
					TriggerOnTurnComplete: {{Kind: ActionMoveToNext}},
				},
			},
		},
		nextSteps: map[int]StepSpec{1: {ID: "step-2", Position: 2}},
		applied:   map[string]bool{},
	}

	eng := New(store, MapRegistry{})
	result, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnTurnComplete,
		EvaluateOnly: true, OperationID: "op-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Transitioned {
		t.Fatalf("expected transition")
	}
	if !result.OperationMarkDeferred {
		t.Fatalf("expected OperationMarkDeferred true")
	}
	if store.applied["op-1"] {
		t.Fatalf("expected op-1 not marked applied")
	}
	for _, call := range store.callLog {
		if call == "MarkOperationApplied" {
			t.Fatalf("MarkOperationApplied must not be invoked, callLog: %v", store.callLog)
		}
	}
}

// --- AC-EO-2: EvaluateOnly with no deferred transition still marks, whether
// or not a non-transition callback produced a data patch. A patch does not
// change marker ownership — it is dropped on this path (spec.md § The
// contract), which this test deliberately does not assert either way. ---

func TestHandleTrigger_EvaluateOnlyNoTransitionStillMarks(t *testing.T) {
	for _, tc := range []struct {
		name  string
		patch map[string]any
	}{
		{name: "no data patch"},
		{name: "with data patch", patch: map[string]any{"k": "v"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{
				state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
				stepsByID: map[string]StepSpec{
					"step-1": {
						ID: "step-1",
						Events: map[Trigger][]Action{
							TriggerOnEnter: {{Kind: ActionSetWorkflowData}},
						},
					},
				},
				applied: map[string]bool{},
			}
			registry := MapRegistry{ActionSetWorkflowData: &fakeCallback{result: ActionResult{DataPatch: tc.patch}}}

			eng := New(store, registry)
			result, err := eng.HandleTrigger(context.Background(), HandleInput{
				TaskID: "t1", SessionID: "s1", Trigger: TriggerOnEnter,
				EvaluateOnly: true, OperationID: "op-1",
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Transitioned {
				t.Fatalf("did not expect transition")
			}
			if !store.applied["op-1"] {
				t.Fatalf("expected op-1 marked applied")
			}
			if result.OperationMarkDeferred {
				t.Fatalf("expected OperationMarkDeferred false on a no-transition path")
			}
		})
	}
}

// --- AC-EO-3: EvaluateOnly false marks after the commit, transition or not.
// ---

func TestHandleTrigger_NotEvaluateOnlyMarksAfterCommit(t *testing.T) {
	store := &fakeStore{
		state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
		stepsByID: map[string]StepSpec{
			"step-1": {
				ID:       "step-1",
				Position: 1,
				Events: map[Trigger][]Action{
					TriggerOnTurnComplete: {{Kind: ActionMoveToNext}},
				},
			},
		},
		nextSteps: map[int]StepSpec{1: {ID: "step-2", Position: 2}},
		applied:   map[string]bool{},
	}

	eng := New(store, MapRegistry{})
	result, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnTurnComplete, OperationID: "op-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Transitioned {
		t.Fatalf("expected transition")
	}
	if !store.applied["op-1"] {
		t.Fatalf("expected op-1 marked applied")
	}
	if result.OperationMarkDeferred {
		t.Fatalf("expected OperationMarkDeferred false when EvaluateOnly is false")
	}
	wantLog := []string{"IsOperationApplied", "ApplyTransition", "MarkOperationApplied"}
	if len(store.callLog) != len(wantLog) {
		t.Fatalf("callLog = %v, want %v", store.callLog, wantLog)
	}
	for i, call := range wantLog {
		if store.callLog[i] != call {
			t.Fatalf("callLog[%d] = %q, want %q (commit must precede mark): %v", i, store.callLog[i], call, store.callLog)
		}
	}
}

// --- AC-EO-4: OperationMarkDeferred is false on every return path other
// than the AC-EO-1 deferred-transition-with-a-non-empty-OperationID case. ---

func TestHandleTrigger_OperationMarkDeferredFalseCases(t *testing.T) {
	t.Run("idempotent short-circuit", func(t *testing.T) {
		store := &fakeStore{
			state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
			stepsByID: map[string]StepSpec{
				"step-1": {ID: "step-1", Position: 1, Events: map[Trigger][]Action{
					TriggerOnTurnComplete: {{Kind: ActionMoveToNext}},
				}},
			},
			nextSteps: map[int]StepSpec{1: {ID: "step-2", Position: 2}},
			applied:   map[string]bool{"op-1": true},
		}
		eng := New(store, MapRegistry{})
		result, err := eng.HandleTrigger(context.Background(), HandleInput{
			TaskID: "t1", SessionID: "s1", Trigger: TriggerOnTurnComplete,
			EvaluateOnly: true, OperationID: "op-1",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Idempotent {
			t.Fatalf("expected idempotent short-circuit")
		}
		if result.OperationMarkDeferred {
			t.Fatalf("expected OperationMarkDeferred false on the idempotent short-circuit")
		}
	})

	t.Run("step declares no actions for the trigger", func(t *testing.T) {
		store := &fakeStore{
			state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
			stepsByID: map[string]StepSpec{
				"step-1": {ID: "step-1", Position: 1},
			},
			applied: map[string]bool{},
		}
		eng := New(store, MapRegistry{})
		result, err := eng.HandleTrigger(context.Background(), HandleInput{
			TaskID: "t1", SessionID: "s1", Trigger: TriggerOnTurnComplete,
			EvaluateOnly: true, OperationID: "op-1",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.OperationMarkDeferred {
			t.Fatalf("expected OperationMarkDeferred false when the step declares no actions")
		}
		if !store.applied["op-1"] {
			t.Fatalf("expected op-1 marked applied (nothing was deferred)")
		}
	})

	t.Run("EvaluateOnly false", func(t *testing.T) {
		store := &fakeStore{
			state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
			stepsByID: map[string]StepSpec{
				"step-1": {ID: "step-1", Position: 1, Events: map[Trigger][]Action{
					TriggerOnTurnComplete: {{Kind: ActionMoveToNext}},
				}},
			},
			nextSteps: map[int]StepSpec{1: {ID: "step-2", Position: 2}},
			applied:   map[string]bool{},
		}
		eng := New(store, MapRegistry{})
		result, err := eng.HandleTrigger(context.Background(), HandleInput{
			TaskID: "t1", SessionID: "s1", Trigger: TriggerOnTurnComplete, OperationID: "op-1",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.OperationMarkDeferred {
			t.Fatalf("expected OperationMarkDeferred false when EvaluateOnly is false")
		}
	})

	t.Run("deferred transition with empty OperationID", func(t *testing.T) {
		store := &fakeStore{
			state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
			stepsByID: map[string]StepSpec{
				"step-1": {ID: "step-1", Position: 1, Events: map[Trigger][]Action{
					TriggerOnTurnComplete: {{Kind: ActionMoveToNext}},
				}},
			},
			nextSteps: map[int]StepSpec{1: {ID: "step-2", Position: 2}},
			applied:   map[string]bool{},
		}
		eng := New(store, MapRegistry{})
		result, err := eng.HandleTrigger(context.Background(), HandleInput{
			TaskID: "t1", SessionID: "s1", Trigger: TriggerOnTurnComplete,
			EvaluateOnly: true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Transitioned {
			t.Fatalf("expected transition")
		}
		if result.OperationMarkDeferred {
			t.Fatalf("expected OperationMarkDeferred false when OperationID is empty")
		}
	})

	t.Run("processActions error", func(t *testing.T) {
		store := &fakeStore{
			state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
			stepsByID: map[string]StepSpec{
				"step-1": {ID: "step-1", Events: map[Trigger][]Action{
					TriggerOnEnter: {{Kind: ActionSetWorkflowData}},
				}},
			},
			applied: map[string]bool{},
		}
		registry := MapRegistry{ActionSetWorkflowData: &erroringCallback{}}
		eng := New(store, registry)
		result, err := eng.HandleTrigger(context.Background(), HandleInput{
			TaskID: "t1", SessionID: "s1", Trigger: TriggerOnEnter,
			EvaluateOnly: true, OperationID: "op-1",
		})
		if err == nil {
			t.Fatalf("expected error")
		}
		if result.Transitioned || result.OperationMarkDeferred || result.ActionCount != 0 {
			t.Fatalf("expected zero HandleResult, got %+v", result)
		}
		if store.applied["op-1"] {
			t.Fatalf("expected op-1 not marked applied")
		}
	})
}

// --- AC-EO-5: a processActions error never marks the operation, regardless
// of EvaluateOnly. ---

func TestHandleTrigger_ProcessActionsErrorNeverMarks(t *testing.T) {
	store := &fakeStore{
		state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
		stepsByID: map[string]StepSpec{
			"step-1": {ID: "step-1", Events: map[Trigger][]Action{
				TriggerOnEnter: {{Kind: ActionSetWorkflowData}},
			}},
		},
		applied: map[string]bool{},
	}
	registry := MapRegistry{ActionSetWorkflowData: &erroringCallback{}}
	eng := New(store, registry)

	_, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnEnter, OperationID: "op-1",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if store.applied["op-1"] {
		t.Fatalf("expected op-1 not marked applied")
	}
}

// --- AC-EO-6: a step declaring no actions for the trigger marks regardless
// of EvaluateOnly, because nothing was deferred. ---

func TestHandleTrigger_NoDeclaredActionsMarksEvenWhenEvaluateOnly(t *testing.T) {
	store := &fakeStore{
		state:     MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
		stepsByID: map[string]StepSpec{"step-1": {ID: "step-1"}},
		applied:   map[string]bool{},
	}
	eng := New(store, MapRegistry{})

	result, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnTurnComplete,
		EvaluateOnly: true, OperationID: "op-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !store.applied["op-1"] {
		t.Fatalf("expected op-1 marked applied")
	}
	if result.OperationMarkDeferred {
		t.Fatalf("expected OperationMarkDeferred false")
	}
}

// --- AC-EO-7: an empty OperationID never touches the store's idempotency
// calls at all, deferred transition or not. ---

func TestHandleTrigger_EmptyOperationIDSkipsStoreCallsEvenWhenDeferred(t *testing.T) {
	store := &fakeStore{
		state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
		stepsByID: map[string]StepSpec{
			"step-1": {ID: "step-1", Position: 1, Events: map[Trigger][]Action{
				TriggerOnTurnComplete: {{Kind: ActionMoveToNext}},
			}},
		},
		nextSteps: map[int]StepSpec{1: {ID: "step-2", Position: 2}},
		applied:   map[string]bool{},
	}
	eng := New(store, MapRegistry{})

	result, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnTurnComplete, EvaluateOnly: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Transitioned {
		t.Fatalf("expected transition")
	}
	if len(store.callLog) != 0 {
		t.Fatalf("expected no store calls at all, callLog: %v", store.callLog)
	}
}

// --- AC-EO-8: HandleTriggerSessionShapedOnly follows the same rules — no
// deferred-transition case is constructible through it, so this pins the
// AC-EO-2/AC-EO-4-false/AC-EO-7 behaviors plus the structural precondition
// (sessionShapedActionKinds admits no transition kind) that makes AC-EO-1
// vacuous here. ---

func TestHandleTriggerSessionShapedOnly_FollowsMarkerRulesAndFilterIsDisjointFromTransitions(t *testing.T) {
	for kind := range sessionShapedActionKinds {
		if isTransitionAction(kind) {
			t.Fatalf("sessionShapedActionKinds admits transition kind %q — AC-EO-1 would become reachable through HandleTriggerSessionShapedOnly and double-dispatch step entry (AC-OFFICE-STEP-ENTRY-001)", kind)
		}
	}

	t.Run("EvaluateOnly with only session-shaped actions still marks (AC-EO-2)", func(t *testing.T) {
		store := &fakeStore{
			state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
			stepsByID: map[string]StepSpec{
				"step-1": {ID: "step-1", Events: map[Trigger][]Action{
					TriggerOnEnter: {{Kind: ActionAutoStartAgent}},
				}},
			},
			applied: map[string]bool{},
		}
		eng := New(store, MapRegistry{ActionAutoStartAgent: &fakeCallback{}})
		result, err := eng.HandleTriggerSessionShapedOnly(context.Background(), HandleInput{
			TaskID: "t1", SessionID: "s1", Trigger: TriggerOnEnter,
			EvaluateOnly: true, OperationID: "op-1",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !store.applied["op-1"] {
			t.Fatalf("expected op-1 marked applied")
		}
		if result.OperationMarkDeferred {
			t.Fatalf("expected OperationMarkDeferred false (AC-EO-4)")
		}
	})

	t.Run("empty OperationID skips store calls (AC-EO-7)", func(t *testing.T) {
		store := &fakeStore{
			state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf1", CurrentStepID: "step-1"},
			stepsByID: map[string]StepSpec{
				"step-1": {ID: "step-1", Events: map[Trigger][]Action{
					TriggerOnEnter: {{Kind: ActionAutoStartAgent}},
				}},
			},
			applied: map[string]bool{},
		}
		eng := New(store, MapRegistry{ActionAutoStartAgent: &fakeCallback{}})
		if _, err := eng.HandleTriggerSessionShapedOnly(context.Background(), HandleInput{
			TaskID: "t1", SessionID: "s1", Trigger: TriggerOnEnter, EvaluateOnly: true,
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(store.callLog) != 0 {
			t.Fatalf("expected no store calls at all, callLog: %v", store.callLog)
		}
	})
}
