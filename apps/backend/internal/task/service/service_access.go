package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// Workspace access scoping.
//
// Reach and permission are two questions, answered in that order:
//
//   - Reach ("can this caller see the workspace at all") comes from the owner,
//     an explicit workspace_members row, or membership of a unit above the
//     one the workspace sits in. A
//     caller who cannot reach a workspace gets the *NotFound sentinels — a
//     foreign workspace is indistinguishable from a nonexistent one.
//   - Permission ("may they do this particular thing") comes from
//     authz.ResolveWorkspace, the single resolver. A caller who can read but
//     lacks the action's scope gets ErrForbidden: existence is already known
//     to them, so 403 leaks nothing and 404 would only confuse.
//
// Scoping keys off the request-context identity placed there by the auth
// middleware:
//   - no identity in ctx  → internal caller (event bus, pollers, office
//     schedulers) → unscoped, exactly as before the auth feature. In-session
//     agent MCP calls arrive with the task owner's identity attached by
//     internal/mcp/scope.
//   - synthetic identity  → auth disabled → unscoped (today's behavior)
//   - real identity       → resolved through authz

// tenancyEnforced mirrors features.multiTenancy. It is a package-level value
// set once at startup rather than threaded through every call: it is a
// process-wide configuration fact, and plumbing it through would touch every
// authorize* signature for no gain.
var tenancyEnforced bool

// SetTenancyEnforced records whether organizations are on.
func SetTenancyEnforced(enforced bool) { tenancyEnforced = enforced }

// callerOrgID returns the requesting identity's organization, empty for
// internal/synthetic callers and when organizations are off.
func callerOrgID(ctx context.Context) string {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic {
		return ""
	}
	return identity.OrgID
}

// callerScope returns the scoping user ID; ok=false means unscoped.
func callerScope(ctx context.Context) (string, bool) {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic {
		return "", false
	}
	return identity.UserID, true
}

// callerSubject builds the authz subject for the request identity.
func callerSubject(ctx context.Context) authz.Subject {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic {
		return authz.Subject{Unscoped: true}
	}
	return authz.Subject{
		UserID:          identity.UserID,
		OrgID:           identity.OrgID,
		OrgRole:         authz.NormalizeOrgRole(string(identity.Role)),
		TenancyEnforced: tenancyEnforced,
	}
}

// workspaceDecision resolves what the caller may do in a workspace. It reads
// the caller's membership row (a single primary-key lookup) and hands both to
// the resolver.
//
// Any failure reading membership returns Denied rather than continuing with no
// row: continuing would silently promote the caller to the org default role,
// which is exactly how a transient database error becomes a privilege
// escalation.
func (s *Service) workspaceDecision(ctx context.Context, workspace *models.Workspace) authz.Decision {
	subject := callerSubject(ctx)
	if subject.Unscoped {
		return authz.ResolveWorkspace(subject, authz.WorkspaceRef{})
	}
	if workspace == nil {
		return authz.Denied()
	}

	ref := authz.WorkspaceRef{
		OwnerID: workspace.OwnerID,
		OrgID:   workspace.OrgID,
	}
	// The tenant boundary is absolute and is checked before owner or
	// membership, so a foreign-org workspace is resolved without touching the
	// member table at all.
	if subject.OrgID != "" && workspace.OrgID != "" && subject.OrgID != workspace.OrgID {
		return authz.Denied()
	}
	// Owner and unowned workspaces resolve without touching the tree.
	if workspace.OwnerID == "" || workspace.OwnerID == subject.UserID {
		return authz.ResolveWorkspace(subject, ref)
	}

	inherited, ok := s.inheritedRole(ctx, subject.UserID, workspace)
	if !ok {
		return authz.Denied()
	}
	ref.InheritedRole = inherited

	member, err := s.workspaces.GetWorkspaceMember(ctx, workspace.ID, subject.UserID)
	if err != nil {
		s.logger.Warn("workspace membership lookup failed; denying access")
		return authz.Denied()
	}
	if member != nil {
		ref.MemberRole = authz.NormalizeWorkspaceRole(member.Role)
	}
	return authz.ResolveWorkspace(subject, ref)
}

// WorkspaceDecisionByID is the public resolver used by handlers that need the
// caller's scopes for a workspace (DTO projection, scope gating).
func (s *Service) WorkspaceDecisionByID(ctx context.Context, workspaceID string) (authz.Decision, error) {
	if workspaceID == "" {
		return authz.ResolveWorkspace(callerSubject(ctx), authz.WorkspaceRef{}), nil
	}
	workspace, err := s.workspaces.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return authz.Denied(), err
	}
	return s.workspaceDecision(ctx, workspace), nil
}

// authorizeWorkspaceID checks reach of a workspace by ID.
func (s *Service) authorizeWorkspaceID(ctx context.Context, workspaceID string) error {
	_, err := s.requireWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead, repoerrors.ErrWorkspaceNotFound)
	return err
}

// AuthorizeWorkspaceScope enforces one scope on a workspace. Callers that only
// need reach use authorizeWorkspaceID.
func (s *Service) AuthorizeWorkspaceScope(ctx context.Context, workspaceID string, scope authz.Scope) error {
	_, err := s.requireWorkspaceScope(ctx, workspaceID, scope, repoerrors.ErrWorkspaceNotFound)
	return err
}

// requireWorkspaceScope resolves the decision and applies the 404-vs-403 rule.
func (s *Service) requireWorkspaceScope(
	ctx context.Context, workspaceID string, scope authz.Scope, notFound error,
) (authz.Decision, error) {
	if _, scoped := callerScope(ctx); !scoped || workspaceID == "" {
		return authz.ResolveWorkspace(authz.Subject{Unscoped: true}, authz.WorkspaceRef{}), nil
	}
	workspace, err := s.workspaces.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return authz.Denied(), err
	}
	decision := s.workspaceDecision(ctx, workspace)
	if !decision.CanRead() {
		return decision, notFound
	}
	if !decision.Has(scope) {
		return decision, ErrForbidden
	}
	return decision, nil
}

// authorizeTaskID checks reach of a task via its workspace.
func (s *Service) authorizeTaskID(ctx context.Context, taskID string) error {
	return s.authorizeTaskScope(ctx, taskID, authz.ScopeWorkspaceRead)
}

// AuthorizeTaskScope enforces one scope on the workspace owning a task.
func (s *Service) AuthorizeTaskScope(ctx context.Context, taskID string, scope authz.Scope) error {
	return s.authorizeTaskScope(ctx, taskID, scope)
}

func (s *Service) authorizeTaskScope(ctx context.Context, taskID string, scope authz.Scope) error {
	if _, scoped := callerScope(ctx); !scoped {
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
		// A dangling workspace reference (the row is genuinely gone) should
		// not hide the task from the single user who can already see
		// everything else about it. Any OTHER lookup failure fails closed: a
		// transient database error must not read as "granted".
		if errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
			return nil
		}
		return err
	}
	decision := s.workspaceDecision(ctx, workspace)
	if !decision.CanRead() {
		return repoerrors.ErrTaskNotFound
	}
	if !decision.Has(scope) {
		return ErrForbidden
	}
	return nil
}

// authorizeWorkflowID checks reach of a workflow via its workspace.
func (s *Service) authorizeWorkflowID(ctx context.Context, workflowID string) error {
	if _, scoped := callerScope(ctx); !scoped {
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
	if !s.workspaceDecision(ctx, workspace).CanRead() {
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

// AuthorizeSessionAccess checks reach of a task session via its task's
// workspace. Wired into the lifecycle manager so session-scoped surfaces
// (files, processes, ports, vscode, terminal, LSP) are covered.
func (s *Service) AuthorizeSessionAccess(ctx context.Context, sessionID string) error {
	return s.AuthorizeSessionScope(ctx, sessionID, authz.ScopeWorkspaceRead)
}

// AuthorizeSessionScope enforces one scope on the workspace owning a session.
// Execution surfaces (terminal, shell, file writes, port previews) use
// authz.ScopeSessionExec so a viewer who may read a transcript never gets a
// shell in the worktree.
func (s *Service) AuthorizeSessionScope(ctx context.Context, sessionID string, scope authz.Scope) error {
	if _, scoped := callerScope(ctx); !scoped {
		return nil
	}
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	return s.authorizeTaskScope(ctx, session.TaskID, scope)
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

// AuthorizeEnvironmentAccess checks reach of a task environment via its task's
// workspace. Used by the terminal environment-shell route, which resolves
// executions by environment ID rather than session ID.
func (s *Service) AuthorizeEnvironmentAccess(ctx context.Context, taskEnvironmentID string) error {
	return s.AuthorizeEnvironmentScope(ctx, taskEnvironmentID, authz.ScopeWorkspaceRead)
}

// AuthorizeEnvironmentScope enforces one scope on a task environment.
func (s *Service) AuthorizeEnvironmentScope(ctx context.Context, taskEnvironmentID string, scope authz.Scope) error {
	if _, scoped := callerScope(ctx); !scoped {
		return nil
	}
	env, err := s.taskEnvironments.GetTaskEnvironment(ctx, taskEnvironmentID)
	if err != nil {
		return err
	}
	return s.authorizeTaskScope(ctx, env.TaskID, scope)
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

// authorizeRepositoryID checks reach of a repository via its workspace.
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
//
// The caller's whole membership map is read once here rather than per
// workspace: a board render walks every workspace, so a per-row lookup would
// be an N+1 on the hottest list in the product.
func (s *Service) filterWorkspacesForCaller(ctx context.Context, workspaces []*models.Workspace) []*models.Workspace {
	subject := callerSubject(ctx)
	if subject.Unscoped {
		return workspaces
	}

	memberRoles, err := s.workspaces.ListWorkspaceIDsForMember(ctx, subject.UserID)
	if err != nil {
		s.logger.Warn("workspace membership list failed; returning no workspaces")
		return nil
	}

	inherited, ok := s.inheritedRolesFor(ctx, subject.UserID, workspaces)
	if !ok {
		return nil
	}

	visible := make([]*models.Workspace, 0, len(workspaces))
	for _, workspace := range workspaces {
		if workspace == nil {
			continue
		}
		ref := authz.WorkspaceRef{
			OwnerID:       workspace.OwnerID,
			OrgID:         workspace.OrgID,
			MemberRole:    authz.NormalizeWorkspaceRole(memberRoles[workspace.ID]),
			InheritedRole: inherited[workspace.ID],
		}
		if authz.ResolveWorkspace(subject, ref).CanRead() {
			visible = append(visible, workspace)
		}
	}
	return visible
}
