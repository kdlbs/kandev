package dto

import (
	"encoding/json"
	"testing"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestFromWorkflowStepIncludesProfileSessionPolicies(t *testing.T) {
	payload, err := json.Marshal(FromWorkflowStep(&wfmodels.WorkflowStep{
		ID:                        "step-1",
		ProfileSessionStartPolicy: "new",
		ProfileSessionEndPolicy:   "park",
	}))
	if err != nil {
		t.Fatalf("marshal workflow DTO: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode workflow DTO: %v", err)
	}
	if got := fields["profile_session_start_policy"]; got != "new" {
		t.Fatalf("profile_session_start_policy = %v, want new", got)
	}
	if got := fields["profile_session_end_policy"]; got != "park" {
		t.Fatalf("profile_session_end_policy = %v, want park", got)
	}
}

func TestFromWorkflowStepDefaultsMissingProfileSessionPolicies(t *testing.T) {
	step := FromWorkflowStep(&wfmodels.WorkflowStep{ID: "step-1"})
	if step.ProfileSessionStartPolicy != "reuse" || step.ProfileSessionEndPolicy != "complete" {
		t.Fatalf("profile session policies = %q/%q, want reuse/complete", step.ProfileSessionStartPolicy, step.ProfileSessionEndPolicy)
	}
}
