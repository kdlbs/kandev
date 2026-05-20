package engine_adapters

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/workflow/engine"
)

// FirstStepResolver resolves a workflow's first runnable step. Implemented
// in production by the workflow service's StartStepResolver — re-declared
// here as a narrow interface so this adapter does not import the
// workflow service package.
type FirstStepResolver interface {
	ResolveStartStep(ctx context.Context, workflowID string) (string, error)
}

// TaskWorkflowMover swaps a task's workflow_id / workflow_step_id in
// place. Implemented in production by *tasksqlite.Repository.AddTaskToWorkflow.
type TaskWorkflowMover interface {
	AddTaskToWorkflow(ctx context.Context, taskID, workflowID, workflowStepID string, position int) error
}

// WorkflowSwitcherAdapter implements engine.WorkflowSwitcher. It mutates
// the task's workflow / step row and resolves a blank step id to the
// workflow's first runnable step before the update.
//
// The adapter does NOT fire on_exit / on_enter — engine.SwitchWorkflowCallback
// drives those triggers via its DispatchTriggerFn.
type WorkflowSwitcherAdapter struct {
	Resolver FirstStepResolver
	Mover    TaskWorkflowMover
}

// NewWorkflowSwitcherAdapter wires the first-step resolver and the task
// workflow mover.
func NewWorkflowSwitcherAdapter(resolver FirstStepResolver, mover TaskWorkflowMover) *WorkflowSwitcherAdapter {
	return &WorkflowSwitcherAdapter{Resolver: resolver, Mover: mover}
}

// SwitchTaskWorkflow satisfies engine.WorkflowSwitcher.
func (a *WorkflowSwitcherAdapter) SwitchTaskWorkflow(
	ctx context.Context, taskID, newWorkflowID, newStepID string,
) (string, error) {
	if taskID == "" {
		return "", fmt.Errorf("task_id is required")
	}
	if newWorkflowID == "" {
		return "", fmt.Errorf("workflow_id is required")
	}
	if a.Mover == nil {
		return "", fmt.Errorf("task workflow mover not configured")
	}
	resolvedStepID := newStepID
	if resolvedStepID == "" {
		if a.Resolver == nil {
			return "", fmt.Errorf("step_id is empty and first-step resolver not configured")
		}
		resolved, err := a.Resolver.ResolveStartStep(ctx, newWorkflowID)
		if err != nil {
			return "", fmt.Errorf("resolve first step of workflow %s: %w", newWorkflowID, err)
		}
		if resolved == "" {
			return "", fmt.Errorf("workflow %s has no runnable first step", newWorkflowID)
		}
		resolvedStepID = resolved
	}
	if err := a.Mover.AddTaskToWorkflow(ctx, taskID, newWorkflowID, resolvedStepID, 0); err != nil {
		return "", fmt.Errorf("update task %s to workflow %s/%s: %w",
			taskID, newWorkflowID, resolvedStepID, err)
	}
	return resolvedStepID, nil
}

// Compile-time interface assertion.
var _ engine.WorkflowSwitcher = (*WorkflowSwitcherAdapter)(nil)
