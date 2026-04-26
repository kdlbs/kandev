// Package handlers provides HTTP route registration for the orchestrate API.
package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrate/controller"

	"go.uber.org/zap"
)

// Handlers provides HTTP handlers for orchestrate routes.
type Handlers struct {
	ctrl   *controller.Controller
	logger *logger.Logger
}

// RegisterRoutes registers all orchestrate HTTP routes on the given router.
func RegisterRoutes(router *gin.Engine, ctrl *controller.Controller, log *logger.Logger) {
	h := &Handlers{ctrl: ctrl, logger: log.WithFields(zap.String("component", "orchestrate-handlers"))}
	api := router.Group("/api/v1/orchestrate")

	registerAgentRoutes(api, h)
	registerSkillRoutes(api, h)
	registerProjectRoutes(api, h)
	registerCostRoutes(api, h)
	registerBudgetRoutes(api, h)
	registerRoutineRoutes(api, h)
	registerApprovalRoutes(api, h)
	registerActivityRoutes(api, h)
	registerInboxRoutes(api, h)
	registerMemoryRoutes(api, h)
	registerDashboardRoutes(api, h)
	registerWakeupRoutes(api, h)
}

func registerAgentRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/workspaces/:wsId/agents", h.listAgents)
	api.POST("/workspaces/:wsId/agents", h.createAgent)
	api.GET("/agents/:id", h.getAgent)
	api.PATCH("/agents/:id", h.updateAgent)
	api.PATCH("/agents/:id/status", h.updateAgentStatus)
	api.DELETE("/agents/:id", h.deleteAgent)
}

func registerSkillRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/workspaces/:wsId/skills", h.listSkills)
	api.POST("/workspaces/:wsId/skills", h.createSkill)
	api.GET("/skills/:id", h.getSkill)
	api.PATCH("/skills/:id", h.updateSkill)
	api.DELETE("/skills/:id", h.deleteSkill)
}

func registerProjectRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/workspaces/:wsId/projects", h.listProjects)
	api.POST("/workspaces/:wsId/projects", h.createProject)
	api.GET("/projects/:id", h.getProject)
	api.PATCH("/projects/:id", h.updateProject)
	api.DELETE("/projects/:id", h.deleteProject)
}

func registerCostRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/workspaces/:wsId/costs", h.listCosts)
	api.GET("/workspaces/:wsId/costs/summary", h.costSummary)
	api.GET("/workspaces/:wsId/costs/by-agent", h.costsByAgent)
	api.GET("/workspaces/:wsId/costs/by-project", h.costsByProject)
	api.GET("/workspaces/:wsId/costs/by-model", h.costsByModel)
}

func registerBudgetRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/workspaces/:wsId/budgets", h.listBudgets)
	api.POST("/workspaces/:wsId/budgets", h.createBudget)
	api.PATCH("/budgets/:id", h.updateBudget)
	api.DELETE("/budgets/:id", h.deleteBudget)
}

func registerRoutineRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/workspaces/:wsId/routines", h.listRoutines)
	api.POST("/workspaces/:wsId/routines", h.createRoutine)
	api.GET("/routines/:id", h.getRoutine)
	api.PATCH("/routines/:id", h.updateRoutine)
	api.DELETE("/routines/:id", h.deleteRoutine)
	api.POST("/routines/:id/run", h.runRoutine)
}

func registerApprovalRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/workspaces/:wsId/approvals", h.listApprovals)
	api.POST("/approvals/:id/decide", h.decideApproval)
}

func registerActivityRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/workspaces/:wsId/activity", h.listActivity)
}

func registerInboxRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/workspaces/:wsId/inbox", h.getInbox)
}

func registerMemoryRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/agents/:id/memory", h.listMemory)
	api.PUT("/agents/:id/memory", h.upsertMemory)
	api.DELETE("/agents/:id/memory/:entryId", h.deleteMemory)
	api.GET("/agents/:id/memory/summary", h.memorySummary)
}

func registerDashboardRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/workspaces/:wsId/dashboard", h.getDashboard)
}

func registerWakeupRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/workspaces/:wsId/wakeups", h.listWakeups)
}
