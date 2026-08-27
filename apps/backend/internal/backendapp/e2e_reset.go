package backendapp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

const (
	// errKey is the JSON field used for error responses from the E2E endpoints.
	errKey            = "error"
	statusKey         = "status"
	e2eResetSourceKey = "source"
	e2eResetTypeKey   = "type"
)

// registerE2EResetRoutes registers the E2E test-only endpoints.
// The endpoints are available when KANDEV_MOCK_AGENT is "true" or "only" (dev/E2E modes).
func registerE2EResetRoutes(
	router *gin.Engine,
	repo *sqliterepo.Repository,
	taskSvc *taskservice.Service,
	automationSvc *automation.Service,
	githubSvc *github.Service,
	gitlabSvc *gitlab.Service,
	eventBus bus.EventBus,
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
		api.POST("/automations", handleE2ECreateAutomation(automationSvc, repo, log))
		api.POST("/automation-runs", handleE2ECreateAutomationRun(automationSvc, repo, log))
		api.POST("/automation-triggers", handleE2EAddTrigger(automationSvc, log))
		// Fire a fake github_pr_merged event into the in-process event bus so
		// E2E tests can exercise the automation subscriber without real GitHub
		// polling. Returns the run task id once the run record appears.
		api.POST("/github/fire-pr-merged", handleE2EFirePRMerged(automationSvc, eventBus, log))
		// Fire a manual automation trigger, mirroring the "Run" button path,
		// and return the run task id once the run record appears.
		api.POST("/automations/:id/trigger", handleE2EAutomationManualTrigger(automationSvc, log))
	}
	// Seeds a task_sessions row directly (e.g. a CANCELLED primary session)
	// so tests can assert on session-state-derived behavior without a full
	// agent stop/resume cycle.
	api.POST("/task-sessions", handleE2ECreateTaskSession(repo, log))
	// Force-sets a session's read cursor, bypassing the production mark-read
	// endpoint's monotonic (forward-only) guard, so specs can deterministically
	// seed "the user last read up through an earlier message" without a real
	// background agent turn.
	api.PATCH("/task-sessions/:id/read-cursor", handleE2ESetSessionReadCursor(repo, log))
	// Stamps a task's origin. Production sets origin=automation_run inside the
	// automation firing path, so a task seeded through the ordinary task API is
	// an ordinary task — it shows on the kanban and in task lists, which is the
	// opposite of what an automation run does. Specs that assert on (or
	// photograph) that hiding need the seeded state to match production.
	api.PATCH("/tasks/:id/origin", handleE2ESetTaskOrigin(repo, log))

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
		// Repository sets outlive the tasks a reset removes, so a set seeded by
		// one spec would still be offered in the next spec's create dialog. The
		// items cascade from the set row.
		for _, q := range []string{
			`DELETE FROM repository_set_items WHERE repository_set_id IN (SELECT id FROM repository_sets WHERE workspace_id = ?)`,
			`DELETE FROM repository_sets WHERE workspace_id = ?`,
		} {
			if _, err := repo.DB().ExecContext(ctx, q, workspaceID); err != nil {
				log.Warn("e2e reset: repository set cleanup failed", zap.String("sql", q), zap.Error(err))
			}
		}
		if _, err := repo.DB().ExecContext(ctx, `DELETE FROM github_workspace_settings WHERE workspace_id = ?`, workspaceID); err != nil {
			log.Warn("e2e reset: GitHub workspace settings cleanup failed", zap.Error(err))
		}
		if err := deleteGitHubAuthForReset(ctx, repo.DB(), workspaceID); err != nil {
			log.Error("e2e reset: GitHub authentication cleanup failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{errKey: "GitHub authentication cleanup failed"})
			return
		}
		if githubSvc != nil {
			githubSvc.ResetMockAuth(workspaceID)
			if err := resetGitHubAppRegistrationsForE2E(ctx, githubSvc, workspaceID); err != nil {
				log.Error("e2e reset: GitHub deployment App cleanup failed", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{errKey: "GitHub deployment App cleanup failed"})
				return
			}
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

		// Clear native code-review runs and findings before deleting the tasks
		// that own them. A review pass left in flight by an earlier spec would
		// otherwise publish findings mid-reset and leak them into a later test.
		if err := repo.DeleteTaskReviewByWorkspace(ctx, workspaceID); err != nil {
			log.Error("e2e reset: task review cleanup failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
			return
		}

		// Automation deletion must run while its bound tasks still exist. It
		// stops live turns before removing run rows and owns cleanup of hidden
		// automation tasks. Deleting tasks first can strand an open run on a
		// missing task and make the next test's reset fail.
		deletedAutomations, autoErr := deleteAutomationsForReset(ctx, automationSvc, workspaceID)
		if autoErr != nil {
			log.Error("e2e reset: failed to delete automations", zap.Error(autoErr))
			c.JSON(http.StatusInternalServerError, gin.H{errKey: autoErr.Error()})
			return
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

func resetGitHubAppRegistrationsForE2E(
	ctx context.Context,
	service *github.Service,
	workspaceID string,
) error {
	if service == nil {
		return nil
	}
	return service.ResetAppRegistrationsForE2E(ctx, workspaceID)
}

func deleteGitHubAuthForReset(ctx context.Context, database *sql.DB, workspaceID string) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, query := range []string{
		`DELETE FROM github_auth_flows WHERE workspace_id = ?`,
		`DELETE FROM github_user_connections WHERE workspace_id = ?`,
		`DELETE FROM github_workspace_connections WHERE workspace_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, query, workspaceID); err != nil {
			return err
		}
	}
	userSecretPrefix := "github:user:" + workspaceID + ":"
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM secrets
		WHERE id = ? OR substr(id, 1, length(?)) = ?`,
		github.WorkspacePATSecretKey(workspaceID), userSecretPrefix, userSecretPrefix); err != nil {
		return err
	}
	return tx.Commit()
}

func deleteAutomationsForReset(
	ctx context.Context,
	automationSvc *automation.Service,
	workspaceID string,
) (int, error) {
	if automationSvc == nil {
		return 0, nil
	}
	return automationSvc.DeleteAutomationsByWorkspace(ctx, workspaceID)
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
	WorkspaceID    string                            `json:"workspace_id"`
	Name           string                            `json:"name"`
	WorkflowID     string                            `json:"workflow_id"`
	WorkflowStepID string                            `json:"workflow_step_id"`
	TaskMode       automation.TaskMode               `json:"task_mode"`
	RepositoryMode automation.RepositoryMode         `json:"repository_mode"`
	RepositoryIDs  []string                          `json:"repository_ids"`
	Repositories   []automation.AutomationRepository `json:"repositories"`
	// Prompt is the automation's standing instruction. Optional, but the run
	// view only renders the instruction card when there is one, so a spec
	// asserting on where that card lives has to seed it.
	Prompt string `json:"prompt"`
	// AgentProfileID and ExecutorProfileID mirror the fields a UI-created
	// automation carries. Without them autoStartAutomationTask calls StartTask
	// with empty profile IDs and the lifecycle layer fails to resolve the agent,
	// so tests that need the automation to actually launch an agent must supply
	// these (typically seedData.agentProfileId / seedData.worktreeExecutorProfileId).
	AgentProfileID    string `json:"agent_profile_id"`
	ExecutorProfileID string `json:"executor_profile_id"`
	// LegacyBoardCard rewrites the row's execution_mode to 'task' after
	// creation, reproducing on disk exactly what an install that predates the
	// withdrawal of execution modes carries. There is no input path to this:
	// CreateAutomation deliberately writes the empty string so a row created
	// today can never be mistaken for a pre-upgrade one (see
	// automation.Store.CreateAutomation), and the field is ignored on the
	// wire. The migration notice is derived from this column by production
	// SQL (`execution_mode = 'task' AS legacy_board_card`), so seeding the
	// column is the only honest way to put a workspace in the state the
	// notice exists to explain.
	LegacyBoardCard bool `json:"legacy_board_card"`
}

// handleE2ECreateAutomation seeds an automation for E2E tests via HTTP so tests
// don't require Node 24 (WS API requires a global WebSocket that isn't in Node 20).
func handleE2ECreateAutomation(
	svc *automation.Service,
	repo *sqliterepo.Repository,
	log *logger.Logger,
) gin.HandlerFunc {
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
			TaskMode:          body.TaskMode,
			RepositoryMode:    body.RepositoryMode,
			RepositoryIDs:     body.RepositoryIDs,
			Repositories:      body.Repositories,
			Prompt:            body.Prompt,
			AgentProfileID:    body.AgentProfileID,
			ExecutorProfileID: body.ExecutorProfileID,
			MaxConcurrentRuns: 10,
		})
		if err != nil {
			log.Error("e2e: failed to create automation", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
			return
		}
		if body.LegacyBoardCard {
			// Automations live in the same SQLite database the task repository
			// holds open, so no second handle is needed.
			if _, err := repo.DB().ExecContext(c.Request.Context(),
				`UPDATE automations SET execution_mode = 'task' WHERE id = ?`, a.ID); err != nil {
				log.Error("e2e: failed to backdate automation execution_mode", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
				return
			}
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
	// When a task is supplied, the endpoint derives the current session and
	// open turn so E2E tests can exercise exact-run actions such as stop.
	SessionID string `json:"session_id,omitempty"`
	TurnID    string `json:"turn_id,omitempty"`
}

// handleE2ECreateAutomationRun seeds an automation run row for E2E tests.
func handleE2ECreateAutomationRun(svc *automation.Service, repo *sqliterepo.Repository, log *logger.Logger) gin.HandlerFunc {
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
			SessionID:    body.SessionID,
			TurnID:       body.TurnID,
			TriggerData:  []byte(`{}`),
		}
		if run.TaskID != "" && (run.SessionID == "" || run.TurnID == "") {
			if session, sessionErr := repo.GetActiveTaskSessionByTaskID(c.Request.Context(), run.TaskID); sessionErr == nil && session != nil {
				run.SessionID = session.ID
				if turn, turnErr := repo.GetActiveTurnBySessionID(c.Request.Context(), session.ID); turnErr == nil && turn != nil {
					run.TurnID = turn.ID
				}
			}
		}
		if err := svc.RecordRun(c.Request.Context(), run); err != nil {
			log.Error("e2e: failed to create automation run", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{
			"id": run.ID, "automation_id": run.AutomationID, statusKey: run.Status, taskIDPayloadKey: run.TaskID,
			"session_id": run.SessionID, "turn_id": run.TurnID,
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
			TaskID: body.TaskID,
			State:  taskmodels.TaskSessionState(body.State),
		}
		if err := repo.CreateTaskSession(c.Request.Context(), session); err != nil {
			log.Error("e2e: failed to create task session", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
			return
		}
		// SetSessionPrimary clears is_primary on every other session for this
		// task before setting it here, mirroring production's resume path
		// (Repository.SetSessionPrimary) so seeding a second primary session
		// for a task that already has one can't leave two is_primary = 1 rows
		// behind — that would corrupt the run-status query's read of the
		// task's "current" session (see listRunsWithTaskState).
		if isPrimary {
			if err := repo.SetSessionPrimary(c.Request.Context(), session.ID); err != nil {
				log.Error("e2e: failed to set primary task session", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
				return
			}
			session.IsPrimary = true
		}
		c.JSON(http.StatusCreated, gin.H{
			"id": session.ID, taskIDPayloadKey: session.TaskID, statusKey: session.State,
		})
	}
}

type e2eSetSessionReadCursorRequest struct {
	MessageID string `json:"message_id"`
}

// handleE2ESetSessionReadCursor force-sets a session's last_read_message_id
// via a raw update, bypassing UpdateTaskSessionLastReadMessageID's monotonic
// guard (production only ever advances the cursor forward — see
// repository/sqlite/session.go). E2E specs use this to seed "the user last
// read up through an earlier message" deterministically, since replaying an
// older messageId through the real mark-read endpoint is now a rejected
// no-op rather than a rewind.
type e2eSetTaskOriginRequest struct {
	Origin string `json:"origin"`
}

// e2eForceColumn is the shape both force-set seeders share: bind one field,
// write one column by id, and translate "no rows" into a 404 so a spec that
// seeds against a typo fails where the mistake is rather than three assertions
// later. Extracted because the two handlers were byte-for-byte alike apart
// from the table, and a copied handler is a handler that drifts.
func e2eForceColumn(
	c *gin.Context,
	repo *sqliterepo.Repository,
	log *logger.Logger,
	opts e2eForceColumnOpts,
) {
	result, err := repo.DB().ExecContext(c.Request.Context(), opts.query, opts.value, time.Now().UTC(), opts.id)
	if err != nil {
		log.Error("e2e: "+opts.failLog, zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{errKey: opts.subject + " not found: " + opts.id})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": opts.id, opts.responseKey: opts.value})
}

type e2eForceColumnOpts struct {
	query       string
	id          string
	value       any
	subject     string
	responseKey string
	failLog     string
}

func handleE2ESetTaskOrigin(repo *sqliterepo.Repository, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body e2eSetTaskOriginRequest
		if err := c.ShouldBindJSON(&body); err != nil || body.Origin == "" {
			c.JSON(http.StatusBadRequest, gin.H{errKey: "origin is required"})
			return
		}
		e2eForceColumn(c, repo, log, e2eForceColumnOpts{
			query:       `UPDATE tasks SET origin = ?, updated_at = ? WHERE id = ?`,
			id:          c.Param("id"),
			value:       body.Origin,
			subject:     "task",
			responseKey: "origin",
			failLog:     "failed to set task origin",
		})
	}
}

type e2eFirePRMergedRequest struct {
	TaskID       string `json:"task_id"`
	AutomationID string `json:"automation_id"`
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	PRNumber     int    `json:"pr_number"`
	BaseBranch   string `json:"base_branch"`
}

// handleE2EFirePRMerged publishes a fake events.GitHubTaskPRUpdated event with
// State="merged" directly into the in-process event bus. The subscriber picks it
// up synchronously (memory bus), triggers the matching automations, and spawns a
// goroutine to create the run task. The handler then polls until the run record
// appears in the DB and returns its task id.
//
// This endpoint exists only in E2E / mock-agent mode. It does NOT call GitHub or
// add webhook subscriptions — it fires the internal event bus only.
func handleE2EFirePRMerged(svc *automation.Service, eventBus bus.EventBus, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body e2eFirePRMergedRequest
		if err := c.ShouldBindJSON(&body); err != nil ||
			body.TaskID == "" || body.AutomationID == "" || body.Owner == "" || body.Repo == "" {
			c.JSON(http.StatusBadRequest, gin.H{errKey: "task_id, automation_id, owner, and repo are required"})
			return
		}
		if body.PRNumber <= 0 {
			body.PRNumber = 1
		}

		ctx := c.Request.Context()

		// Snapshot the latest run id before firing so we can detect the new one.
		beforeID := ""
		existing, _ := svc.ListRuns(ctx, body.AutomationID, 1)
		if len(existing) > 0 {
			beforeID = existing[0].ID
		}

		// Build a fake TaskPR and publish the event.
		now := time.Now().UTC()
		prURL := fmt.Sprintf("https://github.com/%s/%s/pull/%d", body.Owner, body.Repo, body.PRNumber)
		pr := &github.TaskPR{
			TaskID:     body.TaskID,
			Owner:      body.Owner,
			Repo:       body.Repo,
			PRNumber:   body.PRNumber,
			PRURL:      prURL,
			BaseBranch: body.BaseBranch,
			State:      "merged",
			MergedAt:   &now,
		}
		evt := bus.NewEvent(events.GitHubTaskPRUpdated, "e2e", pr)
		if err := eventBus.Publish(ctx, events.GitHubTaskPRUpdated, evt); err != nil {
			log.Error("e2e: failed to publish GitHubTaskPRUpdated event", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
			return
		}

		taskID, err := e2ePollNewRun(ctx, svc, body.AutomationID, beforeID)
		if err != nil {
			log.Warn("e2e: timed out waiting for automation run after PR merged event",
				zap.String("automation_id", body.AutomationID))
			c.JSON(http.StatusGatewayTimeout, gin.H{errKey: "timeout waiting for automation run to be created"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"run_task_id": taskID})
	}
}

// handleE2EAutomationManualTrigger fires a manual automation trigger (mirroring
// the "Run" button path) and polls for the resulting run task id.
func handleE2EAutomationManualTrigger(svc *automation.Service, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		automationID := c.Param("id")
		if automationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{errKey: "automation id is required"})
			return
		}

		ctx := c.Request.Context()

		a, err := svc.GetAutomation(ctx, automationID)
		if err != nil || a == nil {
			c.JSON(http.StatusNotFound, gin.H{errKey: "automation not found"})
			return
		}

		// Snapshot the latest run before firing.
		beforeID := ""
		existing, _ := svc.ListRuns(ctx, automationID, 1)
		if len(existing) > 0 {
			beforeID = existing[0].ID
		}

		// Build manual trigger data matching the production path.
		data, _ := json.Marshal(map[string]string{e2eResetSourceKey: "manual"})
		triggerID := ""
		if len(a.Triggers) > 0 {
			triggerID = a.Triggers[0].ID
		}
		result, fireErr := svc.FireTrigger(ctx, automationID, triggerID, "manual", data, "")
		if fireErr != nil {
			log.Error("e2e: manual trigger failed", zap.String("automation_id", automationID), zap.Error(fireErr))
			c.JSON(http.StatusInternalServerError, gin.H{errKey: fireErr.Error()})
			return
		}
		if result.Skipped {
			c.JSON(http.StatusOK, gin.H{"skipped": true, "reason": result.Reason})
			return
		}

		taskID, err := e2ePollNewRun(ctx, svc, automationID, beforeID)
		if err != nil {
			log.Warn("e2e: timed out waiting for automation run after manual trigger",
				zap.String("automation_id", automationID))
			c.JSON(http.StatusGatewayTimeout, gin.H{errKey: "timeout waiting for automation run to be created"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"run_task_id": taskID})
	}
}

// e2ePollNewRun blocks until a new run (with a different id than beforeID) has
// a task binding for automationID, or until 5 seconds elapse. The trigger row
// is admitted before the event handler creates/binds its task, so observing the
// row alone can return an empty task ID to an E2E caller.
func e2ePollNewRun(ctx context.Context, svc *automation.Service, automationID, beforeID string) (string, error) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		runs, err := svc.ListRuns(ctx, automationID, 1)
		if err != nil || len(runs) == 0 {
			continue
		}
		if runs[0].ID != beforeID && runs[0].TaskID != "" {
			return runs[0].TaskID, nil
		}
	}
	return "", fmt.Errorf("timeout")
}

func handleE2ESetSessionReadCursor(repo *sqliterepo.Repository, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("id")
		var body e2eSetSessionReadCursorRequest
		if err := c.ShouldBindJSON(&body); err != nil || body.MessageID == "" {
			c.JSON(http.StatusBadRequest, gin.H{errKey: "message_id is required"})
			return
		}
		e2eForceColumn(c, repo, log, e2eForceColumnOpts{
			query:       `UPDATE task_sessions SET last_read_message_id = ?, updated_at = ? WHERE id = ?`,
			id:          sessionID,
			value:       body.MessageID,
			subject:     "session",
			responseKey: "last_read_message_id",
			failLog:     "failed to force-set session read cursor",
		})
	}
}

// handleE2EAddTrigger seeds an automation trigger via HTTP for E2E tests that
// need a pre-existing trigger without driving the WS API (which requires a
// global WebSocket unavailable in Node 20).
func handleE2EAddTrigger(svc *automation.Service, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req automation.AddTriggerRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.AutomationID == "" || req.Type == "" {
			c.JSON(http.StatusBadRequest, gin.H{errKey: "automation_id and type are required"})
			return
		}
		t, err := svc.AddTrigger(c.Request.Context(), &req)
		if err != nil {
			log.Error("e2e: failed to add trigger", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{
			"id":            t.ID,
			"automation_id": t.AutomationID,
			e2eResetTypeKey: t.Type,
			"enabled":       t.Enabled,
		})
	}
}
