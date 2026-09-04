package models

import "testing"

func TestNormalizeWorkflowProfileSessionStartPolicy(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  WorkflowProfileSessionStartPolicy
	}{
		{name: "empty defaults to reuse", value: "", want: WorkflowProfileSessionStartPolicyReuse},
		{name: "unknown defaults to reuse", value: "retain", want: WorkflowProfileSessionStartPolicyReuse},
		{name: "canonical value is preserved", value: "reuse", want: WorkflowProfileSessionStartPolicyReuse},
		{name: "known value is canonicalized", value: " new ", want: WorkflowProfileSessionStartPolicyNew},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeWorkflowProfileSessionStartPolicy(tt.value); got != tt.want {
				t.Fatalf("NormalizeWorkflowProfileSessionStartPolicy(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestNormalizeWorkflowProfileSessionEndPolicy(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  WorkflowProfileSessionEndPolicy
	}{
		{name: "empty defaults to complete", value: "", want: WorkflowProfileSessionEndPolicyComplete},
		{name: "unknown defaults to complete", value: "retain", want: WorkflowProfileSessionEndPolicyComplete},
		{name: "canonical value is preserved", value: "complete", want: WorkflowProfileSessionEndPolicyComplete},
		{name: "known value is canonicalized", value: " park ", want: WorkflowProfileSessionEndPolicyPark},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeWorkflowProfileSessionEndPolicy(tt.value); got != tt.want {
				t.Fatalf("NormalizeWorkflowProfileSessionEndPolicy(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
