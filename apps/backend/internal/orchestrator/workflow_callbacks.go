package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// buildWorkflowCallbacks creates the callback registry for the workflow engine.
// Each callback wraps an existing orchestrator Service method, keeping side-effect
// logic in the orchestrator while letting the engine drive evaluation.
func buildWorkflowCallbacks(svc *Service) engine.MapRegistry {
	return engine.MapRegistry{
		engine.ActionEnablePlanMode:    &enablePlanModeCallback{svc: svc},
		engine.ActionDisablePlanMode:   &disablePlanModeCallback{svc: svc},
		engine.ActionResetAgentContext: &resetAgentContextCallback{svc: svc},
		engine.ActionAutoStartAgent:    &autoStartAgentCallback{svc: svc},
		engine.ActionSetWorkflowData:   &setWorkflowDataCallback{},
	}
}

// enablePlanModeCallback enables plan mode on the session.
type enablePlanModeCallback struct {
	svc *Service
}

func (c *enablePlanModeCallback) Execute(ctx context.Context, in engine.ActionInput) (engine.ActionResult, error) {
	if in.State.IsPassthrough {
		return engine.ActionResult{}, nil
	}
	session, err := c.svc.repo.GetTaskSession(ctx, in.State.SessionID)
	if err != nil {
		return engine.ActionResult{}, fmt.Errorf("load session for enable plan mode: %w", err)
	}
	c.svc.setSessionPlanMode(ctx, session, true)
	return engine.ActionResult{}, nil
}

// disablePlanModeCallback disables plan mode on the session.
type disablePlanModeCallback struct {
	svc *Service
}

func (c *disablePlanModeCallback) Execute(ctx context.Context, in engine.ActionInput) (engine.ActionResult, error) {
	if in.State.IsPassthrough {
		return engine.ActionResult{}, nil
	}
	session, err := c.svc.repo.GetTaskSession(ctx, in.State.SessionID)
	if err != nil {
		return engine.ActionResult{}, fmt.Errorf("load session for disable plan mode: %w", err)
	}
	c.svc.clearSessionPlanMode(ctx, session)
	return engine.ActionResult{}, nil
}

// resetAgentContextCallback restarts the agent subprocess with a fresh ACP session.
type resetAgentContextCallback struct {
	svc *Service
}

func (c *resetAgentContextCallback) Execute(ctx context.Context, in engine.ActionInput) (engine.ActionResult, error) {
	session, err := c.svc.repo.GetTaskSession(ctx, in.State.SessionID)
	if err != nil {
		return engine.ActionResult{}, fmt.Errorf("load session for reset agent context: %w", err)
	}
	ok := c.svc.resetAgentContext(ctx, in.State.TaskID, session, in.Step.Name)
	if !ok {
		return engine.ActionResult{}, fmt.Errorf("failed to reset agent context for session %s", in.State.SessionID)
	}
	return engine.ActionResult{}, nil
}

// autoStartAgentCallback sends the auto-start prompt for a workflow step.
type autoStartAgentCallback struct {
	svc *Service
}

func (c *autoStartAgentCallback) Execute(ctx context.Context, in engine.ActionInput) (engine.ActionResult, error) {
	if in.State.IsPassthrough {
		return engine.ActionResult{}, nil
	}

	session, err := c.svc.repo.GetTaskSession(ctx, in.State.SessionID)
	if err != nil {
		return engine.ActionResult{}, fmt.Errorf("load session for auto-start: %w", err)
	}

	// Reconstruct the workflow step to build the prompt.
	// The step's prompt template and plan mode flag drive prompt construction.
	step, err := c.svc.workflowStepGetter.GetStep(ctx, in.Step.ID)
	if err != nil {
		return engine.ActionResult{}, fmt.Errorf("load step for auto-start: %w", err)
	}

	effectivePrompt := c.svc.buildWorkflowPrompt(in.State.TaskDescription, step, in.State.TaskID)
	planMode := step.HasOnEnterAction(wfmodels.OnEnterEnablePlanMode)

	err = c.svc.autoStartStepPrompt(ctx, in.State.TaskID, session, in.Step.Name, effectivePrompt, planMode, true)
	if err != nil {
		return engine.ActionResult{}, fmt.Errorf("auto-start prompt failed: %w", err)
	}
	return engine.ActionResult{}, nil
}

// setWorkflowDataCallback writes key/value data into the workflow data bag.
type setWorkflowDataCallback struct{}

func (c *setWorkflowDataCallback) Execute(_ context.Context, in engine.ActionInput) (engine.ActionResult, error) {
	if in.Action.SetWorkflowData == nil {
		return engine.ActionResult{}, nil
	}
	return engine.ActionResult{
		DataPatch: map[string]any{
			in.Action.SetWorkflowData.Key: in.Action.SetWorkflowData.Value,
		},
	}, nil
}
