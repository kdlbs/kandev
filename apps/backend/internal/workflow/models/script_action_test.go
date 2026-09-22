package models

import (
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestValidateStepEventsRejectsMalformedRunScript(t *testing.T) {
	// @covers AC-TASKS-WORKFLOW-STEP-SCRIPT-001.5
	tests := []struct {
		name   string
		config map[string]interface{}
	}{
		{
			name:   "empty command",
			config: map[string]interface{}{"command": "   "},
		},
		{
			name:   "timeout below minimum",
			config: map[string]interface{}{"command": "echo ok", "timeout_seconds": 0},
		},
		{
			name:   "timeout above maximum",
			config: map[string]interface{}{"command": "echo ok", "timeout_seconds": 86401},
		},
		{
			name:   "invalid failure policy",
			config: map[string]interface{}{"command": "echo ok", "failure_policy": "retry"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := StepEvents{OnEnter: []OnEnterAction{{
				Type:   OnEnterActionType("run_script"),
				Config: tt.config,
			}}}
			if err := ValidateStepEvents(events, false); err == nil {
				t.Fatal("ValidateStepEvents accepted malformed run_script")
			}
		})
	}
}

func TestValidateStepEventsAcceptsOrderedRunScriptsWithDefaults(t *testing.T) {
	// @covers AC-TASKS-WORKFLOW-STEP-SCRIPT-001.1 AC-TASKS-WORKFLOW-STEP-SCRIPT-001.2 AC-TASKS-WORKFLOW-STEP-SCRIPT-001.3
	events := StepEvents{
		OnEnter: []OnEnterAction{
			{Type: OnEnterActionType("run_script"), Config: map[string]interface{}{"command": "echo first"}},
			{Type: OnEnterActionType("run_script"), Config: map[string]interface{}{
				"command":         "echo second",
				"timeout_seconds": 30,
				"failure_policy":  "continue",
			}},
		},
		OnTurnComplete: []OnTurnCompleteAction{{
			Type:   OnTurnCompleteActionType("run_script"),
			Config: map[string]interface{}{"command": "echo complete"},
		}},
		OnExit: []OnExitAction{{
			Type:   OnExitActionType("run_script"),
			Config: map[string]interface{}{"command": "echo exit"},
		}},
	}
	if err := ValidateStepEvents(events, false); err != nil {
		t.Fatalf("ValidateStepEvents rejected valid run_script actions: %v", err)
	}
}

func TestBuildWorkflowExportNormalizesRunScriptDefaults(t *testing.T) {
	// @covers AC-TASKS-WORKFLOW-STEP-SCRIPT-001.4
	workflow := &taskmodels.Workflow{ID: "workflow-1", Name: "Workflow"}
	steps := map[string][]*WorkflowStep{
		workflow.ID: {{
			ID:         "step-1",
			WorkflowID: workflow.ID,
			Name:       "Build",
			Events: StepEvents{OnEnter: []OnEnterAction{{
				Type:   OnEnterRunScript,
				Config: map[string]interface{}{"command": "echo exact"},
			}}},
		}},
	}

	export := BuildWorkflowExport([]*taskmodels.Workflow{workflow}, steps, nil)
	config := export.Workflows[0].Steps[0].Events.OnEnter[0].Config
	if config["command"] != "echo exact" {
		t.Fatalf("command = %#v, want exact command text", config["command"])
	}
	if config["timeout_seconds"] != WorkflowScriptDefaultTimeoutSeconds {
		t.Errorf("timeout_seconds = %#v, want %d", config["timeout_seconds"], WorkflowScriptDefaultTimeoutSeconds)
	}
	if config["failure_policy"] != string(WorkflowScriptFailurePolicyBlock) {
		t.Errorf("failure_policy = %#v, want block", config["failure_policy"])
	}
}
