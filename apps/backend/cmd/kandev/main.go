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

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/httpmw"
	"go.uber.org/zap"

	// Common packages
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"

	// Event bus
	"github.com/kandev/kandev/internal/events"

	// WebSocket gateway
	gateways "github.com/kandev/kandev/internal/gateway/websocket"

	editorcontroller "github.com/kandev/kandev/internal/editors/controller"
	notificationcontroller "github.com/kandev/kandev/internal/notifications/controller"
	promptcontroller "github.com/kandev/kandev/internal/prompts/controller"
	usercontroller "github.com/kandev/kandev/internal/user/controller"

	// Worktree package
	"github.com/kandev/kandev/internal/worktree"
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
	cleanups = append(cleanups, func() error { cancel(); return nil })

	// 4. Initialize event bus (in-memory for unified mode, or NATS if configured)
	eventBusProvider, cleanup, err := events.Provide(cfg, log)
	if err != nil {
		log.Fatal("Failed to initialize event bus", zap.Error(err))
	}
	cleanups = append(cleanups, cleanup)
	eventBus := eventBusProvider.Bus

	// 5. Initialize Docker client
	dockerClient := initDockerClient(ctx, cfg, log)
	if dockerClient != nil {
		defer func() { _ = dockerClient.Close() }()
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
	analyticsRepo := repos.Analytics
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
	var worktreeRecreator *worktree.Recreator
	_, worktreeRecreator, worktreeCleanup, err = provideWorktreeManager(dbConn, cfg, log, lifecycleMgr, taskSvc)
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

	orchestratorSvc, msgCreator, err := provideOrchestrator(log, eventBus, taskRepo, taskSvc, userSvc, lifecycleMgr, agentRegistry, services.Workflow, worktreeRecreator)
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
	gateway.Hub.SetSessionDataProvider(buildSessionDataProvider(taskRepo, lifecycleMgr, log))
	log.Info("Session data provider configured for session subscriptions (git status from snapshots)")

	waitForAgentctlControlHealthy(ctx, cfg, log)
	if err := orchestratorSvc.Start(ctx); err != nil {
		log.Fatal("Failed to start orchestrator", zap.Error(err))
	}
	log.Info("Orchestrator initialized")

	// Start auto-archive background loop
	taskSvc.StartAutoArchiveLoop(ctx)

	// Subscribe to message and session events and broadcast to WebSocket subscribers
	if _, err = eventBus.Subscribe(events.MessageAdded, newMessageAddedHandler(gateway, log)); err != nil {
		log.Error("Failed to subscribe to message.added events", zap.Error(err))
	} else {
		log.Debug("Subscribed to message.added events for WebSocket broadcasting")
	}
	if _, err = eventBus.Subscribe(events.MessageUpdated, newMessageUpdatedHandler(gateway, log)); err != nil {
		log.Error("Failed to subscribe to message.updated events", zap.Error(err))
	} else {
		log.Debug("Subscribed to message.updated events for WebSocket broadcasting")
	}
	if _, err = eventBus.Subscribe(events.TaskSessionStateChanged, newSessionStateChangedHandler(gateway, log)); err != nil {
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

	registerRoutes(routeParams{
		router:                  router,
		gateway:                 gateway,
		taskSvc:                 taskSvc,
		taskRepo:                taskRepo,
		analyticsRepo:           analyticsRepo,
		orchestratorSvc:         orchestratorSvc,
		lifecycleMgr:            lifecycleMgr,
		eventBus:                eventBus,
		services:                services,
		agentSettingsController: agentSettingsController,
		userCtrl:                userCtrl,
		notificationCtrl:        notificationCtrl,
		editorCtrl:              editorCtrl,
		promptCtrl:              promptCtrl,
		msgCreator:              msgCreator,
		log:                     log,
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
	cancel()
	runGracefulShutdown(server, orchestratorSvc, lifecycleMgr, runCleanups, log)
}
