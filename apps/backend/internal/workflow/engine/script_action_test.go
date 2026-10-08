package engine

import (
	"context"
	"reflect"
	"testing"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestCompileStepPreservesOrderedRunScriptActions(t *testing.T) {
	// @covers AC-TASKS-WORKFLOW-STEP-SCRIPT-001.1 AC-TASKS-WORKFLOW-STEP-SCRIPT-001.2 AC-TASKS-WORKFLOW-STEP-SCRIPT-001.3
	step := &wfmodels.WorkflowStep{Events: wfmodels.StepEvents{
		OnEnter: []wfmodels.OnEnterAction{
			{Type: wfmodels.OnEnterActionType("run_script"), Config: map[string]interface{}{"command": "echo first"}},
			{Type: wfmodels.OnEnterActionType("run_script"), Config: map[string]interface{}{
				"command":         "echo second",
				"timeout_seconds": 30,
				"failure_policy":  "continue",
			}},
		},
		OnTurnComplete: []wfmodels.OnTurnCompleteAction{{
			Type:   wfmodels.OnTurnCompleteActionType("run_script"),
			Config: map[string]interface{}{"command": "echo complete"},
		}},
		OnExit: []wfmodels.OnExitAction{{
			Type:   wfmodels.OnExitActionType("run_script"),
			Config: map[string]interface{}{"command": "echo exit"},
		}},
	}}

	compiled := CompileStep(step)
	assertRunScript := func(trigger Trigger, wantCommands []string) {
		actions := compiled.Events[trigger]
		if len(actions) != len(wantCommands) {
			t.Fatalf("%s compiled %d actions, want %d", trigger, len(actions), len(wantCommands))
		}
		for i, action := range actions {
			field := reflect.ValueOf(action).FieldByName("RunScript")
			if action.Kind != ActionKind("run_script") || !field.IsValid() || field.IsNil() {
				t.Fatalf("%s action %d = %+v, want run_script", trigger, i, action)
			}
			command := field.Elem().FieldByName("Command").String()
			if command != wantCommands[i] {
				t.Errorf("%s action %d command = %q, want %q", trigger, i, command, wantCommands[i])
			}
		}
	}

	assertRunScript(TriggerOnEnter, []string{"echo first", "echo second"})
	assertRunScript(TriggerOnTurnComplete, []string{"echo complete"})
	assertRunScript(TriggerOnExit, []string{"echo exit"})

	first := reflect.ValueOf(compiled.Events[TriggerOnEnter][0]).FieldByName("RunScript").Elem()
	second := reflect.ValueOf(compiled.Events[TriggerOnEnter][1]).FieldByName("RunScript").Elem()
	if first.FieldByName("TimeoutSeconds").Int() != 600 || first.FieldByName("FailurePolicy").String() != "block" {
		t.Errorf("default config = %+v, want 600 seconds and block", first.Interface())
	}
	if second.FieldByName("TimeoutSeconds").Int() != 30 || second.FieldByName("FailurePolicy").String() != "continue" {
		t.Errorf("explicit config = %+v, want 30 seconds and continue", second.Interface())
	}
}

func TestHandleTriggerCarriesOrderedActionPositionToScriptCallbacks(t *testing.T) {
	store := &fakeStore{
		state: MachineState{TaskID: "task-1", SessionID: "session-1", WorkflowID: "workflow-1", CurrentStepID: "step-1"},
		stepsByID: map[string]StepSpec{
			"step-1": {
				ID: "step-1", WorkflowID: "workflow-1",
				Events: map[Trigger][]Action{TriggerOnExit: {
					{Kind: ActionRunScript, RunScript: &wfmodels.WorkflowScriptAction{Command: "first"}},
					{Kind: ActionRunScript, RunScript: &wfmodels.WorkflowScriptAction{Command: "second"}},
				}},
			},
		},
		applied: map[string]bool{},
	}
	callback := &recordingCallback{}
	eng := New(store, MapRegistry{ActionRunScript: callback})
	result, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "task-1", SessionID: "session-1", Trigger: TriggerOnExit, OperationID: "transition-1",
	})
	if err != nil {
		t.Fatalf("HandleTrigger: %v", err)
	}
	if result.OperationID != "transition-1" {
		t.Fatalf("result operation id = %q, want %q", result.OperationID, "transition-1")
	}
	if len(callback.calls) != 2 {
		t.Fatalf("callback calls = %d, want 2", len(callback.calls))
	}
	if callback.calls[0].ActionPosition != 0 || callback.calls[1].ActionPosition != 1 {
		t.Fatalf("action positions = %d, %d, want 0, 1", callback.calls[0].ActionPosition, callback.calls[1].ActionPosition)
	}
}
