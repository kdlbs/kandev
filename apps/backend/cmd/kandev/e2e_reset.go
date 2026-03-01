package main

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// registerE2EResetRoutes registers the E2E data-reset endpoint.
// The endpoint is only available when KANDEV_MOCK_AGENT=true (i.e., during E2E tests).
func registerE2EResetRoutes(router *gin.Engine, repo *sqliterepo.Repository, log *logger.Logger) {
	if os.Getenv("KANDEV_MOCK_AGENT") != "true" {
		return
	}

	api := router.Group("/api/v1/e2e")
	api.DELETE("/reset/:workspaceId", func(c *gin.Context) {
		workspaceID := c.Param("workspaceId")

		// Optional: comma-separated workflow IDs to keep (e.g., the seeded workflow).
		var keepWorkflowIDs []string
		if raw := c.Query("keep_workflows"); raw != "" {
			keepWorkflowIDs = strings.Split(raw, ",")
		}

		ctx := c.Request.Context()

		deletedTasks, err := repo.DeleteTasksByWorkspace(ctx, workspaceID)
		if err != nil {
			log.Error("e2e reset: failed to delete tasks", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
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
	})

	log.Info("registered E2E reset endpoint (test-only)")
}
