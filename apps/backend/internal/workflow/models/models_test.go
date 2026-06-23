package models

import "testing"

func TestRemapStepEvents_RemapGenericMoveToStep(t *testing.T) {
	events := StepEvents{
		OnChildrenCompleted: []GenericAction{
			{Type: GenericActionMoveToStep, Config: map[string]any{"step_id": "old-step"}},
			{Type: GenericActionMoveToNext},
		},
	}

	remapped := RemapStepEvents(events, map[string]string{"old-step": "new-step"})

	if got := remapped.OnChildrenCompleted[0].Config["step_id"]; got != "new-step" {
		t.Fatalf("generic move_to_step step_id = %v, want new-step", got)
	}
	if got := events.OnChildrenCompleted[0].Config["step_id"]; got != "old-step" {
		t.Fatalf("source events mutated, step_id = %v", got)
	}
}
