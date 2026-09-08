package models

import (
	"encoding/json"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestBuildWorkflowExportIncludesStepProfileSessionPolicies(t *testing.T) {
	export := BuildWorkflowExport([]*taskmodels.Workflow{{
		ID:   "workflow-1",
		Name: "Workflow",
	}}, map[string][]*WorkflowStep{"workflow-1": {{
		ID:                        "step-1",
		Name:                      "Step",
		ProfileSessionStartPolicy: taskmodels.WorkflowProfileSessionStartPolicyNew,
		ProfileSessionEndPolicy:   taskmodels.WorkflowProfileSessionEndPolicyPark,
	}}}, nil)

	payload, err := json.Marshal(export.Workflows[0])
	if err != nil {
		t.Fatalf("marshal portable workflow: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode portable workflow: %v", err)
	}
	stepFields := fields["steps"].([]any)[0].(map[string]any)
	if got := stepFields["profile_session_start_policy"]; got != string(taskmodels.WorkflowProfileSessionStartPolicyNew) {
		t.Fatalf("profile_session_start_policy = %v, want %q", got, taskmodels.WorkflowProfileSessionStartPolicyNew)
	}
	if got := stepFields["profile_session_end_policy"]; got != string(taskmodels.WorkflowProfileSessionEndPolicyPark) {
		t.Fatalf("profile_session_end_policy = %v, want %q", got, taskmodels.WorkflowProfileSessionEndPolicyPark)
	}
}
