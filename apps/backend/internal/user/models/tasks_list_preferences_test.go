package models

import "testing"

// @covers AC-UI-LIST-STEP-GROUPING-001.5
func TestNormalizeTasksListGroup(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"state", "workflow_step"},
		{" state ", "workflow_step"},
		{"workflow_step", "workflow_step"},
		{"", "workflow_step"},
		{"invalid", "workflow_step"},
		{"workflow", "workflow"},
		{"repository", "repository"},
		{"none", "none"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			if got := NormalizeTasksListGroup(tc.input); got != tc.want {
				t.Fatalf("NormalizeTasksListGroup(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestIsValidTasksListGroupWorkflowStep(t *testing.T) {
	for _, value := range []string{"workflow_step", "state", " workflow_step "} {
		if !IsValidTasksListGroup(value) {
			t.Errorf("IsValidTasksListGroup(%q) = false", value)
		}
	}
}
