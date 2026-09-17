package runtimeauth

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"net/http"
	"strings"
)

const (
	errorResponseKey = "error"
)

type IdentityReader interface {
	GetAgentInstance(context.Context, string) (*models.AgentProfile, error)
}
type TokenValidator interface {
	ValidateAgentJWT(string) (*AgentClaims, error)
}

// Middleware preserves the existing signed runtime context contract.
func Middleware(auth TokenValidator, identities IdentityReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			c.Next()
			return
		}
		claims, err := auth.ValidateAgentJWT(strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{errorResponseKey: "invalid token"})
			return
		}
		agent, err := identities.GetAgentInstance(c.Request.Context(), claims.AgentProfileID)
		if err != nil || agent.WorkspaceID != claims.WorkspaceID {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{errorResponseKey: "agent not found"})
			return
		}
		if ws := c.Param("wsId"); ws != "" && ws != claims.WorkspaceID {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{errorResponseKey: "token workspace mismatch"})
			return
		}
		c.Set("agent_claims", claims)
		c.Set("agent_caller", agent)
		c.Next()
	}
}
