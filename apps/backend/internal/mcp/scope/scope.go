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
//
// ok=false means "no active account for this user" — the account is missing or
// has been disabled. Callers must deny rather than substitute an identity.
type IdentityLookup interface {
	IdentityForUser(ctx context.Context, userID string) (authn.Identity, bool)
}

// unownedScopeUserID is the owner attached when the stream's own workspace has
// no owner: workspaces created before auth was enabled, and any created since
// by an internal (unscoped) caller, since CreateWorkspace only stamps an owner
// for scoped callers.
//
// Such a stream must not fall back to a no-identity context — the task service
// reads that as an internal caller and grants everything. Instead we scope to
// an ID no stored account can hold. The service's visibility rule is "an empty
// owner_id, or my own user ID", so this yields exactly the intended reach: the
// unowned rows that are public by the pre-auth compatibility contract, and no
// owned row belonging to anyone else. The colon keeps it from colliding with a
// UUID or with the default-user row.
const unownedScopeUserID = "kandev:unowned-workspace"

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
// authentication is disabled so single-user instances keep today's behavior;
// identities must be non-nil for any enforced use, or every dispatch is denied.
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
// The only case that stays fully unscoped is authentication being disabled,
// which preserves single-user behavior exactly. Under enforced auth every
// dispatch is scoped to somebody:
//
//   - a resolvable owner with an active account → that user's real identity
//   - no owner (unowned workspace, or a task with no workspace) →
//     unownedScopeUserID, which reaches unowned rows and nothing else
//   - anything unresolvable (missing task/workspace row, lookup failure, or an
//     owner whose account is gone or disabled) → an error, which the caller
//     turns into a denied dispatch
//
// The last case is deliberate: returning ctx unchanged would hand the agent an
// identity-free context, which the task service reads as an internal caller and
// serves unscoped — the exact bug this package closes. "We cannot tell who owns
// this stream" must deny foreign data, not grant all of it.
func (r *Resolver) Scope(ctx context.Context, taskID string) (context.Context, error) {
	if r == nil || taskID == "" || r.enforced == nil || !r.enforced() {
		return ctx, nil
	}
	// A context that already carries an identity came from a credentialed
	// caller; never replace or widen it.
	if _, ok := authn.IdentityFromContext(ctx); ok {
		return ctx, nil
	}
	ownerID, err := r.ownerOf(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if ownerID == "" {
		r.logger.Warn("in-session MCP stream has no workspace owner; limiting it to unowned rows",
			zap.String("task_id", taskID))
		return authn.WithIdentity(ctx, authn.Identity{UserID: unownedScopeUserID, Role: authn.RoleMember}), nil
	}
	if r.identities == nil {
		return nil, fmt.Errorf("resolve owner of task %s: no identity lookup wired", taskID)
	}
	// A disabled or deleted account must not keep serving its agent: disabling
	// a user revokes their sessions and PATs, so an in-flight agent session is
	// the one remaining way in. Deny instead of minting an identity for them.
	identity, ok := r.identities.IdentityForUser(ctx, ownerID)
	if !ok {
		return nil, fmt.Errorf("resolve owner of task %s: user %s has no active account", taskID, ownerID)
	}
	return authn.WithIdentity(ctx, identity), nil
}

// ownerOf returns the user ID owning taskID's workspace, or "" when the task
// has no workspace or that workspace has no owner.
//
// A missing task or workspace row is an error, not an empty owner: an agent
// whose task or workspace was deleted mid-session must be denied rather than
// fall through to unscoped access.
func (r *Resolver) ownerOf(ctx context.Context, taskID string) (string, error) {
	task, err := r.tasks.GetTask(ctx, taskID)
	if err != nil {
		return "", fmt.Errorf("resolve owner of task %s: %w", taskID, err)
	}
	if task == nil {
		return "", fmt.Errorf("resolve owner of task %s: task not found", taskID)
	}
	if task.WorkspaceID == "" {
		return "", nil
	}
	workspace, err := r.tasks.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return "", fmt.Errorf("resolve owner of workspace %s: %w", task.WorkspaceID, err)
	}
	if workspace == nil {
		return "", fmt.Errorf("resolve owner of workspace %s: workspace not found", task.WorkspaceID)
	}
	return workspace.OwnerID, nil
}
