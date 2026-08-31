package dto

import (
	"encoding/json"
	"testing"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestFromWorkflowStepIncludesProfileSessionPolicy(t *testing.T) {
	payload, err := json.Marshal(FromWorkflowStep(&wfmodels.WorkflowStep{
		ID:                   "step-1",
		ProfileSessionPolicy: "park_reuse",
	}))
	if err != nil {
		t.Fatalf("marshal workflow DTO: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode workflow DTO: %v", err)
	}
	if got := fields["profile_session_policy"]; got != "park_reuse" {
		t.Fatalf("profile_session_policy = %v, want park_reuse", got)
	}
}

func TestFromWorkflowStepDefaultsMissingProfileSessionPolicy(t *testing.T) {
	step := FromWorkflowStep(&wfmodels.WorkflowStep{ID: "step-1"})
	if step.ProfileSessionPolicy != "complete" {
		t.Fatalf("profile_session_policy = %q, want complete", step.ProfileSessionPolicy)
	}
}
