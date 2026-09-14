// Package authn carries the per-request caller identity for the opt-in
// authentication feature. It is intentionally tiny and dependency-light so
// any package (HTTP handlers, WS gateway, boot-payload builders) can consume
// the identity without importing the auth service.
package authn

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Role is the coarse authorization level of a user.
type Role string

const (
	// RoleAdmin unlocks user management and system-settings mutation. It does
	// NOT grant visibility into other users' workspaces (hard privacy).
	RoleAdmin Role = "admin"
	// RoleMember is a regular authenticated user.
	RoleMember Role = "member"
	// RoleGuest holds no org scopes and reaches only workspaces it is an
	// explicit member of, including none of the org-visible ones.
	RoleGuest Role = "guest"
)

// GinContextKey is the gin-context key holding the request Identity.
const GinContextKey = "kandev_auth_identity"

type ctxKey struct{}

// Identity describes the authenticated caller of a request or WS connection.
// When authentication is disabled, the middleware injects a synthetic admin
// identity for the pre-auth single user so downstream code never branches on
// auth mode — only on identity.
type Identity struct {
	UserID string
	Role   Role
	// Synthetic marks the implicit single-user identity injected when
	// authentication is disabled (preserves today's behavior exactly).
	Synthetic bool
	// SessionID is set when the identity came from a browser session cookie.
	SessionID string
	// TokenID is set when the identity came from a personal access token.
	TokenID string
	// OrgID is the caller's tenant. It comes from the user record and from
	// nowhere else: no route, payload, header or WS frame may name an org, so
	// the tenant is a total function of the authenticated identity and no
	// request can move a caller between tenants.
	OrgID string
	// Instance marks the operator tier, which manages organizations, feature
	// toggles and instance-wide configuration. An operator holds NO org scopes
	// and reaches no org's workspaces: it is an administration tier, not a
	// visibility one.
	Instance bool
}

// IsAdmin reports whether the identity carries the admin role.
func (i Identity) IsAdmin() bool { return i.Role == RoleAdmin }

// WithIdentity returns a context carrying the identity.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// IdentityFromContext reads the identity from a context. It works for both
// gin-request contexts (SetOnGin mirrors onto the request context) and plain
// net/http paths such as the SPA boot-payload builder.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}

// SetOnGin stores the identity on the gin context AND the underlying request
// context, so both gin handlers (FromGin) and plain net/http consumers (the
// NoRoute SPA handler only receives *http.Request) can read it.
func SetOnGin(c *gin.Context, id Identity) {
	c.Set(GinContextKey, id)
	c.Request = c.Request.WithContext(WithIdentity(c.Request.Context(), id))
}

// FromGin reads the identity from a gin context.
func FromGin(c *gin.Context) (Identity, bool) {
	if v, ok := c.Get(GinContextKey); ok {
		if id, ok := v.(Identity); ok {
			return id, true
		}
	}
	return IdentityFromContext(c.Request.Context())
}

// RequireRealIdentity aborts unless the request carries a non-synthetic
// authenticated identity. While authentication is disabled the middleware
// injects a synthetic single-user admin (see httpmw.SyntheticIdentity) that
// carries RoleAdmin — so RequireAdmin alone would let anyone hitting a
// not-yet-enabled instance mint a PAT or plant an admin that survives
// enablement and hijacks first-run setup. Guarding credential- and
// user-management routes with this closes that hole; public bootstrap routes
// (setup/login/me/invite-accept) are intentionally NOT guarded by it. It
// responds 404 so the management surface simply does not exist while disabled.
func RequireRealIdentity() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := FromGin(c)
		if !ok || id.Synthetic {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.Next()
	}
}

// RequireAdmin aborts with 403 unless the request identity is an admin.
// Requests without any identity are rejected with 401.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := FromGin(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		if !id.IsAdmin() {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin role required"})
			return
		}
		c.Next()
	}
}
