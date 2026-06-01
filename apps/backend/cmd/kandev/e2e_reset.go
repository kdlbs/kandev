package main

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/common/logger"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// errorField is the JSON key for error responses across every E2E endpoint.
// Extracted to satisfy goconst; the value is part of the public contract with
// the FE test harness so don't rename without updating the consumers.
const errorField = "error"

// errUnknownConnection is the 404 message body for connection-id lookups
// against the WS gateway's ring buffer. Extracted to satisfy goconst.
const errUnknownConnection = "unknown connection_id"

// registerE2EResetRoutes registers the E2E test-only endpoints.
// The endpoints are available when KANDEV_MOCK_AGENT is "true" or "only" (dev/E2E modes).
func registerE2EResetRoutes(
	router *gin.Engine,
	repo *sqliterepo.Repository,
	taskSvc *taskservice.Service,
	automationSvc *automation.Service,
	hub *gateways.Hub,
	log *logger.Logger,
) {
	mockMode := os.Getenv("KANDEV_MOCK_AGENT")
	if mockMode != "true" && mockMode != "only" {
		return
	}

	api := router.Group("/api/v1/e2e")
	api.DELETE("/reset/:workspaceId", handleE2EReset(repo, taskSvc, automationSvc, log))
	// Hidden-workflow factory: lets E2E tests cover the system-only
	// workflow path (e.g. improve-kandev) without depending on the real
	// bootstrap endpoint, which clones from GitHub and shells out to gh.
	api.POST("/hidden-workflow", handleE2ECreateHiddenWorkflow(taskSvc, log))
	// WS send-log inspector: lets E2E tests diff what the BE sent vs what
	// the FE received per WS connection. Gaps in the FE-side seq sequence
	// against this server-of-record indicate a real WS regression rather
	// than a noisy UI test.
	api.GET("/ws-sent", handleE2EWsSent(hub))

	log.Info("registered E2E endpoints (test-only)")
}

// e2eWsSentResponse matches the contract the FE-side accountant consumes.
// Keep field names stable — changes here require a matching FE update.
type e2eWsSentResponse struct {
	ConnectionID string                 `json:"connection_id"`
	Events       []gateways.WsSentEvent `json:"events"`
	MaxSeq       int64                  `json:"max_seq"`
}

func handleE2EWsSent(hub *gateways.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		connectionID := c.Query("connection_id")
		if connectionID == "" {
			c.JSON(http.StatusBadRequest, gin.H{errorField: "connection_id is required"})
			return
		}
		var sinceSeq int64
		if raw := c.Query("since_seq"); raw != "" {
			parsed, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{errorField: "since_seq must be an integer"})
				return
			}
			sinceSeq = parsed
		}

		// Optional per-session filter (Workstream 1): when set, returns only
		// entries whose stamped SessionID matches, sorted by SessionSeq
		// ascending. MaxSeq in the response then carries the max SessionSeq
		// for the filter (the per-session counter, not per-connection). The
		// FE per-session diff uses this to catch cross-session misrouting
		// that per-connection seq alone cannot detect.
		if sessionID := c.Query("session_id"); sessionID != "" {
			events, maxSessionSeq, ok := hub.GetSentEventsForSession(connectionID, sessionID)
			if !ok {
				c.JSON(http.StatusNotFound, gin.H{errorField: errUnknownConnection})
				return
			}
			if events == nil {
				events = []gateways.WsSentEvent{}
			}
			c.JSON(http.StatusOK, e2eWsSentResponse{
				ConnectionID: connectionID,
				Events:       events,
				MaxSeq:       maxSessionSeq,
			})
			return
		}

		events, maxSeq, ok := hub.GetSentEventsFor(connectionID, sinceSeq)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{errorField: errUnknownConnection})
			return
		}
		if events == nil {
			events = []gateways.WsSentEvent{}
		}
		c.JSON(http.StatusOK, e2eWsSentResponse{
			ConnectionID: connectionID,
			Events:       events,
			MaxSeq:       maxSeq,
		})
	}
}

func handleE2EReset(
	repo *sqliterepo.Repository,
	taskSvc *taskservice.Service,
	automationSvc *automation.Service,
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
			c.JSON(http.StatusInternalServerError, gin.H{errorField: err.Error()})
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
				c.JSON(http.StatusInternalServerError, gin.H{errorField: err.Error()})
				return
			}
			deletedTasks++
		}

		deletedWorkflows, err := repo.DeleteWorkflowsByWorkspace(ctx, workspaceID, keepWorkflowIDs)
		if err != nil {
			log.Error("e2e reset: failed to delete workflows", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{errorField: err.Error()})
			return
		}

		deletedAutomations, autoErr := deleteAutomationsForReset(ctx, automationSvc, workspaceID)
		if autoErr != nil {
			log.Error("e2e reset: failed to delete automations", zap.Error(autoErr))
			c.JSON(http.StatusInternalServerError, gin.H{errorField: autoErr.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"deleted_tasks":       deletedTasks,
			"deleted_workflows":   deletedWorkflows,
			"deleted_automations": deletedAutomations,
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

type e2eHiddenWorkflowRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
}

func handleE2ECreateHiddenWorkflow(taskSvc *taskservice.Service, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body e2eHiddenWorkflowRequest
		if err := c.ShouldBindJSON(&body); err != nil || body.WorkspaceID == "" || body.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{errorField: "workspace_id and name are required"})
			return
		}
		workflow, err := taskSvc.CreateWorkflow(c.Request.Context(), &taskservice.CreateWorkflowRequest{
			WorkspaceID: body.WorkspaceID,
			Name:        body.Name,
			Hidden:      true,
		})
		if err != nil {
			log.Error("e2e: failed to create hidden workflow", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{errorField: err.Error()})
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
