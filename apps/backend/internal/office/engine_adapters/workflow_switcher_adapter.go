package engine_adapters

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/steptelemetry"
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
	attachCtx := steptelemetry.WithAttribution(ctx, workflowAttachedAttribution(ctx))
	if err := a.Mover.AddTaskToWorkflow(attachCtx, taskID, newWorkflowID, resolvedStepID, 0); err != nil {
		return "", fmt.Errorf("update task %s to workflow %s/%s: %w",
			taskID, newWorkflowID, resolvedStepID, err)
	}
	return resolvedStepID, nil
}

// workflowAttachedAttribution is this adapter's ledger row attribution: the
// trigger is always workflow_attached (SwitchTaskWorkflow is that trigger's
// sole production meaning), and the actor prefers an explicit attribution
// already on ctx over recomputing one. The engine's SwitchWorkflowCallback
// (this adapter's sole caller) sets one carrying the causal session's ID
// before calling here, since a switch_workflow action is always genuinely
// caused by a session's turn — the authn-seam fallback below would
// otherwise silently record actor_kind=system for a session-caused
// transition, because an engine-internal ctx carries no authenticated
// identity. The fallback still exists for any future caller that doesn't
// set attribution.
func workflowAttachedAttribution(ctx context.Context) steptelemetry.Attribution {
	if preset := steptelemetry.FromContext(ctx); preset.ActorKind != steptelemetry.ActorUnknown {
		return steptelemetry.Attribution{
			Trigger:   steptelemetry.TriggerWorkflowAttached,
			ActorKind: preset.ActorKind,
			ActorID:   preset.ActorID,
			SessionID: preset.SessionID,
		}
	}
	actorKind, actorID := steptelemetry.HumanOrSystemActor(ctx)
	return steptelemetry.Attribution{
		Trigger:   steptelemetry.TriggerWorkflowAttached,
		ActorKind: actorKind,
		ActorID:   actorID,
	}
}

// Compile-time interface assertion.
var _ engine.WorkflowSwitcher = (*WorkflowSwitcherAdapter)(nil)
