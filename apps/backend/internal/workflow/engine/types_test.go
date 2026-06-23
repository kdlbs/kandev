package engine

import (
	"testing"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestCompileStep_CompilesLegacyActionsToTypedActions(t *testing.T) {
	step := &wfmodels.WorkflowStep{
		ID:         "s1",
		WorkflowID: "wf1",
		Name:       "Step 1",
		Position:   0,
		Prompt:     "Do the work for {{task_prompt}}",
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{
				{Type: wfmodels.OnEnterAutoStartAgent},
				{Type: wfmodels.OnEnterResetAgentContext},
			},
			OnTurnStart: []wfmodels.OnTurnStartAction{
				{Type: wfmodels.OnTurnStartMoveToNext},
			},
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]any{"step_id": "s2"}},
			},
			OnExit: []wfmodels.OnExitAction{{Type: wfmodels.OnExitDisablePlanMode}},
		},
	}

	spec := CompileStep(step)
	if len(spec.Events[TriggerOnEnter]) != 2 {
		t.Fatalf("expected 2 on_enter actions, got %d", len(spec.Events[TriggerOnEnter]))
	}
	if spec.Events[TriggerOnEnter][0].Kind != ActionAutoStartAgent {
		t.Fatalf("unexpected first on_enter action: %s", spec.Events[TriggerOnEnter][0].Kind)
	}
	if spec.Events[TriggerOnTurnComplete][0].Kind != ActionMoveToStep {
		t.Fatalf("unexpected on_turn_complete action: %s", spec.Events[TriggerOnTurnComplete][0].Kind)
	}
	if spec.Events[TriggerOnTurnComplete][0].MoveToStep == nil || spec.Events[TriggerOnTurnComplete][0].MoveToStep.StepID != "s2" {
		t.Fatalf("expected compiled move_to_step target s2")
	}
	if spec.Prompt != "Do the work for {{task_prompt}}" {
		t.Fatalf("expected prompt to be compiled, got %q", spec.Prompt)
	}
}

func TestCompileStep_CompilesGenericTransitionActions(t *testing.T) {
	step := &wfmodels.WorkflowStep{
		ID:         "s1",
		WorkflowID: "wf1",
		Name:       "Parent Wait",
		Position:   0,
		Events: wfmodels.StepEvents{
			OnChildrenCompleted: []wfmodels.GenericAction{
				{Type: wfmodels.GenericActionMoveToNext},
				{Type: wfmodels.GenericActionMoveToStep, Config: map[string]any{"step_id": "s3"}},
				{Type: wfmodels.GenericActionAutoStartAgent},
			},
		},
	}

	spec := CompileStep(step)
	actions := spec.Events[TriggerOnChildrenCompleted]
	if len(actions) != 3 {
		t.Fatalf("expected 3 on_children_completed actions, got %d", len(actions))
	}
	if actions[0].Kind != ActionMoveToNext {
		t.Fatalf("expected first action to move_to_next, got %s", actions[0].Kind)
	}
	if actions[1].Kind != ActionMoveToStep {
		t.Fatalf("expected second action to move_to_step, got %s", actions[1].Kind)
	}
	if actions[1].MoveToStep == nil || actions[1].MoveToStep.StepID != "s3" {
		t.Fatalf("expected move_to_step target s3, got %+v", actions[1].MoveToStep)
	}
	if actions[2].Kind != ActionAutoStartAgent {
		t.Fatalf("expected third action to auto_start_agent, got %s", actions[2].Kind)
	}
}

func TestCompileStep_SetSessionMode(t *testing.T) {
	t.Run("compiles set_session_mode with mode config", func(t *testing.T) {
		step := &wfmodels.WorkflowStep{
			ID:         "s1",
			WorkflowID: "wf1",
			Name:       "Implement",
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterSetSessionMode, Config: map[string]any{"mode": "acceptEdits"}},
				},
			},
		}

		spec := CompileStep(step)
		actions := spec.Events[TriggerOnEnter]
		if len(actions) != 1 {
			t.Fatalf("expected 1 on_enter action, got %d", len(actions))
		}
		if actions[0].Kind != ActionSetSessionMode {
			t.Fatalf("unexpected action kind: %s", actions[0].Kind)
		}
		if actions[0].SetSessionMode == nil || actions[0].SetSessionMode.Mode != "acceptEdits" {
			t.Fatalf("expected compiled set_session_mode mode=acceptEdits, got %+v", actions[0].SetSessionMode)
		}
	})

	t.Run("skips set_session_mode with no mode", func(t *testing.T) {
		step := &wfmodels.WorkflowStep{
			ID: "s1",
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{
					{Type: wfmodels.OnEnterSetSessionMode},
				},
			},
		}
		spec := CompileStep(step)
		if len(spec.Events[TriggerOnEnter]) != 0 {
			t.Fatalf("expected set_session_mode with no mode to be skipped, got %d actions", len(spec.Events[TriggerOnEnter]))
		}
	})
}

func TestCompileStep_RequiresApproval(t *testing.T) {
	step := &wfmodels.WorkflowStep{
		ID:         "s1",
		WorkflowID: "wf1",
		Name:       "Review",
		Position:   1,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{
					Type:   wfmodels.OnTurnCompleteMoveToNext,
					Config: map[string]any{"requires_approval": true},
				},
				{
					Type: wfmodels.OnTurnCompleteMoveToStep,
					Config: map[string]any{
						"step_id":           "s3",
						"requires_approval": false,
					},
				},
				{
					Type: wfmodels.OnTurnCompleteDisablePlanMode,
				},
			},
		},
	}

	spec := CompileStep(step)
	actions := spec.Events[TriggerOnTurnComplete]

	if len(actions) != 3 {
		t.Fatalf("expected 3 on_turn_complete actions, got %d", len(actions))
	}
	if !actions[0].RequiresApproval {
		t.Fatalf("expected first action to require approval")
	}
	if actions[1].RequiresApproval {
		t.Fatalf("expected second action to not require approval")
	}
	if actions[2].RequiresApproval {
		t.Fatalf("expected disable_plan_mode to not require approval")
	}
}
