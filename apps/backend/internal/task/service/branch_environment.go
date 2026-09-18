package service

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

type branchMaterializationTarget struct {
	environment *models.TaskEnvironment
	session     *models.TaskSession
}

// SelectBranchMaterializationSession returns the most recently updated
// session that can own an immediate branch materialization.
func SelectBranchMaterializationSession(sessions []*models.TaskSession) *models.TaskSession {
	var selected *models.TaskSession
	for _, session := range sessions {
		if !branchMaterializationSessionEligible(session) {
			continue
		}
		if selected == nil || session.UpdatedAt.After(selected.UpdatedAt) {
			selected = session
		}
	}
	return selected
}

func branchMaterializationSessionEligible(session *models.TaskSession) bool {
	if session == nil {
		return false
	}
	switch session.State {
	case models.TaskSessionStateRunning,
		models.TaskSessionStateStarting,
		models.TaskSessionStateWaitingForInput,
		models.TaskSessionStateCreated:
		return true
	default:
		return false
	}
}

// resolveBranchMaterializationTarget follows the active session when a task
// borrows an environment owned by a parent or shared workspace member.
func (s *Service) resolveBranchMaterializationTarget(ctx context.Context, taskID string) (*branchMaterializationTarget, error) {
	if s.taskEnvironments == nil {
		return nil, nil
	}
	var selected *models.TaskSession
	if s.sessions != nil {
		sessions, listErr := s.sessions.ListTaskSessions(ctx, taskID)
		if listErr != nil {
			return nil, fmt.Errorf("list task sessions: %w", listErr)
		}
		selected = SelectBranchMaterializationSession(sessions)
	}
	if selected != nil && selected.TaskEnvironmentID != "" {
		env, err := s.taskEnvironments.GetTaskEnvironment(ctx, selected.TaskEnvironmentID)
		if err != nil {
			return nil, fmt.Errorf("%w: session task environment %s could not be resolved: %w", models.ErrWorkspaceReuseUnsafe, selected.TaskEnvironmentID, err)
		}
		if env == nil {
			return nil, fmt.Errorf("%w: session task environment %s no longer exists", models.ErrWorkspaceReuseUnsafe, selected.TaskEnvironmentID)
		}
		if err := s.validateBranchEnvironmentOwner(ctx, taskID, env); err != nil {
			return nil, err
		}
		return &branchMaterializationTarget{environment: env, session: selected}, nil
	}

	owned, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("look up task environment: %w", err)
	}
	if owned == nil {
		return nil, nil
	}
	if err := s.validateBranchEnvironmentOwner(ctx, taskID, owned); err != nil {
		return nil, err
	}
	return &branchMaterializationTarget{environment: owned, session: selected}, nil
}

func (s *Service) validateBranchEnvironmentOwner(ctx context.Context, taskID string, env *models.TaskEnvironment) error {
	if env == nil || env.TaskID == "" {
		return fmt.Errorf("%w: task environment owner could not be verified", models.ErrWorkspaceReuseUnsafe)
	}
	if env.TaskID == taskID {
		return nil
	}
	owner, err := s.tasks.GetTask(ctx, env.TaskID)
	if err != nil {
		return fmt.Errorf("%w: inherited task environment owner %s could not be verified: %w", models.ErrWorkspaceReuseUnsafe, env.TaskID, err)
	}
	if owner == nil {
		return fmt.Errorf("%w: inherited task environment owner %s could not be verified", models.ErrWorkspaceReuseUnsafe, env.TaskID)
	}
	if owner.ArchivedAt != nil {
		return fmt.Errorf("%w: %s", models.ErrWorkspaceReuseUnsafe,
			models.DescribeInheritedEnvironmentUnavailable(env.TaskID, owner))
	}
	return nil
}
