package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// Per-user workspace scoping (opt-in authentication).
//
// Every user-facing service entry point calls one of the authorize* helpers.
// Scoping keys off the request-context identity placed there by the auth
// middleware:
//   - no identity in ctx  → internal caller (event bus, pollers, office
//     schedulers) → unscoped, exactly as before the auth feature. In-session
//     agent MCP calls used to land here too; they now arrive with the task
//     owner's identity attached by internal/mcp/scope.
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
	// A workflow whose workspace cannot be resolved has no owner to check
	// against, so neither outcome below is "authorized". This used to be one
	// `return nil` covering both, which handed any workflow ID that survived
	// a failed lookup to whoever guessed it.
	//
	// The pre-auth unowned row is not this case: workspace_id == "" is
	// answered above and stays visible to everyone. Here the workflow names a
	// workspace that is not there — `workflows.workspace_id` carries no
	// foreign key, so a deleted workspace can leave one behind — and an
	// orphan belongs to nobody, which under per-user scoping means nobody
	// sees it.
	workspace, err := s.workspaces.GetWorkspace(ctx, workflow.WorkspaceID)
	switch {
	case errors.Is(err, repoerrors.ErrWorkspaceNotFound):
		return repoerrors.ErrWorkspaceNotFound
	case err != nil:
		// A failed lookup is not an answer at all: propagate it rather than
		// letting a transient database error read as either allow or deny.
		return err
	}
	if !workspaceVisibleTo(workspace, userID) {
		return repoerrors.ErrWorkspaceNotFound
	}
	return nil
}

// AuthorizeTaskAccess is the public form of authorizeTaskID, consumed by the
// WS gateway's subscription checks.
func (s *Service) AuthorizeTaskAccess(ctx context.Context, taskID string) error {
	return s.authorizeTaskID(ctx, taskID)
}

// AuthorizeWorkflowAccess is the public form of authorizeWorkflowID, consumed
// by the workflow service, whose step/export/import surface reaches workflows
// by ID but does not own workspace permissions.
func (s *Service) AuthorizeWorkflowAccess(ctx context.Context, workflowID string) error {
	return s.authorizeWorkflowID(ctx, workflowID)
}

// AuthorizeWorkspaceAccess is the public form of authorizeWorkspaceID,
// consumed by the office route-scoping middleware.
func (s *Service) AuthorizeWorkspaceAccess(ctx context.Context, workspaceID string) error {
	return s.authorizeWorkspaceID(ctx, workspaceID)
}

// AuthorizeSessionAccess checks visibility of a task session via its task's
// workspace. Wired into the lifecycle manager so session-scoped surfaces
// (files, processes, ports, vscode, terminal, LSP) are covered.
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

// AuthorizeTaskSessionAccess checks that both identifiers are visible to the
// caller and that the session belongs to the supplied task. Mismatches use the
// task not-found sentinel so callers cannot enumerate another task's sessions.
func (s *Service) AuthorizeTaskSessionAccess(ctx context.Context, taskID, sessionID string) error {
	if err := s.AuthorizeTaskAccess(ctx, taskID); err != nil {
		return err
	}
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil || session.TaskID != taskID {
		return repoerrors.ErrTaskNotFound
	}
	return nil
}

// AuthorizeEnvironmentAccess checks visibility of a task environment via its
// task's workspace. Used by the terminal environment-shell route, which
// resolves executions by environment ID rather than session ID.
func (s *Service) AuthorizeEnvironmentAccess(ctx context.Context, taskEnvironmentID string) error {
	_, scoped := callerScope(ctx)
	if !scoped {
		return nil
	}
	env, err := s.taskEnvironments.GetTaskEnvironment(ctx, taskEnvironmentID)
	if err != nil {
		return err
	}
	return s.authorizeTaskID(ctx, env.TaskID)
}

// AuthorizeTaskEnvironmentAccess checks that both identifiers are visible to
// the caller and that the environment is one the task is actually bound to.
// Mismatches use the task not-found sentinel, mirroring
// AuthorizeTaskSessionAccess, so callers cannot enumerate environments by
// pairing them against a task they do own.
//
// Authorizing the two IDs separately is not enough: both checks pass for a
// caller who owns the task and, independently, owns some unrelated
// environment, and the terminal route then merges state from the two.
//
// The relationship is deliberately not `env.TaskID == taskID`. inherit_parent
// binds a subtask's session to the parent task's environment, and shared_group
// binds every member of a workspace group to one canonical environment, so the
// row's owning task is legitimately a different task in both. What establishes
// the pair is a session of this task pointing at this environment.
func (s *Service) AuthorizeTaskEnvironmentAccess(ctx context.Context, taskID, taskEnvironmentID string) error {
	if _, scoped := callerScope(ctx); !scoped {
		return nil
	}
	if err := s.AuthorizeTaskAccess(ctx, taskID); err != nil {
		return err
	}
	if err := s.AuthorizeEnvironmentAccess(ctx, taskEnvironmentID); err != nil {
		return err
	}
	env, err := s.taskEnvironments.GetTaskEnvironment(ctx, taskEnvironmentID)
	if err != nil {
		return err
	}
	if env != nil && env.TaskID == taskID {
		return nil
	}
	sessions, err := s.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if session != nil && session.TaskEnvironmentID == taskEnvironmentID {
			return nil
		}
	}
	return repoerrors.ErrTaskNotFound
}

// authorizeRepositoryID checks visibility of a repository via its workspace.
// Denials use ErrRepositoryNotFound (no existence leak).
func (s *Service) authorizeRepositoryID(ctx context.Context, repositoryID string) error {
	if _, scoped := callerScope(ctx); !scoped {
		return nil
	}
	repo, err := s.repoEntities.GetRepository(ctx, repositoryID)
	if err != nil {
		return err
	}
	if repo == nil {
		return nil
	}
	if err := s.authorizeWorkspaceID(ctx, repo.WorkspaceID); err != nil {
		return repoerrors.ErrRepositoryNotFound
	}
	return nil
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
