package models

import (
	"encoding/json"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestBuildWorkflowExportIncludesProfileSessionPolicy(t *testing.T) {
	export := BuildWorkflowExport([]*taskmodels.Workflow{{
		ID:   "workflow-1",
		Name: "Workflow",
	}}, map[string][]*WorkflowStep{"workflow-1": {{
		ID:                   "step-1",
		Name:                 "Step",
		ProfileSessionPolicy: taskmodels.WorkflowProfileSessionPolicyParkNew,
	}}}, nil)

	payload, err := json.Marshal(export.Workflows[0])
	if err != nil {
		t.Fatalf("marshal portable workflow: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode portable workflow: %v", err)
	}
	if got := fields["steps"].([]any)[0].(map[string]any)["profile_session_policy"]; got != string(taskmodels.WorkflowProfileSessionPolicyParkNew) {
		t.Fatalf("profile_session_policy = %v, want %q", got, taskmodels.WorkflowProfileSessionPolicyParkNew)
	}
}
