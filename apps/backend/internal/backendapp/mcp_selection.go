package backendapp

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/task/models"
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

func mcpSessionResolutionContext(p routeParams) func(context.Context, string, string) (mcpconfig.ResolutionContext, error) {
	return func(ctx context.Context, workspaceID, sessionID string) (mcpconfig.ResolutionContext, error) {
		if p.lifecycleMgr != nil {
			if execution, ok := p.lifecycleMgr.GetExecutionBySessionID(sessionID); ok && execution.WorkspaceID == workspaceID {
				return mcpconfig.ResolutionContext{WorkspaceID: workspaceID, SessionID: sessionID, TaskID: execution.TaskID, ProfileID: execution.AgentProfileID, RepositoryIDs: execution.RepositoryIDs}, nil
			}
		}
		session, err := p.taskSvc.GetTaskSession(ctx, sessionID)
		if err != nil || session == nil {
			return mcpconfig.ResolutionContext{}, fmt.Errorf("session context unavailable")
		}
		task, err := p.taskSvc.GetTask(ctx, session.TaskID)
		if err != nil || task == nil || task.WorkspaceID != workspaceID {
			return mcpconfig.ResolutionContext{}, fmt.Errorf("task context unavailable")
		}
		rc := mcpconfig.ResolutionContext{WorkspaceID: workspaceID, SessionID: sessionID, TaskID: task.ID, ProfileID: session.AgentProfileID}
		for _, repository := range task.Repositories {
			rc.RepositoryIDs = append(rc.RepositoryIDs, repository.RepositoryID)
		}
		return rc, nil
	}
}

// Profile eligibility is checked in the settings transaction. Task ownership
// remains in the task service; recheck it at admission without starting an agent.
func mcpSelectionAdditionValidator(p routeParams) mcpconfig.SelectionOwnerValidator {
	return func(ctx context.Context, scope mcpconfig.SelectionScope, workspaceID, ownerID string) error {
		if scope == mcpconfig.SelectionScopeProfile {
			return nil
		}
		if err := mcpSelectionOwnerValidator(p)(ctx, scope, workspaceID, ownerID); err != nil {
			return err
		}
		taskID := ownerID
		if scope == mcpconfig.SelectionScopeTaskSession {
			session, err := p.taskSvc.GetTaskSession(ctx, ownerID)
			if err != nil || session == nil {
				return fmt.Errorf("task session unavailable")
			}
			if err := validateMCPSessionAddition(ctx, p, workspaceID, session); err != nil {
				return err
			}
			taskID = session.TaskID
		}
		if scope == mcpconfig.SelectionScopeTask || scope == mcpconfig.SelectionScopeTaskSession {
			task, err := p.taskSvc.GetTask(ctx, taskID)
			if err != nil || task == nil || task.ArchivedAt != nil {
				return fmt.Errorf("task unavailable")
			}
		}
		return nil
	}
}

func validateMCPSessionAddition(ctx context.Context, p routeParams, workspaceID string, session *models.TaskSession) error {
	if session.IsPassthrough || session.State == models.TaskSessionStateCancelled || session.State == models.TaskSessionStateCompleted || session.State == models.TaskSessionStateFailed {
		return fmt.Errorf("task session unavailable")
	}
	if p.agentSettingsRepo == nil {
		return fmt.Errorf("session profile unavailable")
	}
	profile, err := p.agentSettingsRepo.GetAgentProfile(ctx, session.AgentProfileID)
	if err != nil || profile == nil || !profile.Enabled || (profile.WorkspaceID != "" && profile.WorkspaceID != workspaceID) {
		return fmt.Errorf("session profile unavailable")
	}
	agent, err := p.agentSettingsRepo.GetAgent(ctx, profile.AgentID)
	if err != nil || agent == nil || !agent.SupportsMCP {
		return fmt.Errorf("session agent does not support MCP")
	}
	return nil
}
