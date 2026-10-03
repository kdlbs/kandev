package backendapp

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

func mcpSelectionOwnerValidator(p routeParams) mcpconfig.SelectionOwnerValidator {
	return func(ctx context.Context, scope mcpconfig.SelectionScope, workspaceID, ownerID string) error {
		switch scope {
		case mcpconfig.SelectionScopeProfile:
			return validateMCPProfileOwner(ctx, p, workspaceID, ownerID)
		case mcpconfig.SelectionScopeRepository:
			return validateMCPRepositoryOwner(ctx, p, workspaceID, ownerID)
		case mcpconfig.SelectionScopeTask:
			return validateMCPTaskOwner(ctx, p, workspaceID, ownerID)
		case mcpconfig.SelectionScopeTaskSession:
			return validateMCPTaskSessionOwner(ctx, p, workspaceID, ownerID)
		default:
			return fmt.Errorf("unsupported MCP selection scope")
		}
	}
}

func validateMCPProfileOwner(ctx context.Context, p routeParams, workspaceID, ownerID string) error {
	profile, err := p.agentSettingsRepo.GetAgentProfile(ctx, ownerID)
	if err != nil || profile == nil {
		return fmt.Errorf("agent profile %s not found", ownerID)
	}
	if profile.WorkspaceID != "" && profile.WorkspaceID != workspaceID {
		return fmt.Errorf("agent profile belongs to another workspace")
	}
	return nil
}

func validateMCPRepositoryOwner(ctx context.Context, p routeParams, workspaceID, ownerID string) error {
	repository, err := p.taskSvc.GetRepository(ctx, ownerID)
	if err != nil || repository == nil || repository.WorkspaceID != workspaceID {
		return fmt.Errorf("repository does not belong to workspace")
	}
	return nil
}

func validateMCPTaskOwner(ctx context.Context, p routeParams, workspaceID, ownerID string) error {
	task, err := p.taskSvc.GetTask(ctx, ownerID)
	if err != nil || task == nil || task.WorkspaceID != workspaceID {
		return fmt.Errorf("task does not belong to workspace")
	}
	return nil
}

func validateMCPTaskSessionOwner(ctx context.Context, p routeParams, workspaceID, ownerID string) error {
	session, err := p.taskSvc.GetTaskSession(ctx, ownerID)
	if err != nil || session == nil {
		return fmt.Errorf("task session %s not found", ownerID)
	}
	task, err := p.taskSvc.GetTask(ctx, session.TaskID)
	if err != nil || task == nil || task.WorkspaceID != workspaceID {
		return fmt.Errorf("task session does not belong to workspace")
	}
	return nil
}
