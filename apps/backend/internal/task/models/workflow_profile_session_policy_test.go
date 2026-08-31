package models

import "testing"

func TestNormalizeWorkflowProfileSessionPolicy(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  WorkflowProfileSessionPolicy
	}{
		{name: "empty defaults to complete", value: "", want: WorkflowProfileSessionPolicyComplete},
		{name: "unknown defaults to complete", value: "retain", want: WorkflowProfileSessionPolicyComplete},
		{name: "canonical value is preserved", value: "park_reuse", want: WorkflowProfileSessionPolicyParkReuse},
		{name: "known value is canonicalized", value: " park_new ", want: WorkflowProfileSessionPolicyParkNew},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeWorkflowProfileSessionPolicy(tt.value); got != tt.want {
				t.Fatalf("NormalizeWorkflowProfileSessionPolicy(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
