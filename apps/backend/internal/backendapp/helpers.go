package backendapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	agentcapabilities "github.com/kandev/kandev/internal/agent/capabilities/handlers"
	"github.com/kandev/kandev/internal/agent/docker"
	agenthandlers "github.com/kandev/kandev/internal/agent/handlers"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/loginpty"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/registry"
	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	agentsettingshandlers "github.com/kandev/kandev/internal/agent/settings/handlers"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/agentctl/tracing"
	analyticshandlers "github.com/kandev/kandev/internal/analytics/handlers"
	analyticsrepository "github.com/kandev/kandev/internal/analytics/repository"
	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/ports"
	debughandlers "github.com/kandev/kandev/internal/debug"
	editorcontroller "github.com/kandev/kandev/internal/editors/controller"
	editorhandlers "github.com/kandev/kandev/internal/editors/handlers"
	"github.com/kandev/kandev/internal/events/bus"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/health"
	"github.com/kandev/kandev/internal/health/oslimits"
	"github.com/kandev/kandev/internal/improvekandev"
	"github.com/kandev/kandev/internal/jira"
	"github.com/kandev/kandev/internal/linear"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	notificationcontroller "github.com/kandev/kandev/internal/notifications/controller"
	notificationhandlers "github.com/kandev/kandev/internal/notifications/handlers"
	"github.com/kandev/kandev/internal/office"
	officeagents "github.com/kandev/kandev/internal/office/agents"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	officetestharness "github.com/kandev/kandev/internal/office/testharness"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/plugins"
	pluginstore "github.com/kandev/kandev/internal/plugins/store"
	promptcontroller "github.com/kandev/kandev/internal/prompts/controller"
	prompthandlers "github.com/kandev/kandev/internal/prompts/handlers"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/runtimeflags"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/sentry"
	"github.com/kandev/kandev/internal/slack"
	spriteshandlers "github.com/kandev/kandev/internal/sprites"
	sshhandlers "github.com/kandev/kandev/internal/ssh"
	systemsvc "github.com/kandev/kandev/internal/system"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	usercontroller "github.com/kandev/kandev/internal/user/controller"
	userhandlers "github.com/kandev/kandev/internal/user/handlers"
	utilitycontroller "github.com/kandev/kandev/internal/utility/controller"
	utilityhandlers "github.com/kandev/kandev/internal/utility/handlers"
	voicehandlers "github.com/kandev/kandev/internal/voice/handlers"
	"github.com/kandev/kandev/internal/voice/transcribe"
	"github.com/kandev/kandev/internal/webapp"
	webembedded "github.com/kandev/kandev/internal/webapp/embedded"
	workflowcontroller "github.com/kandev/kandev/internal/workflow/controller"
	workflowhandlers "github.com/kandev/kandev/internal/workflow/handlers"
	"github.com/kandev/kandev/internal/workflowsync"
	"github.com/kandev/kandev/internal/worktree"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const (
	desktopHealthTokenEnv    = "KANDEV_DESKTOP_HEALTH_TOKEN"
	desktopHealthTokenHeader = "X-Kandev-Desktop-Health-Token"
	agentShutdownTimeout     = 20 * time.Second
	httpShutdownTimeout      = 10 * time.Second
	tracingShutdownTimeout   = 5 * time.Second
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
const taskIDPayloadKey = "task_id"
const newStatePayloadKey = "new_state"
const sessionUpdatedAtPayloadKey = "updated_at"

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
	if execution.WorkspacePath != "" {
		payload["worktree_path"] = execution.WorkspacePath
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
		sessionIDPayloadKey:        sessionID,
		taskIDPayloadKey:           session.TaskID,
		newStatePayloadKey:         string(session.State),
		sessionUpdatedAtPayloadKey: session.UpdatedAt.UTC().Format(time.RFC3339Nano),
		"name":                     session.Name,
	}
	if session.ReviewStatus != models.ReviewStatusNone {
		payload["review_status"] = string(session.ReviewStatus)
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
	// Force fresh git query: cache can wedge when the poll loop misses a HEAD change.
	multi, err := agentClient.GetGitStatusMultiFresh(rpcCtx)
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
		ConfigBaseline: sessionACPConfigBaseline(session),
	})
	if err == nil {
		result = append(result, notification)
	}
	return result
}

func sessionACPConfigBaseline(session *models.TaskSession) map[string]string {
	if session == nil {
		return nil
	}
	baseline, _ := models.LoadSessionACPConfigBaseline(session.Metadata)
	return baseline
}

// routeParams holds all dependencies needed for HTTP and WebSocket route registration.
type routeParams struct {
	router                  *gin.Engine
	gateway                 *gateways.Gateway
	taskSvc                 *taskservice.Service
	taskRepo                *sqliterepo.Repository
	officeRepo              *officesqlite.Repository
	analyticsRepo           analyticsrepository.Repository
	orchestratorSvc         *orchestrator.Service
	lifecycleMgr            *lifecycle.Manager
	hostUtilityMgr          *hostutility.Manager
	eventBus                bus.EventBus
	services                *Services
	systemSvc               *systemsvc.Service
	workspaceRestorer       taskhandlers.WorkspaceQuarantineRestorer
	runtimeFlagsSvc         *runtimeflags.Service
	agentSettingsController *agentsettingscontroller.Controller
	agentSettingsRepo       settingsstore.Repository
	agentList               taskhandlers.AgentLister
	agentRegistry           *registry.Registry
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
	devMode                 bool
	httpPort                int
	features                config.FeaturesConfig
	voice                   config.VoiceConfig
	log                     *logger.Logger
}

// registerRoutes sets up all HTTP and WebSocket routes on the given router.
func registerRoutes(p routeParams) {
	workflowCtrl := workflowcontroller.NewController(p.services.Workflow)
	planService := taskservice.NewPlanService(p.taskRepo, p.eventBus, p.log)
	clarificationStore := clarification.NewStore(2 * time.Hour)
	clarificationCanceller := clarification.NewCanceller(clarificationStore, p.taskRepo, p.eventBus, p.log)
	p.orchestratorSvc.SetClarificationCanceller(clarificationCanceller)

	// Wire pending clarification requests into the office inbox.
	if p.services.OfficeSvcs != nil && p.services.OfficeSvcs.Dashboard != nil {
		p.services.OfficeSvcs.Dashboard.SetPermissionLister(clarificationStore)
	}

	// Office task-handoffs phase 4 + 5 — single HandoffService instance
	// shared by the MCP path (office agents) and the HTTP path (Kanban
	// subtask UI). Constructed here so registerTaskRoutes can wire it
	// into TaskHandlers.SetHandoffService and registerMCPAndDebugRoutes
	// reuses the same instance via SetHandoffService on mcpHandlers.
	handoffDocSvc := taskservice.NewDocumentService(p.taskRepo, p.log)
	handoffSvc := taskservice.NewHandoffService(p.taskRepo, p.taskRepo, handoffDocSvc,
		p.officeRepo, p.officeRepo, p.log)
	// Phase 6 wirings — materializer hook + disk cleaner. The
	// SessionWorktreeReader and WorkspaceCleaner interfaces are both
	// satisfied by adapters that delegate to existing services.
	handoffSvc.SetSessionReader(p.taskRepo)
	if p.lifecycleMgr != nil {
		if wtMgr := p.lifecycleMgr.WorktreeManager(); wtMgr != nil {
			handoffSvc.SetWorkspaceCleaner(worktree.NewHandoffCleaner(wtMgr, p.log))
		}
	}
	handoffSvc.SetRunCanceller(p.orchestratorSvc)
	// Cascade archive/delete must re-publish task.updated / task.deleted
	// events; HandoffService walks the repo directly and bypasses the
	// Service wrappers that normally publish these. Without this wiring
	// the kanban board doesn't react to subtree archive/delete until a
	// full reload.
	handoffSvc.SetTaskEventPublisher(p.taskSvc)
	if p.services.Office != nil {
		p.services.Office.SetWorkspaceGroupCleaner(handoffSvc)
	}
	// Cascade archive/delete must tear down runtime resources
	// (container, sandbox, worktree, executor_running rows) for every
	// task in the tree. Without this wiring the agent gets stopped via
	// runCanceller but its container leaks because the cascade bypasses
	// Service.ArchiveTask's runAsyncTaskCleanup branch.
	handoffSvc.SetTaskResourceCleaner(p.taskSvc)
	// Watch reset (Reset button on integration settings) cascade-deletes
	// every task a watch previously created. The integrations re-use the
	// shared HandoffService so the reset path goes through the same
	// cleanup machinery as the regular delete-task surface.
	if p.services.GitHub != nil {
		p.services.GitHub.SetCascadeTaskDeleter(handoffSvc)
	}
	// repoLookup validates a watcher's optional repository binding (workspace
	// ownership + default-branch fill) on create/update. Shared across the three
	// repo-less watchers; one concrete adapter satisfies each package's
	// structurally-identical RepositoryLookup interface.
	repoLookup := &repositoryLookupAdapter{svc: p.taskSvc}
	if p.services.Jira != nil {
		p.services.Jira.SetTaskDeleter(handoffSvc)
		p.services.Jira.SetRepositoryLookup(repoLookup)
	}
	if p.services.Linear != nil {
		p.services.Linear.SetTaskDeleter(handoffSvc)
		p.services.Linear.SetRepositoryLookup(repoLookup)
	}
	if p.services.Sentry != nil {
		p.services.Sentry.SetTaskDeleter(handoffSvc)
		p.services.Sentry.SetRepositoryLookup(repoLookup)
	}
	p.orchestratorSvc.SetWorkspaceMaterializer(handoffSvc)
	// Phase 8 prompt enrichment — wire the office scheduler's
	// TaskContextProvider so every run prompt rendered by the active
	// office/service/BuildPrompt sees Related-tasks / Documents
	// available / Workspace sections.
	if p.services.OrchScheduler != nil {
		p.services.OrchScheduler.SetTaskContextProvider(handoffSvc)
	}

	p.gateway.SetupRoutes(p.router)
	registerTaskRoutes(p, planService, handoffSvc)
	registerSecondaryRoutes(p, workflowCtrl, clarificationStore, clarificationCanceller, planService, handoffSvc)

	// /health is a readiness probe, not a liveness probe. It only
	// returns 200 after main has flipped the package-level `ready`
	// flag — which happens after route registration, agent-registry
	// seeding, the HTTP listener accepting connections, and (when
	// KANDEV_E2E_MOCK is set) the testharness routes being mounted.
	// Before that, return 503 so callers (including the e2e fixture's
	// waitForHealth) keep polling instead of racing ahead and hitting
	// 404s on routes that aren't wired yet.
	p.router.GET("/health", func(c *gin.Context) {
		if !ready.Load() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "starting", "service": "kandev"})
			return
		}
		if token := desktopHealthToken(); token != "" {
			c.Header(desktopHealthTokenHeader, token)
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "kandev", "mode": "websocket+http"})
	})

	// /api/v1/features is a public, unauthenticated read of the runtime
	// feature-flag map. The frontend SSR-fetches it once per page render to
	// decide whether to mount Office (and any future flagged feature). The
	// `json:` tags on FeaturesConfig drive serialization, so adding a new
	// field to the struct is enough — no edit here required.
	// See docs/decisions/0007-runtime-feature-flags.md.
	p.router.GET("/api/v1/features", func(c *gin.Context) {
		c.JSON(http.StatusOK, p.features)
	})
	p.router.GET("/api/v1/app-state", func(c *gin.Context) {
		routePath := c.Query("path")
		if routePath == "" {
			routePath = c.Request.URL.Path
		}
		route := webapp.ClassifyRoute(routePath)
		payload := bootPayload(c.Request.Context(), c.Request, p, route)
		c.JSON(http.StatusOK, payload)
	})

	if p.webInternalURL == "" {
		if handler, distDir, ok := newWebAppHandler(p); ok {
			p.router.NoRoute(func(c *gin.Context) {
				handler.ServeHTTP(c.Writer, c.Request)
			})
			p.log.Info("Web SPA static serving enabled", zap.String("dist_dir", distDir))
			return
		}
	}

	if p.webInternalURL != "" {
		handler, err := newWebDevHandler(p)
		if err != nil {
			p.log.Error("Invalid web internal URL, skipping web dev handler", zap.String("url", p.webInternalURL), zap.Error(err))
		} else {
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
				handler.ServeHTTP(c.Writer, c.Request)
			})
			p.log.Info("Web dev handler enabled", zap.String("target", p.webInternalURL))
		}
	}
}

func desktopHealthToken() string {
	return strings.TrimSpace(os.Getenv(desktopHealthTokenEnv))
}

func newWebAppHandler(p routeParams) (*webapp.Handler, string, bool) {
	assets, source, ok := webAssetsFS()
	if !ok {
		return nil, "", false
	}
	handler := webapp.NewHandler(assets, webAppHandlerOptions(p)...)
	return handler, source, true
}

func newWebDevHandler(p routeParams) (*webapp.DevHandler, error) {
	return webapp.NewDevHandler(p.webInternalURL, webAppHandlerOptions(p)...)
}

func webAppHandlerOptions(p routeParams) []webapp.HandlerOption {
	return []webapp.HandlerOption{
		webapp.WithRuntimeConfig(webapp.RuntimeConfig{APIPrefix: "/api/v1", WebSocketPath: "/ws"}),
		webapp.WithPayloadBuilder(func(req *http.Request, route webapp.RouteClassification) webapp.BootPayload {
			return bootPayload(req.Context(), req, p, route)
		}),
	}
}

func bootPayload(ctx context.Context, req *http.Request, p routeParams, route webapp.RouteClassification) webapp.BootPayload {
	payload := webapp.NewBootPayload(
		route,
		webapp.RuntimeConfig{APIPrefix: "/api/v1", WebSocketPath: "/ws", Debug: p.devMode},
		bootInitialState(ctx, req, p, route),
	)
	payload.RouteData = bootRouteData(ctx, req, p, route)
	payload.Plugins = bootActivePlugins(p)
	return payload
}

// bootActivePlugins populates the boot payload's Plugins list from every
// active, UI-bundle-declaring plugin, per
// docs/plans/plugins/PLUGIN-API.md ("Loading model"). Gated on
// features.Plugins — separate from the /api/v1/features flag map itself,
// this is active-bundle data the frontend still gates loading on via
// useFeature("plugins").
func bootActivePlugins(p routeParams) []webapp.ActivePluginPayload {
	if !p.features.Plugins || p.services == nil || p.services.Plugins == nil {
		return nil
	}
	records := p.services.Plugins.ActiveUIPlugins()
	out := make([]webapp.ActivePluginPayload, 0, len(records))
	for _, rec := range records {
		out = append(out, webapp.ActivePluginPayload{
			ID:        rec.ID,
			Name:      rec.DisplayName,
			BundleURL: "/api/plugins/" + rec.ID + "/bundle",
			StyleURLs: pluginStyleURLs(rec),
		})
	}
	return out
}

// pluginStyleURLs maps a plugin's root-relative ui.styles paths to
// browser-facing URLs served through the /api/plugins/:id/ui/* proxy (the
// plugin's own base_url is never exposed to the browser directly).
func pluginStyleURLs(rec pluginstore.Record) []string {
	if len(rec.UI.Styles) == 0 {
		return nil
	}
	urls := make([]string, 0, len(rec.UI.Styles))
	for _, style := range rec.UI.Styles {
		urls = append(urls, "/api/plugins/"+rec.ID+"/ui"+style)
	}
	return urls
}

func webAssetsFS() (fs.FS, string, bool) {
	distDir := os.Getenv("KANDEV_WEB_DIST_DIR")
	if distDir == "" {
		distDir = firstExistingDir("apps/web/dist", "../web/dist", "../../apps/web/dist")
	}
	if distDir != "" {
		return os.DirFS(distDir), distDir, true
	}
	assets, err := webembedded.FS()
	if err != nil {
		return nil, "", false
	}
	return assets, "embedded", true
}

func firstExistingDir(candidates ...string) string {
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
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

func resolveRepositoryIDForSessionSubpath(ctx context.Context, taskRepo *sqliterepo.Repository, sessionID, subpath string, log *logger.Logger) string {
	worktrees, err := taskRepo.ListTaskSessionWorktrees(ctx, sessionID)
	if err != nil {
		log.Warn("session worktrees lookup failed",
			zap.String("session_id", sessionID), zap.Error(err))
		return ""
	}
	if subpath == "" {
		if len(worktrees) == 1 {
			return worktrees[0].RepositoryID
		}
		log.Warn("branch rename did not specify repo for multi-repo session",
			zap.String("session_id", sessionID), zap.Int("worktree_count", len(worktrees)))
		return ""
	}
	for _, wt := range worktrees {
		// Multi-repo sessions are small today and repository lookup is only on
		// branch rename, so keep this direct until the repository interface grows
		// a batch lookup.
		repo, err := taskRepo.GetRepository(ctx, wt.RepositoryID)
		if err != nil || repo == nil {
			continue
		}
		if repo.Name == subpath || worktree.SanitizeRepoDirName(repo.Name) == subpath {
			return wt.RepositoryID
		}
	}
	log.Warn("no session worktree repository matches subpath",
		zap.String("session_id", sessionID), zap.String("subpath", subpath))
	return ""
}

// registerTaskRoutes registers all task-related HTTP and WebSocket routes.
func registerTaskRoutes(p routeParams, planService *taskservice.PlanService, handoffSvc *taskservice.HandoffService) {
	taskhandlers.RegisterWorkspaceRoutes(p.router, p.gateway.Dispatcher, p.taskSvc, p.log)
	taskhandlers.RegisterWorkflowRoutes(p.router, p.gateway.Dispatcher, p.taskSvc, p.services.Workflow, p.log)
	taskH := taskhandlers.RegisterTaskRoutes(p.router, p.gateway.Dispatcher, p.taskSvc, p.orchestratorSvc, p.taskRepo, planService, p.log)
	if p.services != nil && p.services.User != nil {
		taskH.SetTaskCreateLastUsedRecorder(p.services.User)
	}
	if handoffSvc != nil {
		taskH.SetHandoffService(handoffSvc)
	}
	if p.workspaceRestorer != nil {
		taskH.SetWorkspaceQuarantineRestorer(p.workspaceRestorer)
	}
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
	if p.services.Share != nil {
		p.services.Share.RegisterRoutes(p.router)
		p.log.Debug("Registered Public Share Links handlers (HTTP)")
	}
	p.log.Debug("Registered Task Service handlers (HTTP + WebSocket)")
}

// registerSecondaryRoutes registers workflow, agent settings, user, notification, editor,
// prompt, clarification, MCP, and debug routes.
func registerSecondaryRoutes(
	p routeParams,
	workflowCtrl *workflowcontroller.Controller,
	clarificationStore *clarification.Store,
	clarificationCanceller *clarification.Canceller,
	planService *taskservice.PlanService,
	handoffSvc *taskservice.HandoffService,
) {
	workflowhandlers.RegisterRoutes(p.router, p.gateway.Dispatcher, workflowCtrl, p.log)
	p.log.Info("Registered Workflow handlers (HTTP + WebSocket)")

	agentsettingshandlers.RegisterRoutes(p.router, p.agentSettingsController, p.gateway.Hub, p.log)
	p.log.Debug("Registered Agent Settings handlers (HTTP)")

	// Login PTY: spawns agent login commands under a PTY on the kandev host
	// (claude auth login, auggie login, ...). The user explicitly closes the
	// dialog when done, so invalidate the discovery cache on every session
	// end regardless of exit code — rescanning is cheap and correctly picks
	// up new auth state for agents whose login flow lives inside the TUI
	// (e.g. gemini) where the process keeps running after auth completes.
	loginMgr := loginpty.NewManager(p.log, func(_ string, _ int, _ error) {
		p.agentSettingsController.InvalidateDiscoveryCache()
	})
	loginpty.NewHandlers(loginMgr, p.agentRegistry, p.log.Zap(), nil).RegisterRoutes(p.router)
	p.log.Debug("Registered Login PTY handlers (HTTP + WebSocket)")

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

	// Voice transcription fallback. The route always mounts, but returns 503
	// when no API key is configured so the frontend can hide the path.
	voicehandlers.RegisterRoutes(p.router, transcribe.New(p.voice.OpenAIAPIKey), p.log)
	p.log.Debug("Registered Voice handlers (HTTP)")

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

	if p.taskRepo != nil {
		sshhandlers.RegisterRoutes(
			p.router,
			p.gateway.Dispatcher,
			p.taskRepo,
			p.services.Task,
			p.agentRegistry,
			lifecycle.NewAgentctlResolver(p.log),
			p.log,
		)
		p.log.Debug("Registered SSH handlers (HTTP + WebSocket)")
	}

	if p.services.GitHub != nil {
		github.RegisterRoutes(p.router, p.gateway.Dispatcher, p.services.GitHub, p.log)
		github.RegisterMockRoutes(p.router, p.services.GitHub, p.log)
		p.log.Debug("Registered GitHub handlers (HTTP + WebSocket)")
	}

	if p.services.GitLab != nil {
		gitlab.RegisterRoutesWithDispatcher(p.router, p.gateway.Dispatcher, p.services.GitLab, p.log)
		gitlab.RegisterMockRoutes(p.router, p.services.GitLab, p.log)
		p.log.Debug("Registered GitLab handlers (HTTP + WebSocket)")
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

	if p.services.Sentry != nil {
		sentry.RegisterRoutes(p.router, p.gateway.Dispatcher, p.services.Sentry, p.log)
		sentry.RegisterMockRoutes(p.router, p.services.Sentry, p.log)
		p.log.Debug("Registered Sentry handlers (HTTP)")
	}

	if p.services.Slack != nil {
		slack.RegisterRoutes(p.router, p.gateway.Dispatcher, p.services.Slack, p.log)
		p.log.Debug("Registered Slack handlers (HTTP + WebSocket)")
	}

	if p.services.WorkflowSync != nil {
		workflowsync.RegisterRoutes(p.router, p.services.WorkflowSync, p.log)
		p.log.Debug("Registered workflow sync handlers (HTTP)")
	}

	if p.services.Automation != nil {
		automation.RegisterRoutes(p.router, p.gateway.Dispatcher, p.services.Automation.Service, p.log)
		p.log.Debug("Registered Automation handlers (HTTP + WebSocket)")
	}

	if p.features.Plugins && p.services.Plugins != nil {
		plugins.RegisterRoutes(p.router, p.services.Plugins, p.services.Plugins.Deliverer(), p.log)
		p.log.Debug("Registered Plugins handlers (HTTP)")
	}

	docker.RegisterDockerRoutes(p.router, p.lifecycleMgr.DockerClientProvider(), dockerTaskTitleProvider(p.taskRepo, p.log), p.log)
	p.log.Debug("Registered Docker management handlers (HTTP)")

	registerHealthRoutes(p)
	registerSystemRoutes(p)
	if p.runtimeFlagsSvc != nil {
		runtimeflags.RegisterRoutes(p.router, p.runtimeFlagsSvc)
	}

	if p.repoCloner != nil {
		ikHandler := improvekandev.NewHandler(p.taskSvc, p.repoCloner, p.version, p.log)
		improvekandev.RegisterRoutes(p.router, ikHandler)
		improvekandev.CleanupStaleBundles(func(path string, err error) {
			p.log.Warn("Improve Kandev: failed to clean stale bundle", zap.String("path", path), zap.Error(err))
		})
		p.log.Debug("Registered Improve Kandev handlers (HTTP)")
	}

	registerMCPAndDebugRoutes(p, workflowCtrl, clarificationStore, clarificationCanceller, planService, handoffSvc)

	var automationSvc *automation.Service
	if p.services.Automation != nil {
		automationSvc = p.services.Automation.Service
	}
	registerE2EResetRoutes(p.router, p.taskRepo, p.taskSvc, automationSvc, p.services.GitHub, p.log)

	if officetestharness.Enabled() {
		var officeAgentSvc *officeagents.AgentService
		if p.services.OfficeSvcs != nil {
			officeAgentSvc = p.services.OfficeSvcs.Agents
		}
		officetestharness.RegisterRoutes(
			p.router,
			p.taskRepo,
			p.officeRepo,
			p.agentSettingsRepo,
			officeAgentSvc,
			p.eventBus,
			p.log,
		)
		p.log.Info("E2E mock routes enabled at /api/v1/_test/* — DO NOT enable in production")
	}

	// Register office routes
	if p.services.OfficeSvcs != nil {
		api := p.router.Group("/api/v1/office")
		api.Use(officeagents.AgentAuthMiddleware(p.services.OfficeSvcs.Agents))
		office.RegisterAllRoutes(api, p.services.OfficeSvcs, p.log)
		p.log.Debug("Registered Office handlers (HTTP)")
	}
}

func dockerTaskTitleProvider(taskRepo *sqliterepo.Repository, log *logger.Logger) docker.TaskTitleProvider {
	return func(ctx context.Context, taskID string) (string, bool) {
		if taskRepo == nil || taskID == "" {
			return "", false
		}
		task, err := taskRepo.GetTask(ctx, taskID)
		if err != nil {
			log.Debug("docker container task title lookup failed",
				zap.String("task_id", taskID), zap.Error(err))
			return "", false
		}
		return task.Title, task.Title != ""
	}
}

// registerSystemRoutes mounts the System pages backend onto /api/v1/system/*.
// The system service is constructed upstream (startGatewayAndServe) so the
// updates-poller goroutine can be started with the main run context; here we
// only register HTTP handlers. The systemSvc field is nil during partial
// builds (tests, CLI subcommands) — registration is then a no-op.
func registerSystemRoutes(p routeParams) {
	if p.systemSvc == nil {
		return
	}
	p.systemSvc.RegisterRoutes(p.router, p.log)
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
	osLimitsChecker := health.NewCachedChecker(
		oslimits.NewOSLimitsChecker(oslimits.NewInotifyProbe()),
		5*time.Minute,
	)
	checkers := []health.Checker{
		health.NewGitExecutableChecker(),
		githubChecker,
		health.NewAgentChecker(p.agentSettingsController),
		osLimitsChecker,
	}
	if p.systemSvc != nil && p.systemSvc.StorageRuntime != nil {
		checkers = append(checkers, p.systemSvc.StorageRuntime)
	}
	healthSvc := health.NewService(p.log, checkers...)
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

// mcpTaskPRListerAdapter adapts *github.Service to the MCP handlers'
// TaskPRLister interface so list_tasks responses can carry per-task PR
// summaries. Returns an empty map when the github service is nil.
type mcpTaskPRListerAdapter struct {
	gh *github.Service
}

func (a mcpTaskPRListerAdapter) ListTaskPRsByTaskIDs(
	ctx context.Context, taskIDs []string,
) (map[string][]mcphandlers.TaskPRInfo, error) {
	out := make(map[string][]mcphandlers.TaskPRInfo)
	if a.gh == nil || len(taskIDs) == 0 {
		return out, nil
	}
	prs, err := a.gh.ListTaskPRsByTaskIDs(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	for taskID, list := range prs {
		infos := make([]mcphandlers.TaskPRInfo, 0, len(list))
		for _, pr := range list {
			if pr == nil {
				continue
			}
			infos = append(infos, mcphandlers.TaskPRInfo{
				Number:   pr.PRNumber,
				URL:      pr.PRURL,
				Title:    pr.PRTitle,
				State:    pr.State,
				MergedAt: pr.MergedAt,
			})
		}
		if len(infos) > 0 {
			out[taskID] = infos
		}
	}
	return out, nil
}

// registerMCPAndDebugRoutes registers MCP and debug routes and wires the MCP handler.
func registerMCPAndDebugRoutes(
	p routeParams,
	wfCtrl *workflowcontroller.Controller,
	clarificationStore *clarification.Store,
	clarificationCanceller *clarification.Canceller,
	planService *taskservice.PlanService,
	handoffSvc *taskservice.HandoffService,
) {
	walkthroughService := taskservice.NewWalkthroughService(p.taskRepo, p.eventBus, p.log)
	mcpHandlers := mcphandlers.NewHandlers(
		p.taskSvc, wfCtrl,
		clarificationStore, clarificationCanceller, p.msgCreator, p.taskRepo, p.taskRepo, p.eventBus, planService, walkthroughService, p.orchestratorSvc, p.orchestratorSvc.GetMessageQueue(), p.log,
	)
	// Wire config-mode dependencies for agent-native configuration
	mcpHandlers.SetConfigDeps(p.services.Workflow, p.agentSettingsController, p.mcpConfigSvc)
	mcpHandlers.SetClarificationInputPauser(p.orchestratorSvc)
	mcpHandlers.SetPromptReferenceResolver(p.services.Prompts)

	// Enrich list_tasks responses with associated GitHub PRs (link, title,
	// number, state) when the github service is available.
	if p.services.GitHub != nil {
		mcpHandlers.SetTaskPRLister(mcpTaskPRListerAdapter{gh: p.services.GitHub})
	}

	// Reuse the cross-task handoff service constructed in registerRoutes —
	// the same instance backs the MCP path and the HTTP Kanban path so
	// workspace-group state stays consistent across both surfaces.
	if handoffSvc != nil {
		mcpHandlers.SetHandoffService(handoffSvc)
	}

	mcpHandlers.RegisterHandlers(p.gateway.Dispatcher)
	p.log.Debug("Registered MCP handlers (WebSocket)")

	p.lifecycleMgr.SetMCPHandler(p.gateway.Dispatcher)
	p.log.Debug("MCP handler configured for agent lifecycle manager")

	// External MCP endpoint — exposes config tools + create_task to external coding
	// agents (Claude Code, Cursor, etc.) at /mcp on the backend HTTP server.
	registerExternalMCP(p)

	debughandlers.RegisterRoutes(p.router, p.log)
	p.log.Debug("Registered Debug handlers (HTTP)")

	if p.devMode {
		debughandlers.RegisterExportRoute(p.router, p.version, Commit, p.log)
		debughandlers.RegisterPprofRoutes(p.router, p.log)
		debughandlers.RegisterMemoryRoute(p.router, p.log)
	}
}

// registerExternalMCP mounts an MCP server on the backend HTTP router so external
// coding agents can connect to Kandev at /mcp, /mcp/sse, /mcp/message.
func registerExternalMCP(p routeParams) {
	port := p.httpPort
	if port == 0 {
		port = ports.Backend
	}
	baseURL := fmt.Sprintf("http://localhost:%d", port)

	backendClient := mcpserver.NewDispatcherBackendClient(p.gateway.Dispatcher, p.log)
	srv := mcpserver.NewExternal(backendClient, p.log, "")
	mcpGroup := p.router.Group("", externalMCPOpenMiddleware())
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

// externalMCPOpenMiddleware documents the external MCP access policy: the
// endpoint is open on every interface the backend listens on. Kandev does not
// yet have a user auth boundary, so protecting MCP separately would create a
// false sense of security while the rest of the app remains reachable.
func externalMCPOpenMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

// runGracefulShutdown gracefully stops all services and runs cleanups.
func runGracefulShutdown(
	server *http.Server,
	listeners *serverListeners,
	orchestratorSvc *orchestrator.Service,
	lifecycleMgr *lifecycle.Manager,
	runCleanups func(),
	log *logger.Logger,
) {
	start := time.Now()
	var shutdownErrs []error
	log.Info("Graceful shutdown started",
		zap.Int("http_timeout_seconds", int(httpShutdownTimeout/time.Second)),
		zap.Int("agent_timeout_seconds", int(agentShutdownTimeout/time.Second)),
		zap.Int("tracing_timeout_seconds", int(tracingShutdownTimeout/time.Second)))

	// Stop the background bind-retry loop before Shutdown so no new listener
	// can be created after the server begins closing its listeners.
	listeners.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		shutdownErrs = append(shutdownErrs, err)
		log.Error("HTTP server shutdown error", zap.Error(err))
	}

	if err := orchestratorSvc.Stop(); err != nil {
		shutdownErrs = append(shutdownErrs, err)
		log.Error("Orchestrator stop error", zap.Error(err))
	}

	if err := stopLifecycleManager(lifecycleMgr, log); err != nil {
		shutdownErrs = append(shutdownErrs, err)
	}
	runCleanups()

	// Flush pending OTel spans before exit
	traceCtx, traceCancel := context.WithTimeout(context.Background(), tracingShutdownTimeout)
	if err := tracing.Shutdown(traceCtx); err != nil {
		shutdownErrs = append(shutdownErrs, err)
		log.Error("Tracer shutdown error", zap.Error(err))
	}
	traceCancel()

	log.Info("Graceful shutdown complete",
		zap.Duration("duration", time.Since(start)),
		zap.Int("error_count", len(shutdownErrs)))
	_ = log.Sync()
}

// stopLifecycleManager gracefully stops all agents and the lifecycle manager.
func stopLifecycleManager(lifecycleMgr *lifecycle.Manager, log *logger.Logger) error {
	if lifecycleMgr == nil {
		return nil
	}
	var shutdownErrs []error
	log.Info("Stopping agents gracefully",
		zap.Int("timeout_seconds", int(agentShutdownTimeout/time.Second)))
	stopCtx, stopCancel := context.WithTimeout(context.Background(), agentShutdownTimeout)
	if err := lifecycleMgr.StopAllAgents(stopCtx); err != nil {
		shutdownErrs = append(shutdownErrs, err)
		log.Error("Graceful agent stop error", zap.Error(err))
	}
	stopCancel()

	if err := lifecycleMgr.Stop(); err != nil {
		shutdownErrs = append(shutdownErrs, err)
		log.Error("Lifecycle manager stop error", zap.Error(err))
	}
	if len(shutdownErrs) == 0 {
		log.Info("Agents stopped gracefully")
	}
	return errors.Join(shutdownErrs...)
}
