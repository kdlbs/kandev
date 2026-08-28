package executor

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

type launchFailureClassification struct {
	code    string
	message string
}

func classifyLaunchFailure(err error) launchFailureClassification {
	switch {
	case errors.Is(err, worktree.ErrInvalidBaseBranch):
		return launchFailureClassification{
			code:    models.LaunchErrorCategoryBaseBranchMissing,
			message: "The selected base branch is not available.",
		}
	case errors.Is(err, worktree.ErrRemoteDefaultUnresolved):
		return launchFailureClassification{
			code:    models.LaunchErrorCategoryDefaultBranchUnresolved,
			message: "The repository default branch could not be resolved.",
		}
	default:
		return launchFailureClassification{
			code:    models.LaunchErrorCategoryGenericLaunchFailure,
			message: "The agent could not start.",
		}
	}
}

func launchFailureRecoveryActions(taskRepositoryID string, markReviewDone bool) []string {
	actions := make([]string, 0, 3)
	if strings.TrimSpace(taskRepositoryID) != "" {
		actions = append(actions,
			models.RecoveryActionRetryDefault,
			models.RecoveryActionPickBaseBranch,
		)
	}
	if markReviewDone {
		actions = append(actions, models.RecoveryActionMarkReviewDone)
	}
	return models.NormalizeRecoveryActions(actions)
}

func (e *Executor) buildLastAgentError(
	ctx context.Context,
	taskID, taskRepositoryID string,
	launchErr error,
) models.LastAgentError {
	classification := classifyLaunchFailure(launchErr)
	markReviewDone := false
	if e.launchFailureReviewEligibility != nil {
		eligible, err := e.launchFailureReviewEligibility(ctx, taskID)
		if err != nil {
			if e.logger != nil {
				e.logger.Debug("launch failure review eligibility lookup failed",
					zap.String("task_id", taskID), zap.Error(err))
			}
		} else {
			markReviewDone = eligible
		}
	}
	occurredAt := time.Now().UTC()
	details := ""
	if launchErr != nil {
		details = routingerr.Sanitize(launchErr.Error())
	}
	return models.LastAgentError{
		Message:          classification.message,
		OccurredAt:       occurredAt,
		Code:             classification.code,
		Details:          details,
		RecoveryActions:  launchFailureRecoveryActions(taskRepositoryID, markReviewDone),
		TaskRepositoryID: taskRepositoryID,
		StampValue: models.StableLaunchErrorStamp(
			taskID, classification.code, taskRepositoryID, occurredAt.Format(time.RFC3339Nano),
		),
	}
}

func (e *Executor) persistLastAgentError(
	ctx context.Context,
	sessionID string,
	errorValue models.LastAgentError,
) {
	if err := e.repo.SetSessionMetadataKey(
		ctx, sessionID, models.SessionMetaKeyLastAgentError, errorValue,
	); err != nil {
		e.logger.Warn("failed to persist typed launch error",
			zap.String("session_id", sessionID), zap.Error(err))
	}
}
