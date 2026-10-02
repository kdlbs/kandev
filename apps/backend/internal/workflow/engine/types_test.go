package engine

import (
	"context"
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
				{Type: wfmodels.GenericActionMoveToNext, Config: map[string]any{"requires_approval": true}},
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
	if !actions[0].RequiresApproval {
		t.Fatalf("expected generic move_to_next to preserve requires_approval")
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

func TestCompileStep_CompilesGenericTransitionGuards(t *testing.T) {
	step := &wfmodels.WorkflowStep{
		ID:         "s1",
		WorkflowID: "wf1",
		Name:       "Review",
		Events: wfmodels.StepEvents{
			OnChildrenCompleted: []wfmodels.GenericAction{
				{
					Type: wfmodels.GenericActionMoveToStep,
					Config: map[string]any{
						"step_id": "s2",
						"if": map[string]any{
							"wait_for_quorum": map[string]any{
								"role":      "reviewer",
								"threshold": "all_approve",
							},
						},
					},
				},
			},
		},
	}

	spec := CompileStep(step)
	action := spec.Events[TriggerOnChildrenCompleted][0]
	if action.Guard == nil || action.Guard.WaitForQuorum == nil {
		t.Fatalf("expected generic move_to_step to preserve transition guard")
	}
	if action.Guard.WaitForQuorum.Role != "reviewer" {
		t.Fatalf("guard role = %q, want reviewer", action.Guard.WaitForQuorum.Role)
	}
	if action.Guard.WaitForQuorum.Threshold != "all_approve" {
		t.Fatalf("guard threshold = %q, want all_approve", action.Guard.WaitForQuorum.Threshold)
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

// TestCompileStep_OnTurnCompleteMovesMatchAdvancesOnTurnComplete keeps the
// model-level WorkflowStep.AdvancesOnTurnComplete in step with the workflow
// engine's actual transition result for the same actions.
func TestCompileStep_OnTurnCompleteMovesMatchAdvancesOnTurnComplete(t *testing.T) {
	const guardedCase = "wait_for_quorum guarded move"
	const guardedSelfThenMoveCase = "guarded self-target falls through to later move"
	cases := map[string][]wfmodels.OnTurnCompleteAction{
		"no actions":          nil,
		"disable_plan_mode":   {{Type: wfmodels.OnTurnCompleteDisablePlanMode}},
		"move_to_next":        {{Type: wfmodels.OnTurnCompleteMoveToNext}},
		"move_to_previous":    {{Type: wfmodels.OnTurnCompleteMoveToPrevious}},
		"move_to_step":        {{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]any{"step_id": "s3"}}},
		"self-targeting move": {{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]any{"step_id": "current"}}},
		"self-target then valid move": {
			{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]any{"step_id": "current"}},
			{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]any{"step_id": "s3"}},
		},
		"invalid guard self-target blocks later move": {
			{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]any{
				"step_id": "current",
				"if":      map[string]any{"wait_for_quorum": map[string]any{"role": "", "threshold": QuorumAllApprove}},
			}},
			{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]any{"step_id": "s3"}},
		},
		guardedSelfThenMoveCase: {
			{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]any{
				"step_id": "current",
				"if":      map[string]any{"wait_for_quorum": map[string]any{"role": "approver", "threshold": QuorumAllApprove}},
			}},
			{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]any{"step_id": "s3"}},
		},
		"move_to_step without config": {{Type: wfmodels.OnTurnCompleteMoveToStep}},
		"move_to_step empty step_id":  {{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]any{"step_id": ""}}},
		"requires_approval move": {{
			Type:   wfmodels.OnTurnCompleteMoveToNext,
			Config: map[string]any{"requires_approval": true},
		}},
		"malformed then valid move": {
			{Type: wfmodels.OnTurnCompleteMoveToStep},
			{Type: wfmodels.OnTurnCompleteMoveToPrevious},
		},
		guardedCase: {{
			Type: wfmodels.OnTurnCompleteMoveToStep,
			Config: map[string]any{
				"step_id": "s3",
				"if":      map[string]any{"wait_for_quorum": map[string]any{"role": "approver", "threshold": QuorumAllApprove}},
			},
		}},
	}
	for name, actions := range cases {
		t.Run(name, func(t *testing.T) {
			step := &wfmodels.WorkflowStep{ID: "current", WorkflowID: "wf", Events: wfmodels.StepEvents{OnTurnComplete: actions}}
			spec := CompileStep(step)
			compiled := spec.Events[TriggerOnTurnComplete]
			if name == guardedCase && (len(compiled) != 1 || compiled[0].Guard == nil) {
				t.Fatalf("expected one move compiled with a wait_for_quorum guard, got %+v", compiled)
			}
			if name == guardedSelfThenMoveCase && (len(compiled) != 2 || compiled[0].Guard == nil) {
				t.Fatalf("expected guarded self-target followed by a move, got %+v", compiled)
			}

			store := &fakeStore{
				nextSteps: map[int]StepSpec{0: {ID: "next", Position: 1}},
				prevSteps: map[int]StepSpec{0: {ID: "previous", Position: -1}},
			}
			decisions := newFakeDecisionStore()
			participants := fakeParticipants{list: []ParticipantInfo{{ID: "p1", Role: "approver", AgentProfileID: "approver-1", DecisionRequired: true}}}
			if name == guardedCase {
				decisions.byKey[dkey("task", "current")] = []DecisionInfo{{TaskID: "task", StepID: "current", ParticipantID: "p1", Decision: DecisionApproved}}
			}
			eng := New(store, MapRegistry{}, WithDecisionStore(decisions), WithParticipantStore(participants))
			result, err := eng.processActions(context.Background(), HandleInput{
				TaskID: "task", SessionID: "session", Trigger: TriggerOnTurnComplete, EvaluateOnly: true,
			}, MachineState{TaskID: "task", SessionID: "session", WorkflowID: "wf", CurrentStepID: "current"}, spec, compiled, nil)
			if err != nil {
				t.Fatalf("processActions() error = %v", err)
			}
			engineMoves := result.Transitioned
			if got := step.AdvancesOnTurnComplete(); got != engineMoves {
				t.Fatalf("AdvancesOnTurnComplete() = %t, engine transition result = %t (%+v)", got, engineMoves, result)
			}
		})
	}
}
