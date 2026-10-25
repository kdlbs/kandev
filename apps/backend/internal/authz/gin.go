package authz

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
)

// SubjectFromGin builds the authz subject for a gin request. A request with no
// identity, or the synthetic identity injected while authentication is
// disabled, is unscoped and holds every scope — preserving pre-auth behavior
// byte for byte.
func SubjectFromGin(c *gin.Context) Subject {
	identity, ok := authn.FromGin(c)
	if !ok || identity.Synthetic {
		return Subject{Unscoped: true}
	}
	return Subject{
		UserID:  identity.UserID,
		OrgID:   identity.OrgID,
		OrgRole: NormalizeOrgRole(string(identity.Role)),
	}
}

// RequireOrgScope aborts with 403 unless the caller holds the org scope.
// Requests with no identity at all are rejected with 401.
//
// This replaces the previous per-route admin bit. Naming the capability rather
// than the role is the point: "which routes need org.config.manage" is a
// question the registry can answer, "which routes called RequireAdmin" was not.
func RequireOrgScope(scope Scope) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := authn.FromGin(c); !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		if !SubjectOrgScopes(SubjectFromGin(c)).Has(scope) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "insufficient permissions",
				"scope": string(scope),
			})
			return
		}
		c.Next()
	}
}
