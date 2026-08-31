package dto

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestFromWorkflowIncludesProfileSessionPolicy(t *testing.T) {
	payload, err := json.Marshal(FromWorkflow(&models.Workflow{
		ID:                   "workflow-1",
		ProfileSessionPolicy: models.WorkflowProfileSessionPolicyParkReuse,
	}))
	if err != nil {
		t.Fatalf("marshal workflow DTO: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode workflow DTO: %v", err)
	}
	if got := fields["profile_session_policy"]; got != string(models.WorkflowProfileSessionPolicyParkReuse) {
		t.Fatalf("profile_session_policy = %v, want %q", got, models.WorkflowProfileSessionPolicyParkReuse)
	}
}

func TestFromWorkflowDefaultsMissingProfileSessionPolicy(t *testing.T) {
	workflow := FromWorkflow(&models.Workflow{ID: "workflow-1"})
	if workflow.ProfileSessionPolicy != models.WorkflowProfileSessionPolicyComplete {
		t.Fatalf("profile_session_policy = %q, want %q", workflow.ProfileSessionPolicy, models.WorkflowProfileSessionPolicyComplete)
	}
}
