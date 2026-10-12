package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/office/models"
)

// resolveReviewStage returns the task, workflow step, and stage type for a
// review-capable run. The persisted workflow step is authoritative for the
// task_assigned path. Role remains authoritative for task_review_requested,
// because that wake is explicitly fanned out per reviewer or approver.
func (s *Service) resolveReviewStage(
	ctx context.Context, reason string, parsed map[string]string,
) (string, string, string) {
	taskID := parsed["task_id"]
	stepID := parsed["stage_id"]
	if stepID == "" {
		stepID = parsed["workflow_step_id"]
	}
	stepID = s.resolveMissingReviewStepID(ctx, reason, taskID, stepID)

	roleStageType := reviewStageTypeForRole(parsed["role"])
	stageType := reviewStageTypeFallback(reason, parsed["stage_type"], roleStageType)
	if shouldResolveAuthoritativeStage(reason, roleStageType) {
		stageType = s.resolveAuthoritativeStageType(ctx, reason, stepID, stageType)
	}

	return taskID, stepID, stageType
}

func (s *Service) resolveMissingReviewStepID(
	ctx context.Context, reason, taskID, stepID string,
) string {
	if stepID != "" || taskID == "" {
		return stepID
	}
	currentStepID, err := s.repo.GetTaskWorkflowStepID(ctx, taskID)
	if err == nil {
		return currentStepID
	}
	s.logger.Debug("resolve current workflow step for review run failed",
		zap.String("task_id", taskID), zap.String("reason", reason), zap.Error(err))
	return ""
}

func reviewStageTypeFallback(reason, payloadStageType, roleStageType string) string {
	if reason == RunReasonTaskReviewRequested && roleStageType != "" {
		return roleStageType
	}
	if payloadStageType != "" {
		return payloadStageType
	}
	switch reason {
	case legacyRunReasonReviewStarted:
		return stageTypeReview
	case legacyRunReasonApprovalStarted:
		return stageTypeApproval
	default:
		return ""
	}
}

func shouldResolveAuthoritativeStage(reason, roleStageType string) bool {
	return reason == RunReasonTaskAssigned ||
		(reason == RunReasonTaskReviewRequested && roleStageType == "")
}

func (s *Service) resolveAuthoritativeStageType(
	ctx context.Context, reason, stepID, fallback string,
) string {
	if stepID == "" {
		return fallback
	}
	authoritative, err := s.repo.GetWorkflowStepStageType(ctx, stepID)
	if err != nil {
		s.logger.Debug("resolve workflow step stage type for review run failed",
			zap.String("step_id", stepID), zap.String("reason", reason), zap.Error(err))
		return fallback
	}
	if authoritative == "" {
		return fallback
	}
	return authoritative
}

func reviewStageTypeForRole(role string) string {
	switch role {
	case models.ParticipantRoleReviewer:
		return stageTypeReview
	case models.ParticipantRoleApprover:
		return stageTypeApproval
	default:
		return ""
	}
}

func isReviewOrApprovalStage(stageType string) bool {
	return stageType == stageTypeReview || stageType == stageTypeApproval
}

// resolveGateCommentStage returns the stage type for a task_comment run
// whose payload carries stage_type — i.e. a gate comment fan-out wake.
// workflow_step_id names the step the run was queued at, not the task's
// current step, so a card that moved after the wake was queued still gets
// the stage of the step it was queued at.
func (s *Service) resolveGateCommentStage(ctx context.Context, parsed map[string]string) string {
	stepID := parsed["workflow_step_id"]
	if stepID != "" {
		if stageType, err := s.repo.GetWorkflowStepStageType(ctx, stepID); err == nil && stageType != "" {
			return stageType
		}
	}
	return parsed["stage_type"]
}
