package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// errKey is the JSON field used for error responses from the E2E endpoints.
const errKey = "error"

// registerE2EResetRoutes registers the E2E test-only endpoints.
// The endpoints are available when KANDEV_MOCK_AGENT is "true" or "only" (dev/E2E modes).
func registerE2EResetRoutes(
	router *gin.Engine,
	repo *sqliterepo.Repository,
	taskSvc *taskservice.Service,
	automationSvc *automation.Service,
	githubSvc *github.Service,
	log *logger.Logger,
) {
	mockMode := os.Getenv("KANDEV_MOCK_AGENT")
	if mockMode != "true" && mockMode != "only" {
		return
	}

	api := router.Group("/api/v1/e2e")
	api.DELETE("/reset/:workspaceId", handleE2EReset(repo, taskSvc, automationSvc, githubSvc, log))
	// Hidden-workflow factory: lets E2E tests cover the system-only
	// workflow path (e.g. improve-kandev) without depending on the real
	// bootstrap endpoint, which clones from GitHub and shells out to gh.
	api.POST("/hidden-workflow", handleE2ECreateHiddenWorkflow(taskSvc, log))

	log.Info("registered E2E endpoints (test-only)")
}

func handleE2EReset(
	repo *sqliterepo.Repository,
	taskSvc *taskservice.Service,
	automationSvc *automation.Service,
	githubSvc *github.Service,
	log *logger.Logger,
) gin.HandlerFunc {
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
		if _, err := repo.DB().ExecContext(ctx, `DELETE FROM runtime_flag_overrides`); err != nil {
			log.Warn("e2e reset: runtime flag override cleanup failed", zap.Error(err))
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

		// Wipe Mantis configs + issue watches (and their dedup rows) so
		// per-workspace Mantis state doesn't bleed across specs. Done before
		// task deletion so the Mantis forward poller (once wired in T05)
		// can't recreate dedup rows / tasks mid-reset — same race the
		// GitHub review watch cleanup below was hardened against.
		if err := deleteMantisStateForReset(ctx, repo.DB(), workspaceID); err != nil {
			log.Error("e2e reset: mantis cleanup failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
			return
		}

		// Wipe GitHub review watches (and their dedup rows + owned tasks) so
		// review-watch specs stay isolated. seedData/backend are worker-scoped,
		// so a watch an earlier test created stays enabled; the global review
		// poller polls every enabled watch and would create a duplicate task
		// for any PR a later test adds that the stale watch also matches.
		// Done before task deletion so the poller can't recreate tasks mid-reset.
		var deletedWatches int
		if githubSvc != nil {
			n, err := githubSvc.DeleteReviewWatchesByWorkspace(ctx, workspaceID)
			if err != nil {
				// Abort like every other cleanup below: leaving stale watches
				// behind reintroduces the cross-test poller pollution this
				// endpoint exists to prevent.
				log.Error("e2e reset: review watch cleanup failed", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
				return
			}
			deletedWatches = n
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
			c.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
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
				c.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
				return
			}
			deletedTasks++
		}

		deletedWorkflows, err := repo.DeleteWorkflowsByWorkspace(ctx, workspaceID, keepWorkflowIDs)
		if err != nil {
			log.Error("e2e reset: failed to delete workflows", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
			return
		}

		deletedAutomations, autoErr := deleteAutomationsForReset(ctx, automationSvc, workspaceID)
		if autoErr != nil {
			log.Error("e2e reset: failed to delete automations", zap.Error(autoErr))
			c.JSON(http.StatusInternalServerError, gin.H{errKey: autoErr.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"deleted_tasks":          deletedTasks,
			"deleted_workflows":      deletedWorkflows,
			"deleted_automations":    deletedAutomations,
			"deleted_review_watches": deletedWatches,
		})
	}
}

func deleteAutomationsForReset(
	ctx context.Context,
	automationSvc *automation.Service,
	workspaceID string,
) (int, error) {
	if automationSvc == nil {
		return 0, nil
	}
	return automationSvc.Store().DeleteAutomationsByWorkspace(ctx, workspaceID)
}

// deleteMantisStateForReset wipes per-workspace Mantis state in the order
// required by the foreign-key topology: dedup rows first (so the FK to
// mantis_issue_watches doesn't fire after the parent is gone), then watches,
// then the workspace config. Callers must invoke this BEFORE the task
// deletion phase so the Mantis forward poller can't recreate rows mid-reset
// — see apps/backend/CLAUDE.md "E2E reset invariant".
func deleteMantisStateForReset(ctx context.Context, db *sql.DB, workspaceID string) error {
	for _, q := range []string{
		`DELETE FROM mantis_issue_watch_tasks WHERE issue_watch_id IN (SELECT id FROM mantis_issue_watches WHERE workspace_id = ?)`,
		`DELETE FROM mantis_issue_watches WHERE workspace_id = ?`,
		`DELETE FROM mantis_configs WHERE workspace_id = ?`,
	} {
		if _, err := db.ExecContext(ctx, q, workspaceID); err != nil {
			return err
		}
	}
	return nil
}

type e2eHiddenWorkflowRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
}

func handleE2ECreateHiddenWorkflow(taskSvc *taskservice.Service, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body e2eHiddenWorkflowRequest
		if err := c.ShouldBindJSON(&body); err != nil || body.WorkspaceID == "" || body.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{errKey: "workspace_id and name are required"})
			return
		}
		workflow, err := taskSvc.CreateWorkflow(c.Request.Context(), &taskservice.CreateWorkflowRequest{
			WorkspaceID: body.WorkspaceID,
			Name:        body.Name,
			Hidden:      true,
		})
		if err != nil {
			log.Error("e2e: failed to create hidden workflow", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
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
