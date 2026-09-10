package models

import "testing"

func TestWorkflowScriptRunStatusTransitionsAreOneWay(t *testing.T) {
	tests := []struct {
		from WorkflowScriptRunStatus
		to   WorkflowScriptRunStatus
		want bool
	}{
		{WorkflowScriptRunPending, WorkflowScriptRunStarting, true},
		{WorkflowScriptRunStarting, WorkflowScriptRunRunning, true},
		{WorkflowScriptRunRunning, WorkflowScriptRunSucceeded, true},
		{WorkflowScriptRunStarting, WorkflowScriptRunFailed, true},
		{WorkflowScriptRunPending, WorkflowScriptRunInterrupted, true},
		{WorkflowScriptRunSucceeded, WorkflowScriptRunRunning, false},
		{WorkflowScriptRunInterrupted, WorkflowScriptRunStarting, false},
		{WorkflowScriptRunRunning, WorkflowScriptRunPending, false},
	}
	for _, tt := range tests {
		if got := tt.from.CanTransitionTo(tt.to); got != tt.want {
			t.Errorf("%q -> %q = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestWorkflowScriptRunTriggerAndFailurePolicyValidation(t *testing.T) {
	if !WorkflowScriptRunTriggerOnEnter.IsValid() || !WorkflowScriptRunTriggerOnTurnComplete.IsValid() || !WorkflowScriptRunTriggerOnExit.IsValid() {
		t.Fatal("known script run triggers rejected")
	}
	if WorkflowScriptRunTrigger("manual").IsValid() {
		t.Fatal("unknown script run trigger accepted")
	}
	if !IsValidWorkflowScriptFailurePolicy(WorkflowScriptFailurePolicyBlock) || !IsValidWorkflowScriptFailurePolicy(WorkflowScriptFailurePolicyContinue) {
		t.Fatal("known failure policy rejected")
	}
	if IsValidWorkflowScriptFailurePolicy("retry") {
		t.Fatal("unknown failure policy accepted")
	}
}
