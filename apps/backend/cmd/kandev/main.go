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
	"github.com/kandev/kandev/internal/common/httpmw"
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
	agentsettingshandlers "github.com/kandev/kandev/internal/agent/settings/handlers"

	editorcontroller "github.com/kandev/kandev/internal/editors/controller"
	editorhandlers "github.com/kandev/kandev/internal/editors/handlers"
	notificationcontroller "github.com/kandev/kandev/internal/notifications/controller"
	notificationhandlers "github.com/kandev/kandev/internal/notifications/handlers"
	promptcontroller "github.com/kandev/kandev/internal/prompts/controller"
	prompthandlers "github.com/kandev/kandev/internal/prompts/handlers"

	"github.com/kandev/kandev/internal/clarification"

	// Debug handlers
	debughandlers "github.com/kandev/kandev/internal/debug"

	// MCP handlers
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"

	// Task Service packages
	taskcontroller "github.com/kandev/kandev/internal/task/controller"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	taskservice "github.com/kandev/kandev/internal/task/service"
	usercontroller "github.com/kandev/kandev/internal/user/controller"
	userhandlers "github.com/kandev/kandev/internal/user/handlers"

	// Workflow packages
	workflowcontroller "github.com/kandev/kandev/internal/workflow/controller"
	workflowhandlers "github.com/kandev/kandev/internal/workflow/handlers"

	// API types
	ws "github.com/kandev/kandev/pkg/websocket"
)

// Command-line flags
var (
	flagPort     = flag.Int("port", 0, "HTTP server port (default: 8080)")
	flagLogLevel = flag.String("log-level", "", "Log level: debug, info, warn, error")
	flagHelp     = flag.Bool("help", false, "Show help message")
	flagVersion  = flag.Bool("version", false, "Show version information")
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: kandev [options]\n\n")
		fmt.Fprintf(os.Stderr, "Kandev is an AI-powered development task orchestrator.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  kandev                              # Start with default settings\n")
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
	cleanupsRan := false
	runCleanups := func() {
		if cleanupsRan {
			return
		}
		cleanupsRan = true
		for i := len(cleanups) - 1; i >= 0; i-- {
			if cleanups[i] == nil {
				continue
			}
			if err := cleanups[i](); err != nil {
				log.Warn("cleanup failed", zap.Error(err))
			}
		}
	}
	defer func() {
		runCleanups()
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
	var gateway *gateways.Gateway
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
	// AGENT MANAGER
	// ============================================
	// Note: MCP server is now embedded in agentctl (co-located with agents)
	// and tunnels tool calls back to the backend via WebSocket.
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

	// Set task service as workspace info provider for session recovery
	if lifecycleMgr != nil {
		lifecycleMgr.SetWorkspaceInfoProvider(taskSvc)
		log.Info("Workspace info provider configured for session recovery")
	}

	// ============================================
	// ORCHESTRATOR
	// ============================================
	log.Info("Initializing Orchestrator...")

	orchestratorSvc, msgCreator, err := provideOrchestrator(log, eventBus, taskRepo, taskSvc, userSvc, lifecycleMgr, agentRegistry, services.Workflow)
	if err != nil {
		log.Fatal("Failed to initialize orchestrator", zap.Error(err))
	}

	// ============================================
	// WEBSOCKET GATEWAY (All communication via WebSocket)
	// ============================================
	log.Info("Initializing WebSocket Gateway...")
	gateway, _, notificationCtrl, err = provideGateway(
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

	// Note: Hub is started and TaskNotifications/UserNotifications/TaskSessionStateChanged
	// are already registered by provideGateway.
	// Only register SessionStreamNotifications here as it's not part of provideGateway
	gateways.RegisterSessionStreamNotifications(ctx, eventBus, gateway.Hub, log)

	// Set up session data provider for session subscriptions
	// Sends initial git status when a client subscribes to a session
	gateway.Hub.SetSessionDataProvider(func(ctx context.Context, sessionID string) ([]*ws.Message, error) {
		session, err := taskRepo.GetTaskSession(ctx, sessionID)
		if err != nil {
			return nil, nil // Session not found, no data to send
		}

		var result []*ws.Message

		// Send git status from latest snapshot
		latestSnapshot, err := taskRepo.GetLatestGitSnapshot(ctx, sessionID)
		if err == nil && latestSnapshot != nil {
			// Extract file lists from snapshot metadata
			metadata := latestSnapshot.Metadata
			modified, _ := metadata["modified"].([]interface{})
			added, _ := metadata["added"].([]interface{})
			deleted, _ := metadata["deleted"].([]interface{})
			untracked, _ := metadata["untracked"].([]interface{})
			renamed, _ := metadata["renamed"].([]interface{})
			timestamp, _ := metadata["timestamp"].(string)

			gitStatusData := map[string]interface{}{
				"session_id":    sessionID,
				"task_id":       session.TaskID,
				"branch":        latestSnapshot.Branch,
				"remote_branch": latestSnapshot.RemoteBranch,
				"ahead":         latestSnapshot.Ahead,
				"behind":        latestSnapshot.Behind,
				"files":         latestSnapshot.Files,
				"modified":      modified,
				"added":         added,
				"deleted":       deleted,
				"untracked":     untracked,
				"renamed":       renamed,
				"timestamp":     timestamp,
			}

			gitStatusNotification, err := ws.NewNotification("session.git.status", gitStatusData)
			if err == nil {
				result = append(result, gitStatusNotification)
			}
		}

		// Send context window if available in session metadata
		if session.Metadata != nil {
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

		// Send available commands from the execution (for slash command menu after page refresh)
		if lifecycleMgr != nil {
			commands := lifecycleMgr.GetAvailableCommandsForSession(sessionID)
			if len(commands) > 0 {
				notification, err := ws.NewNotification(ws.ActionSessionAvailableCommands, map[string]interface{}{
					"session_id":         sessionID,
					"task_id":            session.TaskID,
					"available_commands": commands,
				})
				if err == nil {
					result = append(result, notification)
				}
			}
		}

		return result, nil
	})
	log.Info("Session data provider configured for session subscriptions (git status from snapshots)")

	waitForAgentctlControlHealthy(ctx, cfg, log)
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
		log.Debug("Subscribed to message.added events for WebSocket broadcasting")
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
		log.Debug("Subscribed to message.updated events for WebSocket broadcasting")
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
		log.Debug("Subscribed to task_session.state_changed events for WebSocket broadcasting")
	}

	// ============================================
	// HTTP SERVER (WebSocket + HTTP endpoints)
	// ============================================
	if cfg.Logging.Level != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(httpmw.RequestLogger(log, "kandev"))
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())

	workspaceController := taskcontroller.NewWorkspaceController(taskSvc)
	boardController := taskcontroller.NewBoardController(taskSvc)
	boardController.SetWorkflowStepLister(services.Workflow)
	taskController := taskcontroller.NewTaskController(taskSvc)
	messageController := taskcontroller.NewMessageController(taskSvc)
	repositoryController := taskcontroller.NewRepositoryController(taskSvc)
	executorController := taskcontroller.NewExecutorController(taskSvc)
	environmentController := taskcontroller.NewEnvironmentController(taskSvc)
	workflowCtrl := workflowcontroller.NewController(services.Workflow)

	// WebSocket endpoint - primary realtime transport
	gateway.SetupRoutes(router)

	// Task Service handlers (HTTP + WebSocket)
	taskhandlers.RegisterWorkspaceRoutes(router, gateway.Dispatcher, workspaceController, log)
	taskhandlers.RegisterBoardRoutes(router, gateway.Dispatcher, boardController, log)
	planService := taskservice.NewPlanService(taskRepo, eventBus, log)
	taskhandlers.RegisterTaskRoutes(router, gateway.Dispatcher, taskController, orchestratorSvc, taskRepo, planService, log)
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
	taskhandlers.RegisterProcessRoutes(router, taskSvc, lifecycleMgr, log)
	log.Debug("Registered Task Service handlers (HTTP + WebSocket)")

	workflowhandlers.RegisterRoutes(router, gateway.Dispatcher, workflowCtrl, log)
	log.Info("Registered Workflow handlers (HTTP + WebSocket)")

	agentsettingshandlers.RegisterRoutes(router, agentSettingsController, gateway.Hub, log)
	log.Debug("Registered Agent Settings handlers (HTTP)")

	userhandlers.RegisterRoutes(router, gateway.Dispatcher, userCtrl, log)
	log.Debug("Registered User handlers (HTTP + WebSocket)")

	notificationhandlers.RegisterRoutes(router, notificationCtrl, log)
	log.Debug("Registered Notification handlers (HTTP)")

	editorhandlers.RegisterRoutes(router, editorCtrl, log)
	log.Debug("Registered Editors handlers (HTTP)")

	prompthandlers.RegisterRoutes(router, promptCtrl, log)
	log.Debug("Registered Prompts handlers (HTTP)")

	// Clarification routes for agent questions
	clarificationStore := clarification.NewStore(10 * time.Minute)
	clarification.RegisterRoutes(router, clarificationStore, gateway.Hub, msgCreator, taskRepo, log)
	log.Debug("Registered Clarification handlers (HTTP)")

	// MCP handlers for agentctl WS tunnel (agentctl -> backend)
	mcpHandlers := mcphandlers.NewHandlers(
		workspaceController,
		boardController,
		taskController,
		workflowCtrl,
		clarificationStore,
		msgCreator,
		planService,
		log,
	)
	mcpHandlers.RegisterHandlers(gateway.Dispatcher)
	log.Debug("Registered MCP handlers (WebSocket)")

	// Set MCP handler for lifecycle manager to dispatch MCP requests from agents
	// MCP requests flow: agent -> agentctl -> agent stream (WS) -> backend -> dispatcher
	lifecycleMgr.SetMCPHandler(gateway.Dispatcher)
	log.Debug("MCP handler configured for agent lifecycle manager")

	debughandlers.RegisterRoutes(router, log)
	log.Debug("Registered Debug handlers (HTTP)")

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
		_ = log.Sync()
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

	// Run all cleanups before exiting so logs appear before the shell prompt
	runCleanups()

	log.Info("Kandev stopped")
	_ = log.Sync()
}
