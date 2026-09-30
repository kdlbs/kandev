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

func TestWorkflowStepAdvancesOnTurnComplete(t *testing.T) {
	moveToStep := func(config map[string]any) OnTurnCompleteAction {
		return OnTurnCompleteAction{Type: OnTurnCompleteMoveToStep, Config: config}
	}
	cases := []struct {
		name    string
		actions []OnTurnCompleteAction
		want    bool
	}{
		{name: "no actions", want: false},
		{name: "disable_plan_mode only", actions: []OnTurnCompleteAction{{Type: OnTurnCompleteDisablePlanMode}}, want: false},
		{name: "move_to_next", actions: []OnTurnCompleteAction{{Type: OnTurnCompleteMoveToNext}}, want: true},
		{name: "move_to_previous", actions: []OnTurnCompleteAction{{Type: OnTurnCompleteMoveToPrevious}}, want: true},
		{name: "move_to_step with step_id", actions: []OnTurnCompleteAction{moveToStep(map[string]any{"step_id": "review"})}, want: true},
		{name: "move_to_step without config", actions: []OnTurnCompleteAction{moveToStep(nil)}, want: false},
		{name: "move_to_step with empty step_id", actions: []OnTurnCompleteAction{moveToStep(map[string]any{"step_id": ""})}, want: false},
		{name: "move_to_step with non-string step_id", actions: []OnTurnCompleteAction{moveToStep(map[string]any{"step_id": 3})}, want: false},
		{
			name: "requires_approval move only",
			actions: []OnTurnCompleteAction{{
				Type:   OnTurnCompleteMoveToNext,
				Config: map[string]any{"requires_approval": true},
			}},
			want: false,
		},
		{
			name: "requires_approval false still moves",
			actions: []OnTurnCompleteAction{{
				Type:   OnTurnCompleteMoveToNext,
				Config: map[string]any{"requires_approval": false},
			}},
			want: true,
		},
		{
			name: "wait_for_quorum guarded move",
			actions: []OnTurnCompleteAction{moveToStep(map[string]any{
				"step_id": "done",
				"if":      map[string]any{"wait_for_quorum": map[string]any{"role": "approver", "threshold": "all_approve"}},
			})},
			want: true,
		},
		{
			name: "malformed move_to_step followed by a valid move",
			actions: []OnTurnCompleteAction{
				{Type: OnTurnCompleteDisablePlanMode},
				moveToStep(nil),
				{Type: OnTurnCompleteMoveToNext},
			},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			step := &WorkflowStep{Events: StepEvents{OnTurnComplete: tc.actions}}
			if got := step.AdvancesOnTurnComplete(); got != tc.want {
				t.Fatalf("AdvancesOnTurnComplete() = %t, want %t", got, tc.want)
			}
		})
	}
}
