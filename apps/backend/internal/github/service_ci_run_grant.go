package github

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

type CreateCIRunGrantInput struct {
	WorkspaceID    string `json:"workspace_id"`
	ActorTaskID    string `json:"actor_task_id"`
	TargetTaskID   string `json:"target_task_id"`
	WorkflowID     string `json:"workflow_id"`
	WorkflowStepID string `json:"workflow_step_id"`
	RepositoryID   string `json:"repository_id"`
}

func (s *Service) CreateCIRunGrant(
	ctx context.Context,
	userID string,
	input CreateCIRunGrantInput,
) (*CIRunGrant, error) {
	if userID == "" || input.WorkspaceID == "" || input.ActorTaskID == "" ||
		input.TargetTaskID == "" || input.WorkflowID == "" || input.WorkflowStepID == "" ||
		input.RepositoryID == "" {
		return nil, errors.New("complete CI run grant identity is required")
	}
	if err := s.authorizeWorkspaceAccess(ctx, input.WorkspaceID); err != nil {
		return nil, err
	}
	if err := s.validateCIRunGrantScope(ctx, input); err != nil {
		return nil, err
	}
	now := s.ciRunClock()().UTC()
	generation := int64(1)
	if previous, lookupErr := s.store.GetActiveCIRunGrant(ctx, input.WorkspaceID, input.ActorTaskID, input.TargetTaskID, input.WorkflowID, input.WorkflowStepID, input.RepositoryID); lookupErr != nil {
		return nil, lookupErr
	} else if previous != nil {
		generation = previous.Generation + 1
		if err := s.store.RevokeCIRunGrant(ctx, input.WorkspaceID, previous.ID, now); err != nil {
			return nil, err
		}
	}
	grant := &CIRunGrant{
		ID: uuid.NewString(), Generation: generation, WorkspaceID: input.WorkspaceID, ActorTaskID: input.ActorTaskID,
		TargetTaskID: input.TargetTaskID, WorkflowID: input.WorkflowID,
		WorkflowStepID: input.WorkflowStepID, RepositoryID: input.RepositoryID,
		CreatedByUserID: userID, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.store.UpsertCIRunGrant(ctx, grant); err != nil {
		return nil, err
	}
	return grant, nil
}

func (s *Service) validateCIRunGrantScope(ctx context.Context, input CreateCIRunGrantInput) error {
	var count int
	err := s.store.ro.GetContext(ctx, &count, s.store.ro.Rebind(`
		SELECT COUNT(*) FROM tasks actor
		JOIN tasks target ON target.id = ?
		JOIN task_repositories tr ON tr.task_id = target.id AND tr.repository_id = ?
		JOIN repositories r ON r.id = tr.repository_id
		WHERE actor.id = ? AND actor.workspace_id = ? AND target.workspace_id = ?
			AND target.workflow_id = ? AND target.workflow_step_id = ?
			AND r.workspace_id = ? AND r.provider = 'github'`),
		input.TargetTaskID, input.RepositoryID, input.ActorTaskID, input.WorkspaceID,
		input.WorkspaceID, input.WorkflowID, input.WorkflowStepID, input.WorkspaceID)
	if err != nil {
		return err
	}
	if count != 1 {
		return &CIRunRequestError{Class: CIRunFailureNotAuthorized}
	}
	return nil
}

func (s *Service) RevokeCIRunGrant(
	ctx context.Context, userID, workspaceID, grantID string,
) error {
	if userID == "" || workspaceID == "" || grantID == "" {
		return errors.New("grant, workspace, and user identity are required")
	}
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return err
	}
	err := s.store.RevokeCIRunGrant(ctx, workspaceID, grantID, time.Now().UTC())
	if errors.Is(err, sql.ErrNoRows) {
		return &CIRunRequestError{Class: CIRunFailureNotAuthorized}
	}
	return err
}
