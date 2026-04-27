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
	api.Use(agentAuthMiddleware(h.ctrl.Svc))

	api.GET("/meta", h.getMeta)

	registerOnboardingRoutes(api, h)
	registerAgentRoutes(api, h)
	registerInstructionRoutes(api, h)
	registerSkillRoutes(api, h)
	registerProjectRoutes(api, h)
	registerCostRoutes(api, h)
	registerBudgetRoutes(api, h)
	registerRoutineRoutes(api, h)
	registerApprovalRoutes(api, h)
	registerActivityRoutes(api, h)
	registerInboxRoutes(api, h)
	registerMemoryRoutes(api, h)
	registerChannelRoutes(api, h)
	registerConfigRoutes(api, h)
	registerDashboardRoutes(api, h)
	registerWakeupRoutes(api, h)
	registerIssueRoutes(api, h)
	registerGitRoutes(api, h)
}

func registerIssueRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/workspaces/:wsId/tasks/search", h.searchTasks)
}

func registerAgentRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/workspaces/:wsId/agents", h.listAgents)
	api.POST("/workspaces/:wsId/agents", h.createAgent)
	api.GET("/agents/:id", h.getAgent)
	api.PATCH("/agents/:id", h.updateAgent)
	api.PATCH("/agents/:id/status", h.updateAgentStatus)
	api.DELETE("/agents/:id", h.deleteAgent)
}

func registerInstructionRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/agents/:id/instructions", h.listInstructions)
	api.GET("/agents/:id/instructions/:filename", h.getInstruction)
	api.PUT("/agents/:id/instructions/:filename", h.upsertInstruction)
	api.DELETE("/agents/:id/instructions/:filename", h.deleteInstruction)
}

func registerSkillRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/workspaces/:wsId/skills", h.listSkills)
	api.POST("/workspaces/:wsId/skills", h.createSkill)
	api.POST("/workspaces/:wsId/skills/import", h.importSkill)
	api.GET("/skills/:id", h.getSkill)
	api.GET("/skills/:id/files", h.getSkillFile)
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
	api.GET("/workspaces/:wsId/routine-runs", h.listAllRuns)
	api.GET("/routines/:id", h.getRoutine)
	api.PATCH("/routines/:id", h.updateRoutine)
	api.DELETE("/routines/:id", h.deleteRoutine)
	api.POST("/routines/:id/run", h.runRoutine)
	api.GET("/routines/:id/triggers", h.listTriggers)
	api.POST("/routines/:id/triggers", h.createTrigger)
	api.DELETE("/routine-triggers/:triggerId", h.deleteTrigger)
	api.GET("/routines/:id/runs", h.listRuns)
	api.POST("/routine-triggers/:publicId/fire", h.fireWebhookTrigger)
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
	api.DELETE("/agents/:id/memory/all", h.deleteAllMemory)
	api.DELETE("/agents/:id/memory/:entryId", h.deleteMemory)
	api.GET("/agents/:id/memory/summary", h.memorySummary)
	api.GET("/agents/:id/memory/export", h.exportMemory)
}

func registerChannelRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/agents/:id/channels", h.listChannels)
	api.POST("/agents/:id/channels", h.setupChannel)
	api.DELETE("/agents/:id/channels/:channelId", h.deleteChannel)
	api.POST("/channels/:channelId/inbound", h.channelInbound)
}

func registerConfigRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/workspaces/:wsId/config/export", h.exportConfig)
	api.GET("/workspaces/:wsId/config/export/zip", h.exportConfigZip)
	api.POST("/workspaces/:wsId/config/preview", h.previewImport)
	api.POST("/workspaces/:wsId/config/import", h.applyImport)
	api.GET("/workspaces/:wsId/config/sync/incoming", h.syncIncomingDiff)
	api.GET("/workspaces/:wsId/config/sync/outgoing", h.syncOutgoingDiff)
	api.POST("/workspaces/:wsId/config/sync/import-fs", h.syncApplyIncoming)
	api.POST("/workspaces/:wsId/config/sync/export-fs", h.syncApplyOutgoing)
}

func registerDashboardRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/workspaces/:wsId/dashboard", h.getDashboard)
}

func registerWakeupRoutes(api *gin.RouterGroup, h *Handlers) {
	api.GET("/workspaces/:wsId/wakeups", h.listWakeups)
}

func registerGitRoutes(api *gin.RouterGroup, h *Handlers) {
	api.POST("/workspaces/:wsId/git/clone", h.gitClone)
	api.POST("/workspaces/:wsId/git/pull", h.gitPull)
	api.POST("/workspaces/:wsId/git/push", h.gitPush)
	api.GET("/workspaces/:wsId/git/status", h.gitStatus)
}
