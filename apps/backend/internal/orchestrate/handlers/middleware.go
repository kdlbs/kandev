package handlers

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

const (
	ctxKeyAgentClaims = "agent_claims"
	ctxKeyAgentCaller = "agent_caller"
)

// agentAuthMiddleware extracts a Bearer JWT from the Authorization header,
// validates it, loads the agent instance, and sets both in the gin context.
// Requests without a JWT are treated as UI/admin requests and pass through.
func agentAuthMiddleware(svc *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			c.Next()
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		claims, err := svc.ValidateAgentJWT(token)
		if err != nil {
			c.AbortWithStatusJSON(401, gin.H{"error": "invalid token"})
			return
		}
		agent, err := svc.GetAgentInstance(c.Request.Context(), claims.AgentInstanceID)
		if err != nil {
			c.AbortWithStatusJSON(401, gin.H{"error": "agent not found"})
			return
		}
		c.Set(ctxKeyAgentClaims, claims)
		c.Set(ctxKeyAgentCaller, agent)
		c.Next()
	}
}

// agentCallerFromCtx returns the authenticated agent or nil for UI requests.
func agentCallerFromCtx(c *gin.Context) *models.AgentInstance {
	val, ok := c.Get(ctxKeyAgentCaller)
	if !ok {
		return nil
	}
	agent, _ := val.(*models.AgentInstance)
	return agent
}

// agentClaimsFromCtx returns the JWT claims or nil for UI requests.
func agentClaimsFromCtx(c *gin.Context) *service.AgentClaims {
	val, ok := c.Get(ctxKeyAgentClaims)
	if !ok {
		return nil
	}
	claims, _ := val.(*service.AgentClaims)
	return claims
}

// taskScopeCheck verifies that an agent caller is authorised to operate
// on the task identified by the :id route parameter. UI/admin requests
// pass through. Agents with can_assign_tasks may access any task; others
// can only operate on their assigned task.
func taskScopeCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		agent := agentCallerFromCtx(c)
		if agent == nil {
			c.Next()
			return
		}
		claims := agentClaimsFromCtx(c)
		if claims == nil {
			c.Next()
			return
		}
		perms := service.ResolvePermissions(agent.Role, agent.Permissions)
		if service.HasPermission(perms, service.PermCanAssignTasks) {
			c.Next()
			return
		}
		taskID := c.Param("id")
		if claims.TaskID != "" && taskID != claims.TaskID {
			c.AbortWithStatusJSON(403, gin.H{"error": "cannot operate on unassigned task"})
			return
		}
		c.Next()
	}
}

// Ensure taskScopeCheck and agentClaimsFromCtx are available for route wiring.
// They are used by registerIssueRoutes when task-specific routes are added.
var _ = taskScopeCheck
var _ = agentClaimsFromCtx
