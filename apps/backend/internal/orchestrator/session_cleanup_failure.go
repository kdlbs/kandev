package orchestrator

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

type cleanupFailureSnapshotUpdater interface {
	UpdateTaskSessionIfCurrentSnapshot(context.Context, *models.TaskSession, models.TaskSessionState, time.Time, map[string]interface{}) (bool, error)
}

// recordResumeCleanupFailure preserves the observed lifecycle state. The
// revision CAS includes the error metadata and refuses a newer session owner.
func (s *Service) recordResumeCleanupFailure(ctx context.Context, attempt *resumeAttempt, err error) bool {
	var failure *executor.ResumeCleanupFailure
	if !errors.As(err, &failure) {
		return false
	}
	observed := failure.ObservedSession
	if observed == nil || s.validateResumeAttempt(attempt) != nil {
		return true
	}
	updater, ok := s.repo.(cleanupFailureSnapshotUpdater)
	if !ok {
		return true
	}
	cause, ok := failure.SafeAgentErrorCause(models.AgentErrorCauseOperationResume)
	if !ok {
		return true
	}
	now := time.Now().UTC()
	errorValue := models.LastAgentError{
		Message: failure.SafeDetail(), Details: failure.SafeDetail(),
		Scope: models.ErrorScopeSession, Phase: models.LaunchErrorPhaseBootstrap,
		Code:       models.LaunchErrorCategoryGenericLaunchFailure,
		OccurredAt: now, AttemptID: attempt.identity(), Causes: []models.AgentErrorCause{cause},
		RecoveryActions: []string{models.RecoveryActionRetryLaunch},
		StampValue:      models.StableLaunchErrorStamp(observed.TaskID, observed.ID, attempt.identity(), now.Format(time.RFC3339Nano)),
	}
	updated := *observed
	updated.Metadata = cloneSessionMetadata(observed.Metadata)
	updated.Metadata[models.SessionMetaKeyLastAgentError] = errorValue
	updated.ErrorMessage = errorValue.Message
	changed, saveErr := updater.UpdateTaskSessionIfCurrentSnapshot(ctx, &updated, observed.State, observed.UpdatedAt, updated.Metadata)
	if saveErr != nil {
		s.logger.Warn("failed to persist resume cleanup failure", zap.String("session_id", observed.ID), zap.Error(saveErr))
		return true
	}
	if !changed {
		return true
	}
	if historyErr := s.persistBootstrapFailureMessage(ctx, observed.TaskID, observed.ID, "", errorValue); historyErr != nil {
		s.logger.Warn("failed to persist resume cleanup history", zap.String("session_id", observed.ID), zap.Error(historyErr))
	}
	s.publishTaskSessionStateChanged(ctx, observed.TaskID, observed.ID, observed.State, updated.State, updated.ErrorMessage, &updated.UpdatedAt, &updated)
	return true
}
