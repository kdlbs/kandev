package engine

// Tests for docs/specs/workflow-evaluate-only-operation-marking/spec.md
// (AC-EO-1 through AC-EO-8): split out of engine_test.go to keep that file
// under the 800-effective-line limit (apps/backend/AGENTS.md). Shares
// fakeStore/fakeCallback/erroringCallback/MapRegistry from engine_test.go.

import (
	"context"
	"testing"
)

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
