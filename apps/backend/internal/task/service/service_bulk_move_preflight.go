package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/completion"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

const (
	BulkMoveRefusedCode           = "bulk_move_refused"
	BulkMovePartialCode           = "bulk_move_partial"
	bulkMoveGenericCode           = "task_move_refused"
	bulkMoveFailedCode            = "task_move_failed"
	bulkMovePolicyUnavailableCode = "task_move_completion_preflight_unavailable"
	bulkMoveApprovalsPendingCode  = "task_approvals_pending"
)

// TaskRefusal describes one task that did not pass bulk-move admission.
type TaskRefusal struct {
	TaskID           string
	TaskTitle        string
	Code             string
	Message          string
	PendingApprovers []PendingApprover
}

// BulkMoveRefusedError means preflight refused the batch before any task write.
type BulkMoveRefusedError struct {
	Refusals []TaskRefusal
}

func (e *BulkMoveRefusedError) Error() string {
	return fmt.Sprintf("bulk move refused for %d task(s)", len(e.Refusals))
}

// BulkMovePartialError reports a write-time refusal after preflight passed.
type BulkMovePartialError struct {
	MovedTaskIDs []string
	Refusal      TaskRefusal
	Cause        error
}

func (e *BulkMovePartialError) Error() string {
	return fmt.Sprintf("bulk move stopped after moving %d task(s): %s", len(e.MovedTaskIDs), e.Refusal.TaskID)
}

func (e *BulkMovePartialError) Unwrap() error { return e.Cause }

type completionAdmissionPreflight interface {
	CheckCompletionAdmission(ctx context.Context, current, candidate *models.Task, admission *completion.Admission) error
}

type bulkMoveOccupancy struct {
	targetStep *wfmodels.WorkflowStep
	occupants  int
	loaded     bool
	known      bool
}

func (s *Service) preflightBulkMove(
	ctx context.Context,
	tasks []*models.Task,
	targetWorkflowID, targetStepID string,
	actor wfmodels.StepTransitionActor,
) error {
	refusals := make([]TaskRefusal, 0)
	occupancy := bulkMoveOccupancy{}
	for _, task := range tasks {
		if refusal := s.preflightBulkMoveTask(
			ctx, task, targetWorkflowID, targetStepID, actor, &occupancy,
		); refusal != nil {
			refusals = append(refusals, *refusal)
		}
	}
	if len(refusals) > 0 {
		return &BulkMoveRefusedError{Refusals: refusals}
	}
	return nil
}

func (s *Service) preflightBulkMoveTask(
	ctx context.Context,
	task *models.Task,
	targetWorkflowID, targetStepID string,
	actor wfmodels.StepTransitionActor,
	occupancy *bulkMoveOccupancy,
) *TaskRefusal {
	if task.WorkflowID == targetWorkflowID && task.WorkflowStepID == targetStepID {
		return nil
	}
	targetStep, err := s.validateTaskMove(ctx, task, targetWorkflowID, targetStepID, MoveTaskOptions{})
	if err != nil {
		return s.bulkMoveTaskRefusalPointer(ctx, task, err)
	}
	if err := s.loadBulkMoveOccupancy(ctx, targetStepID, targetStep, occupancy); err != nil {
		return s.bulkMoveTaskRefusalPointer(ctx, task, err)
	}
	candidate, willQueue, needsCompletionPreflight, err := s.prepareBulkMoveCandidate(
		ctx, task, targetWorkflowID, targetStepID, targetStep, occupancy,
	)
	if err != nil {
		return s.bulkMoveTaskRefusalPointer(ctx, task, err)
	}
	if needsCompletionPreflight {
		if refusal := s.checkBulkMoveCompletionAdmission(ctx, task, candidate, actor, willQueue, occupancy); refusal != nil {
			return refusal
		}
	}
	if !willQueue {
		occupancy.admit(task)
	}
	return nil
}

func (s *Service) prepareBulkMoveCandidate(
	ctx context.Context,
	task *models.Task,
	targetWorkflowID, targetStepID string,
	targetStep *wfmodels.WorkflowStep,
	occupancy *bulkMoveOccupancy,
) (*models.Task, bool, bool, error) {
	willQueue := occupancy.willQueue(task)
	candidate := *task
	candidate.WorkflowID = targetWorkflowID
	candidate.WorkflowStepID = targetStepID
	if !willQueue && task.WorkflowStepID != targetStepID {
		if err := s.syncTaskStateForWorkflowMove(ctx, &candidate, task.WorkflowStepID, targetStepID, MoveTaskOptions{}); err != nil {
			return nil, false, false, err
		}
	}
	needsCompletionPreflight := candidate.State == v1.TaskStateCompleted
	if willQueue {
		completing, err := s.terminalWorkflowStep(ctx, targetStepID)
		if err != nil {
			return nil, false, false, err
		}
		needsCompletionPreflight = completing
	}
	return &candidate, willQueue, needsCompletionPreflight, nil
}

func (s *Service) checkBulkMoveCompletionAdmission(
	ctx context.Context,
	task, candidate *models.Task,
	actor wfmodels.StepTransitionActor,
	willQueue bool,
	occupancy *bulkMoveOccupancy,
) *TaskRefusal {
	if !occupancy.known {
		return s.bulkMoveTaskRefusalPointer(ctx, task, errBulkMovePreflightUnavailable)
	}
	checker, ok := s.tasks.(completionAdmissionPreflight)
	if !ok {
		return &TaskRefusal{
			TaskID: task.ID, TaskTitle: task.Title, Code: bulkMovePolicyUnavailableCode,
			Message: "task move preflight is unavailable",
		}
	}
	var admission *completion.Admission
	if actor == wfmodels.StepTransitionActorHuman && !willQueue {
		admission = humanCompletionAdmission(task, completion.Move)
	}
	if err := checker.CheckCompletionAdmission(ctx, task, candidate, admission); err != nil {
		return s.bulkMoveTaskRefusalPointer(ctx, task, err)
	}
	return nil
}

func (s *Service) loadBulkMoveOccupancy(
	ctx context.Context,
	targetStepID string,
	targetStep *wfmodels.WorkflowStep,
	occupancy *bulkMoveOccupancy,
) error {
	if occupancy.loaded {
		return nil
	}
	occupancy.loaded = true
	occupancy.targetStep = targetStep
	if targetStep == nil || targetStep.WIPLimit <= 0 {
		occupancy.known = true
		return nil
	}
	counts, ok := s.tasks.(workflowAdmittedCountRepository)
	if !ok {
		return errBulkMovePreflightUnavailable
	}
	occupants, err := counts.CountAdmittedTasksByWorkflowStep(ctx, targetStepID)
	if err != nil {
		return errBulkMovePreflightUnavailable
	}
	occupancy.occupants = occupants
	occupancy.known = true
	return nil
}

func (occupancy bulkMoveOccupancy) willQueue(task *models.Task) bool {
	return occupancy.targetStep != nil && occupancy.targetStep.WIPLimit > 0 &&
		!task.IsEphemeral && occupancy.occupants >= occupancy.targetStep.WIPLimit
}

func (occupancy *bulkMoveOccupancy) admit(task *models.Task) {
	if occupancy.targetStep != nil && occupancy.targetStep.WIPLimit > 0 && !task.IsEphemeral {
		occupancy.occupants++
	}
}

func (s *Service) bulkMoveTaskRefusalPointer(ctx context.Context, task *models.Task, err error) *TaskRefusal {
	refusal := s.bulkMoveTaskRefusal(ctx, task, err)
	return &refusal
}

func (s *Service) bulkMoveTaskRefusal(ctx context.Context, task *models.Task, err error) TaskRefusal {
	translated := s.translateCompletionError(ctx, err)
	refusal := TaskRefusal{TaskID: task.ID, TaskTitle: task.Title, Code: bulkMoveErrorCode(translated)}
	var approvals *ApprovalsPendingMoveError
	if errors.As(translated, &approvals) {
		refusal.Code = bulkMoveApprovalsPendingCode
		refusal.Message = "Approvals are pending."
		refusal.PendingApprovers = append([]PendingApprover(nil), approvals.Pending...)
	}
	var policyRefusal *completion.RefusedError
	if errors.As(err, &policyRefusal) && policyRefusal.Code != completion.CodeApprovalsPending {
		refusal.Code = policyRefusal.Code
	}
	if refusal.Message == "" {
		refusal.Message = "Task move was refused."
	}
	return refusal
}

var errBulkMovePreflightUnavailable = errors.New("bulk move preflight is unavailable")

func bulkMoveErrorCode(err error) string {
	var approvals *ApprovalsPendingMoveError
	if errors.As(err, &approvals) {
		return bulkMoveApprovalsPendingCode
	}
	switch {
	case errors.Is(err, taskrepo.ErrTaskCompletionGateBlocked):
		return "task_completion_gate_blocked"
	case errors.Is(err, taskrepo.ErrTaskCompletionCriteriaConflict):
		return "task_completion_criteria_conflict"
	case errors.Is(err, taskrepo.ErrTaskCompletionHumanConfirmationRequired):
		return "task_completion_human_confirmation_required"
	case errors.Is(err, repoerrors.ErrWorkflowChangeConflict), errors.Is(err, ErrWorkflowResolutionConflict), errors.Is(err, workflowmove.ErrMoveConflict):
		return "task_move_workflow_conflict"
	}
	switch {
	case errors.Is(err, ErrTaskMoveArchived):
		return "task_move_archived"
	case errors.Is(err, ErrTaskMoveActiveSession):
		return "task_move_active_session"
	case errors.Is(err, ErrTaskMoveDifferentWorkspace):
		return "task_move_different_workspace"
	case errors.Is(err, ErrTaskMoveWorkflowStep):
		return "task_move_workflow_step"
	case errors.Is(err, errBulkMovePreflightUnavailable):
		return bulkMovePolicyUnavailableCode
	default:
		return bulkMoveGenericCode
	}
}

func isBulkMoveWriteRefusal(err error) bool {
	return bulkMoveErrorCode(err) != bulkMoveGenericCode
}

func (s *Service) bulkMoveWriteError(ctx context.Context, task *models.Task, movedTaskIDs []string, hadWrites bool, err error) error {
	if !isBulkMoveWriteRefusal(err) {
		if hadWrites {
			s.logger.Error("bulk task move failed after partial writes", zap.String("task_id", task.ID), zap.Error(err))
			return &BulkMovePartialError{
				MovedTaskIDs: append([]string(nil), movedTaskIDs...),
				Refusal: TaskRefusal{
					TaskID: task.ID, TaskTitle: task.Title,
					Code: bulkMoveFailedCode, Message: "Task move failed.",
				},
				Cause: err,
			}
		}
		return fmt.Errorf("failed to move task %s: %w", task.ID, err)
	}
	refusal := s.bulkMoveTaskRefusal(ctx, task, err)
	if !hadWrites {
		return &BulkMoveRefusedError{Refusals: []TaskRefusal{refusal}}
	}
	return &BulkMovePartialError{MovedTaskIDs: append([]string(nil), movedTaskIDs...), Refusal: refusal, Cause: err}
}
