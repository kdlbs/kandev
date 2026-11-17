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

func TestFinalAndCompletingPredicates(t *testing.T) {
	fixtures := completionPredicateFixtures()
	for i := range fixtures {
		step := &fixtures[i]
		next := nextStepWithHigherPosition(fixtures, step)
		t.Run(step.ID, func(t *testing.T) {
			wantFinal := next == nil
			if got := IsFinalStep(step, next); got != wantFinal {
				t.Errorf("IsFinalStep() = %v, want %v", got, wantFinal)
			}
			wantCompleting := wantFinal && step.CompleteTaskOnEnter
			if got := CompletesTaskOnEnter(step, next); got != wantCompleting {
				t.Errorf("CompletesTaskOnEnter() = %v, want %v", got, wantCompleting)
			}
		})
	}

	if IsFinalStep(nil, nil) || CompletesTaskOnEnter(nil, nil) {
		t.Fatal("nil step must not be final or completing")
	}
}

func completionPredicateFixtures() []WorkflowStep {
	return []WorkflowStep{
		{ID: "backlog", WorkflowID: "workflow", Position: 0},
		{ID: "checked-nonfinal", WorkflowID: "workflow", Position: 2, CompleteTaskOnEnter: true},
		{ID: "unchecked-final", WorkflowID: "workflow", Position: 4},
		{ID: "checked-final-tie", WorkflowID: "workflow", Position: 4, CompleteTaskOnEnter: true},
	}
}

func nextStepWithHigherPosition(steps []WorkflowStep, step *WorkflowStep) *WorkflowStep {
	var next *WorkflowStep
	for i := range steps {
		candidate := &steps[i]
		if candidate.WorkflowID != step.WorkflowID || candidate.Position <= step.Position {
			continue
		}
		if next == nil || candidate.Position < next.Position {
			next = candidate
		}
	}
	return next
}
