package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

var ErrWorkspaceRecoveryProjectionFailed = errors.New("workspace recovery state could not be saved")

func (s *Service) workspaceRecoverySelectionSnapshot(
	ctx context.Context,
	session *models.TaskSession,
	environment *models.TaskEnvironment,
) (models.WorkspaceRecoverySelectionSnapshot, error) {
	repositories := make(map[string]*models.Repository, len(environment.Repos))
	for _, row := range environment.Repos {
		if row == nil || row.DeletedAt != nil || (row.Status != "" && row.Status != "active") {
			continue
		}
		if row.RepositoryID == "" {
			return models.WorkspaceRecoverySelectionSnapshot{}, errors.New("workspace recovery repository identity is incomplete")
		}
		if s.repoEntities == nil {
			return models.WorkspaceRecoverySelectionSnapshot{}, errors.New("workspace recovery repository inventory is unavailable")
		}
		repository, err := s.repoEntities.GetRepository(ctx, row.RepositoryID)
		if err != nil {
			return models.WorkspaceRecoverySelectionSnapshot{}, fmt.Errorf("load workspace recovery repository %q: %w", row.RepositoryID, err)
		}
		if repository == nil {
			return models.WorkspaceRecoverySelectionSnapshot{}, fmt.Errorf("workspace recovery repository %q is missing", row.RepositoryID)
		}
		repositories[row.RepositoryID] = repository
	}
	return models.NewWorkspaceRecoverySelectionSnapshot(session, environment, repositories), nil
}

// ReportManagedCloneRelocationRequired records a verified dirty-worktree
// refusal from a trusted lifecycle or orchestrator caller. It returns an empty
// stamp when the captured session identity was superseded before the write.
func (s *Service) ReportManagedCloneRelocationRequired(
	ctx context.Context,
	observation models.WorkspaceRecoveryErrorObservation,
) (string, error) {
	if s == nil || s.sessions == nil || observation.TaskID == "" || observation.SessionID == "" {
		return "", nil
	}
	now := time.Now().UTC()
	errorValue := models.LastAgentError{
		Message:         "The task workspace contains local changes and needs explicit relocation.",
		OccurredAt:      now,
		Scope:           models.ErrorScopeSession,
		Phase:           models.LaunchErrorPhaseBootstrap,
		Code:            models.LaunchErrorCategoryManagedCloneRelocationRequired,
		Details:         "Move the workspace files and resume to continue this task session.",
		RecoveryActions: []string{models.RecoveryActionRelocateAndResume},
		StampValue: models.StableLaunchErrorStamp(
			observation.TaskID, observation.SessionID,
			models.LaunchErrorCategoryManagedCloneRelocationRequired, now.Format(time.RFC3339Nano),
		),
	}
	stored, stamp, err := s.sessions.CommitWorkspaceRecoveryErrorIfCurrent(ctx, observation, errorValue)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("failed to persist workspace recovery error",
				zap.String("task_id", observation.TaskID),
				zap.String("session_id", observation.SessionID),
				zap.Error(err))
		}
		return "", ErrWorkspaceRecoveryProjectionFailed
	}
	if !stored || stamp == "" {
		return stamp, nil
	}
	if s.eventBus != nil {
		eventCtx := context.WithoutCancel(ctx)
		if err := s.eventBus.Publish(eventCtx, events.TaskSessionErrorChanged, bus.NewEvent(
			events.TaskSessionErrorChanged,
			"task-service",
			map[string]interface{}{
				"task_id":    observation.TaskID,
				"session_id": observation.SessionID,
				"active":     true,
				"stamp":      stamp,
			},
		)); err != nil && s.logger != nil {
			s.logger.Warn("failed to publish workspace recovery error event",
				zap.String("task_id", observation.TaskID),
				zap.String("session_id", observation.SessionID),
				zap.Error(err))
		}
	}
	return stamp, nil
}
