package runtime

import (
	"context"
	"errors"
	"testing"

	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

// TestSelectHandoffDestinationStep is AC-CROSS-WORKSPACE-TASK-HANDOFF-
// PROFILES-001.8's exhaustive selection-algorithm table. Every prior test of
// the handoff surface wires a single-step fixture, which cannot distinguish
// "picks the only step" from "picks the right step among several" — this
// exercises ordering, the position/id tiebreak, the startAgent auto-start
// preference, and both of its fallbacks directly against the pure selector.
func TestSelectHandoffDestinationStep(t *testing.T) {
	autoStart := workflowmodels.StepEvents{OnEnter: []workflowmodels.OnEnterAction{{Type: workflowmodels.OnEnterAutoStartAgent}}}

	tests := []struct {
		name       string
		steps      []*workflowmodels.WorkflowStep
		startAgent bool
		want       string // step ID, "" for nil
	}{
		{
			name:       "empty steps resolves to nothing",
			steps:      nil,
			startAgent: false,
			want:       "",
		},
		{
			name: "nil entries in the slice are skipped",
			steps: []*workflowmodels.WorkflowStep{
				nil,
				{ID: "step-a", Position: 0, IsStartStep: true},
				nil,
			},
			startAgent: false,
			want:       "step-a",
		},
		{
			name: "startAgent false picks the start step regardless of position order",
			steps: []*workflowmodels.WorkflowStep{
				{ID: "step-a", Position: 0},
				{ID: "step-b", Position: 1, IsStartStep: true},
			},
			startAgent: false,
			want:       "step-b",
		},
		{
			name: "startAgent false with no start step falls back to earliest position",
			steps: []*workflowmodels.WorkflowStep{
				{ID: "step-b", Position: 1},
				{ID: "step-a", Position: 0},
			},
			startAgent: false,
			want:       "step-a",
		},
		{
			name: "equal position ties break on ascending step id",
			steps: []*workflowmodels.WorkflowStep{
				{ID: "step-z", Position: 0},
				{ID: "step-a", Position: 0},
			},
			startAgent: false,
			want:       "step-a",
		},
		{
			name: "startAgent true prefers the earliest auto-start step over a later start step",
			steps: []*workflowmodels.WorkflowStep{
				{ID: "step-a", Position: 0, IsStartStep: true},
				{ID: "step-b", Position: 1, Events: autoStart},
			},
			startAgent: true,
			want:       "step-b",
		},
		{
			name: "startAgent true picks the earliest auto-start step among several",
			steps: []*workflowmodels.WorkflowStep{
				{ID: "step-b", Position: 1, Events: autoStart},
				{ID: "step-a", Position: 0, Events: autoStart},
			},
			startAgent: true,
			want:       "step-a",
		},
		{
			name: "startAgent true with no auto-start step falls back to the start-step rule",
			steps: []*workflowmodels.WorkflowStep{
				{ID: "step-a", Position: 0},
				{ID: "step-b", Position: 1, IsStartStep: true},
			},
			startAgent: true,
			want:       "step-b",
		},
		{
			name: "startAgent true with neither auto-start nor start step falls back to earliest overall",
			steps: []*workflowmodels.WorkflowStep{
				{ID: "step-b", Position: 1},
				{ID: "step-a", Position: 0},
			},
			startAgent: true,
			want:       "step-a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectHandoffDestinationStep(tt.steps, tt.startAgent)
			if tt.want == "" {
				if got != nil {
					t.Fatalf("got step %q, want nil", got.ID)
				}
				return
			}
			if got == nil {
				t.Fatalf("got nil, want step %q", tt.want)
			}
			if got.ID != tt.want {
				t.Fatalf("got step %q, want %q", got.ID, tt.want)
			}
		})
	}
}

// TestResolveHandoffDestinationStep_NoResolvableStepIsValidationError is
// AC-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001.7's configuration half: a
// workflow with zero steps (steps list successfully but yield no resolvable
// step) must be reported as a *HandoffValidationError (HTTP 400), not the
// generic error a listing failure produces.
func TestResolveHandoffDestinationStep_NoResolvableStepIsValidationError(t *testing.T) {
	actions := NewActions(ActionDependencies{Handoff: HandoffDependencies{
		Workflows: &fakeHandoffWorkflowSteps{steps: nil},
	}})

	_, err := actions.resolveHandoffDestinationStep(context.Background(), "wf-empty", false)

	var validation *HandoffValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v (%T), want *HandoffValidationError", err, err)
	}
}

// TestResolveHandoffDestinationStep_ListFailureIsNotValidationError is AC-
// CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001.7's failure half: a failure to
// read the steps must be distinguishable from the configuration case above
// so the route can answer 500 (safe to retry), not 400 (caller error).
func TestResolveHandoffDestinationStep_ListFailureIsNotValidationError(t *testing.T) {
	actions := NewActions(ActionDependencies{Handoff: HandoffDependencies{
		Workflows: &fakeHandoffWorkflowSteps{err: errors.New("workflow store unavailable")},
	}})

	_, err := actions.resolveHandoffDestinationStep(context.Background(), "wf-broken", false)

	if err == nil {
		t.Fatal("error = nil, want a propagated listing failure")
	}
	var validation *HandoffValidationError
	if errors.As(err, &validation) {
		t.Fatalf("error = %v, want a non-validation error distinguishable from the no-resolvable-step case", err)
	}
}
