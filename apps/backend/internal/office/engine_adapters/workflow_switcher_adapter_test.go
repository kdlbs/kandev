package engine_adapters

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeFirstStepResolver struct {
	stepID string
	err    error
}

func (f *fakeFirstStepResolver) ResolveStartStep(_ context.Context, _ string) (string, error) {
	return f.stepID, f.err
}

type fakeMover struct {
	calls []struct {
		TaskID, WorkflowID, StepID string
		Position                   int
	}
	err error
}

func (f *fakeMover) AddTaskToWorkflow(_ context.Context, taskID, workflowID, stepID string, position int) error {
	f.calls = append(f.calls, struct {
		TaskID, WorkflowID, StepID string
		Position                   int
	}{TaskID: taskID, WorkflowID: workflowID, StepID: stepID, Position: position})
	return f.err
}

func TestWorkflowSwitcherAdapter_ExplicitStep(t *testing.T) {
	mover := &fakeMover{}
	resolver := &fakeFirstStepResolver{stepID: "should-not-be-called"}
	a := NewWorkflowSwitcherAdapter(resolver, mover)
	got, err := a.SwitchTaskWorkflow(context.Background(), "task-1", "wf-2", "step-explicit")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "step-explicit" {
		t.Errorf("resolved step = %q, want step-explicit", got)
	}
	if len(mover.calls) != 1 || mover.calls[0].StepID != "step-explicit" {
		t.Errorf("mover call = %+v", mover.calls)
	}
}

func TestWorkflowSwitcherAdapter_BlankStepUsesResolver(t *testing.T) {
	mover := &fakeMover{}
	resolver := &fakeFirstStepResolver{stepID: "first-step"}
	a := NewWorkflowSwitcherAdapter(resolver, mover)
	got, err := a.SwitchTaskWorkflow(context.Background(), "task-1", "wf-2", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "first-step" {
		t.Errorf("resolved step = %q, want first-step", got)
	}
	if len(mover.calls) != 1 || mover.calls[0].StepID != "first-step" {
		t.Errorf("mover step = %s, want first-step", mover.calls[0].StepID)
	}
}

func TestWorkflowSwitcherAdapter_RequiresTaskID(t *testing.T) {
	a := NewWorkflowSwitcherAdapter(&fakeFirstStepResolver{}, &fakeMover{})
	_, err := a.SwitchTaskWorkflow(context.Background(), "", "wf-2", "step-1")
	if err == nil || !strings.Contains(err.Error(), "task_id") {
		t.Fatalf("expected task_id error, got: %v", err)
	}
}

func TestWorkflowSwitcherAdapter_RequiresWorkflowID(t *testing.T) {
	a := NewWorkflowSwitcherAdapter(&fakeFirstStepResolver{}, &fakeMover{})
	_, err := a.SwitchTaskWorkflow(context.Background(), "task-1", "", "step-1")
	if err == nil || !strings.Contains(err.Error(), "workflow_id") {
		t.Fatalf("expected workflow_id error, got: %v", err)
	}
}

func TestWorkflowSwitcherAdapter_BubblesResolverError(t *testing.T) {
	resolverErr := errors.New("resolve boom")
	a := NewWorkflowSwitcherAdapter(&fakeFirstStepResolver{err: resolverErr}, &fakeMover{})
	_, err := a.SwitchTaskWorkflow(context.Background(), "task-1", "wf-2", "")
	if err == nil || !errors.Is(err, resolverErr) {
		t.Fatalf("expected resolver error to bubble, got: %v", err)
	}
}

func TestWorkflowSwitcherAdapter_ResolverReturnsEmpty(t *testing.T) {
	a := NewWorkflowSwitcherAdapter(&fakeFirstStepResolver{stepID: ""}, &fakeMover{})
	_, err := a.SwitchTaskWorkflow(context.Background(), "task-1", "wf-2", "")
	if err == nil || !strings.Contains(err.Error(), "no runnable first step") {
		t.Fatalf("expected first-step error, got: %v", err)
	}
}

func TestWorkflowSwitcherAdapter_BubblesMoverError(t *testing.T) {
	moveErr := errors.New("update boom")
	a := NewWorkflowSwitcherAdapter(&fakeFirstStepResolver{stepID: "s"}, &fakeMover{err: moveErr})
	_, err := a.SwitchTaskWorkflow(context.Background(), "task-1", "wf-2", "")
	if err == nil || !errors.Is(err, moveErr) {
		t.Fatalf("expected mover error to bubble, got: %v", err)
	}
}
