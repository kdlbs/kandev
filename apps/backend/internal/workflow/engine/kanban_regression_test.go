package engine

import (
	"context"
	"testing"
)

// TestKanbanRegression_OnlyOnEnterAutoStart pins that a step compiled from
// the kanban-style workflow model — using only on_enter+auto_start_agent and
// on_turn_complete+move_to_next — drives the engine the same way it did
// before Phase 2 lands. The new triggers/actions are additive: with no Phase
// 2 dependencies wired, the engine must continue to exercise this path
// identically.
func TestKanbanRegression_OnlyOnEnterAutoStart(t *testing.T) {
	store := &fakeStore{
		state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf", CurrentStepID: "step-1"},
		stepsByID: map[string]StepSpec{
			"step-1": {
				ID: "step-1", WorkflowID: "wf", Position: 1,
				Events: map[Trigger][]Action{
					TriggerOnEnter:        {{Kind: ActionAutoStartAgent, AutoStartAgent: &AutoStartAgentAction{QueueIfBusy: true}}},
					TriggerOnTurnComplete: {{Kind: ActionMoveToNext}},
				},
			},
		},
		nextSteps: map[int]StepSpec{1: {ID: "step-2", Position: 2}},
		applied:   map[string]bool{},
	}
	autoStart := &fakeCallback{}
	eng := New(store, MapRegistry{ActionAutoStartAgent: autoStart})

	// on_enter: callback runs, no transition.
	res, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnEnter,
	})
	if err != nil {
		t.Fatalf("on_enter error: %v", err)
	}
	if res.Transitioned {
		t.Fatalf("on_enter must not transition")
	}
	if !autoStart.executed {
		t.Fatalf("auto_start_agent callback must execute on_enter")
	}

	// on_turn_complete: transition to next step, no guard, no payload.
	res, err = eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnTurnComplete,
	})
	if err != nil {
		t.Fatalf("on_turn_complete error: %v", err)
	}
	if !res.Transitioned {
		t.Fatalf("on_turn_complete must transition to next step")
	}
	if res.ToStepID != "step-2" {
		t.Fatalf("unexpected target: %q", res.ToStepID)
	}
}

// TestKanbanRegression_NoGuard_TransitionsImmediately pins that move_to_next
// without a guard transitions on the first eligible action — preserving
// pre-Phase-2 behaviour where no quorum or decision plumbing exists.
func TestKanbanRegression_NoGuard_TransitionsImmediately(t *testing.T) {
	store := &fakeStore{
		state: MachineState{TaskID: "t1", SessionID: "s1", WorkflowID: "wf", CurrentStepID: "step-1"},
		stepsByID: map[string]StepSpec{
			"step-1": {
				ID: "step-1", WorkflowID: "wf", Position: 1,
				Events: map[Trigger][]Action{
					TriggerOnTurnComplete: {{Kind: ActionMoveToNext}}, // Guard nil
				},
			},
		},
		nextSteps: map[int]StepSpec{1: {ID: "step-2", Position: 2}},
		applied:   map[string]bool{},
	}
	eng := New(store, MapRegistry{})
	res, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "t1", SessionID: "s1", Trigger: TriggerOnTurnComplete,
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !res.Transitioned {
		t.Fatalf("expected transition without guard")
	}
}
