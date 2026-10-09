package models

import (
	"testing"
)

func TestWorkflowAgentOverridesNormalizeAndResolveByStep(t *testing.T) {
	t.Parallel()

	overrides, err := NewWorkflowAgentOverrides("workflow-1", []WorkflowAgentOverrideBinding{
		{StepID: "analysis", SourceProfileID: "profile-a", ReplacementProfileID: "profile-a"},
		{StepID: "implement", SourceProfileID: "profile-a", ReplacementProfileID: "profile-b"},
		{StepID: "review", SourceProfileID: "profile-a", ReplacementProfileID: "profile-b"},
		{StepID: "pr", SourceProfileID: "profile-b", ReplacementProfileID: "profile-c"},
	})
	if err != nil {
		t.Fatalf("NewWorkflowAgentOverrides: %v", err)
	}
	if overrides == nil {
		t.Fatal("expected non-empty overrides")
	}
	if len(overrides.Steps) != 3 {
		t.Fatalf("binding count = %d, want 3", len(overrides.Steps))
	}

	if got, ok := overrides.ReplacementFor("workflow-1", "implement"); !ok || got != "profile-b" {
		t.Fatalf("implement replacement = %q, %v; want profile-b, true", got, ok)
	}
	if got, ok := overrides.ReplacementFor("workflow-1", "pr"); !ok || got != "profile-c" {
		t.Fatalf("pr replacement = %q, %v; want profile-c, true", got, ok)
	}
	if got, ok := overrides.ReplacementFor("workflow-1", "analysis"); ok || got != "" {
		t.Fatalf("self replacement = %q, %v; want empty, false", got, ok)
	}
	if got, ok := overrides.ReplacementFor("workflow-2", "implement"); ok || got != "" {
		t.Fatalf("foreign workflow replacement = %q, %v; want empty, false", got, ok)
	}
}

func TestWorkflowAgentOverridesRoundTripAndRejectMalformedRecords(t *testing.T) {
	t.Parallel()

	overrides, err := NewWorkflowAgentOverrides("workflow-1", []WorkflowAgentOverrideBinding{
		{StepID: "implement", SourceProfileID: "profile-a", ReplacementProfileID: "profile-b"},
	})
	if err != nil {
		t.Fatalf("NewWorkflowAgentOverrides: %v", err)
	}
	raw, err := EncodeWorkflowAgentOverrides(overrides)
	if err != nil {
		t.Fatalf("EncodeWorkflowAgentOverrides: %v", err)
	}
	decoded, err := DecodeWorkflowAgentOverrides(raw)
	if err != nil {
		t.Fatalf("DecodeWorkflowAgentOverrides: %v", err)
	}
	if decoded == nil || decoded.WorkflowID != "workflow-1" || len(decoded.Steps) != 1 {
		t.Fatalf("decoded = %#v", decoded)
	}

	for _, raw := range []string{
		`{"workflow_id":"workflow-1","steps":[{"step_id":"","source_profile_id":"a","replacement_profile_id":"b"}]}`,
		`{"workflow_id":"workflow-1","steps":[{"step_id":"step","source_profile_id":"a","replacement_profile_id":"b"},{"step_id":"step","source_profile_id":"a","replacement_profile_id":"c"}]}`,
		`{"workflow_id":"workflow-1","steps":[{"step_id":"step","source_profile_id":"a","replacement_profile_id":""}]}`,
	} {
		if _, err := DecodeWorkflowAgentOverrides(raw); err == nil {
			t.Fatalf("DecodeWorkflowAgentOverrides(%s) succeeded", raw)
		}
	}
}
