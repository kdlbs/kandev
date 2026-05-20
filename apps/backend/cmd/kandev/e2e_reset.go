package main

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// registerE2EResetRoutes registers the E2E test-only endpoints.
// The endpoints are available when KANDEV_MOCK_AGENT is "true" or "only" (dev/E2E modes).
func registerE2EResetRoutes(router *gin.Engine, repo *sqliterepo.Repository, taskSvc *taskservice.Service, log *logger.Logger) {
	mockMode := os.Getenv("KANDEV_MOCK_AGENT")
	if mockMode != "true" && mockMode != "only" {
		return
	}

	api := router.Group("/api/v1/e2e")
	api.DELETE("/reset/:workspaceId", handleE2EReset(repo, taskSvc, log))
	// Hidden-workflow factory: lets E2E tests cover the system-only
	// workflow path (e.g. improve-kandev) without depending on the real
	// bootstrap endpoint, which clones from GitHub and shells out to gh.
	api.POST("/hidden-workflow", handleE2ECreateHiddenWorkflow(taskSvc, log))

	log.Info("registered E2E endpoints (test-only)")
}

func handleE2EReset(repo *sqliterepo.Repository, taskSvc *taskservice.Service, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		workspaceID := c.Param("workspaceId")

		// Optional: comma-separated workflow IDs to keep (e.g., the seeded workflow).
		var keepWorkflowIDs []string
		if raw := c.Query("keep_workflows"); raw != "" {
			keepWorkflowIDs = strings.Split(raw, ",")
		}

		ctx := c.Request.Context()

		// Wipe routing state so the office-routing-* specs don't leak
		// degraded health rows / route attempts / parked runs between
		// each other. Office tables live in the same SQLite db so the
		// task repo's connection can hit them. Tables are no-ops when
		// the office routing feature isn't enabled.
		for _, q := range []string{
			`DELETE FROM office_run_route_attempts WHERE run_id IN (SELECT id FROM runs WHERE agent_profile_id IN (SELECT id FROM agent_profiles WHERE workspace_id = ?))`,
			`DELETE FROM runs WHERE agent_profile_id IN (SELECT id FROM agent_profiles WHERE workspace_id = ?)`,
			`DELETE FROM office_provider_health WHERE workspace_id = ?`,
			`DELETE FROM office_workspace_routing WHERE workspace_id = ?`,
		} {
			if _, err := repo.DB().ExecContext(ctx, q, workspaceID); err != nil {
				// Best-effort: log + continue. Some routing tables may
				// not exist when the feature is gated off.
				log.Warn("e2e reset: routing cleanup failed", zap.String("sql", q), zap.Error(err))
			}
		}

		// Reset every agent's routing override to the inherit-markers
		// shape onboarding writes. Without this, an agent-override test
		// leaves the CEO pinned to a single provider, which derails
		// subsequent workspace-level routing specs that expect the
		// resolver to walk the full provider_order.
		if _, err := repo.DB().ExecContext(ctx, `
			UPDATE agent_profiles
			SET settings = '{"routing":{"provider_order_source":"inherit","tier_source":"inherit"}}'
			WHERE workspace_id = ?
		`, workspaceID); err != nil {
			log.Warn("e2e reset: agent settings reset failed", zap.Error(err))
		}

		// Route through the task service (rather than a raw SQL DELETE) so
		// each delete spawns the async cleanup goroutine that stops the
		// agentctl instance and releases its port. Without this, instances
		// accumulate across tests in the same Playwright worker and
		// eventually exhaust the per-worker port range.
		const resetPageSize = 10000
		tasks, total, err := repo.ListTasksByWorkspace(ctx, workspaceID, "", "", "", 1, resetPageSize, true, true, false, false)
		if err != nil {
			log.Error("e2e reset: failed to list tasks", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if total > resetPageSize {
			// Fail loudly rather than silently leaving tasks behind, which
			// would leak agentctl instances and exhaust ports.
			log.Error("e2e reset: task count exceeds page size",
				zap.Int("total", total), zap.Int("page_size", resetPageSize))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "task count exceeds reset page size",
			})
			return
		}
		var deletedTasks int64
		for _, t := range tasks {
			if err := taskSvc.DeleteTask(ctx, t.ID); err != nil {
				// Abort: leaving an undeleted task with its workflow gone
				// would create orphan rows visible to subsequent tests.
				log.Error("e2e reset: failed to delete task",
					zap.String("task_id", t.ID), zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			deletedTasks++
		}

		deletedWorkflows, err := repo.DeleteWorkflowsByWorkspace(ctx, workspaceID, keepWorkflowIDs)
		if err != nil {
			log.Error("e2e reset: failed to delete workflows", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"deleted_tasks":     deletedTasks,
			"deleted_workflows": deletedWorkflows,
		})
	}
}

type e2eHiddenWorkflowRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
}

func handleE2ECreateHiddenWorkflow(taskSvc *taskservice.Service, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body e2eHiddenWorkflowRequest
		if err := c.ShouldBindJSON(&body); err != nil || body.WorkspaceID == "" || body.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id and name are required"})
			return
		}
		workflow, err := taskSvc.CreateWorkflow(c.Request.Context(), &taskservice.CreateWorkflowRequest{
			WorkspaceID: body.WorkspaceID,
			Name:        body.Name,
			Hidden:      true,
		})
		if err != nil {
			log.Error("e2e: failed to create hidden workflow", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{
			"id":           workflow.ID,
			"workspace_id": workflow.WorkspaceID,
			"name":         workflow.Name,
			"hidden":       workflow.Hidden,
		})
	}
}
