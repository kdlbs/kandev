package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/office/agents"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/orchestration"
	orchestrationruntime "github.com/kandev/kandev/internal/orchestration/runtime"
	"net/http"
	"strings"
)

func orchestrationRunGuard(features config.FeaturesConfig, repo *officesqlite.Repository) func(context.Context, string) (bool, error) {
	return func(ctx context.Context, id string) (bool, error) {
		role, err := repo.OrchestratorRoleID(ctx, id)
		if err != nil {
			return false, err
		}
		if role != "" {
			_, err := repo.OrchestrationStore().PersonaUserOwner(ctx, id)
			return features.Orchestration, err
		}
		owner, err := repo.OrchestrationStore().PersonaUserOwner(ctx, id)
		return features.Office && owner == "", err
	}
}

func registerOrchestration(p routeParams) {
	if !p.features.Orchestration || p.services.Orchestration == nil {
		return
	}
	runtimeGroup := p.router.Group("/api/v1/orchestration", runtimeauth.Middleware(p.services.Orchestration.Auth, p.services.Orchestration.Personas))
	orchestrationruntime.RegisterRoutes(runtimeGroup, &orchestrationruntime.Handler{Service: p.services.Orchestration, Authorize: p.taskSvc.AuthorizeWorkspaceAccess})
	group := p.router.Group("/api/v1/orchestration", runtimeauth.Middleware(p.services.Orchestration.Auth, p.services.Orchestration.Personas))
	orchestration.RegisterRoutes(group, &orchestration.Handler{Registry: p.orchestrationRepo, Repo: p.orchestrationRepo, Agents: p.services.Orchestration.Personas, Authorize: p.taskSvc.AuthorizeWorkspaceAccess, RoleWrite: authn.RequireAdmin(), ValidateExecutor: func(ctx context.Context, raw string) error {
		var preference struct {
			ID string `json:"executor_profile_id"`
		}
		if err := json.Unmarshal([]byte(raw), &preference); err != nil || preference.ID == "" {
			return fmt.Errorf("select an executor profile")
		}
		_, err := p.taskSvc.GetExecutorProfile(ctx, preference.ID)
		return err
	}})
}

// Office APIs never own registered Orchestration personas or conversations.
func orchestrationCompatibilityGate(p routeParams) gin.HandlerFunc {
	return func(c *gin.Context) {
		caller := agents.CallerFromContext(c)
		if caller != nil {
			legacy, err := legacyOfficePersona(c.Request.Context(), p.officeRepo, caller.ID)
			allowed := p.features.Office && legacy
			if err != nil || !allowed {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{errKey: "feature disabled"})
				return
			}
			c.Next()
			return
		}
		allowed, err := orchestrationBrowserRouteAllowed(c.Request.Context(), p, c.Request.URL.Path)
		if err != nil || !allowed {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{errKey: "feature disabled"})
			return
		}
		c.Next()
	}
}

func orchestrationBrowserRouteAllowed(ctx context.Context, p routeParams, path string) (bool, error) {
	if !p.features.Office {
		return false, nil
	}
	parts := strings.Split(strings.TrimPrefix(path, "/api/v1/office/"), "/")
	if len(parts) < 2 {
		return true, nil
	}
	id := ""
	switch parts[0] {
	case "agents":
		id = parts[1]
	case workspaceTasksKey:
		owner, err := p.officeRepo.OrchestrationStore().ConversationUserOwner(ctx, parts[1])
		if err != nil || owner != "" {
			return false, err
		}
		fields, err := p.officeRepo.GetTaskExecutionFields(ctx, parts[1])
		if err != nil {
			return false, err
		}
		id = fields.AssigneeAgentProfileID
	default:
		return true, nil
	}
	return legacyOfficePersona(ctx, p.officeRepo, id)
}

func legacyOfficePersona(ctx context.Context, repo *officesqlite.Repository, id string) (bool, error) {
	role, err := repo.OrchestratorRoleID(ctx, id)
	if err != nil || role != "" {
		return false, err
	}
	owner, err := repo.OrchestrationStore().PersonaUserOwner(ctx, id)
	return owner == "", err
}
