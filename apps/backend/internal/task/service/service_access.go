package service

import (
	"context"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// Per-user workspace scoping (opt-in authentication).
//
// Every user-facing service entry point calls one of the authorize* helpers.
// Scoping keys off the request-context identity placed there by the auth
// middleware:
//   - no identity in ctx  → internal caller (event bus, pollers, MCP stream,
//     office schedulers) → unscoped, exactly as before the auth feature
//   - synthetic identity  → auth disabled → unscoped (today's behavior)
//   - real identity       → workspaces are visible only to their owner;
//     rows with an empty owner_id (created pre-auth) stay visible to everyone
//     until the setup wizard claims them for the admin
//
// Denials use the *NotFound sentinels — a foreign workspace is
// indistinguishable from a nonexistent one (no existence leak).

// callerScope returns the scoping user ID; ok=false means unscoped.
func callerScope(ctx context.Context) (string, bool) {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic {
		return "", false
	}
	return identity.UserID, true
}

func workspaceVisibleTo(workspace *models.Workspace, userID string) bool {
	return workspace == nil || workspace.OwnerID == "" || workspace.OwnerID == userID
}

// authorizeWorkspaceID checks visibility of a workspace by ID.
func (s *Service) authorizeWorkspaceID(ctx context.Context, workspaceID string) error {
	userID, scoped := callerScope(ctx)
	if !scoped || workspaceID == "" {
		return nil
	}
	workspace, err := s.workspaces.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return err
	}
	if !workspaceVisibleTo(workspace, userID) {
		return repoerrors.ErrWorkspaceNotFound
	}
	return nil
}

// authorizeTaskID checks visibility of a task via its workspace.
func (s *Service) authorizeTaskID(ctx context.Context, taskID string) error {
	userID, scoped := callerScope(ctx)
	if !scoped {
		return nil
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task.WorkspaceID == "" {
		return nil
	}
	workspace, err := s.workspaces.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		// A dangling workspace reference should not hide the task from the
		// single user who can already see everything else about it.
		return nil //nolint:nilerr // visibility fallback, not an operation failure
	}
	if !workspaceVisibleTo(workspace, userID) {
		return repoerrors.ErrTaskNotFound
	}
	return nil
}

// authorizeWorkflowID checks visibility of a workflow via its workspace.
func (s *Service) authorizeWorkflowID(ctx context.Context, workflowID string) error {
	userID, scoped := callerScope(ctx)
	if !scoped {
		return nil
	}
	workflow, err := s.workflows.GetWorkflow(ctx, workflowID)
	if err != nil {
		return err
	}
	if workflow.WorkspaceID == "" {
		return nil
	}
	workspace, err := s.workspaces.GetWorkspace(ctx, workflow.WorkspaceID)
	if err != nil {
		return nil //nolint:nilerr // visibility fallback, not an operation failure
	}
	if !workspaceVisibleTo(workspace, userID) {
		return repoerrors.ErrWorkspaceNotFound
	}
	return nil
}

// AuthorizeSessionAccess checks visibility of a task session via its task's
// workspace. Wired into the lifecycle manager (SetSessionAccessChecker) so
// every session-scoped HTTP surface (shell, files, processes, ports, vscode,
// LSP) is covered by one chokepoint.
func (s *Service) AuthorizeSessionAccess(ctx context.Context, sessionID string) error {
	_, scoped := callerScope(ctx)
	if !scoped {
		return nil
	}
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	return s.authorizeTaskID(ctx, session.TaskID)
}

// filterWorkspacesForCaller narrows a workspace list to the caller's view.
func filterWorkspacesForCaller(ctx context.Context, workspaces []*models.Workspace) []*models.Workspace {
	userID, scoped := callerScope(ctx)
	if !scoped {
		return workspaces
	}
	visible := make([]*models.Workspace, 0, len(workspaces))
	for _, workspace := range workspaces {
		if workspaceVisibleTo(workspace, userID) {
			visible = append(visible, workspace)
		}
	}
	return visible
}
