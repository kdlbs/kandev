package backendapp

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/coordinator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
)

// undoTaskAPI is the part of the task service the undo seam adapts.
type undoTaskAPI interface {
	ArchiveTask(ctx context.Context, id string) error
	GetTask(ctx context.Context, id string) (*taskmodels.Task, error)
	MoveTaskWithOptions(ctx context.Context, id, workflowID, workflowStepID string, position int, opts taskservice.MoveTaskOptions) (*taskservice.MoveTaskResult, error)
	ListTaskSessions(ctx context.Context, taskID string) ([]*taskmodels.TaskSession, error)
}

// undoStepAPI is the part of the workflow service the undo seam adapts.
type undoStepAPI interface {
	GetStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error)
	ListStepsByWorkflow(ctx context.Context, workflowID string) ([]*wfmodels.WorkflowStep, error)
}

// coordinatorUndoSeam adapts the task and workflow services to
// coordinator.UndoTaskService, mapping their errors onto the seam's.
type coordinatorUndoSeam struct {
	tasks undoTaskAPI
	steps undoStepAPI
}

var _ coordinator.UndoTaskService = (*coordinatorUndoSeam)(nil)

func (a *coordinatorUndoSeam) ArchiveTask(ctx context.Context, id string) error {
	err := a.tasks.ArchiveTask(ctx, id)
	switch {
	case errors.Is(err, taskservice.ErrTaskAlreadyArchived):
		return coordinator.ErrTaskAlreadyArchived
	case errors.Is(err, repoerrors.ErrTaskNotFound):
		return coordinator.ErrTaskNotFound
	}
	return err
}

func (a *coordinatorUndoSeam) GetTask(ctx context.Context, id string) (*coordinator.UndoTask, error) {
	task, err := a.tasks.GetTask(ctx, id)
	if errors.Is(err, repoerrors.ErrTaskNotFound) {
		return nil, coordinator.ErrTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	return &coordinator.UndoTask{
		Identifier: task.Identifier, ArchivedAt: task.ArchivedAt,
		WorkflowID: task.WorkflowID, WorkflowStepID: task.WorkflowStepID,
	}, nil
}

func (a *coordinatorUndoSeam) MoveTaskWithOptions(ctx context.Context, id, workflowID, stepID string, position int, opts coordinator.UndoMoveOptions) (coordinator.UndoMoveResult, error) {
	moveOpts := taskservice.MoveTaskOptions{}
	if opts.ExpectedWorkflowID != "" {
		expected := opts.ExpectedWorkflowID
		moveOpts.ExpectedWorkflowID = &expected
	}
	if opts.SkipStepPrompt {
		moveOpts.EntryOptions = &workflowmove.EntryOptions{SkipStepPrompt: true}
	}
	result, err := a.tasks.MoveTaskWithOptions(ctx, id, workflowID, stepID, position, moveOpts)
	switch {
	case errors.Is(err, taskservice.ErrWIPLimitExceeded):
		return coordinator.UndoMoveResult{}, coordinator.ErrWIPLimitExceeded
	case errors.Is(err, taskservice.ErrWorkflowResolutionConflict), errors.Is(err, workflowmove.ErrMoveConflict):
		return coordinator.UndoMoveResult{}, coordinator.ErrMoveConflict
	case errors.Is(err, repoerrors.ErrTaskNotFound):
		return coordinator.UndoMoveResult{}, coordinator.ErrTaskNotFound
	case err != nil:
		return coordinator.UndoMoveResult{}, err
	}
	if result == nil {
		return coordinator.UndoMoveResult{}, nil
	}
	return coordinator.UndoMoveResult{
		Admitted:   result.Task != nil && result.Task.WIPAdmitted,
		FromStepID: result.FromStepID,
	}, nil
}

func (a *coordinatorUndoSeam) GetStep(ctx context.Context, stepID string) (*coordinator.UndoStep, error) {
	step, err := a.steps.GetStep(ctx, stepID)
	if errors.Is(err, wfmodels.ErrWorkflowStepNotFound) {
		return nil, coordinator.ErrStepNotFound
	}
	if err != nil {
		return nil, err
	}
	return &coordinator.UndoStep{
		Name: step.Name, WorkflowID: step.WorkflowID,
		AutoStart:        step.HasOnEnterAction(wfmodels.OnEnterAutoStartAgent),
		CompletesOnEnter: step.CompleteTaskOnEnter,
	}, nil
}

func (a *coordinatorUndoSeam) ListSteps(ctx context.Context, workflowID string) ([]coordinator.StepNode, error) {
	return coordinator.LoadStepGraph(ctx, a.steps, workflowID)
}

// HasActiveSession reports whether any session of the task is starting or
// running, the two states the task service blocks a move on.
func (a *coordinatorUndoSeam) HasActiveSession(ctx context.Context, taskID string) (bool, error) {
	sessions, err := a.tasks.ListTaskSessions(ctx, taskID)
	if err != nil {
		return false, err
	}
	for _, s := range sessions {
		if s.State == taskmodels.TaskSessionStateStarting || s.State == taskmodels.TaskSessionStateRunning {
			return true, nil
		}
	}
	return false, nil
}
