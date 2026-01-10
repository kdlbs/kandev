package api

import (
	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
)

// SetupRoutes configures the agent manager API routes
// router should be the /api/v1 group
func SetupRoutes(
	router *gin.RouterGroup,
	lm *lifecycle.Manager,
	reg *registry.Registry,
	docker DockerClient,
	acp ACPManager,
	log *logger.Logger,
) {
	handler := NewHandler(lm, reg, docker, acp, log)

	// Agent instance endpoints under /api/v1/agents
	agents := router.Group("/agents")
	{
		// List all active agents
		agents.GET("", handler.ListAgents)

		// Launch a new agent
		agents.POST("/launch", handler.LaunchAgent)

		// Agent types endpoints
		agents.GET("/types", handler.ListAgentTypes)
		agents.GET("/types/:typeId", handler.GetAgentType)

		// Single agent instance operations
		agents.GET("/:instanceId/status", handler.GetAgentStatus)
		agents.GET("/:instanceId/logs", handler.GetAgentLogs)
		agents.DELETE("/:instanceId", handler.StopAgent)

		// ACP communication endpoints
		agents.POST("/:instanceId/prompt", handler.SendPrompt)
		agents.POST("/:instanceId/cancel", handler.CancelAgent)
		agents.GET("/:instanceId/session", handler.GetSession)
	}
}

