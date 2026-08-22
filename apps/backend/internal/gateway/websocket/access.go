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

// WorkspaceReadersResolver resolves the set of user IDs that may currently
// read a workspace. An empty, non-nil slice means "nobody", which is a real
// answer and must not be treated as "everybody".
type WorkspaceReadersResolver func(ctx context.Context, workspaceID string) ([]string, error)

// AuthPolicy groups the gateway's auth hooks.
type AuthPolicy struct {
	// Enforced reports whether authentication is currently required
	// (auth mode != disabled). Consulted per connection attempt.
	Enforced func() bool
	// ResolveToken authenticates a ?token=<PAT> query credential for
	// programmatic WS clients that cannot send cookies or headers.
	ResolveToken func(ctx context.Context, token string) (authn.Identity, bool)
	// ActiveUser reports whether a user account is still active. It is
	// consulted after capability authentication: the subtree capability
	// restores the signed identity without a live credential, so without this
	// check a user disabled mid-preview would keep the sliding-mint capability
	// alive indefinitely. Nil skips the check (zero-policy compatibility).
	ActiveUser func(ctx context.Context, userID string) bool
	// Subscriptions gates task/session topic subscriptions.
	Subscriptions SubscriptionAccessPolicy
	// WorkspaceOwner powers BroadcastToWorkspace owner routing.
	WorkspaceOwner WorkspaceOwnerResolver
	// WorkspaceReaders resolves every user that can currently read a
	// workspace: its owner plus explicit members plus, when the workspace is
	// visible to the organization, that organization's non-guest users.
	//
	// Owner-only routing predates team access and would silently deliver a
	// shared board's updates to nobody but its owner, so this is the resolver
	// BroadcastToWorkspace uses when it is wired.
	WorkspaceReaders WorkspaceReadersResolver
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
// any upgrade) and accepts ?token=<PAT> for headerless clients. Port-proxy
// requests additionally accept the short-lived subtree capability minted after
// the preview document authenticated (see PortProxyHandler): carried as a
// path-scoped cookie for ordinary subresource fetches, or as a query parameter
// appended to rewritten asset URLs for fetches that never send cookies (the
// browser's <link rel="manifest"> fetch, sandboxed iframes).
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
		if g.resolvePortProxyCapability(c) {
			// A preview reloaded with ?token=<PAT> after the capability cookie
			// was installed authenticates via the capability, but the PAT is
			// still a gateway credential: it must be stripped from the
			// forwarded query (never reach agentctl or the app), while a
			// cookie-authenticated app's own token parameter is preserved.
			if token := c.Query("token"); token != "" && policy.ResolveToken != nil {
				if _, ok := policy.ResolveToken(c.Request.Context(), token); ok {
					c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), proxyTokenConsumedKey{}, true))
				}
			}
			c.Next()
			return
		}
		if token := c.Query("token"); token != "" && policy.ResolveToken != nil {
			if identity, ok := policy.ResolveToken(c.Request.Context(), token); ok {
				authn.SetOnGin(c, identity)
				// Remember that the gateway consumed ?token= so the port proxy
				// can strip it from the forwarded query; a cookie-authenticated
				// app's own token parameter must be preserved.
				c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), proxyTokenConsumedKey{}, true))
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
	}
}

// resolvePortProxyCapability authenticates a /port-proxy/ request via the
// short-lived subtree capability minted after the preview document
// authenticated. It accepts the query-parameter form (appended to rewritten
// asset URLs for fetches that never send cookies, like the browser's manifest
// fetch) and the path-scoped cookie form (ordinary subresource fetches). The
// credential only validates against the exact session:port subtree it was
// minted for; on success the issuing identity is restored so downstream
// session-ownership checks still run as the real user.
func (g *Gateway) resolvePortProxyCapability(c *gin.Context) bool {
	sessionID, port, ok := portProxyTarget(c)
	if !ok || g.PortProxyHandler == nil {
		return false
	}
	// Accept ANY value of the capability parameter that validates: a
	// credential-less request may carry multiple kandev_cap values (the app's
	// own or an encoded duplicate), and the first one must not be able to
	// shadow a valid one. A restored identity whose account is no longer
	// active is rejected (same active-user gate the cookie/PAT paths run).
	active := func(identity authn.Identity) bool {
		if g.authPolicy.ActiveUser == nil {
			return true
		}
		return g.authPolicy.ActiveUser(c.Request.Context(), identity.UserID)
	}
	if values := c.Request.URL.Query()[proxyCapabilityQueryParam]; len(values) > 0 {
		for _, raw := range values {
			if identity, valid := g.PortProxyHandler.validateCapability(raw, sessionID, port); valid && active(identity) {
				authn.SetOnGin(c, identity)
				return true
			}
		}
	}
	// Accept ANY capability cookie that validates: duplicate kandev_port_proxy
	// cookies (a stale or app-created value on a more-specific path is sent
	// before the gateway's subtree cookie) must not let the first one shadow a
	// valid value, matching the duplicate-safe query-parameter handling.
	for _, cookie := range c.Request.Cookies() {
		if cookie.Name != proxyCapabilityCookieName || cookie.Value == "" {
			continue
		}
		if identity, valid := g.PortProxyHandler.validateCapability(cookie.Value, sessionID, port); valid && active(identity) {
			authn.SetOnGin(c, identity)
			return true
		}
	}
	return false
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

// clientMayReceiveAny reports whether a client is one of the users allowed to
// receive a workspace broadcast.
//
// Synthetic identities (authentication disabled) and identity-less clients
// still receive everything, preserving pre-auth behavior. An authenticated
// client receives only when its user is in the set, so a member who just lost
// access stops receiving on the next broadcast rather than at disconnect.
func clientMayReceiveAny(client *Client, allowed map[string]struct{}) bool {
	if client.identity.Synthetic || client.identity.UserID == "" {
		return true
	}
	_, ok := allowed[client.identity.UserID]
	return ok
}
