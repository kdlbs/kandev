package websocket

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
)

// Per-user scoping hooks for the WebSocket gateway (opt-in authentication).
//
// All hooks are optional: a zero AuthPolicy keeps today's behavior exactly
// (anonymous connections, unchecked subscriptions, broadcast-to-all).

// SubscriptionAccessPolicy verifies that the subscribing client may observe a
// topic. Implementations receive a context carrying the client identity
// (authn.IdentityFromContext) and return a *NotFound-style error to deny.
type SubscriptionAccessPolicy struct {
	Task    func(ctx context.Context, taskID string) error
	Session func(ctx context.Context, sessionID string) error
}

// WorkspaceOwnerResolver resolves a workspace's owning user ID ("" = unowned
// pre-auth row, delivered to everyone).
type WorkspaceOwnerResolver func(ctx context.Context, workspaceID string) (string, error)

// AuthPolicy groups the gateway's auth hooks.
type AuthPolicy struct {
	// Enforced reports whether authentication is currently required
	// (auth mode != disabled). Consulted per connection attempt.
	Enforced func() bool
	// ResolveToken authenticates a ?token=<PAT> query credential for
	// programmatic WS clients that cannot send cookies or headers.
	ResolveToken func(ctx context.Context, token string) (authn.Identity, bool)
	// Subscriptions gates task/session topic subscriptions.
	Subscriptions SubscriptionAccessPolicy
	// WorkspaceOwner powers BroadcastToWorkspace owner routing.
	WorkspaceOwner WorkspaceOwnerResolver
	// ActionEnvironment gates dispatched actions that name a task environment
	// rather than a task or session — the user-shell actions, which treat
	// task_id as optional. Kept off SubscriptionAccessPolicy because there is
	// no environment subscription topic; this is dispatch-only.
	ActionEnvironment func(ctx context.Context, taskEnvironmentID string) error
}

// SetAuthPolicy installs the auth hooks. Call during startup wiring, before
// SetupRoutes and before the HTTP server accepts connections.
func (g *Gateway) SetAuthPolicy(policy AuthPolicy) {
	g.authPolicy = policy
	g.Hub.setAuthPolicy(policy)
}

// requireConnectionAuth guards WS upgrades and proxy routes. The global HTTP
// auth middleware has already resolved cookie/bearer credentials into the gin
// context; this closes the gap for unauthenticated attempts (JSON 401 before
// any upgrade) and accepts ?token=<PAT> for headerless clients.
func (g *Gateway) requireConnectionAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		policy := g.authPolicy
		if policy.Enforced == nil || !policy.Enforced() {
			c.Next()
			return
		}
		if _, ok := authn.FromGin(c); ok {
			c.Next()
			return
		}
		if token := c.Query("token"); token != "" && policy.ResolveToken != nil {
			if identity, ok := policy.ResolveToken(c.Request.Context(), token); ok {
				authn.SetOnGin(c, identity)
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
	}
}

// clientMayReceive reports whether a client may receive workspace-scoped
// traffic for the given owner. Synthetic identities (auth disabled) see
// everything; unowned workspaces ("" owner) are visible to all.
func clientMayReceive(client *Client, owner string) bool {
	if owner == "" || client.identity.Synthetic || client.identity.UserID == "" {
		return true
	}
	return client.identity.UserID == owner
}
