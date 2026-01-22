// Package main is the unified entry point for Kandev.
// This single binary runs all services together with shared infrastructure.
// The server exposes WebSocket and HTTP endpoints.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	// Common packages
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"

	// Event bus
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"

	// WebSocket gateway
	gateways "github.com/kandev/kandev/internal/gateway/websocket"

	// Agent Manager packages
	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	agentsettingshandlers "github.com/kandev/kandev/internal/agent/settings/handlers"

	editorcontroller "github.com/kandev/kandev/internal/editors/controller"
	editorhandlers "github.com/kandev/kandev/internal/editors/handlers"
	notificationcontroller "github.com/kandev/kandev/internal/notifications/controller"
	notificationhandlers "github.com/kandev/kandev/internal/notifications/handlers"
	notificationservice "github.com/kandev/kandev/internal/notifications/service"
	promptcontroller "github.com/kandev/kandev/internal/prompts/controller"
	prompthandlers "github.com/kandev/kandev/internal/prompts/handlers"

	// Task Service packages
	taskcontroller "github.com/kandev/kandev/internal/task/controller"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	usercontroller "github.com/kandev/kandev/internal/user/controller"
	userhandlers "github.com/kandev/kandev/internal/user/handlers"

	// API types
	ws "github.com/kandev/kandev/pkg/websocket"
)

// Command-line flags
var (
	flagAgentRuntime = flag.String("agent-runtime", "", "Agent runtime mode: 'docker' or 'standalone'")
	flagPort         = flag.Int("port", 0, "HTTP server port (default: 8080)")
	flagLogLevel     = flag.String("log-level", "", "Log level: debug, info, warn, error")
	flagHelp         = flag.Bool("help", false, "Show help message")
	flagVersion      = flag.Bool("version", false, "Show version information")
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: kandev [options]\n\n")
		fmt.Fprintf(os.Stderr, "Kandev is an AI-powered development task orchestrator.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  kandev                              # Start with Docker runtime (default)\n")
		fmt.Fprintf(os.Stderr, "  kandev -agent-runtime=standalone    # Start with standalone agentctl\n")
		fmt.Fprintf(os.Stderr, "  kandev -port=9000 -log-level=debug  # Custom port and log level\n")
	}
}

func main() {
	flag.Parse()

	if *flagHelp {
		flag.Usage()
		os.Exit(0)
	}

	if *flagVersion {
		fmt.Println("kandev version 0.1.0")
		os.Exit(0)
	}

	// 1. Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Apply command-line flag overrides (flags take precedence over config/env)
	if *flagAgentRuntime != "" {
		cfg.Agent.Runtime = *flagAgentRuntime
	}
	if *flagPort > 0 {
		cfg.Server.Port = *flagPort
	}
	if *flagLogLevel != "" {
		cfg.Logging.Level = *flagLogLevel
	}

	// 2. Initialize logger
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	cleanups := make([]func() error, 0)
	defer func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			if cleanups[i] == nil {
				continue
			}
			if err := cleanups[i](); err != nil {
				log.Warn("cleanup failed", zap.Error(err))
			}
		}
		_ = log.Sync()
	}()
	logger.SetDefault(log)

	log.Info("Starting Kandev (unified mode)...")

	// 3. Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 4. Initialize event bus (in-memory for unified mode, or NATS if configured)
	eventBusProvider, cleanup, err := events.Provide(cfg, log)
	if err != nil {
		log.Fatal("Failed to initialize event bus", zap.Error(err))
	}
	cleanups = append(cleanups, cleanup)
	eventBus := eventBusProvider.Bus

	// 5. Initialize Docker client
	dockerClient, err := docker.NewClient(cfg.Docker, log)
	if err != nil {
		log.Warn("Failed to initialize Docker client - agent features will be disabled", zap.Error(err))
		dockerClient = nil
	} else {
		if err := dockerClient.Ping(ctx); err != nil {
			log.Warn("Docker daemon not available - agent features will be disabled", zap.Error(err))
			_ = dockerClient.Close()
			dockerClient = nil
		} else {
			defer func() {
				_ = dockerClient.Close()
			}()
			log.Info("Connected to Docker daemon")
		}
	}

	// ============================================
	// TASK SERVICE
	// ============================================
	log.Info("Initializing Task Service...")

	dbConn, repos, repoCleanups, err := provideRepositories(cfg, log)
	if err != nil {
		log.Fatal("Failed to initialize repositories", zap.Error(err))
	}
	cleanups = append(cleanups, repoCleanups...)

	services, agentSettingsController, err := provideServices(cfg, log, repos, eventBus)
	if err != nil {
		log.Fatal("Failed to initialize services", zap.Error(err))
	}
	taskRepo := repos.Task
	agentSettingsRepo := repos.AgentSettings
	notificationRepo := repos.Notification
	taskSvc := services.Task
	userSvc := services.User
	editorSvc := services.Editor
	promptSvc := services.Prompts
	log.Info("Task Service initialized")

	userCtrl := usercontroller.NewController(userSvc)
	var notificationSvc *notificationservice.Service
	var notificationCtrl *notificationcontroller.Controller
	editorCtrl := editorcontroller.NewController(editorSvc)
	promptCtrl := promptcontroller.NewController(promptSvc)

	if err := runInitialAgentSetup(ctx, userSvc, agentSettingsController, log); err != nil {
		log.Warn("Failed to run initial agent setup", zap.Error(err))
	}

	// ACP messages are now stored as comments in the task_comments table
	// The comment system provides unified storage for all task-related communication
	log.Info("ACP messages will be stored as comments")

	// ============================================
	// AGENTCTL LAUNCHER (for standalone mode)
	// ============================================
	var agentctlCleanup func() error
	agentctlCleanup, err = provideAgentctlLauncher(ctx, cfg, log)
	if err != nil {
		log.Fatal("Failed to start agentctl subprocess", zap.Error(err))
	}
	if agentctlCleanup != nil {
		cleanups = append(cleanups, agentctlCleanup)
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic recovered, stopping agentctl", zap.Any("panic", r))
				if err := agentctlCleanup(); err != nil {
					log.Error("failed to stop agentctl on panic", zap.Error(err))
				}
				panic(r)
			}
		}()
	}

	// ============================================
	// MCP SERVER (embedded)
	// ============================================
	mcpServerURL, mcpCleanup, err := provideMcpServer(ctx, cfg, log)
	if err != nil {
		log.Fatal("Failed to start MCP server", zap.Error(err))
	}
	if mcpCleanup != nil {
		cleanups = append(cleanups, mcpCleanup)
		log.Info("MCP server started", zap.String("url", mcpServerURL))
		// Auto-set the MCP server URL if not explicitly configured
		if cfg.Agent.McpServerURL == "" {
			cfg.Agent.McpServerURL = mcpServerURL
		}
	}

	// ============================================
	// AGENT MANAGER
	// ============================================
	lifecycleMgr, agentRegistry, err := provideLifecycleManager(ctx, cfg, log, eventBus, dockerClient, agentSettingsRepo)
	if err != nil {
		log.Fatal("Failed to initialize agent manager", zap.Error(err))
	}

	// ============================================
	// WORKTREE MANAGER
	// ============================================
	log.Info("Initializing Worktree Manager...")

	var worktreeCleanup func() error
	_, worktreeCleanup, err = provideWorktreeManager(dbConn, cfg, log, lifecycleMgr, taskSvc)
	if err != nil {
		log.Fatal("Failed to initialize worktree manager", zap.Error(err))
	}
	cleanups = append(cleanups, worktreeCleanup)
	log.Info("Worktree Manager initialized",
		zap.Bool("enabled", cfg.Worktree.Enabled),
		zap.String("base_path", cfg.Worktree.BasePath))

	// ============================================
	// ORCHESTRATOR
	// ============================================
	log.Info("Initializing Orchestrator...")

	orchestratorSvc, err := provideOrchestrator(log, eventBus, taskRepo, taskSvc, userSvc, lifecycleMgr, agentRegistry)
	if err != nil {
		log.Fatal("Failed to initialize orchestrator", zap.Error(err))
	}

	// ============================================
	// WEBSOCKET GATEWAY (All communication via WebSocket)
	// ============================================
	log.Info("Initializing WebSocket Gateway...")
	gateway, _, _, err := provideGateway(
		ctx,
		log,
		eventBus,
		taskSvc,
		userSvc,
		orchestratorSvc,
		lifecycleMgr,
		agentRegistry,
		notificationRepo,
		taskRepo,
	)
	if err != nil {
		log.Fatal("Failed to initialize WebSocket gateway", zap.Error(err))
	}

	// Start the WebSocket hub
	go gateway.Hub.Run(ctx)
	gateways.RegisterTaskNotifications(ctx, eventBus, gateway.Hub, log)
	gateways.RegisterUserNotifications(ctx, eventBus, gateway.Hub, log)

	notificationSvc = notificationservice.NewService(notificationRepo, taskRepo, gateway.Hub, log)
	notificationCtrl = notificationcontroller.NewController(notificationSvc)
	if eventBus != nil {
		_, err = eventBus.Subscribe(events.TaskSessionStateChanged, func(ctx context.Context, event *bus.Event) error {
			data, ok := event.Data.(map[string]interface{})
			if !ok {
				return nil
			}
			taskID, _ := data["task_id"].(string)
			sessionID, _ := data["session_id"].(string)
			newState, _ := data["new_state"].(string)
			notificationSvc.HandleTaskSessionStateChanged(ctx, taskID, sessionID, newState)
			return nil
		})
		if err != nil {
			log.Error("Failed to subscribe to task session notifications", zap.Error(err))
		}
	}

	// Set up session data provider for session subscriptions
	// Sends initial git status when a client subscribes to a session
	gateway.Hub.SetSessionDataProvider(func(ctx context.Context, sessionID string) ([]*ws.Message, error) {
		session, err := taskRepo.GetTaskSession(ctx, sessionID)
		if err != nil {
			return nil, nil // Session not found, no data to send
		}

		var result []*ws.Message

		// Send git status if available in session metadata
		if session.Metadata != nil {
			if gitStatus, ok := session.Metadata["git_status"]; ok {
				gitStatusData, ok := gitStatus.(map[string]interface{})
				if !ok {
					gitStatusData = map[string]interface{}{"data": gitStatus}
				}
				// Ensure session_id is included
				gitStatusData["session_id"] = sessionID
				if session.TaskID != "" {
					gitStatusData["task_id"] = session.TaskID
				}

				gitStatusNotification, err := ws.NewNotification("session.git.status", gitStatusData)
				if err == nil {
					result = append(result, gitStatusNotification)
				}
			}

			if contextWindow, ok := session.Metadata["context_window"]; ok {
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
			}
		}

		return result, nil
	})
	log.Info("Session data provider configured for session subscriptions (git status)")

	if err := orchestratorSvc.Start(ctx); err != nil {
		log.Fatal("Failed to start orchestrator", zap.Error(err))
	}
	log.Info("Orchestrator initialized")

	// Subscribe to message.added events and broadcast to WebSocket subscribers
	_, err = eventBus.Subscribe(events.MessageAdded, func(ctx context.Context, event *bus.Event) error {
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
	})
	if err != nil {
		log.Error("Failed to subscribe to message.added events", zap.Error(err))
	} else {
		log.Info("Subscribed to message.added events for WebSocket broadcasting")
	}

	// Subscribe to message.updated events and broadcast to WebSocket subscribers
	_, err = eventBus.Subscribe(events.MessageUpdated, func(ctx context.Context, event *bus.Event) error {
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
	})
	if err != nil {
		log.Error("Failed to subscribe to message.updated events", zap.Error(err))
	} else {
		log.Info("Subscribed to message.updated events for WebSocket broadcasting")
	}

	// Subscribe to task_session.state_changed events and broadcast to task subscribers
	_, err = eventBus.Subscribe(events.TaskSessionStateChanged, func(ctx context.Context, event *bus.Event) error {
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
	})
	if err != nil {
		log.Error("Failed to subscribe to task_session.state_changed events", zap.Error(err))
	} else {
		log.Info("Subscribed to task_session.state_changed events for WebSocket broadcasting")
	}

	// Subscribe to git status events and broadcast to WebSocket subscribers
	_, err = eventBus.Subscribe(events.BuildGitStatusWildcardSubject(), func(ctx context.Context, event *bus.Event) error {
		// Extract session_id from the event data
		var taskSessionID string
		switch data := event.Data.(type) {
		case lifecycle.GitStatusEventPayload:
			taskSessionID = data.SessionID
		case *lifecycle.GitStatusEventPayload:
			taskSessionID = data.SessionID
		case map[string]interface{}:
			taskSessionID, _ = data["session_id"].(string)
		}
		if taskSessionID == "" {
			return nil
		}

		// Broadcast git status update to session subscribers
		notification, _ := ws.NewNotification("session.git.status", event.Data)
		gateway.Hub.BroadcastToSession(taskSessionID, notification)
		return nil
	})
	if err != nil {
		log.Error("Failed to subscribe to git status events", zap.Error(err))
	} else {
		log.Info("Subscribed to git status events for WebSocket broadcasting")
	}

	// Subscribe to file change events and broadcast to WebSocket subscribers
	_, err = eventBus.Subscribe(events.BuildFileChangeWildcardSubject(), func(ctx context.Context, event *bus.Event) error {
		// Extract session_id from the event data
		var taskSessionID string
		switch data := event.Data.(type) {
		case lifecycle.FileChangeEventPayload:
			taskSessionID = data.SessionID
		case *lifecycle.FileChangeEventPayload:
			taskSessionID = data.SessionID
		case map[string]interface{}:
			taskSessionID, _ = data["session_id"].(string)
		}
		if taskSessionID == "" {
			return nil
		}

		// Broadcast file change notification to session subscribers
		notification, _ := ws.NewNotification(ws.ActionWorkspaceFileChanges, event.Data)
		gateway.Hub.BroadcastToSession(taskSessionID, notification)
		return nil
	})
	if err != nil {
		log.Error("Failed to subscribe to file change events", zap.Error(err))
	} else {
		log.Info("Subscribed to file change events for WebSocket broadcasting")
	}

	// Subscribe to shell output events and broadcast to WebSocket subscribers
	_, err = eventBus.Subscribe(events.BuildShellOutputWildcardSubject(), func(ctx context.Context, event *bus.Event) error {
		// Extract session_id from the event data
		var taskSessionID string
		switch data := event.Data.(type) {
		case lifecycle.ShellOutputEventPayload:
			taskSessionID = data.SessionID
		case *lifecycle.ShellOutputEventPayload:
			taskSessionID = data.SessionID
		case map[string]interface{}:
			taskSessionID, _ = data["session_id"].(string)
		}
		if taskSessionID == "" {
			return nil
		}

		// Broadcast shell output to session subscribers
		notification, _ := ws.NewNotification(ws.ActionSessionShellOutput, event.Data)
		gateway.Hub.BroadcastToSession(taskSessionID, notification)
		return nil
	})
	if err != nil {
		log.Error("Failed to subscribe to shell output events", zap.Error(err))
	} else {
		log.Info("Subscribed to shell output events for WebSocket broadcasting")
	}

	// Subscribe to shell exit events and broadcast to WebSocket subscribers
	// Note: Frontend expects shell exit via session.shell.output with type: "exit"
	_, err = eventBus.Subscribe(events.BuildShellExitWildcardSubject(), func(ctx context.Context, event *bus.Event) error {
		// Extract session_id from the event data
		var taskSessionID string
		switch data := event.Data.(type) {
		case lifecycle.ShellExitEventPayload:
			taskSessionID = data.SessionID
		case *lifecycle.ShellExitEventPayload:
			taskSessionID = data.SessionID
		case map[string]interface{}:
			taskSessionID, _ = data["session_id"].(string)
		}
		if taskSessionID == "" {
			return nil
		}

		// Broadcast shell exit to session subscribers via session.shell.output action
		// Frontend handles exit events via type: "exit" in the same handler
		notification, _ := ws.NewNotification(ws.ActionSessionShellOutput, event.Data)
		gateway.Hub.BroadcastToSession(taskSessionID, notification)
		return nil
	})
	if err != nil {
		log.Error("Failed to subscribe to shell exit events", zap.Error(err))
	} else {
		log.Info("Subscribed to shell exit events for WebSocket broadcasting")
	}

	// ============================================
	// HTTP SERVER (WebSocket + HTTP endpoints)
	// ============================================
	if cfg.Logging.Level != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())

	workspaceController := taskcontroller.NewWorkspaceController(taskSvc)
	boardController := taskcontroller.NewBoardController(taskSvc)
	taskController := taskcontroller.NewTaskController(taskSvc)
	messageController := taskcontroller.NewMessageController(taskSvc)
	repositoryController := taskcontroller.NewRepositoryController(taskSvc)
	executorController := taskcontroller.NewExecutorController(taskSvc)
	environmentController := taskcontroller.NewEnvironmentController(taskSvc)

	// WebSocket endpoint - primary realtime transport
	gateway.SetupRoutes(router)

	// Task Service handlers (HTTP + WebSocket)
	taskhandlers.RegisterWorkspaceRoutes(router, gateway.Dispatcher, workspaceController, log)
	taskhandlers.RegisterBoardRoutes(router, gateway.Dispatcher, boardController, log)
	taskhandlers.RegisterTaskRoutes(router, gateway.Dispatcher, taskController, orchestratorSvc, log)
	taskhandlers.RegisterRepositoryRoutes(router, gateway.Dispatcher, repositoryController, log)
	taskhandlers.RegisterExecutorRoutes(router, gateway.Dispatcher, executorController, log)
	taskhandlers.RegisterEnvironmentRoutes(router, gateway.Dispatcher, environmentController, log)
	taskhandlers.RegisterMessageRoutes(
		router,
		gateway.Dispatcher,
		messageController,
		taskController,
		&orchestratorAdapter{svc: orchestratorSvc},
		log,
	)
	log.Info("Registered Task Service handlers (HTTP + WebSocket)")

	agentsettingshandlers.RegisterRoutes(router, agentSettingsController, gateway.Hub, log)
	log.Info("Registered Agent Settings handlers (HTTP)")

	userhandlers.RegisterRoutes(router, gateway.Dispatcher, userCtrl, log)
	log.Info("Registered User handlers (HTTP + WebSocket)")

	notificationhandlers.RegisterRoutes(router, notificationCtrl, log)
	log.Info("Registered Notification handlers (HTTP)")

	editorhandlers.RegisterRoutes(router, editorCtrl, log)
	log.Info("Registered Editors handlers (HTTP)")

	prompthandlers.RegisterRoutes(router, promptCtrl, log)
	log.Info("Registered Prompts handlers (HTTP)")

	// Health check (simple HTTP for load balancers/monitoring)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "kandev",
			"mode":    "websocket+http",
		})
	})

	// Create HTTP server
	port := cfg.Server.Port
	if port == 0 {
		port = 8080
	}
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeoutDuration(),
		WriteTimeout: cfg.Server.WriteTimeoutDuration(),
	}

	// Start server
	go func() {
		log.Info("WebSocket server listening", zap.Int("port", port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Print routes summary
	log.Info("API configured",
		zap.String("websocket", "/ws"),
		zap.String("health", "/health"),
		zap.String("http", "/api/v1"),
	)

	// ============================================
	// GRACEFUL SHUTDOWN
	// ============================================
	quit := make(chan os.Signal, 2)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	sig := <-quit

	// If we get a second signal, exit immediately.
	go func() {
		second := <-quit
		log.Warn("Received second shutdown signal, forcing exit", zap.String("signal", second.String()))
		os.Exit(1)
	}()

	log.Info("Received shutdown signal", zap.String("signal", sig.String()))

	log.Info("Shutting down Kandev...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("HTTP server shutdown error", zap.Error(err))
	}

	if err := orchestratorSvc.Stop(); err != nil {
		log.Error("Orchestrator stop error", zap.Error(err))
	}

	if lifecycleMgr != nil {
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

	log.Info("Kandev stopped")
}
