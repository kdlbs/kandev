package engine

import (
	"context"
	"errors"
	"testing"
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
	return nil
}

func (s *fakeStore) PersistData(_ context.Context, _ string, data map[string]any) error {
	s.persistedData = data
	return nil
}

func (s *fakeStore) IsOperationApplied(_ context.Context, operationID string) (bool, error) {
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
	return nil
}

type fakeCallback struct {
	result   ActionResult
	executed bool
}

func (c *fakeCallback) Execute(_ context.Context, _ ActionInput) (ActionResult, error) {
	c.executed = true
	return c.result, nil
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
