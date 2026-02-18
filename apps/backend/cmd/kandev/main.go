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
	"github.com/kandev/kandev/internal/events/bus"

	// Agent infrastructure
	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"

	// WebSocket gateway
	gateways "github.com/kandev/kandev/internal/gateway/websocket"

	editorcontroller "github.com/kandev/kandev/internal/editors/controller"
	notificationcontroller "github.com/kandev/kandev/internal/notifications/controller"
	promptcontroller "github.com/kandev/kandev/internal/prompts/controller"
	usercontroller "github.com/kandev/kandev/internal/user/controller"

	// Orchestrator
	"github.com/kandev/kandev/internal/orchestrator"

	// Database
	"github.com/kandev/kandev/internal/db"
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

// main parses flags and delegates to realMain, using os.Exit with its return code.
// os.Exit is only called from main() before any defers are registered, so that
// deferred cleanup functions inside realMain() always execute.
func main() {
	os.Exit(realMain())
}

// realMain contains all startup logic and returns 0 on success or 1 on fatal error.
// Deferred cleanup is registered here so it always executes before realMain returns.
func realMain() int {
	flag.Parse()

	if *flagHelp {
		flag.Usage()
		return 0
	}

	if *flagVersion {
		fmt.Println("kandev version 0.1.0")
		return 0
	}

	// 1. Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		return 1
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
		return 1
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

	if !run(cfg, log, &cleanups, runCleanups) {
		return 1
	}
	return 0
}

// run initializes all services and runs the server. Returns false on fatal startup error.
func run(cfg *config.Config, log *logger.Logger, cleanups *[]func() error, runCleanups func()) bool {
	addCleanup := func(fn func() error) { *cleanups = append(*cleanups, fn) }

	// 3. Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	addCleanup(func() error { cancel(); return nil })

	// 4. Initialize event bus (in-memory for unified mode, or NATS if configured)
	eventBusProvider, cleanup, err := events.Provide(cfg, log)
	if err != nil {
		log.Error("Failed to initialize event bus", zap.Error(err))
		return false
	}
	addCleanup(cleanup)
	eventBus := eventBusProvider.Bus

	// 5. Initialize Docker client
	dockerClient := initDockerClient(ctx, cfg, log)
	if dockerClient != nil {
		defer func() { _ = dockerClient.Close() }()
	}

	return startServices(ctx, cfg, log, addCleanup, eventBus, dockerClient, runCleanups)
}

// startServices initializes task-level services and all downstream infrastructure.
func startServices( //nolint:cyclop
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	addCleanup func(func() error),
	eventBus bus.EventBus,
	dockerClient *docker.Client,
	runCleanups func(),
) bool {
	// ============================================
	// TASK SERVICE
	// ============================================
	log.Info("Initializing Task Service...")

	dbPool, repos, repoCleanups, err := provideRepositories(cfg, log)
	if err != nil {
		log.Error("Failed to initialize repositories", zap.Error(err))
		return false
	}
	for _, c := range repoCleanups {
		addCleanup(c)
	}

	services, agentSettingsController, err := provideServices(cfg, log, repos, eventBus)
	if err != nil {
		log.Error("Failed to initialize services", zap.Error(err))
		return false
	}
	log.Info("Task Service initialized")

	if err := runInitialAgentSetup(ctx, services.User, agentSettingsController, log); err != nil {
		log.Warn("Failed to run initial agent setup", zap.Error(err))
	}
	log.Info("ACP messages will be stored as comments")

	// ============================================
	// AGENTCTL LAUNCHER (for standalone mode)
	// ============================================
	agentctlCleanup, err := provideAgentctlLauncher(ctx, cfg, log)
	if err != nil {
		log.Error("Failed to start agentctl subprocess", zap.Error(err))
		return false
	}
	if agentctlCleanup != nil {
		addCleanup(agentctlCleanup)
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic recovered, stopping agentctl", zap.Any("panic", r))
				if stopErr := agentctlCleanup(); stopErr != nil {
					log.Error("failed to stop agentctl on panic", zap.Error(stopErr))
				}
				panic(r)
			}
		}()
	}

	return startAgentInfrastructure(ctx, cfg, log, addCleanup, eventBus, dockerClient, dbPool, repos, services, agentSettingsController, runCleanups)
}

// startAgentInfrastructure initializes the agent lifecycle manager, worktree, orchestrator,
// gateway, and HTTP server.
func startAgentInfrastructure(
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	addCleanup func(func() error),
	eventBus bus.EventBus,
	dockerClient *docker.Client,
	dbPool *db.Pool,
	repos *Repositories,
	services *Services,
	agentSettingsController *agentsettingscontroller.Controller,
	runCleanups func(),
) bool {
	// ============================================
	// AGENT MANAGER
	// ============================================
	lifecycleMgr, agentRegistry, err := provideLifecycleManager(ctx, cfg, log, eventBus, dockerClient, repos.AgentSettings)
	if err != nil {
		log.Error("Failed to initialize agent manager", zap.Error(err))
		return false
	}

	// ============================================
	// WORKTREE MANAGER
	// ============================================
	log.Info("Initializing Worktree Manager...")

	_, worktreeRecreator, worktreeCleanup, err := provideWorktreeManager(dbPool, cfg, log, lifecycleMgr, services.Task)
	if err != nil {
		log.Error("Failed to initialize worktree manager", zap.Error(err))
		return false
	}
	addCleanup(worktreeCleanup)
	log.Info("Worktree Manager initialized",
		zap.Bool("enabled", cfg.Worktree.Enabled),
		zap.String("base_path", cfg.Worktree.BasePath))

	lifecycleMgr.SetWorkspaceInfoProvider(services.Task)
	log.Info("Workspace info provider configured for session recovery")

	// ============================================
	// ORCHESTRATOR
	// ============================================
	log.Info("Initializing Orchestrator...")

	orchestratorSvc, msgCreator, err := provideOrchestrator(log, eventBus, repos.Task, services.Task, services.User,
		lifecycleMgr, agentRegistry, services.Workflow, worktreeRecreator)
	if err != nil {
		log.Error("Failed to initialize orchestrator", zap.Error(err))
		return false
	}

	return startGatewayAndServe(ctx, cfg, log, eventBus, repos, services,
		agentSettingsController, lifecycleMgr, agentRegistry, orchestratorSvc, msgCreator, runCleanups)
}

// startGatewayAndServe sets up the WebSocket gateway, HTTP routes, starts the server,
// and blocks until a shutdown signal.
func startGatewayAndServe(
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	eventBus bus.EventBus,
	repos *Repositories,
	services *Services,
	agentSettingsController *agentsettingscontroller.Controller,
	lifecycleMgr *lifecycle.Manager,
	agentRegistry *registry.Registry,
	orchestratorSvc *orchestrator.Service,
	msgCreator *messageCreatorAdapter,
	runCleanups func(),
) bool {
	// ============================================
	// WEBSOCKET GATEWAY
	// ============================================
	log.Info("Initializing WebSocket Gateway...")
	gateway, _, notificationCtrl, err := provideGateway(
		ctx, log, eventBus, services.Task, services.User,
		orchestratorSvc, lifecycleMgr, agentRegistry,
		repos.Notification, repos.Task,
	)
	if err != nil {
		log.Error("Failed to initialize WebSocket gateway", zap.Error(err))
		return false
	}

	gateways.RegisterSessionStreamNotifications(ctx, eventBus, gateway.Hub, log)
	gateway.Hub.SetSessionDataProvider(buildSessionDataProvider(repos.Task, lifecycleMgr, log))
	log.Info("Session data provider configured for session subscriptions (git status from snapshots)")

	waitForAgentctlControlHealthy(ctx, cfg, log)
	if err := orchestratorSvc.Start(ctx); err != nil {
		log.Error("Failed to start orchestrator", zap.Error(err))
		return false
	}
	log.Info("Orchestrator initialized")

	services.Task.StartAutoArchiveLoop(ctx)

	subscribeEventBusHandlers(eventBus, gateway, log)

	// ============================================
	// HTTP SERVER
	// ============================================
	server := buildHTTPServer(cfg, log, gateway, repos, services, agentSettingsController,
		lifecycleMgr, eventBus, orchestratorSvc, notificationCtrl, msgCreator)

	port := cfg.Server.Port
	if port == 0 {
		port = 8080
	}
	go func() {
		log.Info("WebSocket server listening", zap.Int("port", port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("Server listen error", zap.Error(err))
		}
	}()

	log.Info("API configured",
		zap.String("websocket", "/ws"),
		zap.String("health", "/health"),
		zap.String("http", "/api/v1"),
	)

	awaitShutdown(server, orchestratorSvc, lifecycleMgr, runCleanups, log)
	return true
}

// buildHTTPServer creates the HTTP server with all routes registered.
func buildHTTPServer(
	cfg *config.Config,
	log *logger.Logger,
	gateway *gateways.Gateway,
	repos *Repositories,
	services *Services,
	agentSettingsController *agentsettingscontroller.Controller,
	lifecycleMgr *lifecycle.Manager,
	eventBus bus.EventBus,
	orchestratorSvc *orchestrator.Service,
	notificationCtrl *notificationcontroller.Controller,
	msgCreator *messageCreatorAdapter,
) *http.Server {
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
		taskSvc:                 services.Task,
		taskRepo:                repos.Task,
		analyticsRepo:           repos.Analytics,
		orchestratorSvc:         orchestratorSvc,
		lifecycleMgr:            lifecycleMgr,
		eventBus:                eventBus,
		services:                services,
		agentSettingsController: agentSettingsController,
		userCtrl:                usercontroller.NewController(services.User),
		notificationCtrl:        notificationCtrl,
		editorCtrl:              editorcontroller.NewController(services.Editor),
		promptCtrl:              promptcontroller.NewController(services.Prompts),
		msgCreator:              msgCreator,
		log:                     log,
	})

	port := cfg.Server.Port
	if port == 0 {
		port = 8080
	}
	return &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeoutDuration(),
		WriteTimeout: cfg.Server.WriteTimeoutDuration(),
	}
}

// subscribeEventBusHandlers subscribes WebSocket broadcast handlers to the event bus.
func subscribeEventBusHandlers(eventBus bus.EventBus, gateway *gateways.Gateway, log *logger.Logger) {
	if _, err := eventBus.Subscribe(events.MessageAdded, newMessageAddedHandler(gateway, log)); err != nil {
		log.Error("Failed to subscribe to message.added events", zap.Error(err))
	} else {
		log.Debug("Subscribed to message.added events for WebSocket broadcasting")
	}
	if _, err := eventBus.Subscribe(events.MessageUpdated, newMessageUpdatedHandler(gateway, log)); err != nil {
		log.Error("Failed to subscribe to message.updated events", zap.Error(err))
	} else {
		log.Debug("Subscribed to message.updated events for WebSocket broadcasting")
	}
	if _, err := eventBus.Subscribe(events.TaskSessionStateChanged, newSessionStateChangedHandler(gateway, log)); err != nil {
		log.Error("Failed to subscribe to task_session.state_changed events", zap.Error(err))
	} else {
		log.Debug("Subscribed to task_session.state_changed events for WebSocket broadcasting")
	}
}

// awaitShutdown waits for an OS signal then performs graceful shutdown.
func awaitShutdown(
	server *http.Server,
	orchestratorSvc *orchestrator.Service,
	lifecycleMgr *lifecycle.Manager,
	runCleanups func(),
	log *logger.Logger,
) {
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
	runGracefulShutdown(server, orchestratorSvc, lifecycleMgr, runCleanups, log)
}
