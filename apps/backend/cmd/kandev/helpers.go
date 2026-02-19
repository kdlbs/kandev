package main

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/docker"
	agenthandlers "github.com/kandev/kandev/internal/agent/handlers"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	agentsettingshandlers "github.com/kandev/kandev/internal/agent/settings/handlers"
	analyticshandlers "github.com/kandev/kandev/internal/analytics/handlers"
	analyticsrepository "github.com/kandev/kandev/internal/analytics/repository"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	debughandlers "github.com/kandev/kandev/internal/debug"
	editorcontroller "github.com/kandev/kandev/internal/editors/controller"
	editorhandlers "github.com/kandev/kandev/internal/editors/handlers"
	"github.com/kandev/kandev/internal/events/bus"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	notificationcontroller "github.com/kandev/kandev/internal/notifications/controller"
	notificationhandlers "github.com/kandev/kandev/internal/notifications/handlers"
	"github.com/kandev/kandev/internal/orchestrator"
	promptcontroller "github.com/kandev/kandev/internal/prompts/controller"
	prompthandlers "github.com/kandev/kandev/internal/prompts/handlers"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	usercontroller "github.com/kandev/kandev/internal/user/controller"
	userhandlers "github.com/kandev/kandev/internal/user/handlers"
	workflowcontroller "github.com/kandev/kandev/internal/workflow/controller"
	workflowhandlers "github.com/kandev/kandev/internal/workflow/handlers"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// initDockerClient initializes the Docker client and verifies connectivity.
// Returns nil if Docker is unavailable or the connection check fails.
func initDockerClient(ctx context.Context, cfg *config.Config, log *logger.Logger) *docker.Client {
	dockerClient, err := docker.NewClient(cfg.Docker, log)
	if err != nil {
		log.Warn("Failed to initialize Docker client - agent features will be disabled", zap.Error(err))
		return nil
	}
	if err := dockerClient.Ping(ctx); err != nil {
		log.Warn("Docker daemon not available - agent features will be disabled", zap.Error(err))
		_ = dockerClient.Close()
		return nil
	}
	log.Info("Connected to Docker daemon")
	return dockerClient
}

// buildSessionDataProvider constructs the session data provider function used by the WebSocket hub
// to send initial data (git status, context window, available commands) when a client subscribes.
func buildSessionDataProvider(taskRepo repository.Repository, lifecycleMgr *lifecycle.Manager, log *logger.Logger) func(context.Context, string) ([]*ws.Message, error) {
	return func(ctx context.Context, sessionID string) ([]*ws.Message, error) {
		session, err := taskRepo.GetTaskSession(ctx, sessionID)
		if err != nil {
			return nil, nil // Session not found, no data to send
		}

		var result []*ws.Message
		result = appendGitStatusMessage(ctx, taskRepo, sessionID, session, result)
		result = appendContextWindowMessage(sessionID, session, result)
		result = appendAvailableCommandsMessage(sessionID, session, lifecycleMgr, result)
		return result, nil
	}
}

// appendGitStatusMessage adds a git status notification from the latest snapshot to result.
func appendGitStatusMessage(ctx context.Context, taskRepo repository.Repository, sessionID string, session *models.TaskSession, result []*ws.Message) []*ws.Message {
	latestSnapshot, err := taskRepo.GetLatestGitSnapshot(ctx, sessionID)
	if err != nil || latestSnapshot == nil {
		return result
	}
	metadata := latestSnapshot.Metadata
	gitStatusData := map[string]interface{}{
		"session_id":    sessionID,
		"task_id":       session.TaskID,
		"branch":        latestSnapshot.Branch,
		"remote_branch": latestSnapshot.RemoteBranch,
		"ahead":         latestSnapshot.Ahead,
		"behind":        latestSnapshot.Behind,
		"files":         latestSnapshot.Files,
		"modified":      metadata["modified"],
		"added":         metadata["added"],
		"deleted":       metadata["deleted"],
		"untracked":     metadata["untracked"],
		"renamed":       metadata["renamed"],
		"timestamp":     metadata["timestamp"],
	}
	notification, err := ws.NewNotification("session.git.status", gitStatusData)
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

// newMessageAddedHandler returns an event bus handler that broadcasts message.added events
// to WebSocket subscribers.
func newMessageAddedHandler(gateway *gateways.Gateway, log *logger.Logger) func(context.Context, *bus.Event) error {
	return func(ctx context.Context, event *bus.Event) error {
		data, ok := event.Data.(map[string]interface{})
		if !ok {
			return nil
		}
		taskSessionID, _ := data["session_id"].(string)
		taskID, _ := data["task_id"].(string)
		if taskSessionID == "" {
			return nil
		}
		payload := map[string]interface{}{
			"task_id":        taskID,
			"session_id":     taskSessionID,
			"message_id":     data["message_id"],
			"author_type":    data["author_type"],
			"author_id":      data["author_id"],
			"content":        data["content"],
			"type":           data["type"],
			"requests_input": data["requests_input"],
			"created_at":     data["created_at"],
		}
		if metadata, ok := data["metadata"]; ok && metadata != nil {
			payload["metadata"] = metadata
		}
		notification, err := ws.NewNotification(ws.ActionSessionMessageAdded, payload)
		if err != nil {
			log.Error("Failed to create message.added notification", zap.Error(err))
			return nil
		}
		gateway.Hub.BroadcastToSession(taskSessionID, notification)
		return nil
	}
}

// newMessageUpdatedHandler returns an event bus handler that broadcasts message.updated events
// to WebSocket subscribers.
func newMessageUpdatedHandler(gateway *gateways.Gateway, log *logger.Logger) func(context.Context, *bus.Event) error {
	return func(ctx context.Context, event *bus.Event) error {
		data, ok := event.Data.(map[string]interface{})
		if !ok {
			return nil
		}
		taskSessionID, _ := data["session_id"].(string)
		taskID, _ := data["task_id"].(string)
		if taskSessionID == "" {
			log.Warn("message.updated event has no session_id, skipping")
			return nil
		}
		payload := map[string]interface{}{
			"message_id":     data["message_id"],
			"session_id":     taskSessionID,
			"task_id":        taskID,
			"author_type":    data["author_type"],
			"author_id":      data["author_id"],
			"content":        data["content"],
			"type":           data["type"],
			"requests_input": data["requests_input"],
			"created_at":     data["created_at"],
		}
		if metadata, ok := data["metadata"]; ok && metadata != nil {
			payload["metadata"] = metadata
		}
		notification, err := ws.NewNotification(ws.ActionSessionMessageUpdated, payload)
		if err != nil {
			log.Error("Failed to create message.updated notification", zap.Error(err))
			return nil
		}
		gateway.Hub.BroadcastToSession(taskSessionID, notification)
		return nil
	}
}

// newSessionStateChangedHandler returns an event bus handler that broadcasts
// task_session.state_changed events to WebSocket subscribers.
func newSessionStateChangedHandler(gateway *gateways.Gateway, log *logger.Logger) func(context.Context, *bus.Event) error {
	return func(ctx context.Context, event *bus.Event) error {
		data, ok := event.Data.(map[string]interface{})
		if !ok {
			return nil
		}
		taskSessionID, _ := data["session_id"].(string)
		if taskSessionID == "" {
			return nil
		}
		notification, err := ws.NewNotification(ws.ActionSessionStateChanged, data)
		if err != nil {
			log.Error("Failed to create task_session.state_changed notification", zap.Error(err))
			return nil
		}
		gateway.Hub.BroadcastToSession(taskSessionID, notification)
		return nil
	}
}

// routeParams holds all dependencies needed for HTTP and WebSocket route registration.
type routeParams struct {
	router                  *gin.Engine
	gateway                 *gateways.Gateway
	taskSvc                 *taskservice.Service
	taskRepo                repository.Repository
	analyticsRepo           analyticsrepository.Repository
	orchestratorSvc         *orchestrator.Service
	lifecycleMgr            *lifecycle.Manager
	eventBus                bus.EventBus
	services                *Services
	agentSettingsController *agentsettingscontroller.Controller
	userCtrl                *usercontroller.Controller
	notificationCtrl        *notificationcontroller.Controller
	editorCtrl              *editorcontroller.Controller
	promptCtrl              *promptcontroller.Controller
	msgCreator              *messageCreatorAdapter
	log                     *logger.Logger
}

// registerRoutes sets up all HTTP and WebSocket routes on the given router.
func registerRoutes(p routeParams) {
	workflowCtrl := workflowcontroller.NewController(p.services.Workflow)
	planService := taskservice.NewPlanService(p.taskRepo, p.eventBus, p.log)
	clarificationStore := clarification.NewStore(10 * time.Minute)

	p.gateway.SetupRoutes(p.router)
	registerTaskRoutes(p, planService)
	registerSecondaryRoutes(p, workflowCtrl, clarificationStore, planService)

	p.router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "kandev", "mode": "websocket+http"})
	})
}

// registerTaskRoutes registers all task-related HTTP and WebSocket routes.
func registerTaskRoutes(p routeParams, planService *taskservice.PlanService) {
	taskhandlers.RegisterWorkspaceRoutes(p.router, p.gateway.Dispatcher, p.taskSvc, p.log)
	taskhandlers.RegisterWorkflowRoutes(p.router, p.gateway.Dispatcher, p.taskSvc, p.services.Workflow, p.log)
	taskhandlers.RegisterTaskRoutes(p.router, p.gateway.Dispatcher, p.taskSvc, p.orchestratorSvc, p.taskRepo, planService, p.log)
	taskhandlers.RegisterRepositoryRoutes(p.router, p.gateway.Dispatcher, p.taskSvc, p.log)
	taskhandlers.RegisterExecutorRoutes(p.router, p.gateway.Dispatcher, p.taskSvc, p.log)
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

	clarification.RegisterRoutes(p.router, clarificationStore, p.gateway.Hub, p.msgCreator, p.taskRepo, p.log)
	p.log.Debug("Registered Clarification handlers (HTTP)")

	registerMCPAndDebugRoutes(p, workflowCtrl, clarificationStore, planService)
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
		clarificationStore, p.msgCreator, p.taskRepo, p.taskRepo, p.eventBus, planService, p.log,
	)
	mcpHandlers.RegisterHandlers(p.gateway.Dispatcher)
	p.log.Debug("Registered MCP handlers (WebSocket)")

	p.lifecycleMgr.SetMCPHandler(p.gateway.Dispatcher)
	p.log.Debug("MCP handler configured for agent lifecycle manager")

	debughandlers.RegisterRoutes(p.router, p.log)
	p.log.Debug("Registered Debug handlers (HTTP)")
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
