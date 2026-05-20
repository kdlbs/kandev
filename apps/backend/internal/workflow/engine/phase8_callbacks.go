package engine

import (
	"context"
	"fmt"
)

// CreateChildTaskCallback executes the create_child_task action by asking
// the wired TaskCreator to create a new task whose parent is the trigger's
// task id.
//
// The new task's on_enter trigger fires naturally once the row lands —
// the workflow engine evaluates it the first time the child agent starts
// running. This callback is fire-and-forget on success.
type CreateChildTaskCallback struct {
	Creator TaskCreator
}

// Execute satisfies ActionCallback.
func (c CreateChildTaskCallback) Execute(ctx context.Context, in ActionInput) (ActionResult, error) {
	if c.Creator == nil {
		return ActionResult{}, fmt.Errorf("%w: create_child_task requires TaskCreator", ErrActionNotYetWired)
	}
	if in.Action.CreateChildTask == nil {
		return ActionResult{}, fmt.Errorf("create_child_task action missing CreateChildTask config")
	}
	if in.State.TaskID == "" {
		return ActionResult{}, fmt.Errorf("create_child_task: trigger has no task id")
	}
	cfg := in.Action.CreateChildTask
	if cfg.Title == "" {
		return ActionResult{}, fmt.Errorf("create_child_task: title is required")
	}
	spec := ChildTaskSpec{
		Title:          cfg.Title,
		Description:    cfg.Description,
		WorkflowID:     cfg.WorkflowID,
		StepID:         cfg.StepID,
		AgentProfileID: cfg.AgentProfileID,
	}
	if _, err := c.Creator.CreateChildTask(ctx, in.State.TaskID, spec); err != nil {
		return ActionResult{}, fmt.Errorf("create_child_task: %w", err)
	}
	return ActionResult{}, nil
}

// SwitchWorkflowCallback executes the switch_workflow action by asking the
// wired WorkflowSwitcher to mutate the task's workflow_id /
// workflow_step_id and then dispatching on_exit on the old step + on_enter
// on the new step via the supplied DispatchTrigger.
//
// The dispatcher is the same engine.HandleTrigger used elsewhere; we pass
// it explicitly (rather than calling Engine.HandleTrigger directly) to
// avoid a circular dependency between Engine and its callbacks.
type SwitchWorkflowCallback struct {
	Switcher WorkflowSwitcher
	Dispatch DispatchTriggerFn
}

// DispatchTriggerFn is the closure SwitchWorkflowCallback uses to fire
// on_exit / on_enter on the supplied (taskID, sessionID, trigger).
//
// The closure is responsible for building HandleInput; in production it
// wraps Engine.HandleTrigger.
type DispatchTriggerFn func(ctx context.Context, taskID, sessionID string, trigger Trigger, operationID string) error

// Execute satisfies ActionCallback.
func (c SwitchWorkflowCallback) Execute(ctx context.Context, in ActionInput) (ActionResult, error) {
	if c.Switcher == nil {
		return ActionResult{}, fmt.Errorf("%w: switch_workflow requires WorkflowSwitcher", ErrActionNotYetWired)
	}
	if in.Action.SwitchWorkflow == nil {
		return ActionResult{}, fmt.Errorf("switch_workflow action missing SwitchWorkflow config")
	}
	cfg := in.Action.SwitchWorkflow
	if cfg.WorkflowID == "" {
		return ActionResult{}, fmt.Errorf("switch_workflow: workflow_id is required")
	}
	if in.State.TaskID == "" || in.State.SessionID == "" {
		return ActionResult{}, fmt.Errorf("switch_workflow: trigger has no task/session id")
	}

	// Fire on_exit on the old step first so any cleanup (e.g. plan-mode
	// disable) runs before we swap the workflow row. Skip dispatch in
	// EvaluateOnly mode — the engine itself handles on_exit/on_enter
	// orchestration when the caller drives evaluation.
	if c.Dispatch != nil {
		exitOpID := fmt.Sprintf("%s:switch_workflow:on_exit", in.OperationID)
		if err := c.Dispatch(ctx, in.State.TaskID, in.State.SessionID, TriggerOnExit, exitOpID); err != nil {
			return ActionResult{}, fmt.Errorf("switch_workflow on_exit: %w", err)
		}
	}

	if _, err := c.Switcher.SwitchTaskWorkflow(ctx, in.State.TaskID, cfg.WorkflowID, cfg.StepID); err != nil {
		return ActionResult{}, fmt.Errorf("switch_workflow: %w", err)
	}

	// Fire on_enter on the new step.
	if c.Dispatch != nil {
		enterOpID := fmt.Sprintf("%s:switch_workflow:on_enter", in.OperationID)
		if err := c.Dispatch(ctx, in.State.TaskID, in.State.SessionID, TriggerOnEnter, enterOpID); err != nil {
			return ActionResult{}, fmt.Errorf("switch_workflow on_enter: %w", err)
		}
	}
	return ActionResult{}, nil
}

// Compile-time interface assertions.
var (
	_ ActionCallback = CreateChildTaskCallback{}
	_ ActionCallback = SwitchWorkflowCallback{}
)
