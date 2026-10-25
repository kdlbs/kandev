package models

import "testing"

func TestIsTerminalStepName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "done", want: true},
		{name: "Done", want: true},
		{name: "DONE", want: true},
		{name: " Done ", want: true},
		{name: "complete", want: true},
		{name: "Complete", want: true},
		{name: "COMPLETE", want: true},
		{name: " Complete ", want: true},
		{name: "completed", want: true},
		{name: "Completed", want: true},
		{name: "COMPLETED", want: true},
		{name: " Completed ", want: true},
		{name: "approved", want: true},
		{name: "Approved", want: true},
		{name: "APPROVED", want: true},
		{name: " Approved ", want: true},
		{name: "Work", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTerminalStepName(tt.name); got != tt.want {
				t.Fatalf("IsTerminalStepName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestIsTerminalStep(t *testing.T) {
	t.Run("nil step", func(t *testing.T) {
		if IsTerminalStep(nil, nil) {
			t.Fatalf("nil step reported terminal")
		}
	})

	t.Run("checked final step", func(t *testing.T) {
		step := &WorkflowStep{Name: "Release", CompleteTaskOnEnter: true}
		if !IsTerminalStep(step, nil) {
			t.Fatalf("checked final step was not terminal")
		}
	})

	t.Run("unchecked final step", func(t *testing.T) {
		step := &WorkflowStep{Name: "Done"}
		if IsTerminalStep(step, nil) {
			t.Fatalf("unchecked final step reported terminal")
		}
	})

	t.Run("checked non-final step", func(t *testing.T) {
		step := &WorkflowStep{Name: "Release", CompleteTaskOnEnter: true}
		nextStep := &WorkflowStep{Name: "Archive"}
		if IsTerminalStep(step, nextStep) {
			t.Fatalf("checked non-final step reported terminal")
		}
	})

	t.Run("checked arbitrary final step", func(t *testing.T) {
		step := &WorkflowStep{Name: "Ready for launch", CompleteTaskOnEnter: true}
		if !IsTerminalStep(step, nil) {
			t.Fatalf("checked arbitrary final step reported non-terminal")
		}
	})
}
