package backendapp

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
)

func (a *taskCreatorAdapter) WorkspaceTaskPermissions(ctx context.Context, workspaceID, taskID, sessionID string) (any, error) {
	task, err := a.taskSvc.GetTask(ctx, taskID)
	if err != nil || task.WorkspaceID != workspaceID || task.IsEphemeral || task.IsFromOffice {
		return nil, fmt.Errorf("delivery task unavailable")
	}
	if a.orch == nil {
		return nil, fmt.Errorf("orchestrator unavailable")
	}
	permissions, err := a.orch.ListPendingAgentPermissions(ctx, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"permissions": permissions}, nil
}

func (a *taskCreatorAdapter) controlWorkspacePermission(ctx context.Context, task *models.Task, command shared.WorkspaceTaskCommand) error {
	if a.orch == nil {
		return fmt.Errorf("orchestrator unavailable")
	}
	if command.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if command.Action == "session_mode" {
		return a.orch.SetTaskSessionPermissionMode(ctx, task.ID, command.SessionID, command.Mode)
	}
	permissions, err := a.orch.ListPendingAgentPermissions(ctx, task.ID, command.SessionID)
	if err != nil {
		return err
	}
	if err := validateWorkspacePermissionChoice(permissions, command); err != nil {
		return err
	}
	_, err = a.orch.ResolveAgentPermission(ctx, orchestrator.ResolveAgentPermissionRequest{
		TaskID: task.ID, SessionID: command.SessionID, RequestID: command.RequestID,
		PendingID: command.PendingID, OptionID: command.OptionID, Source: models.PermissionSourceAutomation,
	})
	return err
}

func validateWorkspacePermissionChoice(permissions []streams.PendingAgentPermission, command shared.WorkspaceTaskCommand) error {
	for _, permission := range permissions {
		if permission.SessionID != command.SessionID || permission.RequestID != command.RequestID || permission.PendingID != command.PendingID {
			continue
		}
		for _, option := range permission.Options {
			if option.OptionID == command.OptionID && (option.Kind == streams.PermissionOptionKindAllowOnce || option.Kind == streams.PermissionOptionKindRejectOnce) {
				return nil
			}
		}
	}
	return fmt.Errorf("select an exact live allow_once or reject_once option from task_permissions; persistent grants are unavailable")
}
