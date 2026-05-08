package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	agentcapabilities "github.com/kandev/kandev/internal/agent/capabilities/handlers"
	"github.com/kandev/kandev/internal/agent/docker"
	agenthandlers "github.com/kandev/kandev/internal/agent/handlers"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	agentsettingshandlers "github.com/kandev/kandev/internal/agent/settings/handlers"
	"github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/tracing"
	analyticshandlers "github.com/kandev/kandev/internal/analytics/handlers"
	analyticsrepository "github.com/kandev/kandev/internal/analytics/repository"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/ports"
	debughandlers "github.com/kandev/kandev/internal/debug"
	editorcontroller "github.com/kandev/kandev/internal/editors/controller"
	editorhandlers "github.com/kandev/kandev/internal/editors/handlers"
	"github.com/kandev/kandev/internal/events/bus"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/health"
	"github.com/kandev/kandev/internal/improvekandev"
	"github.com/kandev/kandev/internal/jira"
	"github.com/kandev/kandev/internal/linear"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	notificationcontroller "github.com/kandev/kandev/internal/notifications/controller"
	notificationhandlers "github.com/kandev/kandev/internal/notifications/handlers"
	"github.com/kandev/kandev/internal/orchestrator"
	promptcontroller "github.com/kandev/kandev/internal/prompts/controller"
	prompthandlers "github.com/kandev/kandev/internal/prompts/handlers"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/slack"
	spriteshandlers "github.com/kandev/kandev/internal/sprites"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	usercontroller "github.com/kandev/kandev/internal/user/controller"
	userhandlers "github.com/kandev/kandev/internal/user/handlers"
	utilitycontroller "github.com/kandev/kandev/internal/utility/controller"
	utilityhandlers "github.com/kandev/kandev/internal/utility/handlers"
	workflowcontroller "github.com/kandev/kandev/internal/workflow/controller"
	workflowhandlers "github.com/kandev/kandev/internal/workflow/handlers"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// buildSessionDataProvider constructs the session data provider function used by the WebSocket hub
// to send initial data (git status, context window, available commands) when a client subscribes.
func buildSessionDataProvider(taskRepo *sqliterepo.Repository, lifecycleMgr *lifecycle.Manager, log *logger.Logger) func(context.Context, string) ([]*ws.Message, error) {
	return func(ctx context.Context, sessionID string) ([]*ws.Message, error) {
		session, err := taskRepo.GetTaskSession(ctx, sessionID)
		if err != nil {
			return nil, nil // Session not found, no data to send
		}

		var result []*ws.Message
		result = appendSessionStateMessage(sessionID, session, result)
		result = appendAgentctlStatusMessage(ctx, lifecycleMgr, sessionID, result, log)
		result = appendLiveGitStatusMessage(ctx, taskRepo, lifecycleMgr, sessionID, session, result, log)
		result = appendContextWindowMessage(sessionID, session, result)
		result = appendAvailableCommandsMessage(sessionID, session, lifecycleMgr, result)
		result = appendSessionModeMessage(sessionID, session, lifecycleMgr, result)
		result = appendSessionModelsMessage(sessionID, session, lifecycleMgr, result)
		return result, nil
	}
}

const sessionIDPayloadKey = "session_id"

// appendAgentctlStatusMessage snapshots the current agentctl readiness for a
// session so late-subscribing clients (page reload, task switch, WS reconnect)
// don't sit forever on "Connecting terminal..." waiting for events that have
// already fired.
//
// Non-blocking by design — sendSessionData runs in the WS read loop, so a
// network probe here would delay every subscribe/focus ACK by its timeout.
// Instead, treat the workspace stream's presence as the cached readiness
// signal: streamManager only attaches it AFTER waitForAgentctlReady's Health
// check succeeds. If the stream is wired we emit `agentctl_ready`; otherwise
// `agentctl_starting`. The subsequent waitForAgentctlReady event (or its
// error) will correct the status if the snapshot picked the wrong one.
//
// Emits no message when the session has no live execution — the lazy
// create-on-terminal-connect path publishes events normally in that case.
func appendAgentctlStatusMessage(
	_ context.Context,
	lifecycleMgr *lifecycle.Manager,
	sessionID string,
	result []*ws.Message,
	_ *logger.Logger,
) []*ws.Message {
	if lifecycleMgr == nil {
		return result
	}
	execution, ok := lifecycleMgr.GetExecutionBySessionID(sessionID)
	if !ok {
		return result
	}

	payload := map[string]interface{}{
		sessionIDPayloadKey:  sessionID,
		"agent_execution_id": execution.ID,
	}
	if execution.TaskEnvironmentID != "" {
		payload["task_environment_id"] = execution.TaskEnvironmentID
	}
	action := ws.ActionSessionAgentctlStarting
	if execution.GetWorkspaceStream() != nil {
		action = ws.ActionSessionAgentctlReady
	}

	notification, err := ws.NewNotification(action, payload)
	if err != nil {
		return result
	}
	return append(result, notification)
}

// appendSessionStateMessage always sends the current session state so clients
// that subscribe after a state change still receive the authoritative state.
//
// Includes task_environment_id when present so late-subscribing clients
// (page reload, task switch, WS reconnect) can seed environmentIdBySessionId
// — without it, env-routed shell terminals stall on "Connecting terminal...".
func appendSessionStateMessage(sessionID string, session *models.TaskSession, result []*ws.Message) []*ws.Message {
	payload := map[string]interface{}{
		"session_id": sessionID,
		"task_id":    session.TaskID,
		"new_state":  string(session.State),
	}
	if session.ReviewStatus != nil && *session.ReviewStatus != "" {
		payload["review_status"] = *session.ReviewStatus
	}
	if session.Metadata != nil {
		payload["session_metadata"] = session.Metadata
	}
	if session.TaskEnvironmentID != "" {
		payload["task_environment_id"] = session.TaskEnvironmentID
	}
	notification, err := ws.NewNotification(ws.ActionSessionStateChanged, payload)
	if err == nil {
		result = append(result, notification)
	}
	return result
}

// appendLiveGitStatusMessage adds git status notification(s) by querying
// agentctl for live status. Multi-repo workspaces emit one notification per
// repo (stamped with repository_name); single-repo emits a single untagged
// notification. Falls back to DB snapshot if no execution exists (archived
// sessions only — the snapshot is workspace-wide, not per-repo).
func appendLiveGitStatusMessage(ctx context.Context, taskRepo *sqliterepo.Repository, lifecycleMgr *lifecycle.Manager, sessionID string, session *models.TaskSession, result []*ws.Message, log *logger.Logger) []*ws.Message {
	if msgs := tryGetLiveGitStatus(ctx, lifecycleMgr, sessionID, log); len(msgs) > 0 {
		return append(result, msgs...)
	}
	return appendDBSnapshotGitStatus(ctx, taskRepo, sessionID, result, log)
}

// tryGetLiveGitStatus attempts to get live git status from agentctl.
// Returns one notification per repo (one entry for single-repo workspaces).
// Returns nil when the session has no live execution or agentctl is stuck.
func tryGetLiveGitStatus(ctx context.Context, lifecycleMgr *lifecycle.Manager, sessionID string, log *logger.Logger) []*ws.Message {
	if lifecycleMgr == nil {
		return nil
	}

	execution, ok := lifecycleMgr.GetExecutionBySessionID(sessionID)
	if !ok {
		log.Debug("no execution found for session, will fall back to DB snapshot",
			zap.String("session_id", sessionID))
		return nil
	}

	agentClient := execution.GetAgentCtlClient()
	if agentClient == nil {
		log.Debug("no agentctl client available for session, will fall back to DB snapshot",
			zap.String("session_id", sessionID))
		return nil
	}

	// Use bounded timeout to prevent blocking session hydration if agentctl is stuck.
	rpcCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	multi, err := agentClient.GetGitStatusMulti(rpcCtx)
	if err != nil {
		log.Debug("failed to get live git status, will fall back to DB snapshot",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return nil
	}
	if multi == nil || !multi.Success || len(multi.Repos) == 0 {
		return nil
	}

	out := make([]*ws.Message, 0, len(multi.Repos))
	for _, repo := range multi.Repos {
		if !repo.Status.Success {
			continue
		}
		notification := buildGitStatusNotification(sessionID, repo.RepositoryName, repo.Status)
		if notification != nil {
			out = append(out, notification)
		}
	}
	if len(out) == 0 {
		return nil
	}
	log.Debug("got live git status from agentctl",
		zap.String("session_id", sessionID),
		zap.Int("repos", len(out)))
	return out
}

// buildGitStatusNotification packages a single repo's status as a WS event
// the frontend can route through its existing git-status handler. The
// repository_name is stamped on the inner status payload so the frontend
// stores it under byEnvironmentRepo[envKey][repository_name].
func buildGitStatusNotification(sessionID, repositoryName string, status client.GitStatusResult) *ws.Message {
	statusPayload := map[string]interface{}{
		"branch":           status.Branch,
		"remote_branch":    status.RemoteBranch,
		"ahead":            status.Ahead,
		"behind":           status.Behind,
		"files":            status.Files,
		"modified":         status.Modified,
		"added":            status.Added,
		"deleted":          status.Deleted,
		"untracked":        status.Untracked,
		"renamed":          status.Renamed,
		"branch_additions": status.BranchAdditions,
		"branch_deletions": status.BranchDeletions,
	}
	if repositoryName != "" {
		statusPayload["repository_name"] = repositoryName
	}
	gitEventData := map[string]interface{}{
		"type":       "status_update",
		"session_id": sessionID,
		"timestamp":  status.Timestamp,
		"status":     statusPayload,
	}
	notification, err := ws.NewNotification(ws.ActionSessionGitEvent, gitEventData)
	if err != nil {
		return nil
	}
	return notification
}

// appendDBSnapshotGitStatus appends a git status notification from DB snapshot.
func appendDBSnapshotGitStatus(ctx context.Context, taskRepo *sqliterepo.Repository, sessionID string, result []*ws.Message, log *logger.Logger) []*ws.Message {
	log.Debug("falling back to DB snapshot for git status",
		zap.String("session_id", sessionID))

	latestSnapshot, err := taskRepo.GetLatestGitSnapshot(ctx, sessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Expected for sessions that have not produced a snapshot yet.
			return result
		}
		log.Warn("failed to load DB snapshot for session",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return result
	}
	if latestSnapshot == nil {
		log.Debug("no DB snapshot found for session",
			zap.String("session_id", sessionID))
		return result
	}

	metadata := latestSnapshot.Metadata
	gitEventData := map[string]interface{}{
		"type":       "status_update",
		"session_id": sessionID,
		"timestamp":  metadata["timestamp"],
		"status": map[string]interface{}{
			"branch":           latestSnapshot.Branch,
			"remote_branch":    latestSnapshot.RemoteBranch,
			"ahead":            latestSnapshot.Ahead,
			"behind":           latestSnapshot.Behind,
			"files":            latestSnapshot.Files,
			"modified":         metadata["modified"],
			"added":            metadata["added"],
			"deleted":          metadata["deleted"],
			"untracked":        metadata["untracked"],
			"renamed":          metadata["renamed"],
			"branch_additions": metadata["branch_additions"],
			"branch_deletions": metadata["branch_deletions"],
		},
	}
	notification, err := ws.NewNotification(ws.ActionSessionGitEvent, gitEventData)
	if err == nil {
		result = append(result, notification)
	}
	return result
}

// appendContextWindowMessage adds a context window notification to result if available.
func appendContextWindowMessage(sessionID string, session *models.TaskSession, result []*ws.Message) []*ws.Message {
	if session.Metadata == nil {
		return result
	}
	contextWindow, ok := session.Metadata["context_window"]
	if !ok {
		return result
	}
	notification, err := ws.NewNotification(ws.ActionSessionStateChanged, map[string]interface{}{
		"session_id": sessionID,
		"task_id":    session.TaskID,
		"new_state":  string(session.State),
		"metadata": map[string]interface{}{
			"context_window": contextWindow,
		},
	})
	if err == nil {
		result = append(result, notification)
	}
	return result
}

// appendAvailableCommandsMessage adds available slash commands notification to result if any.
func appendAvailableCommandsMessage(sessionID string, session *models.TaskSession, lifecycleMgr *lifecycle.Manager, result []*ws.Message) []*ws.Message {
	if lifecycleMgr == nil {
		return result
	}
	commands := lifecycleMgr.GetAvailableCommandsForSession(sessionID)
	if len(commands) == 0 {
		return result
	}
	notification, err := ws.NewNotification(ws.ActionSessionAvailableCommands, map[string]interface{}{
		"session_id":         sessionID,
		"task_id":            session.TaskID,
		"available_commands": commands,
	})
	if err == nil {
		result = append(result, notification)
	}
	return result
}

// appendSessionModeMessage adds session mode state notification to result if cached.
func appendSessionModeMessage(sessionID string, session *models.TaskSession, lifecycleMgr *lifecycle.Manager, result []*ws.Message) []*ws.Message {
	if lifecycleMgr == nil {
		return result
	}
	modeState := lifecycleMgr.GetModeStateForSession(sessionID)
	if modeState == nil || (modeState.CurrentModeID == "" && len(modeState.AvailableModes) == 0) {
		return result
	}
	notification, err := ws.NewNotification(ws.ActionSessionModeChanged, lifecycle.SessionModeEventPayload{
		TaskID:         session.TaskID,
		SessionID:      sessionID,
		CurrentModeID:  modeState.CurrentModeID,
		AvailableModes: modeState.AvailableModes,
	})
	if err == nil {
		result = append(result, notification)
	}
	return result
}

// appendSessionModelsMessage adds session models state notification to result if cached.
func appendSessionModelsMessage(sessionID string, session *models.TaskSession, lifecycleMgr *lifecycle.Manager, result []*ws.Message) []*ws.Message {
	if lifecycleMgr == nil {
		return result
	}
	modelState := lifecycleMgr.GetModelStateForSession(sessionID)
	if modelState == nil || (modelState.CurrentModelID == "" && len(modelState.Models) == 0) {
		return result
	}
	notification, err := ws.NewNotification(ws.ActionSessionModelsUpdated, lifecycle.SessionModelsEventPayload{
		TaskID:         session.TaskID,
		SessionID:      sessionID,
		CurrentModelID: modelState.CurrentModelID,
		Models:         modelState.Models,
		ConfigOptions:  modelState.ConfigOptions,
	})
	if err == nil {
		result = append(result, notification)
	}
	return result
}

// routeParams holds all dependencies needed for HTTP and WebSocket route registration.
type routeParams struct {
	router                  *gin.Engine
	gateway                 *gateways.Gateway
	taskSvc                 *taskservice.Service
	taskRepo                *sqliterepo.Repository
	analyticsRepo           analyticsrepository.Repository
	orchestratorSvc         *orchestrator.Service
	lifecycleMgr            *lifecycle.Manager
	hostUtilityMgr          *hostutility.Manager
	eventBus                bus.EventBus
	services                *Services
	agentSettingsController *agentsettingscontroller.Controller
	agentList               taskhandlers.AgentLister
	userCtrl                *usercontroller.Controller
	notificationCtrl        *notificationcontroller.Controller
	editorCtrl              *editorcontroller.Controller
	promptCtrl              *promptcontroller.Controller
	utilityCtrl             *utilitycontroller.Controller
	msgCreator              *messageCreatorAdapter
	secretsSvc              *secrets.Service
	secretStore             secrets.SecretStore
	mcpConfigSvc            *mcpconfig.Service
	addCleanup              func(func() error)
	repoCloner              *repoclone.Cloner
	version                 string
	webInternalURL          string
	pprofEnabled            bool
	httpPort                int
	log                     *logger.Logger
}

// registerRoutes sets up all HTTP and WebSocket routes on the given router.
func registerRoutes(p routeParams) {
	workflowCtrl := workflowcontroller.NewController(p.services.Workflow)
	planService := taskservice.NewPlanService(p.taskRepo, p.eventBus, p.log)
	clarificationStore := clarification.NewStore(2 * time.Hour)
	clarificationCanceller := clarification.NewCanceller(clarificationStore, p.taskRepo, p.eventBus, p.log)
	p.orchestratorSvc.SetClarificationCanceller(clarificationCanceller)

	p.gateway.SetupRoutes(p.router)
	registerTaskRoutes(p, planService)
	registerSecondaryRoutes(p, workflowCtrl, clarificationStore, planService)

	p.router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "kandev", "mode": "websocket+http"})
	})

	if p.webInternalURL != "" {
		target, err := url.Parse(p.webInternalURL)
		if err != nil {
			p.log.Error("Invalid web internal URL, skipping reverse proxy", zap.String("url", p.webInternalURL), zap.Error(err))
		} else {
			proxy := httputil.NewSingleHostReverseProxy(target)
			proxy.FlushInterval = -1
			// ErrorHandler catches upstream failures (e.g., connection refused)
			// that occur before any response headers are written, returning a
			// clean 502 instead of letting the default handler log an error.
			proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
				p.log.Debug("Web proxy error", zap.Error(err))
				w.WriteHeader(http.StatusBadGateway)
			}
			p.router.NoRoute(func(c *gin.Context) {
				path := c.Request.URL.Path
				if strings.HasPrefix(path, "/api/") || path == "/ws" || path == "/health" {
					c.AbortWithStatus(http.StatusNotFound)
					return
				}
				// httputil.ReverseProxy panics with http.ErrAbortHandler as an
				// intentional stdlib signal when a streaming response is aborted
				// (e.g., the client disconnects mid-body, or the upstream dies
				// after response headers were already written). net/http.Server
				// understands this sentinel panic and closes the connection
				// quietly, but Gin's recovery middleware catches it first and
				// logs a noisy stack trace. Swallow that specific panic here
				// while letting any other panic bubble up to Gin's recovery.
				defer func() {
					if r := recover(); r != nil && r != http.ErrAbortHandler {
						panic(r)
					}
				}()
				proxy.ServeHTTP(c.Writer, c.Request)
			})
			p.log.Info("Web reverse proxy enabled", zap.String("target", p.webInternalURL))
		}
	}
}

// resolvePrimaryTaskRepositoryID returns the primary (lowest-position)
// task_repositories.repository_id for a task, or "" if none / on error.
// Used by PR-import callbacks where the PR is associated with the task's
// primary repo (the one the task was created against).
func resolvePrimaryTaskRepositoryID(ctx context.Context, taskRepo *sqliterepo.Repository, taskID string, log *logger.Logger) string {
	repo, err := taskRepo.GetPrimaryTaskRepository(ctx, taskID)
	if err != nil {
		log.Warn("primary task repository lookup failed",
			zap.String("task_id", taskID), zap.Error(err))
		return ""
	}
	if repo == nil {
		return ""
	}
	return repo.RepositoryID
}

// resolveRepositoryIDForSubpath maps a multi-repo subpath name (e.g.
// "kandev") to its task_repositories.repository_id by joining with the
// repositories table on Name. Empty subpath falls back to the primary
// repository so single-repo tasks Just Work. Returns "" if no match — the
// caller will then write a legacy single-repo PR row.
func resolveRepositoryIDForSubpath(ctx context.Context, taskRepo *sqliterepo.Repository, taskID, subpath string, log *logger.Logger) string {
	if subpath == "" {
		return resolvePrimaryTaskRepositoryID(ctx, taskRepo, taskID, log)
	}
	repos, err := taskRepo.ListTaskRepositories(ctx, taskID)
	if err != nil {
		log.Warn("task repositories lookup failed",
			zap.String("task_id", taskID), zap.Error(err))
		return ""
	}
	for _, link := range repos {
		repo, err := taskRepo.GetRepository(ctx, link.RepositoryID)
		if err != nil || repo == nil {
			continue
		}
		if repo.Name == subpath {
			return link.RepositoryID
		}
	}
	log.Warn("no task repository matches subpath",
		zap.String("task_id", taskID), zap.String("subpath", subpath))
	return ""
}

// registerTaskRoutes registers all task-related HTTP and WebSocket routes.
func registerTaskRoutes(p routeParams, planService *taskservice.PlanService) {
	taskhandlers.RegisterWorkspaceRoutes(p.router, p.gateway.Dispatcher, p.taskSvc, p.log)
	taskhandlers.RegisterWorkflowRoutes(p.router, p.gateway.Dispatcher, p.taskSvc, p.services.Workflow, p.log)
	taskH := taskhandlers.RegisterTaskRoutes(p.router, p.gateway.Dispatcher, p.taskSvc, p.orchestratorSvc, p.taskRepo, planService, p.log)
	if p.services.GitHub != nil {
		ghSvc := p.services.GitHub
		taskH.SetOnTaskCreatedWithPR(func(ctx context.Context, taskID, sessionID, prURL, branch string) {
			// Task-create-from-PR runs once per task and the PR maps to the
			// primary repository (first task_repository row). Resolve to that
			// repository_id so the resulting TaskPR/PRWatch are scoped per-repo.
			repositoryID := resolvePrimaryTaskRepositoryID(ctx, p.taskRepo, taskID, p.log)
			ghSvc.AssociatePRByURL(ctx, sessionID, taskID, repositoryID, prURL, branch)
		})
	}
	taskhandlers.RegisterRepositoryRoutes(p.router, p.gateway.Dispatcher, p.taskSvc, p.log)
	taskhandlers.RegisterExecutorRoutes(p.router, p.gateway.Dispatcher, p.taskSvc, p.log)
	taskhandlers.RegisterExecutorProfileRoutes(p.router, p.gateway.Dispatcher, p.taskSvc, p.agentList, p.log)
	taskhandlers.RegisterEnvironmentRoutes(p.router, p.gateway.Dispatcher, p.taskSvc, p.log)
	taskhandlers.RegisterMessageRoutes(
		p.router, p.gateway.Dispatcher, p.taskSvc,
		&orchestratorWrapper{svc: p.orchestratorSvc}, p.log,
	)
	taskhandlers.RegisterProcessRoutes(p.router, p.taskSvc, p.lifecycleMgr, p.log)
	analyticshandlers.RegisterStatsRoutes(p.router, p.analyticsRepo, p.log)
	agenthandlers.RegisterShellRoutes(p.router, p.lifecycleMgr, p.log)
	p.log.Debug("Registered Task Service handlers (HTTP + WebSocket)")
}

// registerSecondaryRoutes registers workflow, agent settings, user, notification, editor,
// prompt, clarification, MCP, and debug routes.
func registerSecondaryRoutes(
	p routeParams,
	workflowCtrl *workflowcontroller.Controller,
	clarificationStore *clarification.Store,
	planService *taskservice.PlanService,
) {
	workflowhandlers.RegisterRoutes(p.router, p.gateway.Dispatcher, workflowCtrl, p.log)
	p.log.Info("Registered Workflow handlers (HTTP + WebSocket)")

	agentsettingshandlers.RegisterRoutes(p.router, p.agentSettingsController, p.gateway.Hub, p.log)
	p.log.Debug("Registered Agent Settings handlers (HTTP)")

	userhandlers.RegisterRoutes(p.router, p.gateway.Dispatcher, p.userCtrl, p.log)
	p.log.Debug("Registered User handlers (HTTP + WebSocket)")

	notificationhandlers.RegisterRoutes(p.router, p.notificationCtrl, p.log)
	p.log.Debug("Registered Notification handlers (HTTP)")

	editorhandlers.RegisterRoutes(p.router, p.editorCtrl, p.log)
	p.log.Debug("Registered Editors handlers (HTTP)")

	prompthandlers.RegisterRoutes(p.router, p.promptCtrl, p.log)
	p.log.Debug("Registered Prompts handlers (HTTP)")

	utilityhandlers.RegisterRoutes(p.router, p.utilityCtrl, p.lifecycleMgr, p.hostUtilityMgr, p.services.User, p.log)
	p.log.Debug("Registered Utility Agents handlers (HTTP)")

	agentcapabilities.RegisterRoutes(p.router, p.hostUtilityMgr, p.log)
	p.log.Debug("Registered Agent Capabilities handlers (HTTP)")

	clarification.RegisterRoutes(p.router, clarificationStore, p.gateway.Hub, p.msgCreator, p.taskRepo, p.eventBus, p.log)
	p.log.Debug("Registered Clarification handlers (HTTP)")

	if p.secretsSvc != nil {
		secrets.RegisterRoutes(p.router, p.gateway.Dispatcher, p.secretsSvc, p.log)
		p.log.Debug("Registered Secrets handlers (HTTP + WebSocket)")
	}

	if p.secretStore != nil {
		spriteshandlers.RegisterRoutes(p.router, p.gateway.Dispatcher, p.secretStore, p.log)
		p.log.Debug("Registered Sprites handlers (HTTP + WebSocket)")
	}

	if p.services.GitHub != nil {
		github.RegisterRoutes(p.router, p.gateway.Dispatcher, p.services.GitHub, p.log)
		github.RegisterMockRoutes(p.router, p.services.GitHub, p.log)
		p.log.Debug("Registered GitHub handlers (HTTP + WebSocket)")
	}

	if p.services.Jira != nil {
		jira.RegisterRoutes(p.router, p.gateway.Dispatcher, p.services.Jira, p.log)
		jira.RegisterMockRoutes(p.router, p.services.Jira, p.log)
		p.log.Debug("Registered JIRA handlers (HTTP + WebSocket)")
	}

	if p.services.Linear != nil {
		linear.RegisterRoutes(p.router, p.gateway.Dispatcher, p.services.Linear, p.log)
		linear.RegisterMockRoutes(p.router, p.services.Linear, p.log)
		p.log.Debug("Registered Linear handlers (HTTP + WebSocket)")
	}

	if p.services.Slack != nil {
		slack.RegisterRoutes(p.router, p.gateway.Dispatcher, p.services.Slack, p.log)
		p.log.Debug("Registered Slack handlers (HTTP + WebSocket)")
	}

	docker.RegisterDockerRoutes(p.router, p.lifecycleMgr.DockerClientProvider(), p.log)
	p.log.Debug("Registered Docker management handlers (HTTP)")

	registerHealthRoutes(p)

	if p.repoCloner != nil {
		ikHandler := improvekandev.NewHandler(p.taskSvc, p.repoCloner, p.version, p.log)
		improvekandev.RegisterRoutes(p.router, ikHandler)
		improvekandev.CleanupStaleBundles(func(path string, err error) {
			p.log.Warn("Improve Kandev: failed to clean stale bundle", zap.String("path", path), zap.Error(err))
		})
		p.log.Debug("Registered Improve Kandev handlers (HTTP)")
	}

	registerMCPAndDebugRoutes(p, workflowCtrl, clarificationStore, planService)

	registerE2EResetRoutes(p.router, p.taskRepo, p.taskSvc, p.log)
}

// registerHealthRoutes sets up the system health endpoint with all health checkers.
func registerHealthRoutes(p routeParams) {
	var githubProvider health.GitHubStatusProvider
	var githubRateProvider health.GitHubRateLimitProvider
	if p.services.GitHub != nil {
		githubProvider = p.services.GitHub
		githubRateProvider = githubRateLimitAdapter{svc: p.services.GitHub}
	}
	githubChecker := health.NewGitHubChecker(githubProvider)
	if githubRateProvider != nil {
		githubChecker.WithRateLimitProvider(githubRateProvider)
	}
	healthSvc := health.NewService(p.log,
		githubChecker,
		health.NewAgentChecker(p.agentSettingsController),
	)
	health.RegisterRoutes(p.router, healthSvc, p.log)
}

// githubRateLimitAdapter bridges the github.Service's per-resource exhaustion
// snapshot to the structural shape consumed by the health package without
// importing health into github (cycle).
type githubRateLimitAdapter struct {
	svc *github.Service
}

func (a githubRateLimitAdapter) ExhaustedRateLimits() []health.GitHubRateLimitStatus {
	if a.svc == nil {
		return nil
	}
	src := a.svc.ExhaustedRateLimits()
	if len(src) == 0 {
		return nil
	}
	out := make([]health.GitHubRateLimitStatus, len(src))
	for i, s := range src {
		out[i] = health.GitHubRateLimitStatus{Resource: s.Resource, ResetAt: s.ResetAt}
	}
	return out
}

// registerMCPAndDebugRoutes registers MCP and debug routes and wires the MCP handler.
func registerMCPAndDebugRoutes(
	p routeParams,
	wfCtrl *workflowcontroller.Controller,
	clarificationStore *clarification.Store,
	planService *taskservice.PlanService,
) {
	mcpHandlers := mcphandlers.NewHandlers(
		p.taskSvc, wfCtrl,
		clarificationStore, p.msgCreator, p.taskRepo, p.taskRepo, p.eventBus, planService, p.orchestratorSvc, p.orchestratorSvc.GetMessageQueue(), p.log,
	)
	// Wire config-mode dependencies for agent-native configuration
	mcpHandlers.SetConfigDeps(p.services.Workflow, p.agentSettingsController, p.mcpConfigSvc)

	mcpHandlers.RegisterHandlers(p.gateway.Dispatcher)
	p.log.Debug("Registered MCP handlers (WebSocket)")

	p.lifecycleMgr.SetMCPHandler(p.gateway.Dispatcher)
	p.log.Debug("MCP handler configured for agent lifecycle manager")

	// External MCP endpoint — exposes config tools + create_task to external coding
	// agents (Claude Code, Cursor, etc.) at /mcp on the backend HTTP server.
	registerExternalMCP(p)

	debughandlers.RegisterRoutes(p.router, p.log)
	p.log.Debug("Registered Debug handlers (HTTP)")

	if p.pprofEnabled {
		debughandlers.RegisterPprofRoutes(p.router, p.log)
		debughandlers.RegisterMemoryRoute(p.router, p.log)
	}
}

// registerExternalMCP mounts an MCP server on the backend HTTP router so external
// coding agents can connect to Kandev at /mcp, /mcp/sse, /mcp/message. The MCP
// routes are gated by a loopback-only middleware because the endpoint is
// unauthenticated in v1 — see docs/specs/external-mcp/spec.md.
func registerExternalMCP(p routeParams) {
	port := p.httpPort
	if port == 0 {
		port = ports.Backend
	}
	baseURL := fmt.Sprintf("http://localhost:%d", port)

	backendClient := mcpserver.NewDispatcherBackendClient(p.gateway.Dispatcher, p.log)
	srv := mcpserver.NewExternal(backendClient, baseURL, p.log, "")
	mcpGroup := p.router.Group("", loopbackOnlyMiddleware(p.log))
	srv.RegisterBackendRoutes(mcpGroup)
	if p.addCleanup != nil {
		p.addCleanup(func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return srv.Close(ctx)
		})
	}
	p.log.Info("Registered external MCP endpoint",
		zap.String("base_url", baseURL),
		zap.String("streamable_http", baseURL+"/mcp"),
		zap.String("sse", baseURL+"/mcp/sse"),
		zap.String("sse_message", baseURL+"/mcp/message"))
}

// loopbackOnlyMiddleware rejects requests that did not originate from the
// loopback interface. The external MCP endpoint has no authentication in v1,
// so it must only accept connections from the same machine.
func loopbackOnlyMiddleware(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
		if err != nil {
			host = c.Request.RemoteAddr
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			log.Warn("rejected non-loopback MCP request",
				zap.String("remote_addr", c.Request.RemoteAddr),
				zap.String("path", c.Request.URL.Path))
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}

// runGracefulShutdown gracefully stops all services and runs cleanups.
func runGracefulShutdown(
	server *http.Server,
	orchestratorSvc *orchestrator.Service,
	lifecycleMgr *lifecycle.Manager,
	runCleanups func(),
	log *logger.Logger,
) {
	log.Info("Shutting down Kandev...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("HTTP server shutdown error", zap.Error(err))
	}

	if err := orchestratorSvc.Stop(); err != nil {
		log.Error("Orchestrator stop error", zap.Error(err))
	}

	stopLifecycleManager(lifecycleMgr, log)
	runCleanups()

	// Flush pending OTel spans before exit
	traceCtx, traceCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := tracing.Shutdown(traceCtx); err != nil {
		log.Error("Tracer shutdown error", zap.Error(err))
	}
	traceCancel()

	log.Info("Kandev stopped")
	_ = log.Sync()
}

// stopLifecycleManager gracefully stops all agents and the lifecycle manager.
func stopLifecycleManager(lifecycleMgr *lifecycle.Manager, log *logger.Logger) {
	if lifecycleMgr == nil {
		return
	}
	log.Info("Stopping agents gracefully...")
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := lifecycleMgr.StopAllAgents(stopCtx); err != nil {
		log.Error("Graceful agent stop error", zap.Error(err))
	}
	stopCancel()

	if err := lifecycleMgr.Stop(); err != nil {
		log.Error("Lifecycle manager stop error", zap.Error(err))
	}
}
