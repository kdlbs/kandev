package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/workflow/engine"
)

// buildWorkflowCallbacks creates the callback registry for the workflow engine.
// Each callback wraps an existing orchestrator Service method, keeping side-effect
// logic in the orchestrator while letting the engine drive evaluation.
//
// Phase 2 (ADR-0004) callbacks — queue_run, clear_decisions,
// queue_run_for_each_participant — are registered conditionally based on
// the orchestrator's wired adapters. If adapters are missing the action
// kinds simply have no callback; the engine treats unknown kinds as no-ops
// (see engine.executeCallback). This keeps kanban-only deployments
// untouched.
func buildWorkflowCallbacks(svc *Service) engine.MapRegistry {
	r := engine.MapRegistry{
		engine.ActionEnablePlanMode:    &enablePlanModeCallback{svc: svc},
		engine.ActionDisablePlanMode:   &disablePlanModeCallback{svc: svc},
		engine.ActionResetAgentContext: &resetAgentContextCallback{svc: svc},
		engine.ActionAutoStartAgent:    &autoStartAgentCallback{svc: svc},
		engine.ActionSetWorkflowData:   &setWorkflowDataCallback{},
	}
	if svc.engineRunQueue != nil {
		r[engine.ActionQueueRun] = engine.QueueRunCallback{
			Adapter:      svc.engineRunQueue,
			Participants: svc.engineParticipants,
			CEOResolver:  svc.engineCEOResolver,
			Primary:      svc.enginePrimary,
		}
		if svc.engineParticipants != nil {
			r[engine.ActionQueueRunForEachParticipant] = engine.QueueRunForEachParticipantCallback{
				Adapter:      svc.engineRunQueue,
				Participants: svc.engineParticipants,
			}
		}
	}
	if svc.engineDecisions != nil {
		r[engine.ActionClearDecisions] = engine.ClearDecisionsCallback{Decisions: svc.engineDecisions}
	}
	if svc.engineTaskCreator != nil {
		r[engine.ActionCreateChildTask] = engine.CreateChildTaskCallback{Creator: svc.engineTaskCreator}
	}
	if svc.engineWorkflowSwitcher != nil {
		r[engine.ActionSwitchWorkflow] = engine.SwitchWorkflowCallback{
			Switcher: svc.engineWorkflowSwitcher,
			Dispatch: switchWorkflowDispatcher(svc),
		}
	}
	return r
}

// switchWorkflowDispatcher returns the closure SwitchWorkflowCallback uses
// to fire on_exit / on_enter. It reads svc.workflowEngine lazily — at
// registration time the engine may not yet be initialised, but it is
// guaranteed by the time the closure runs (callbacks only execute after
// HandleTrigger).
func switchWorkflowDispatcher(svc *Service) engine.DispatchTriggerFn {
	return func(ctx context.Context, taskID, sessionID string, trigger engine.Trigger, operationID string) error {
		eng := svc.workflowEngine
		if eng == nil {
			return nil // engine not initialised; treat as no-op
		}
		_, err := eng.HandleTrigger(ctx, engine.HandleInput{
			TaskID:      taskID,
			SessionID:   sessionID,
			Trigger:     trigger,
			OperationID: operationID,
		})
		return err
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

	_, err := c.svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID:         in.State.TaskID,
		Intent:         IntentWorkflowStep,
		SessionID:      in.State.SessionID,
		WorkflowStepID: in.Step.ID,
	})
	if err != nil {
		return engine.ActionResult{}, fmt.Errorf("auto-start via LaunchSession failed: %w", err)
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
