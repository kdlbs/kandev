package backendapp

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type workspaceCollaborationLookup interface {
	GetTask(context.Context, string) (*models.Task, error)
	GetTaskSession(context.Context, string) (*models.TaskSession, error)
	GetWorkspace(context.Context, string) (*models.Workspace, error)
	GetWorkspaceCollaborationGrant(context.Context, string, string, string) (*models.WorkspaceCollaborationGrant, error)
}

// workspaceCollaborationBoundary interprets only server-installed principal and
// execution contexts. Human/native authorization remains in the task service.
type workspaceCollaborationBoundary struct {
	enabled bool
	store   workspaceCollaborationLookup
}

func (b *workspaceCollaborationBoundary) Enabled() bool { return b.enabled }

func (b *workspaceCollaborationBoundary) validatePluginCall(ctx context.Context, agentTool bool) error {
	if !b.enabled || (!agentTool && !b.IsConversation(ctx)) {
		return nil
	}
	_, err := b.caller(ctx)
	return err
}

func (b *workspaceCollaborationBoundary) IsConversation(ctx context.Context) bool {
	if _, ok := mcpscope.PrincipalFromContext(ctx); ok {
		return true
	}
	execution, ok := streams.MCPExecutionContextFromContext(ctx)
	return ok && execution.TaskID != ""
}

func (b *workspaceCollaborationBoundary) CallerTaskID(ctx context.Context) string {
	principal, _ := mcpscope.PrincipalFromContext(ctx)
	return principal.CallerTaskID
}

func (b *workspaceCollaborationBoundary) Authorize(ctx context.Context, targetID string, action models.WorkspaceCollaborationAction) error {
	if !b.enabled || !b.IsConversation(ctx) {
		return nil
	}
	if targetID == "" {
		return taskservice.ErrWorkspaceCollaborationUnavailable
	}
	task, err := b.caller(ctx)
	if err != nil {
		return err
	}
	if task.WorkspaceID == targetID {
		return nil
	}
	grant, err := b.store.GetWorkspaceCollaborationGrant(ctx, task.ID, task.WorkspaceID, targetID)
	if err != nil {
		return fmt.Errorf("%w: grants", taskservice.ErrWorkspaceCollaborationUnavailable)
	}
	if grant == nil || !grant.Allows(action) {
		return taskservice.ErrForbidden
	}
	return nil
}

func (b *workspaceCollaborationBoundary) caller(ctx context.Context) (*models.Task, error) {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || principal.WorkspaceID == "" || b.store == nil {
		return nil, taskservice.ErrWorkspaceCollaborationUnavailable
	}
	task, err := b.store.GetTask(ctx, principal.CallerTaskID)
	if err != nil {
		return nil, fmt.Errorf("%w: caller task", taskservice.ErrWorkspaceCollaborationUnavailable)
	}
	session, err := b.store.GetTaskSession(ctx, principal.CallerSessionID)
	if err != nil {
		return nil, fmt.Errorf("%w: caller session", taskservice.ErrWorkspaceCollaborationUnavailable)
	}
	if task == nil || session == nil || session.TaskID != task.ID || task.WorkspaceID != principal.WorkspaceID {
		return nil, taskservice.ErrWorkspaceCollaborationUnavailable
	}
	workspace, err := b.store.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil || workspace == nil {
		return nil, fmt.Errorf("%w: caller workspace", taskservice.ErrWorkspaceCollaborationUnavailable)
	}
	return task, nil
}
