package backendapp

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/office/dashboard"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type finalStepTaskService interface {
	GetTask(ctx context.Context, id string) (*models.Task, error)
	MoveTaskWithOptions(
		ctx context.Context, id, workflowID, workflowStepID string, position int, opts taskservice.MoveTaskOptions,
	) (*taskservice.MoveTaskResult, error)
}

type finalStepWorkflowSteps interface {
	ListStepsByWorkflow(ctx context.Context, workflowID string) ([]*wfmodels.WorkflowStep, error)
}

// officeFinalStepMoverAdapter moves an Office task to its workflow's final
// step (the highest position) with the same options as a human
// POST /tasks/:id/move, and classifies the move's refusals for the Office
// status handler.
type officeFinalStepMoverAdapter struct {
	tasks finalStepTaskService
	steps finalStepWorkflowSteps
}

func (a officeFinalStepMoverAdapter) MoveTaskToFinalStep(
	ctx context.Context, taskID string,
) (*dashboard.FinalStepMoveResult, error) {
	task, err := a.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	steps, err := a.steps.ListStepsByWorkflow(ctx, task.WorkflowID)
	if err != nil {
		return nil, fmt.Errorf("list workflow steps: %w", err)
	}
	final := finalWorkflowStep(steps)
	if final == nil {
		return nil, fmt.Errorf("workflow %s has no steps", task.WorkflowID)
	}
	moved, err := a.tasks.MoveTaskWithOptions(ctx, taskID, task.WorkflowID, final.ID, 0, taskservice.MoveTaskOptions{
		AllowActivePrimarySession: true,
		StepHistoryActor:          wfmodels.StepTransitionActorHuman,
		ExpectedWorkflowID:        &task.WorkflowID,
	})
	if err != nil {
		return nil, classifyFinalStepMoveError(err)
	}
	result := &dashboard.FinalStepMoveResult{
		State:    string(moved.Task.State),
		StepID:   final.ID,
		StepName: final.Name,
		Queued:   moved.Task.QueuedForStepID == final.ID,
	}
	if moved.WorkflowStep != nil {
		result.StepID, result.StepName = moved.WorkflowStep.ID, moved.WorkflowStep.Name
	}
	return result, nil
}

// finalWorkflowStep returns the step with the highest position, breaking a
// position tie by the larger ID so the choice does not depend on list order.
func finalWorkflowStep(steps []*wfmodels.WorkflowStep) *wfmodels.WorkflowStep {
	var final *wfmodels.WorkflowStep
	for _, step := range steps {
		if step == nil {
			continue
		}
		if final == nil || step.Position > final.Position ||
			(step.Position == final.Position && step.ID > final.ID) {
			final = step
		}
	}
	return final
}

func classifyFinalStepMoveError(err error) error {
	var approvals *taskservice.ApprovalsPendingMoveError
	if errors.As(err, &approvals) {
		pending := make([]dashboard.PendingApproverDTO, 0, len(approvals.Pending))
		for _, p := range approvals.Pending {
			pending = append(pending, dashboard.PendingApproverDTO{AgentProfileID: p.AgentProfileID, Name: p.Name})
		}
		return &dashboard.FinalStepMoveRefusedError{
			Reason: dashboard.ApprovalGateReasonApprovals, Pending: pending, Err: err,
		}
	}
	if code := taskhandlers.CompletionGateCode(err); code != "" {
		return &dashboard.FinalStepMoveRefusedError{
			Reason: dashboard.FinalStepMoveReasonCompletionGate, Code: code, Err: err,
		}
	}
	if code := taskhandlers.MoveConflictCode(err); code != "" {
		return &dashboard.FinalStepMoveRefusedError{
			Reason: dashboard.FinalStepMoveReasonWorkflowMove, Code: code, Err: err,
		}
	}
	return err
}
