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
	"path/filepath"
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

	// GitHub integration
	githubpkg "github.com/kandev/kandev/internal/github"

	// JIRA integration
	jirapkg "github.com/kandev/kandev/internal/jira"

	// Agent infrastructure
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/registry"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	agentctlclient "github.com/kandev/kandev/internal/agentctl/client"

	// WebSocket gateway
	gateways "github.com/kandev/kandev/internal/gateway/websocket"

	editorcontroller "github.com/kandev/kandev/internal/editors/controller"
	notificationcontroller "github.com/kandev/kandev/internal/notifications/controller"
	promptcontroller "github.com/kandev/kandev/internal/prompts/controller"
	usercontroller "github.com/kandev/kandev/internal/user/controller"
	utilitycontroller "github.com/kandev/kandev/internal/utility/controller"

	// Orchestrator
	"github.com/kandev/kandev/internal/orchestrate/configloader"
	orchestrateservice "github.com/kandev/kandev/internal/orchestrate/service"
	"github.com/kandev/kandev/internal/orchestrator"
	v1 "github.com/kandev/kandev/pkg/api/v1"

	// Repository cloning
	"github.com/kandev/kandev/internal/repoclone"

	// Secrets
	"github.com/kandev/kandev/internal/secrets"

	// Database
	"github.com/kandev/kandev/internal/db"

	"github.com/kandev/kandev/internal/common/ports"
)

// Command-line flags
var (
	flagPort     = flag.Int("port", 0, fmt.Sprintf("HTTP server port (default: %d)", ports.Backend))
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
		fmt.Fprintf(os.Stderr, "  kandev                               # Start with default settings\n")
		fmt.Fprintf(os.Stderr, "  kandev -port=18080 -log-level=debug  # Custom port and log level\n")
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

	log.Info("Starting Kandev (unified mode)...",
		zap.String("db_path", cfg.Database.Path),
	)

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

	return startServices(ctx, cfg, log, addCleanup, eventBus, runCleanups)
}

// startServices initializes task-level services and all downstream infrastructure.
func startServices( //nolint:cyclop
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	addCleanup func(func() error),
	eventBus bus.EventBus,
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

	agentRegistry, _, err := registry.Provide(log)
	if err != nil {
		log.Error("Failed to initialize agent registry", zap.Error(err))
		return false
	}

	services, agentSettingsController, err := provideServices(cfg, log, repos, dbPool, eventBus, agentRegistry)
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

	return startAgentInfrastructure(ctx, cfg, log, addCleanup, eventBus, dbPool, repos, services, agentSettingsController, agentRegistry, runCleanups)
}

// startAgentInfrastructure initializes the agent lifecycle manager, worktree, orchestrator,
// gateway, and HTTP server.
func startAgentInfrastructure(
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	addCleanup func(func() error),
	eventBus bus.EventBus,
	dbPool *db.Pool,
	repos *Repositories,
	services *Services,
	agentSettingsController *agentsettingscontroller.Controller,
	agentRegistry *registry.Registry,
	runCleanups func(),
) bool {
	// ============================================
	// AGENT MANAGER
	// ============================================
	lifecycleMgr, err := provideLifecycleManager(ctx, cfg, log, eventBus, repos.AgentSettings, agentRegistry, repos.Secrets)
	if err != nil {
		log.Error("Failed to initialize agent manager", zap.Error(err))
		return false
	}

	// ============================================
	// WORKTREE MANAGER
	// ============================================
	log.Info("Initializing Worktree Manager...")

	_, _, worktreeCleanup, err := provideWorktreeManager(dbPool, cfg, log, lifecycleMgr, services.Task)
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

	// Configure quick-chat workspace cleanup
	if homeDir := cfg.ResolvedHomeDir(); homeDir != "" {
		quickChatDir := filepath.Join(homeDir, "quick-chat")
		services.Task.SetQuickChatDir(quickChatDir)
		log.Info("Quick-chat workspace cleanup configured", zap.String("quick_chat_dir", quickChatDir))
	}

	// ============================================
	// REPO CLONER
	// ============================================
	repoCloner := repoclone.NewCloner(repoclone.Config{
		BasePath: cfg.RepoClone.BasePath,
	}, repoclone.DetectGitProtocol(), cfg.ResolvedHomeDir(), log)
	log.Info("Repository cloner configured",
		zap.String("base_path", cfg.RepoClone.BasePath))

	// ============================================
	// ORCHESTRATOR
	// ============================================
	log.Info("Initializing Orchestrator...")

	orchestratorSvc, msgCreator, err := provideOrchestrator(cfg, log, eventBus, repos.Task, services.Task, services.User,
		lifecycleMgr, agentRegistry, services.Workflow, repos.Secrets, repoCloner)
	if err != nil {
		log.Error("Failed to initialize orchestrator", zap.Error(err))
		return false
	}

	// Wire GitHub service into orchestrator for PR auto-detection on push
	if services.GitHub != nil {
		orchestratorSvc.SetGitHubService(services.GitHub)
		services.GitHub.SetTaskDeleter(services.Task)
		services.GitHub.SetTaskSessionChecker(&taskSessionCheckerAdapter{repo: repos.Task})
		log.Info("GitHub service configured for orchestrator (PR auto-detection enabled)")

		// Start GitHub background poller
		ghPoller := githubpkg.NewPoller(services.GitHub, eventBus, log)
		ghPoller.SetTaskBranchProvider(orchestratorSvc)
		ghPoller.Start(ctx)
		addCleanup(func() error { ghPoller.Stop(); return nil })
		log.Info("GitHub poller started")
	}

	// Start JIRA auth-health poller. Probes stored credentials for every
	// configured workspace on a fixed cadence so the UI can show a real
	// connected/disconnected status without doing its own polling.
	if services.Jira != nil {
		jiraPoller := jirapkg.NewPoller(services.Jira, log)
		jiraPoller.Start(ctx)
		addCleanup(func() error { jiraPoller.Stop(); return nil })
	}

	return startGatewayAndServe(ctx, cfg, log, eventBus, repos, services,
		agentSettingsController, lifecycleMgr, agentRegistry, orchestratorSvc, msgCreator, addCleanup, runCleanups)
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
	addCleanup func(func() error),
	runCleanups func(),
) bool {
	// ============================================
	// WEBSOCKET GATEWAY
	// ============================================
	log.Info("Initializing WebSocket Gateway...")
	gateway, _, notificationCtrl, err := provideGateway(
		ctx, log, eventBus, services.Task, services.User,
		orchestratorSvc, lifecycleMgr, agentRegistry,
		repos.Notification, repos.Task, services.GitHub,
		cfg.ResolvedHomeDir(),
	)
	if err != nil {
		log.Error("Failed to initialize WebSocket gateway", zap.Error(err))
		return false
	}

	gateways.RegisterSessionStreamNotifications(ctx, eventBus, gateway.Hub, log)
	gateway.Hub.SetSessionDataProvider(buildSessionDataProvider(repos.Task, lifecycleMgr, log))
	log.Info("Session data provider configured for session subscriptions (git status from snapshots)")

	waitForAgentctlControlHealthy(ctx, cfg, log)

	// ============================================
	// HOST UTILITY MANAGER
	// ============================================
	// Long-lived per-agent-type agentctl instances for boot-time capability
	// probes, on-demand refresh via settings, and sessionless utility prompts
	// (e.g. "enhance prompt" before a task/session exists).
	hostControlClient := agentctlclient.NewControlClient(cfg.Agent.StandaloneHost, cfg.Agent.StandalonePort, log,
		agentctlclient.WithControlAuthToken(cfg.Agent.StandaloneAuthToken))
	hostUtilityMgr := hostutility.NewManager(agentRegistry, cfg.Agent.StandaloneHost, cfg.Agent.StandalonePort, hostControlClient, log)
	hostUtilityMgr.SetAuthToken(cfg.Agent.StandaloneAuthToken)
	// Wire the host utility manager into the settings controller so
	// /api/v1/agent-models/:agentName reads live capability data.
	agentSettingsController.SetHostUtility(hostUtilityMgr)
	profileReconciler := agentsettingscontroller.NewProfileReconciler(hostUtilityMgr, agentRegistry, repos.AgentSettings, log)
	go func() {
		if err := hostUtilityMgr.Start(ctx); err != nil {
			log.Warn("host utility manager bootstrap error", zap.Error(err))
		}
		// Reconcile profiles against fresh probe results — seeds defaults for
		// newly probed agents, heals stale profile models/modes, cleans up
		// orphans referencing removed agents.
		if err := profileReconciler.Run(ctx); err != nil {
			log.Warn("profile reconciler error", zap.Error(err))
		}
	}()
	addCleanup(func() error {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		hostUtilityMgr.Stop(stopCtx)
		return nil
	})

	if err := orchestratorSvc.Start(ctx); err != nil {
		log.Error("Failed to start orchestrator", zap.Error(err))
		return false
	}
	log.Info("Orchestrator initialized")

	// ============================================
	// ORCHESTRATE CONFIG LOADER + WAKEUP SCHEDULER
	// ============================================
	if services.Orchestrate != nil {
		// Initialize filesystem config loader
		configBasePath := cfg.ResolvedHomeDir()
		cfgLoader := configloader.NewConfigLoader(configBasePath)
		if err := cfgLoader.EnsureDefaultWorkspace(); err != nil {
			log.Error("Failed to ensure default workspace directory", zap.Error(err))
		}
		if err := cfgLoader.Load(); err != nil {
			log.Error("Failed to load orchestrate config from filesystem", zap.Error(err))
		} else {
			log.Info("Orchestrate config loaded from filesystem",
				zap.String("base_path", configBasePath),
				zap.Int("workspaces", len(cfgLoader.GetWorkspaces())))
		}

		// Write bundled system skills
		if err := configloader.EnsureBundledSkills(configBasePath); err != nil {
			log.Error("Failed to write bundled skills", zap.Error(err))
		} else {
			slugs, _ := configloader.BundledSkillSlugs()
			log.Info("Bundled skills ensured", zap.Strings("slugs", slugs))
		}

		// Clean dangling skill symlinks on startup + register shutdown cleanup
		orchestrateservice.CleanDanglingSymlinks(configBasePath)
		log.Info("Cleaned dangling skill symlinks")
		addCleanup(func() error {
			orchestrateservice.CleanupOwnedSymlinks(configBasePath)
			log.Info("Cleaned owned skill symlinks on shutdown")
			return nil
		})

		// Wire ConfigLoader + FileWriter into the orchestrate service. The
		// service treats the database as the source of truth for config
		// (agents/skills/projects/routines); the loader/writer are only
		// retained for the manual sync UI and bundled-skill discovery.
		cfgWriter := configloader.NewFileWriter(configBasePath, cfgLoader)
		services.Orchestrate.SetConfigLoader(cfgLoader, cfgWriter)
		services.Orchestrate.SetWorkspaceCreator(&taskWorkspaceCreatorAdapter{taskSvc: services.Task})
		apiPort := cfg.Server.Port
		if apiPort == 0 {
			apiPort = ports.Backend
		}
		services.Orchestrate.SetAPIBaseURL(fmt.Sprintf("http://localhost:%d/api/v1", apiPort))
		log.Info("Orchestrate service wired to ConfigLoader (manual-sync only)")

		// Reconcile derived DB state (drop dead runtime/budget/channel rows,
		// seed default routine triggers). No FS↔DB sync is performed here.
		reconciler := orchestrateservice.NewReconciler(services.Orchestrate)
		reconciler.ReconcileAll(ctx)
		log.Info("Orchestrate reconciliation complete")

		// Register event subscribers and start scheduler
		if err := services.Orchestrate.RegisterEventSubscribers(eventBus); err != nil {
			log.Error("Failed to register orchestrate event subscribers", zap.Error(err))
			return false
		}

		// Wire the orchestrator's StartTask into the orchestrate scheduler so
		// wakeups launch real agent sessions instead of dry-running.
		services.Orchestrate.SetTaskStarter(orchestrateservice.TaskStarterFunc(
			func(ctx context.Context, taskID, agentProfileID, executorID,
				executorProfileID string, priority int, prompt, workflowStepID string,
				planMode bool, attachments []v1.MessageAttachment) error {
				_, err := orchestratorSvc.StartTask(ctx, taskID, agentProfileID,
					executorID, executorProfileID, priority, prompt,
					workflowStepID, planMode, attachments)
				return err
			},
		))
		log.Info("Orchestrate scheduler wired to orchestrator StartTask")

		orchScheduler := orchestrateservice.NewSchedulerIntegration(
			services.Orchestrate, orchestrateservice.DefaultTickInterval,
		)
		go orchScheduler.Start(ctx)
		log.Info("Orchestrate wakeup scheduler started")

		// Start GC sweep for orphaned worktrees and containers
		worktreeBase := filepath.Join(cfg.ResolvedHomeDir(), "tasks")
		gc := orchestrateservice.NewGarbageCollector(
			services.Orchestrate,
			worktreeBase,
			nil, // dockerClient - pass if Docker available
			3*time.Hour,
		)
		go gc.Start(ctx)
		log.Info("Orchestrate GC sweep started")
	}

	services.Task.StartAutoArchiveLoop(ctx)

	// ============================================
	// HTTP SERVER
	// ============================================
	server := buildHTTPServer(cfg, log, gateway, repos, services, agentSettingsController,
		lifecycleMgr, eventBus, orchestratorSvc, notificationCtrl, msgCreator, agentRegistry, hostUtilityMgr)

	port := cfg.Server.Port
	if port == 0 {
		port = ports.Backend
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
	agentRegistry *registry.Registry,
	hostUtilityMgr *hostutility.Manager,
) *http.Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(httpmw.RequestLogger(log, "kandev"))
	router.Use(httpmw.OtelTracing("kandev"))
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
		hostUtilityMgr:          hostUtilityMgr,
		eventBus:                eventBus,
		services:                services,
		agentSettingsController: agentSettingsController,
		agentList:               agentRegistry,
		userCtrl:                usercontroller.NewController(services.User),
		notificationCtrl:        notificationCtrl,
		editorCtrl:              editorcontroller.NewController(services.Editor),
		promptCtrl:              promptcontroller.NewController(services.Prompts),
		utilityCtrl:             utilitycontroller.NewController(services.Utility),
		msgCreator:              msgCreator,
		secretsSvc:              secrets.NewService(repos.Secrets, log),
		secretStore:             repos.Secrets,
		mcpConfigSvc:            mcpconfig.NewService(repos.AgentSettings),
		webInternalURL:          cfg.Server.WebInternalURL,
		pprofEnabled:            cfg.Debug.PprofEnabled,
		log:                     log,
	})

	port := cfg.Server.Port
	if port == 0 {
		port = ports.Backend
	}
	return &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeoutDuration(),
		WriteTimeout: cfg.Server.WriteTimeoutDuration(),
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
