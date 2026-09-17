package service

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/office/shared"
)

type workspaceTaskCreator interface {
	CreateWorkspaceTask(context.Context, shared.WorkspaceTaskSpec) (string, error)
}

func (s *Service) CreateWorkspaceTaskAsAgent(ctx context.Context, callerID string, spec shared.WorkspaceTaskSpec) (string, error) {
	if err := s.requireTaskCreatePermission(ctx, callerID); err != nil {
		return "", err
	}
	caller, err := s.repo.GetAgentInstance(ctx, callerID)
	if err != nil || caller.WorkspaceID != spec.WorkspaceID {
		return "", shared.ErrForbidden
	}
	creator, ok := s.taskCreator.(workspaceTaskCreator)
	if !ok {
		return "", fmt.Errorf("workspace task creator unavailable")
	}
	role, err := s.repo.OrchestratorRoleID(ctx, callerID)
	if err != nil {
		return "", err
	}
	spec.DirectProfile = role != ""
	spec.ChiefID = callerID
	return creator.CreateWorkspaceTask(ctx, spec)
}

func (s *Service) WorkspaceCatalog(ctx context.Context, workspaceID string) (any, error) {
	reader, ok := s.taskCreator.(interface {
		WorkspaceCatalog(context.Context, string) (any, error)
	})
	if !ok {
		return nil, fmt.Errorf("workspace catalog unavailable")
	}
	catalog, err := reader.WorkspaceCatalog(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	profiles, err := s.repo.ExecutionProfileDirectory(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if result, ok := catalog.(map[string]any); ok {
		result["execution_profiles"] = profiles
	}
	return catalog, nil
}

func (s *Service) ManageWorkspaceTask(ctx context.Context, command shared.WorkspaceTaskCommand) error {
	agent, err := s.repo.GetAgentInstance(ctx, command.ChiefID)
	if err != nil || agent.WorkspaceID != command.WorkspaceID {
		return shared.ErrForbidden
	}
	permissions := shared.ResolvePermissions(shared.AgentRole(agent.Role), agent.Permissions)
	if !shared.HasPermission(permissions, shared.PermCanAssignTasks) {
		return shared.ErrForbidden
	}
	manager, ok := s.taskCreator.(interface {
		ManageWorkspaceTask(context.Context, shared.WorkspaceTaskCommand) error
	})
	if !ok {
		return fmt.Errorf("workspace task manager unavailable")
	}
	role, err := s.repo.OrchestratorRoleID(ctx, command.ChiefID)
	if err != nil {
		return err
	}
	command.DirectProfile = role != ""
	return manager.ManageWorkspaceTask(ctx, command)
}

// ValidatePersonaRunTask prevents a second scheduler from owning a Kanban task.
func (s *Service) ValidatePersonaRunTask(ctx context.Context, workspaceID, taskID string) error {
	fields, err := s.repo.GetTaskExecutionFields(ctx, taskID)
	if err != nil {
		return err
	}
	if fields.WorkspaceID != workspaceID || !fields.IsFromOffice {
		return fmt.Errorf("kanban tasks must be started through their workflow, not the persona run queue")
	}
	return nil
}

func (s *Service) WorkspaceTaskDetails(ctx context.Context, workspaceID, taskID string) (any, error) {
	reader, ok := s.taskCreator.(interface {
		WorkspaceTaskDetails(context.Context, string, string) (any, error)
	})
	if !ok {
		return nil, fmt.Errorf("workspace task reader unavailable")
	}
	return reader.WorkspaceTaskDetails(ctx, workspaceID, taskID)
}
