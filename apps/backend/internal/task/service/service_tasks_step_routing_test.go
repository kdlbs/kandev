package service

import (
	"context"
	"testing"
)

// recordingStepResolver reports which resolver CreateTask consulted, so the
// routing rule can be asserted without a workflow database.
type recordingStepResolver struct {
	called string
}

func (r *recordingStepResolver) ResolveStartStep(context.Context, string) (string, error) {
	r.called = "start"
	return "start-step", nil
}

func (r *recordingStepResolver) ResolveFirstStep(context.Context, string) (string, error) {
	r.called = "first"
	return "first-step", nil
}

func (r *recordingStepResolver) ResolveAutoStartStep(context.Context, string) (string, error) {
	r.called = "auto-start"
	return "auto-start-step", nil
}

// A create that is about to launch an agent belongs in the first step that runs
// agents, not in the workflow's start step. Routing it through ResolveStartStep
// made `is_start_step` and `auto_start_agent` synonymous: marking Backlog as the
// start step parked the task there and started an agent in it anyway.
func TestResolveWorkflowStep_RoutesByCreateIntent(t *testing.T) {
	cases := []struct {
		name         string
		req          *CreateTaskRequest
		wantResolver string
		wantStepID   string
	}{
		{
			name:         "starting an agent uses the first auto-start step",
			req:          &CreateTaskRequest{WorkflowID: "wf-1", StartAgent: true},
			wantResolver: "auto-start",
			wantStepID:   "auto-start-step",
		},
		{
			name:         "creating without an agent uses the start step",
			req:          &CreateTaskRequest{WorkflowID: "wf-1"},
			wantResolver: "start",
			wantStepID:   "start-step",
		},
		{
			name:         "plan mode still uses the first step by position",
			req:          &CreateTaskRequest{WorkflowID: "wf-1", PlanMode: true},
			wantResolver: "first",
			wantStepID:   "first-step",
		},
		{
			name:         "plan mode wins over an agent start",
			req:          &CreateTaskRequest{WorkflowID: "wf-1", PlanMode: true, StartAgent: true},
			wantResolver: "first",
			wantStepID:   "first-step",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _ := createTestService(t)
			resolver := &recordingStepResolver{}
			svc.SetStartStepResolver(resolver)

			got := svc.resolveWorkflowStep(context.Background(), tc.req)

			if resolver.called != tc.wantResolver {
				t.Errorf("consulted %q resolver; want %q", resolver.called, tc.wantResolver)
			}
			if got != tc.wantStepID {
				t.Errorf("resolveWorkflowStep() = %q; want %q", got, tc.wantStepID)
			}
		})
	}
}

// An explicit workflow_step_id is the caller's decision and outranks every rule
// above — dragging a card into a column, or a watcher targeting a named step,
// must not be re-routed by the create intent.
func TestResolveWorkflowStep_ExplicitStepWins(t *testing.T) {
	svc, _, _ := createTestService(t)
	resolver := &recordingStepResolver{}
	svc.SetStartStepResolver(resolver)

	got := svc.resolveWorkflowStep(context.Background(), &CreateTaskRequest{
		WorkflowID:     "wf-1",
		WorkflowStepID: "chosen-step",
		StartAgent:     true,
	})

	if got != "chosen-step" {
		t.Errorf("resolveWorkflowStep() = %q; want %q", got, "chosen-step")
	}
	if resolver.called != "" {
		t.Errorf("consulted the %q resolver; an explicit step must short-circuit", resolver.called)
	}
}
