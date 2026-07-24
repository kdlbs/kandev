// Package scope resolves the authenticated identity that owns an agent
// session's task, so the MCP tools an agent gets automatically inside its own
// session carry the same per-user scope an HTTP request from that user would.
//
// Why this exists: an agent can reach Kandev's MCP two ways. The external
// /mcp endpoint authenticates the caller with a personal access token, so the
// global auth middleware puts a real identity on the request context and the
// task service's authorize* checks apply. In-session MCP calls instead arrive
// over the agent's own WebSocket stream, which carries no credential — the
// dispatch context had no identity at all, which the task service reads as
// "internal caller" (event bus, pollers, office schedulers) and therefore
// serves unscoped. An agent that supplied another user's task_id or
// workflow_id was given their data.
//
// The owning user is derived from the stream's own execution
// (task → workspace → owner), never from the agent-supplied request payload,
// so an agent cannot widen its scope by naming a different session.
package scope

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// TaskOwnerLookup reads the rows needed to find a task's owning user.
// Implemented by the task SQLite repository.
type TaskOwnerLookup interface {
	GetTask(ctx context.Context, id string) (*models.Task, error)
	GetWorkspace(ctx context.Context, id string) (*models.Workspace, error)
}

// IdentityLookup resolves a stored user ID to the real identity that user
// carries on an authenticated request. Implemented by *auth.Service.
type IdentityLookup interface {
	IdentityForUser(ctx context.Context, userID string) (authn.Identity, bool)
}

// Resolver attaches task-owner identity to in-session MCP dispatch contexts.
type Resolver struct {
	tasks      TaskOwnerLookup
	identities IdentityLookup
	// enforced reports whether per-user authentication is on. Kept as a
	// callback because the auth mode is resolved at runtime (feature toggle)
	// rather than fixed at wiring time.
	enforced func() bool
	logger   *logger.Logger
}

// NewResolver builds the resolver. enforced must report false while
// authentication is disabled so single-user instances keep today's behavior.
func NewResolver(
	tasks TaskOwnerLookup,
	identities IdentityLookup,
	enforced func() bool,
	log *logger.Logger,
) *Resolver {
	return &Resolver{
		tasks:      tasks,
		identities: identities,
		enforced:   enforced,
		logger:     log.WithFields(zap.String("component", "mcp-scope")),
	}
}

// Scope returns ctx carrying the identity of the user who owns taskID.
//
// It deliberately leaves ctx untouched — preserving the pre-auth unscoped
// behavior — when authentication is disabled, when the task has no workspace,
// or when the workspace is still unowned: rows with an empty owner_id, created
// before auth was enabled, stay visible to everyone until the setup wizard
// claims them, which is the same rule the task service applies.
//
// A lookup failure returns an error instead of silently falling back to an
// unscoped context. Under enforced auth, "we cannot tell who owns this stream"
// must deny foreign data rather than grant all of it.
func (r *Resolver) Scope(ctx context.Context, taskID string) (context.Context, error) {
	if r == nil || taskID == "" || r.enforced == nil || !r.enforced() {
		return ctx, nil
	}
	// A context that already carries an identity came from a credentialed
	// caller; never replace or widen it.
	if _, ok := authn.IdentityFromContext(ctx); ok {
		return ctx, nil
	}
	task, err := r.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("resolve owner of task %s: %w", taskID, err)
	}
	if task == nil || task.WorkspaceID == "" {
		return ctx, nil
	}
	workspace, err := r.tasks.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("resolve owner of workspace %s: %w", task.WorkspaceID, err)
	}
	if workspace == nil || workspace.OwnerID == "" {
		return ctx, nil
	}
	return authn.WithIdentity(ctx, r.identityFor(ctx, workspace.OwnerID)), nil
}

// identityFor prefers the owner's real stored identity so role-gated surfaces
// see the right role. When the account is missing or disabled it still scopes
// to the owner ID with the least-privileged role: falling back to an unscoped
// context would hand the agent every user's data, which is the bug this
// package exists to close.
func (r *Resolver) identityFor(ctx context.Context, ownerID string) authn.Identity {
	if r.identities != nil {
		if identity, ok := r.identities.IdentityForUser(ctx, ownerID); ok {
			return identity
		}
	}
	r.logger.Warn("task owner has no active account; scoping in-session MCP with least privilege",
		zap.String("owner_id", ownerID))
	return authn.Identity{UserID: ownerID, Role: authn.RoleMember}
}
