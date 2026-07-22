package backendapp

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// errKey is the JSON field used for error responses from the E2E endpoints.
const errKey = "error"
const statusKey = "status"

// registerE2EResetRoutes registers the E2E test-only endpoints.
// The endpoints are available when KANDEV_MOCK_AGENT is "true" or "only" (dev/E2E modes).
func registerE2EResetRoutes(
	router *gin.Engine,
	repo *sqliterepo.Repository,
	taskSvc *taskservice.Service,
	automationSvc *automation.Service,
	githubSvc *github.Service,
	gitlabSvc *gitlab.Service,
	log *logger.Logger,
) {
	mockMode := os.Getenv("KANDEV_MOCK_AGENT")
	if mockMode != "true" && mockMode != "only" {
		return
	}

	api := router.Group("/api/v1/e2e")
	api.DELETE("/reset/:workspaceId", handleE2EReset(repo, taskSvc, automationSvc, githubSvc, gitlabSvc, log))
	// Hidden-workflow factory: lets E2E tests cover the system-only
	// workflow path (e.g. improve-kandev) without depending on the real
	// bootstrap endpoint, which clones from GitHub and shells out to gh.
	api.POST("/hidden-workflow", handleE2ECreateHiddenWorkflow(taskSvc, log))
	// Automation seeding helpers for E2E tests that need runs without going
	// through the WS API (works on any Node version).
	if automationSvc != nil {
		api.POST("/automations", handleE2ECreateAutomation(automationSvc, log))
		api.POST("/automation-runs", handleE2ECreateAutomationRun(automationSvc, log))
	}
	// Seeds a task_sessions row directly (e.g. a CANCELLED primary session)
	// so tests can assert on session-state-derived behavior without a full
	// agent stop/resume cycle.
	api.POST("/task-sessions", handleE2ECreateTaskSession(repo, log))

	log.Info("registered E2E endpoints (test-only)")
}

func handleE2EReset(
	repo *sqliterepo.Repository,
	taskSvc *taskservice.Service,
	automationSvc *automation.Service,
	githubSvc *github.Service,
	gitlabSvc *gitlab.Service,
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
		if _, err := repo.DB().ExecContext(ctx, `DELETE FROM github_workspace_settings WHERE workspace_id = ?`, workspaceID); err != nil {
			log.Warn("e2e reset: GitHub workspace settings cleanup failed", zap.Error(err))
		}
		// The workflow-sync poller reads these rows globally; delete before
		// task/workflow deletion so a mid-reset tick can't resync workflows.
		// The table always exists, so a failure is a genuine DB error — abort
		// rather than let the poller recreate workflows mid-reset.
		if _, err := repo.DB().ExecContext(ctx, `DELETE FROM workflow_sync_configs WHERE workspace_id = ?`, workspaceID); err != nil {
			log.Error("e2e reset: workflow sync config cleanup failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{errKey: "workflow sync config cleanup failed"})
			return
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

		var gitLabReset gitlab.E2EResetResult
		if gitlabSvc != nil {
			resetResult, resetErr := gitlabSvc.ResetWorkspaceE2E(ctx, workspaceID)
			if resetErr != nil {
				log.Error("e2e reset: GitLab workspace cleanup failed", zap.Error(resetErr))
				c.JSON(http.StatusInternalServerError, gin.H{errKey: resetErr.Error()})
				return
			}
			gitLabReset = resetResult
		}

		// Route through the task service (rather than a raw SQL DELETE) so
		// each delete spawns the async cleanup goroutine that stops the
		// agentctl instance and releases its port. Without this, instances
		// accumulate across tests in the same Playwright worker and
		// eventually exhaust the per-worker port range.
		const resetPageSize = 10000
		tasks, total, err := repo.ListTasksByWorkspace(ctx, workspaceID, "", "", "", 1, resetPageSize, "", true, true, false, false)
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
			"deleted_tasks":                 deletedTasks,
			"deleted_workflows":             deletedWorkflows,
			"deleted_automations":           deletedAutomations,
			"deleted_review_watches":        deletedWatches,
			"deleted_gitlab_review_watches": gitLabReset.ReviewWatches,
			"deleted_gitlab_issue_watches":  gitLabReset.IssueWatches,
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

type e2eCreateAutomationRequest struct {
	WorkspaceID    string `json:"workspace_id"`
	Name           string `json:"name"`
	WorkflowID     string `json:"workflow_id"`
	WorkflowStepID string `json:"workflow_step_id"`
}

// handleE2ECreateAutomation seeds an automation for E2E tests via HTTP so tests
// don't require Node 24 (WS API requires a global WebSocket that isn't in Node 20).
func handleE2ECreateAutomation(svc *automation.Service, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body e2eCreateAutomationRequest
		if err := c.ShouldBindJSON(&body); err != nil || body.WorkspaceID == "" || body.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{errKey: "workspace_id and name are required"})
			return
		}
		a, err := svc.CreateAutomation(c.Request.Context(), &automation.CreateAutomationRequest{
			WorkspaceID:       body.WorkspaceID,
			Name:              body.Name,
			WorkflowID:        body.WorkflowID,
			WorkflowStepID:    body.WorkflowStepID,
			MaxConcurrentRuns: 10,
		})
		if err != nil {
			log.Error("e2e: failed to create automation", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"id": a.ID, "workspace_id": a.WorkspaceID, "name": a.Name})
	}
}

type e2eCreateAutomationRunRequest struct {
	AutomationID string `json:"automation_id"`
	Status       string `json:"status"`
	// TaskID optionally links the seeded run to a real task so E2E tests can
	// exercise task-lifecycle interactions (e.g. archiving the task and
	// asserting the run's displayed status/concurrency accounting reacts).
	TaskID string `json:"task_id"`
}

// handleE2ECreateAutomationRun seeds an automation run row for E2E tests.
func handleE2ECreateAutomationRun(svc *automation.Service, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body e2eCreateAutomationRunRequest
		if err := c.ShouldBindJSON(&body); err != nil || body.AutomationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{errKey: "automation_id is required"})
			return
		}
		status := automation.RunStatus(body.Status)
		if status == "" {
			status = automation.RunStatusSkipped
		}
		run := &automation.AutomationRun{
			AutomationID: body.AutomationID,
			TriggerType:  automation.TriggerTypeScheduled,
			Status:       status,
			TaskID:       body.TaskID,
			TriggerData:  []byte(`{}`),
		}
		if err := svc.RecordRun(c.Request.Context(), run); err != nil {
			log.Error("e2e: failed to create automation run", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{
			"id": run.ID, "automation_id": run.AutomationID, statusKey: run.Status, taskIDPayloadKey: run.TaskID,
		})
	}
}

type e2eCreateTaskSessionRequest struct {
	TaskID string `json:"task_id"`
	State  string `json:"state"`
	// IsPrimary defaults to true: tests seeding a single session almost
	// always want it to be the task's *current* session (the one
	// listRunsWithTaskState/countActiveRunsWithTaskState read). Set to
	// false to seed a stale, superseded session for resume scenarios.
	IsPrimary *bool `json:"is_primary"`
}

// handleE2ECreateTaskSession seeds a task_sessions row directly for E2E
// tests that need to assert on session-state-derived behavior (e.g. the
// automation Recent Runs "Cancelled" status, which is keyed off the task's
// primary session state, not the task's own state — see
// internal/automation.Store.listRunsWithTaskState) without driving a real
// agent through a full stop/resume cycle.
func handleE2ECreateTaskSession(repo *sqliterepo.Repository, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body e2eCreateTaskSessionRequest
		if err := c.ShouldBindJSON(&body); err != nil || body.TaskID == "" || body.State == "" {
			c.JSON(http.StatusBadRequest, gin.H{errKey: "task_id and state are required"})
			return
		}
		isPrimary := true
		if body.IsPrimary != nil {
			isPrimary = *body.IsPrimary
		}
		session := &taskmodels.TaskSession{
			TaskID:    body.TaskID,
			State:     taskmodels.TaskSessionState(body.State),
			IsPrimary: isPrimary,
		}
		if err := repo.CreateTaskSession(c.Request.Context(), session); err != nil {
			log.Error("e2e: failed to create task session", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{
			"id": session.ID, taskIDPayloadKey: session.TaskID, statusKey: session.State,
		})
	}
}
