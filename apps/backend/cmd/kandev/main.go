// Package main is the unified entry point for Kandev.
// This single binary runs all services together with shared infrastructure.
// The server exposes WebSocket and HTTP endpoints.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
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
	linearpkg "github.com/kandev/kandev/internal/linear"
	slackpkg "github.com/kandev/kandev/internal/slack"

	// Agent infrastructure
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/registry"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	runtimeskill "github.com/kandev/kandev/internal/agent/runtime/lifecycle/skill"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"

	// WebSocket gateway
	gateways "github.com/kandev/kandev/internal/gateway/websocket"

	editorcontroller "github.com/kandev/kandev/internal/editors/controller"
	notificationcontroller "github.com/kandev/kandev/internal/notifications/controller"
	promptcontroller "github.com/kandev/kandev/internal/prompts/controller"
	usercontroller "github.com/kandev/kandev/internal/user/controller"
	utilitycontroller "github.com/kandev/kandev/internal/utility/controller"

	// Orchestrator
	"github.com/kandev/kandev/internal/office/configloader"
	officeservice "github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/orchestrator"
	v1 "github.com/kandev/kandev/pkg/api/v1"

	// Office feature packages
	office "github.com/kandev/kandev/internal/office"
	officeagents "github.com/kandev/kandev/internal/office/agents"
	officeapprovals "github.com/kandev/kandev/internal/office/approvals"
	officechannels "github.com/kandev/kandev/internal/office/channels"
	officeconfig "github.com/kandev/kandev/internal/office/config"
	officecosts "github.com/kandev/kandev/internal/office/costs"
	officemodelsdev "github.com/kandev/kandev/internal/office/costs/modelsdev"
	officedashboard "github.com/kandev/kandev/internal/office/dashboard"
	officeinfra "github.com/kandev/kandev/internal/office/infra"
	officelabels "github.com/kandev/kandev/internal/office/labels"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officeonboarding "github.com/kandev/kandev/internal/office/onboarding"
	officeprojects "github.com/kandev/kandev/internal/office/projects"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	officeroutines "github.com/kandev/kandev/internal/office/routines"
	"github.com/kandev/kandev/internal/office/routing"
	officescheduler "github.com/kandev/kandev/internal/office/scheduler"
	officeshared "github.com/kandev/kandev/internal/office/shared"
	officeskills "github.com/kandev/kandev/internal/office/skills"
	officewakeup "github.com/kandev/kandev/internal/office/wakeup"
	orchexecutor "github.com/kandev/kandev/internal/orchestrator/executor"

	// Runs queue (Phase 3 of task-model-unification)
	runsscheduler "github.com/kandev/kandev/internal/runs/scheduler"
	runsservice "github.com/kandev/kandev/internal/runs/service"

	// Workflow engine adapters (Phase 3.2 of task-model-unification)
	officeengineadapters "github.com/kandev/kandev/internal/office/engine_adapters"
	officeenginedispatcher "github.com/kandev/kandev/internal/office/engine_dispatcher"
	workflowadapters "github.com/kandev/kandev/internal/workflow/adapters"
	workflowengine "github.com/kandev/kandev/internal/workflow/engine"

	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"

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

// Build-time variables injected via -ldflags "-X main.Version=... -X main.Commit=... -X main.BuildTime=..."
// (see apps/backend/Makefile). Defaults apply when running un-stamped builds (e.g. `go run`).
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// ready is the readiness flag consulted by the GET /health handler.
// Until it flips true, /health returns 503 — so callers polling for
// readiness (Playwright fixtures, container orchestrators, manual
// curl loops) keep waiting instead of racing ahead. main() flips it
// after every route is mounted, the agent registry is seeded, and
// the HTTP listener is accepting connections.
var ready atomic.Bool

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
		fmt.Printf("kandev version %s (commit %s, built %s)\n", Version, Commit, BuildTime)
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
		Level:      cfg.Logging.Level,
		Format:     cfg.Logging.Format,
		OutputPath: cfg.Logging.OutputPath,
		MaxSizeMB:  cfg.Logging.MaxSizeMB,
		MaxBackups: cfg.Logging.MaxBackups,
		MaxAgeDays: cfg.Logging.MaxAgeDays,
		Compress:   cfg.Logging.Compress,
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
		_ = log.Close()
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
		// Agent registry seeding is a hard prerequisite for every
		// HTTP surface that lists or operates on agents — including
		// the e2e harness, the office onboarding wizard, and the
		// task-create dialog. Letting startup continue with an empty
		// registry produces silent flakes that look like "no agent
		// profile available" with no log trail at the failure site.
		// Fail loudly so the cause shows up in the backend log
		// instead of cascading into downstream UI confusion.
		log.Error("Failed to run initial agent setup — aborting startup", zap.Error(err))
		return false
	}
	log.Info("ACP messages will be stored as comments")

	// ============================================
	// AGENTCTL LAUNCHER (for standalone mode)
	// ============================================
	agentctlResult, err := provideAgentctlLauncher(ctx, cfg, log)
	if err != nil {
		log.Error("Failed to start agentctl subprocess", zap.Error(err))
		return false
	}
	var agentctlBinaryPath string
	if agentctlResult != nil {
		addCleanup(agentctlResult.cleanup)
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic recovered, stopping agentctl", zap.Any("panic", r))
				if stopErr := agentctlResult.cleanup(); stopErr != nil {
					log.Error("failed to stop agentctl on panic", zap.Error(stopErr))
				}
				panic(r)
			}
		}()

		// Capture the binary path so initOfficeServices can include it in the
		// ServiceOptions when constructing the office service.
		agentctlBinaryPath = agentctlResult.binaryPath
	}

	return startAgentInfrastructure(ctx, cfg, log, addCleanup, eventBus, dbPool, repos, services, agentSettingsController, agentRegistry, agentctlBinaryPath, runCleanups)
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
	agentctlBinaryPath string,
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
		zap.Bool("enabled", cfg.Worktree.Enabled))

	lifecycleMgr.SetWorkspaceInfoProvider(services.Task)
	log.Info("Workspace info provider configured for session recovery")

	// TODO(task-model-unification Phase 2, ADR 0004): wire agentruntime.New(lifecycleMgr)
	// once a real consumer (workflow-engine / cron-driven trigger handlers) exists.
	// Allocating the facade in Phase 1 without a caller is dead code.

	// Persistence writer for executors_running. This makes the lifecycle manager
	// the sole writer of agent_execution_id / container_id / runtime / status —
	// the structural fix for the agent-execution-id divergence bug. Must be set
	// before any Launch / EnsureWorkspaceExecutionForSession can run.
	lifecycleMgr.SetExecutorRunningWriter(repos.Task)

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

	// Let the task service treat the cloner's base path as an implicit
	// allow-listed root. Without this, deploys that put the clone base
	// outside HOME (e.g. KANDEV_REPOCLONE_BASEPATH=/data/repos in a
	// container) fail the discoveryRoots() allow-list check and local
	// branch listing returns nothing.
	services.Task.SetRepoCloneLocation(repoCloner)

	// ============================================
	// ORCHESTRATOR
	// ============================================
	log.Info("Initializing Orchestrator...")

	orchestratorSvc, msgCreator, err := provideOrchestrator(cfg, log, dbPool, eventBus, repos.Task, services.Task, services.User,
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

	// Start JIRA poller. Drives two background loops sharing one service: an
	// auth-health probe (so the UI can show connect status without polling
	// JIRA itself) and an issue-watch loop that runs configured JQL queries
	// and emits NewJiraIssueEvent for the orchestrator to turn into tasks.
	if services.Jira != nil {
		orchestratorSvc.SetJiraService(&jiraServiceAdapter{svc: services.Jira})
		jiraPoller := jirapkg.NewPoller(services.Jira, log)
		jiraPoller.Start(ctx)
		addCleanup(func() error { jiraPoller.Stop(); return nil })
	}

	// Start Linear poller. Mirrors the Jira shape: auth-health probe plus an
	// issue-watch loop that runs configured filters and emits
	// NewLinearIssueEvent for the orchestrator to turn into tasks.
	if services.Linear != nil {
		orchestratorSvc.SetLinearService(&linearServiceAdapter{svc: services.Linear})
		linearPoller := linearpkg.NewPoller(services.Linear, log)
		linearPoller.Start(ctx)
		addCleanup(func() error { linearPoller.Stop(); return nil })
	}

	// Start Slack auth-health poller and the trigger loop. The trigger
	// polls each configured workspace every 30s for new `!kandev …`
	// messages from the authenticated user and turns them into Kandev
	// tasks via taskSvc.
	if services.Slack != nil {
		slackPoller := slackpkg.NewPoller(services.Slack, log)
		slackPoller.Start(ctx)
		addCleanup(func() error { slackPoller.Stop(); return nil })

		slackTrigger := slackpkg.NewTrigger(services.Slack, log)
		slackTrigger.Start(ctx)
		addCleanup(func() error { slackTrigger.Stop(); return nil })
	}

	return startGatewayAndServe(ctx, cfg, log, eventBus, repos, services,
		agentSettingsController, lifecycleMgr, agentRegistry, orchestratorSvc, msgCreator, repoCloner, agentctlBinaryPath, addCleanup, runCleanups)
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
	repoCloner *repoclone.Cloner,
	agentctlBinaryPath string,
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

	// Wire the Slack agent runner. Slack triage uses the host-utility
	// inference path (single-shot ACP subprocess) with the Kandev MCP
	// server attached so the agent can call list_workflows_kandev /
	// create_task_kandev / etc. mid-prompt. Both deps land here at the
	// same time: hostUtilityMgr just bootstrapped above, services.Utility
	// was constructed in provideServices.
	if services.Slack != nil && services.Utility != nil {
		mcpURL := buildKandevMCPURL(cfg.Server.Port)
		slackRunner := slackpkg.NewRunner(
			services.Utility,
			services.User,
			slackHostUtilityAdapter{mgr: hostUtilityMgr},
			[]slackpkg.MCPDescriptor{{Name: "kandev", URL: mcpURL}},
			log,
		)
		services.Slack.SetRunner(slackRunner)
	}

	if err := orchestratorSvc.Start(ctx); err != nil {
		log.Error("Failed to start orchestrator", zap.Error(err))
		return false
	}
	log.Info("Orchestrator initialized")

	// ============================================
	// ORCHESTRATE CONFIG LOADER + WAKEUP SCHEDULER
	// ============================================
	if ok := initOfficeServices(ctx, cfg, log, repos, services, orchestratorSvc, eventBus, agentctlBinaryPath, addCleanup, lifecycleMgr, agentRegistry); !ok {
		return false
	}

	// Wire subscription usage provider into the office agents service so the
	// /agents/:id/utilization endpoint can fetch live utilization data.
	// Skipped when the Office feature flag is off (services.OfficeSvcs is nil).
	if services.OfficeSvcs != nil && services.OfficeSvcs.Agents != nil {
		usageAdapter := newUsageProviderAdapter(repos.AgentSettings, agentRegistry)
		services.OfficeSvcs.Agents.SetUsageProvider(usageAdapter)
	}

	services.Task.StartAutoArchiveLoop(ctx)

	// ============================================
	// HTTP SERVER
	// ============================================
	server := buildHTTPServer(cfg, log, gateway, repos, services, agentSettingsController,
		lifecycleMgr, eventBus, orchestratorSvc, notificationCtrl, msgCreator, agentRegistry, hostUtilityMgr, addCleanup, repoCloner)

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

	// Flip the readiness flag once the HTTP listener is actually
	// accepting connections, not just "spawned". ListenAndServe runs
	// in a goroutine and may not have bound the socket yet by the time
	// we reach this line. Probe the local listener with a short retry
	// loop — once a single connect succeeds, the kernel queue is up
	// and any subsequent /health call will land on a wired route.
	go waitListenerThenMarkReady(port, log)

	awaitShutdown(server, orchestratorSvc, lifecycleMgr, runCleanups, log)
	return true
}

// waitListenerThenMarkReady probes the local HTTP listener until a
// connect succeeds, then flips the package-level `ready` flag so the
// /health handler stops returning 503. Runs in its own goroutine so
// the caller can proceed into awaitShutdown.
//
// The probe budget is generous (30s) — under heavy parallel-suite
// load the OS scheduler can delay the listen goroutine for a couple
// of seconds. If the budget expires the flag still flips, so the
// backend doesn't permanently advertise "not ready" if probing fails
// for some unrelated reason (e.g. an iptables hiccup on a dev box).
func waitListenerThenMarkReady(port int, log *logger.Logger) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(30 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			// A successful TCP dial proves the listener is bound.
			ready.Store(true)
			log.Info("backend ready", zap.Int("port", port))
			return
		}
		if time.Now().After(deadline) {
			log.Warn("backend readiness probe never connected; flipping ready anyway",
				zap.Int("port", port))
			ready.Store(true)
			return
		}
		<-ticker.C
	}
}

// initOfficeServices constructs the office service with all dependencies, then
// sets up the reconciler, event subscribers, scheduler, and garbage collector.
// Returns false if a fatal error prevents startup.
func initOfficeServices(
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	repos *Repositories,
	services *Services,
	orchestratorSvc *orchestrator.Service,
	eventBus bus.EventBus,
	agentctlBinaryPath string,
	addCleanup func(func() error),
	lifecycleMgr *lifecycle.Manager,
	agentRegistry *registry.Registry,
) bool {
	// Feature gate: when features.office is off (the production default),
	// skip every Office construction step. Downstream call sites
	// (helpers.go route registration, cron scheduler, skill backfill,
	// usage-provider wiring) already nil-check services.Office /
	// services.OfficeSvcs, so leaving them nil is safe.
	// See docs/decisions/0007-runtime-feature-flags.md.
	if !cfg.Features.Office {
		log.Info("Office feature disabled (features.office=false); skipping initialization")
		return true
	}

	configBasePath := cfg.ResolvedHomeDir()
	cfgLoader, cfgWriter := initOfficeConfigLoader(configBasePath, log, addCleanup)

	apiPort := cfg.Server.Port
	if apiPort == 0 {
		apiPort = ports.Backend
	}

	taskStarter := newOfficeTaskStarter(orchestratorSvc)

	// Construct the office service with all dependencies at once so the
	// compiler catches missing fields rather than failing at runtime.
	services.Office = officeservice.NewService(officeservice.ServiceOptions{
		Repo:               repos.Office,
		Logger:             log,
		CfgLoader:          cfgLoader,
		CfgWriter:          cfgWriter,
		WorkspaceCreator:   &taskWorkspaceCreatorAdapter{taskSvc: services.Task},
		TaskWorkspace:      services.Task,
		TaskCreator:        &taskCreatorAdapter{taskSvc: services.Task},
		TaskPRs:            &taskPRListerAdapter{gh: services.GitHub},
		APIBaseURL:         fmt.Sprintf("http://localhost:%d/api/v1", apiPort),
		TaskStarter:        taskStarter,
		TaskCanceller:      orchestratorSvc,
		AgentctlBinaryPath: agentctlBinaryPath,
		EventBus:           eventBus,
	})
	log.Info("Office service constructed with all dependencies")

	// office-costs Wave B: lazy models.dev pricing lookup. The Client
	// allocates no resources at startup — the first non-claude-acp cost
	// event triggers a disk read; the first missing cache file triggers
	// a background fetch. Workspaces running only claude-acp stay
	// untouched because Layer A handles every event before lookup.
	modelsdevCachePath := filepath.Join(cfg.ResolvedHomeDir(), "cache", "models-dev.json")
	pricingLookup := officemodelsdev.New(officemodelsdev.Config{
		CachePath: modelsdevCachePath,
	}, log)
	services.Office.SetPricingLookup(pricingLookup)
	services.Office.SetSessionUsageWriter(repos.Task)

	// ADR 0005 Wave E: plug the runtime-tier skill deployer into the
	// lifecycle manager. The deployer reads office's skills repo +
	// instructions repo via small adapters; office no longer ships
	// its own delivery code.
	wireRuntimeSkillDeployer(lifecycleMgr, agentRegistry, repos.Office, services.Office, cfg.ResolvedHomeDir(), log)

	// Wire office-owned repositories into the task service for cross-package operations.
	services.Task.SetBlockerRepository(repos.Office)
	services.Task.SetCommentRepository(repos.Office)

	// Build feature-package services and wire all inter-service dependencies.
	services.OfficeSvcs = buildOfficeFeatureServices(
		repos.Office, repos.Task, repos.AgentSettings, cfgLoader, cfgWriter, configBasePath,
		agentRegistry, log, services, cfg.Office.JWTSigningKey,
	)
	wireOfficeSvcsDependencies(services, repos, eventBus, orchestratorSvc, agentRegistry)

	// Reconcile using the new infra package.
	reconciler := officeinfra.NewReconciler(repos.Office, log)
	reconciler.ReconcileAll(ctx)
	log.Info("Office reconciliation complete")

	// System skill sync. Upserts every embedded SKILL.md (the ones written
	// to disk by EnsureBundledSkills above) into office_skills as
	// is_system = true rows for each known workspace, removing system
	// rows that no longer have a matching embed. Per-agent
	// desired_skills references are preserved.
	syncSystemSkills(ctx, repos.Office, services, log)
	// Backfill default skills onto agents that pre-date the system-skill
	// rollout. Idempotent: only agents whose `desired_skills` is empty
	// receive defaults; curated lists are left alone.
	backfillAgentDefaultSkills(ctx, services, log)

	// Register event subscribers and start scheduler
	if err := services.Office.RegisterEventSubscribers(eventBus); err != nil {
		log.Error("Failed to register office event subscribers", zap.Error(err))
		return false
	}

	startOfficeSchedulersAndGC(ctx, cfg, repos, services, eventBus, orchestratorSvc, log)
	return true
}

// wireOfficeSvcsDependencies wires inter-service dependencies into the
// OfficeSvcs feature package. Extracted to keep initOfficeServices within
// funlen limits.
func wireOfficeSvcsDependencies(
	services *Services,
	repos *Repositories,
	eventBus bus.EventBus,
	orchestratorSvc *orchestrator.Service,
	agentRegistry *registry.Registry,
) {
	// Wire the workflow-domain decisions store so approve/request-changes
	// route to workflow_step_decisions (ADR 0005 Wave E).
	services.OfficeSvcs.Dashboard.SetDecisionStore(repos.Workflow)
	// Wire the event bus into the dashboard service for status-change events.
	services.OfficeSvcs.Dashboard.SetEventBus(eventBus)
	services.OfficeSvcs.Channels.SetEventBus(eventBus)
	// Wire the office service as the channel relay's run resolver so
	// relayed-comment activity rows get tagged with the originating
	// run id (Tasks Touched on the run detail page).
	services.OfficeSvcs.Channels.SetRunResolver(services.Office)
	// Wire the office service as the retry canceller for task reassignment.
	services.OfficeSvcs.Dashboard.SetRetryCanceller(services.Office)
	// Wire the office service as the task canceller for status→cancelled hard-cancels.
	services.OfficeSvcs.Dashboard.SetTaskCanceller(services.Office)
	// Wire the reactivity pipeline so property mutations queue downstream runs.
	services.OfficeSvcs.Dashboard.SetReactivityApplier(
		officescheduler.NewDashboardReactivityAdapter(services.OfficeSvcs.Scheduler),
	)
	// Wire the approval-flow run queuer so decisions trigger
	// task_changes_requested / task_ready_to_close runs.
	services.OfficeSvcs.Dashboard.SetApprovalReactivityQueuer(
		officescheduler.NewDashboardApprovalAdapter(services.OfficeSvcs.Scheduler),
	)
	// Wire the office session terminator so participation removal flips the
	// (task, agent) session row to COMPLETED.
	officeSessionTerm := orchestratorSvc.OfficeSessionTerminator()
	services.OfficeSvcs.Dashboard.SetSessionTerminator(officeSessionTerm)
	services.OfficeSvcs.Agents.SetSessionTerminator(officeSessionTerm)
	// Wire the failure notifier so reassignments auto-dismiss the
	// prior (task, agent) inbox entry.
	services.OfficeSvcs.Dashboard.SetFailureNotifier(services.Office)
	// Wire the failure-tracking inbox source + dismiss handler so the
	// inbox surfaces agent_run_failed / agent_paused_after_failures rows.
	services.OfficeSvcs.Dashboard.SetFailureInboxSource(
		newOfficeFailureInboxAdapter(services.Office),
	)
	services.OfficeSvcs.Dashboard.SetMarkFixedHandler(services.Office)
	wireOfficeProviderRouting(services, repos, orchestratorSvc, eventBus, agentRegistry)
}

// wireOfficeProviderRouting builds the routing resolver + TaskStarter
// adapter and wires the scheduler.SchedulerService as the office
// service's routing dispatcher. No-op effect on non-routing launches
// because the resolver short-circuits when no workspace has routing
// enabled.
func wireOfficeProviderRouting(
	services *Services,
	repos *Repositories,
	orchestratorSvc *orchestrator.Service,
	eventBus bus.EventBus,
	agentRegistry *registry.Registry,
) {
	scheduler := services.OfficeSvcs.Scheduler
	resolver := routing.NewResolver(&officeRoutingRepoAdapter{repo: repos.Office}, nil)
	scheduler.SetResolver(resolver)
	scheduler.SetTaskStarter(&schedulerTaskStarterAdapter{orch: orchestratorSvc})
	scheduler.SetEventBus(eventBus)
	services.Office.SetRoutingDispatcher(scheduler)

	provider := routing.NewProvider(repos.Office, agentRegistry, resolver, scheduler)
	services.OfficeSvcs.Dashboard.SetRoutingProvider(provider)
	services.OfficeSvcs.Dashboard.SetRouteAttemptLister(repos.Office)
	services.OfficeSvcs.Agents.SetKnownProvidersFn(func() []routing.ProviderID {
		return routing.KnownProviders(agentRegistry)
	})
}

// officeRoutingRepoAdapter satisfies routing.Repo over the office
// sqlite repo. Lives here (not in the routing package) so the routing
// package stays repo-agnostic.
type officeRoutingRepoAdapter struct {
	repo *officesqlite.Repository
}

func (a *officeRoutingRepoAdapter) GetWorkspaceRouting(
	ctx context.Context, workspaceID string,
) (*routing.WorkspaceConfig, error) {
	return a.repo.GetWorkspaceRouting(ctx, workspaceID)
}

func (a *officeRoutingRepoAdapter) ListProviderHealth(
	ctx context.Context, workspaceID string,
) ([]officemodels.ProviderHealth, error) {
	return a.repo.ListProviderHealth(ctx, workspaceID)
}

// schedulerTaskStarterAdapter satisfies scheduler.TaskStarter against
// the orchestrator service.
type schedulerTaskStarterAdapter struct {
	orch *orchestrator.Service
}

func (a *schedulerTaskStarterAdapter) StartTask(
	ctx context.Context,
	taskID, agentProfileID, executorID, executorProfileID string,
	priority, prompt, workflowStepID string,
	planMode bool, attachments []interface{},
) error {
	_, err := a.orch.StartTask(ctx, taskID, agentProfileID,
		executorID, executorProfileID, priority, prompt,
		workflowStepID, planMode, nil)
	return err
}

func (a *schedulerTaskStarterAdapter) StartTaskWithRoute(
	ctx context.Context,
	taskID, agentProfileID string,
	launch officescheduler.LaunchContext,
	route officescheduler.RouteOverride,
) error {
	return a.orch.StartTaskWithRoute(ctx, taskID, agentProfileID,
		orchexecutor.LaunchContext{
			ExecutorID:        launch.ExecutorID,
			ExecutorProfileID: launch.ExecutorProfileID,
			Priority:          launch.Priority,
			Prompt:            launch.Prompt,
			WorkflowStepID:    launch.WorkflowStepID,
			PlanMode:          launch.PlanMode,
			Attachments:       launch.Attachments,
			Env:               launch.Env,
		},
		orchexecutor.RouteOverride{
			ProviderID: route.ProviderID,
			Model:      route.Model,
			Tier:       route.Tier,
			Mode:       route.Mode,
			Flags:      route.Flags,
			Env:        route.Env,
		})
}

// startOfficeSchedulersAndGC wires the runs service, workflow engine dispatcher,
// runs scheduler, cron scheduler, and GC sweep. Extracted to keep
// initOfficeServices within funlen limits.
func startOfficeSchedulersAndGC(
	ctx context.Context,
	cfg *config.Config,
	repos *Repositories,
	services *Services,
	eventBus bus.EventBus,
	orchestratorSvc *orchestrator.Service,
	log *logger.Logger,
) {
	log.Info("Office scheduler wired to orchestrator StartTask")
	orchScheduler := officeservice.NewSchedulerIntegration(
		services.Office, runsscheduler.TickIntervalFromEnv(),
	)
	// Office task-handoffs prompt enrichment. The HandoffService is
	// constructed alongside the HTTP routes (helpers.go); we stash the
	// scheduler reference on the Services struct so registerRoutes can
	// wire SetTaskContextProvider once both exist.
	services.OrchScheduler = orchScheduler
	// Wire the runs queue service so office.QueueRun delegates the
	// insert + publish + signal to it (Phase 3 of task-model-unification).
	runsSvc := runsservice.New(
		repos.Office.RunsRepository(), eventBus, log, nil,
	)
	services.Office.SetRunsService(runsSvc)
	// Phase 4 (ADR-0004): wire the workflow engine's dependencies and a
	// dispatcher so office event subscribers route through the engine
	// unconditionally.
	engineDispatcher := wireWorkflowEngineForOffice(
		orchestratorSvc, services.Office, services.Task, services.Workflow, repos, runsSvc, log,
	)
	// Start the runs scheduler (tick + signal listener). It drives
	// orchScheduler.Tick on both periodic ticks and event-driven signals.
	runScheduler := runsscheduler.New(
		orchScheduler, runsSvc.SubscribeSignal(),
		runsscheduler.TickIntervalFromEnv(), log,
	)
	go runScheduler.Start(ctx)
	log.Info("Runs scheduler started",
		zap.Duration("tick", runsscheduler.TickIntervalFromEnv()))
	// Phase 5 (ADR-0004): start the shared cron loop. The routines handler
	// degrades to a no-op when routineSvc is nil, so omitting Office's
	// scheduler is safe when features.office is off.
	var officeRoutines *officeroutines.RoutineService
	if services.OfficeSvcs != nil {
		officeRoutines = services.OfficeSvcs.Routines
	}
	startCronScheduler(ctx, repos, engineDispatcher, officeRoutines, log)
	// Start GC sweep for orphaned worktrees and containers.
	worktreeBase := filepath.Join(cfg.ResolvedHomeDir(), "tasks")
	gc := officeinfra.NewGarbageCollector(
		repos.Office, log, worktreeBase,
		nil, // dockerClient - pass if Docker available
		3*time.Hour,
	)
	go gc.Start(ctx)
	log.Info("Office GC sweep started")
}

// wireWorkflowEngineForOffice composes the Phase 2 (ADR-0004)
// dependencies the workflow engine needs to evaluate office triggers,
// then builds an engine-dispatcher and hands it to the office service.
//
// Engine options wired here:
//   - RunQueueAdapter        — runs service (Phase 3.1)
//   - ParticipantStore       — workflow_step_participants
//   - DecisionStore          — workflow_step_decisions
//   - PrimaryAgentResolver   — workflow_steps.agent_profile_id
//   - CEOAgentResolver       — agent_profiles WHERE role='ceo' AND workspace_id != ”
//
// The orchestrator's engine is rebuilt with these options applied, then
// the office service is given a dispatcher pointing at it. The four
// task-scoped event subscribers (comment, blockers_resolved,
// children_completed, approval_resolved) route through the engine
// unconditionally after Phase 4.
func wireWorkflowEngineForOffice(
	orchestratorSvc *orchestrator.Service,
	officeSvc *officeservice.Service,
	taskSvc *taskservice.Service,
	workflowSvc *workflowservice.Service,
	repos *Repositories,
	runsSvc *runsservice.Service,
	log *logger.Logger,
) *officeenginedispatcher.Dispatcher {
	// Build the workflow-domain adapters.
	participants := workflowadapters.NewParticipantAdapter(repos.Workflow)
	decisions := workflowadapters.NewDecisionAdapter(repos.Workflow)
	primary := workflowadapters.NewPrimaryAgentAdapter(repos.Workflow)
	// Office-domain CEO resolver.
	ceo := officeengineadapters.NewCEOAgentAdapter(repos.Office)
	// Phase 8 delegation adapters: task creator + workflow switcher.
	taskCreator := officeengineadapters.NewTaskCreatorAdapter(
		repos.Task, &childTaskCreatorAdapter{taskSvc: taskSvc})
	workflowSwitcher := officeengineadapters.NewWorkflowSwitcherAdapter(
		&startStepResolverAdapter{svc: workflowSvc}, repos.Task)
	// Wire each dependency via its dedicated setter so the orchestrator
	// captures it both for engine.With* options and for the Phase 2 / 8
	// callback registry.
	orchestratorSvc.SetEngineRunQueue(&runsServiceEngineAdapter{svc: runsSvc})
	orchestratorSvc.SetEngineParticipantStore(participants)
	orchestratorSvc.SetEngineDecisionStore(decisions)
	orchestratorSvc.SetEngineCEOResolver(ceo)
	orchestratorSvc.SetPrimaryAgentResolver(primary)
	orchestratorSvc.SetEngineTaskCreator(taskCreator)
	orchestratorSvc.SetEngineWorkflowSwitcher(workflowSwitcher)
	eng := orchestratorSvc.WorkflowEngine()
	if eng == nil {
		log.Warn("workflow engine not initialised; office engine dispatcher disabled")
		return nil
	}
	// Build the dispatcher. The session resolver is the task repo,
	// which exposes GetActiveTaskSessionByTaskID.
	dispatcher := officeenginedispatcher.New(eng, repos.Task, log)
	officeSvc.SetWorkflowEngineDispatcher(dispatcher)
	log.Info("workflow engine dispatcher wired for office")
	return dispatcher
}

// runsServiceEngineAdapter bridges runs/service.Service.QueueRun (which
// takes runs/service.QueueRunRequest) to engine.RunQueueAdapter (which
// takes engine.QueueRunRequest). The two structs have identical fields
// — they are intentionally duplicated so neither package imports the
// other — so this adapter is a field-by-field copy.
type runsServiceEngineAdapter struct {
	svc *runsservice.Service
}

func (a *runsServiceEngineAdapter) QueueRun(ctx context.Context, req workflowengine.QueueRunRequest) error {
	return a.svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: req.AgentProfileID,
		TaskID:         req.TaskID,
		WorkflowStepID: req.WorkflowStepID,
		Reason:         req.Reason,
		IdempotencyKey: req.IdempotencyKey,
		Payload:        req.Payload,
	})
}

// wireRuntimeSkillDeployer plugs the runtime-tier SkillDeployer into the
// lifecycle manager (ADR 0005 Wave E). The deployer bridges office's
// skills repo + instructions repo to the runtime via small adapters in
// internal/office/skills, so kanban and office launches share a single
// skill-deploy code path. A nil officeSvc / repo leaves the manager
// with the Wave A no-op deployer.
func wireRuntimeSkillDeployer(
	lifecycleMgr *lifecycle.Manager,
	agentRegistry *registry.Registry,
	officeRepo *officesqlite.Repository,
	officeSvc *officeservice.Service,
	basePath string,
	log *logger.Logger,
) {
	if lifecycleMgr == nil || officeSvc == nil || officeRepo == nil {
		return
	}
	deployer, err := runtimeskill.New(runtimeskill.Config{
		Logger:                  log,
		BasePath:                basePath,
		SkillReader:             officeskills.NewSkillReaderAdapter(officeSvc),
		InstructionLister:       officeskills.NewInstructionListerAdapter(officeRepo),
		ProjectSkillDirResolver: makeProjectSkillDirResolver(agentRegistry),
		WorkspaceSlugFn:         makeWorkspaceSlugFn(),
	})
	if err != nil {
		log.Warn("failed to construct runtime skill deployer; launches will skip skill delivery",
			zap.Error(err))
		return
	}
	lifecycleMgr.SetSkillDeployer(lifecycle.NewSkillDeployerAdapter(deployer))
	log.Info("Runtime skill deployer wired into lifecycle manager")
}

// makeProjectSkillDirResolver returns a runtimeskill.ProjectSkillDirResolver
// backed by the agent registry. The agent type ID equals the agent_id on
// the agent_profiles row after ADR 0005 — we look up the agent and read
// its declared ProjectSkillDir, falling back to the runtime default.
func makeProjectSkillDirResolver(reg *registry.Registry) runtimeskill.ProjectSkillDirResolver {
	if reg == nil {
		return nil
	}
	return func(agentTypeID string) string {
		ag, ok := reg.Get(agentTypeID)
		if !ok {
			return ""
		}
		if rt := ag.Runtime(); rt != nil && rt.ProjectSkillDir != "" {
			return rt.ProjectSkillDir
		}
		return ""
	}
}

// makeUserSkillDirResolver returns a provider user-skill-dir resolver backed by
// the agent registry. Providers that do not declare a user skill dir are omitted
// from discovery.
func makeUserSkillDirResolver(reg *registry.Registry) officeskills.UserSkillDirResolver {
	if reg == nil {
		return nil
	}
	return func(provider string) (string, bool) {
		ag, ok := reg.Get(provider)
		if !ok {
			return "", false
		}
		if rt := ag.Runtime(); rt != nil && rt.UserSkillDir != "" {
			return rt.UserSkillDir, true
		}
		return "", false
	}
}

// makeWorkspaceSlugFn returns a slug-resolver that maps a workspace ID
// to a slug used in on-host runtime paths. The office stack is currently
// single-workspace-per-install, so every ID resolves to the constant
// "default". When multi-workspace lands, this becomes a real lookup
// against officesqlite.Repository (e.g. GetWorkspaceNameByID) — until
// then we deliberately ignore the workspace ID rather than passing a
// repo dependency that has nothing to query.
func makeWorkspaceSlugFn() func(string) string {
	return func(string) string { return "default" }
}

// initOfficeConfigLoader initialises the filesystem config loader, writes
// officeFailureInboxAdapter forwards from the office Service (which
// returns service-package row types) to the dashboard
// FailureInboxSource interface (which expects dashboard-package row
// types). Trivial conversion — dashboard intentionally avoids
// importing the office repo or service so the package boundary stays
// clean.
type officeFailureInboxAdapter struct {
	svc *officeservice.Service
}

func newOfficeFailureInboxAdapter(svc *officeservice.Service) *officeFailureInboxAdapter {
	return &officeFailureInboxAdapter{svc: svc}
}

func (a *officeFailureInboxAdapter) ListFailedRunInboxRows(
	ctx context.Context, workspaceID, userID string,
) ([]officedashboard.FailureInboxRow, error) {
	rows, err := a.svc.ListFailedRunInboxRows(ctx, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]officedashboard.FailureInboxRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, officedashboard.FailureInboxRow{
			Kind:           "agent_run_failed",
			ItemID:         r.RunID,
			AgentProfileID: r.AgentProfileID,
			AgentName:      r.AgentName,
			TaskID:         r.TaskID,
			ErrorMessage:   r.ErrorMessage,
			FailedAt:       r.FailedAt,
		})
	}
	return out, nil
}

func (a *officeFailureInboxAdapter) ListPausedAgentInboxRows(
	ctx context.Context, workspaceID, userID string,
) ([]officedashboard.FailureInboxRow, error) {
	rows, err := a.svc.ListPausedAgentInboxRows(ctx, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]officedashboard.FailureInboxRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, officedashboard.FailureInboxRow{
			Kind:                "agent_paused_after_failures",
			ItemID:              r.AgentID,
			AgentProfileID:      r.AgentID,
			AgentName:           r.AgentName,
			PauseReason:         r.PauseReason,
			ConsecutiveFailures: r.ConsecutiveFailures,
			FailedAt:            r.UpdatedAt,
		})
	}
	return out, nil
}

// bundled skills, and registers shutdown cleanup for symlinks.
func initOfficeConfigLoader(
	basePath string, log *logger.Logger, addCleanup func(func() error),
) (*configloader.ConfigLoader, *configloader.FileWriter) {
	cfgLoader := configloader.NewConfigLoader(basePath)
	if err := cfgLoader.Load(); err != nil {
		log.Error("Failed to load office config from filesystem", zap.Error(err))
	} else {
		log.Info("Office config loaded from filesystem",
			zap.String("base_path", basePath),
			zap.Int("workspaces", len(cfgLoader.GetWorkspaces())))
	}
	if err := configloader.EnsureBundledSkills(basePath); err != nil {
		log.Error("Failed to write bundled skills", zap.Error(err))
	} else {
		slugs, _ := configloader.BundledSkillSlugs()
		log.Info("Bundled skills ensured", zap.Strings("slugs", slugs))
	}
	return cfgLoader, configloader.NewFileWriter(basePath, cfgLoader)
}

// syncSystemSkills reconciles the office_skills table against the
// embedded bundled skill set for every known workspace. Pulls the
// workspace list from the task service (workspace ids are shared
// across task + office persistence). Failures are logged but do not
// gate startup — system skills are surfaced lazily by the Skills
// page, which simply shows an empty System group until a later
// retry succeeds.
func syncSystemSkills(
	ctx context.Context,
	repo *officesqlite.Repository,
	services *Services,
	log *logger.Logger,
) {
	workspaces, err := services.Task.ListWorkspaces(ctx)
	if err != nil {
		log.Error("system skill sync: list workspaces", zap.Error(err))
		return
	}
	ids := make([]string, 0, len(workspaces))
	for _, w := range workspaces {
		ids = append(ids, w.ID)
	}
	if _, err := officeskills.SyncSystemSkills(ctx, repo, ids, nil, log); err != nil {
		log.Error("system skill sync failed", zap.Error(err))
	}
}

// backfillAgentDefaultSkills delegates to the agents service so each
// workspace's existing agents inherit the system-skill defaults for
// their role when their desired_skills array is empty. Errors per
// workspace are absorbed inside the service call; startup must not
// fail because of a curated-list edge case.
func backfillAgentDefaultSkills(
	ctx context.Context,
	services *Services,
	log *logger.Logger,
) {
	if services.OfficeSvcs == nil || services.OfficeSvcs.Agents == nil {
		return
	}
	workspaces, err := services.Task.ListWorkspaces(ctx)
	if err != nil {
		log.Error("backfill default skills: list workspaces", zap.Error(err))
		return
	}
	for _, w := range workspaces {
		services.OfficeSvcs.Agents.BackfillDefaultSkillsForWorkspace(ctx, w.ID)
	}
}

// newOfficeTaskStarter wraps orchestratorSvc.StartTaskWithEnv in the
// officeservice.TaskStarterWithEnvFunc adapter. Extracted from
// initOfficeServices to keep that function under the funlen cap.
func newOfficeTaskStarter(orchestratorSvc *orchestrator.Service) officeservice.TaskStarter {
	return officeservice.TaskStarterWithEnvFunc(
		func(ctx context.Context, taskID, agentProfileID, executorID,
			executorProfileID string, priority string, prompt, workflowStepID string,
			planMode bool, attachments []v1.MessageAttachment, env map[string]string) error {
			_, err := orchestratorSvc.StartTaskWithEnv(ctx, taskID, agentProfileID,
				executorID, executorProfileID, priority, prompt,
				workflowStepID, planMode, attachments, env)
			return err
		},
	)
}

// newAgentAuth wraps officeagents.NewAgentAuth with a dev-mode warning when
// no signing key is configured, so the empty-key fallback can't silently
// invalidate agent tokens on every restart in production.
func newAgentAuth(jwtSigningKey string, log *logger.Logger) *officeagents.AgentAuth {
	if jwtSigningKey == "" {
		log.Warn("office.jwtSigningKey is empty; generating an ephemeral key. " +
			"Agent JWTs will be invalidated on every backend restart. " +
			"Set KANDEV_OFFICE_JWTSIGNINGKEY for stable tokens.")
	}
	return officeagents.NewAgentAuth(jwtSigningKey)
}

// buildOfficeFeatureServices creates the feature-level office services used by
// the HTTP handler layer (office.RegisterAllRoutes). The monolithic
// services.Office is passed for shared interfaces during the transition period.
func buildOfficeFeatureServices(
	repo *officesqlite.Repository,
	taskRepo *tasksqlite.Repository,
	settingsRepo settingsstore.Repository,
	cfgLoader *configloader.ConfigLoader,
	cfgWriter *configloader.FileWriter,
	homeDir string,
	agentRegistry *registry.Registry,
	log *logger.Logger,
	services *Services,
	jwtSigningKey string,
) *office.Services {
	activity := officeshared.NewActivityLogger(repo, log)

	agentSvc := officeagents.NewAgentService(repo, log, activity)
	agentSvc.SetAuth(newAgentAuth(jwtSigningKey, log))
	if services.Office != nil {
		services.Office.SetAgentTokenMinter(agentSvc)
	}
	skillSvc := officeskills.NewSkillService(repo, log, activity, agentSvc, cfgLoader)
	skillSvc.SetUserSkillDirResolver(makeUserSkillDirResolver(agentRegistry))
	projectSvc := officeprojects.NewProjectService(repo, log, activity)
	costSvc := officecosts.NewCostService(repo, log, activity, agentSvc, agentSvc)
	// Office service delegates budget evaluation (pre-execution + post-event)
	// to the costs feature — the only place that owns CRUD for budget policies.
	if services.Office != nil {
		services.Office.SetBudgetChecker(costSvc)
	}
	routineSvc := officeroutines.NewRoutineService(repo, log, activity)
	// PR 3 of office-heartbeat-rework: wire routines into the wakeup
	// dispatcher so the lightweight (taskless) flow enqueues a fresh
	// taskless run, and into the task path so the heavy flow creates a
	// real task in the routine system workflow.
	routineWakeupDispatcher := officewakeup.NewDispatcher(repo, repo, log)
	routineWakeupDispatcher.SetRoutineLookup(repo)
	routineSvc.SetWakeupEnqueuer(&routineWakeupAdapter{
		repo:       repo,
		dispatcher: routineWakeupDispatcher,
	})
	routineSvc.SetWorkflowEnsurer(&workflowEnsurerAdapter{repo: taskRepo})
	routineSvc.SetTaskCreator(&taskCreatorAdapter{taskSvc: services.Task})
	approvalSvc := officeapprovals.NewApprovalService(repo, log, activity, services.Office)
	approvalSvc.SetAgentWriter(agentSvc)
	channelSvc := officechannels.NewChannelService(repo, log, activity, agentSvc)
	configSvc := officeconfig.NewConfigService(repo, cfgLoader, cfgWriter, log, activity)
	dashboardSvc := buildOfficeDashboardService(
		repo, log, activity, agentSvc, costSvc,
		skillSvc, routineSvc, approvalSvc,
		cfgLoader, cfgWriter,
	)
	documentSvc := taskservice.NewDocumentService(taskRepo, log)
	onboardingSvc := officeonboarding.NewOnboardingService(
		repo, cfgLoader, cfgWriter, log,
		agentSvc, settingsRepo, agentSvc,
		&taskWorkspaceCreatorAdapter{taskSvc: services.Task},
		&workflowEnsurerAdapter{repo: taskRepo},
		&taskCreatorAdapter{taskSvc: services.Task},
		services.Office,
		&configSyncerAdapter{svc: configSvc},
	)
	onboardingSvc.SetCoordinatorRoutineInstaller(routineSvc)
	schedulerSvc := officescheduler.NewSchedulerService(repo, log, services.Office)
	labelSvc := officelabels.NewLabelService(repo)
	gitMgr := configloader.NewGitManager(cfgLoader.BasePath(), cfgLoader, log)

	return &office.Services{
		Agents:       agentSvc,
		Skills:       skillSvc,
		Projects:     projectSvc,
		Costs:        costSvc,
		Routines:     routineSvc,
		Approvals:    approvalSvc,
		Channels:     channelSvc,
		Config:       configSvc,
		Dashboard:    dashboardSvc,
		Documents:    documentSvc,
		Labels:       labelSvc,
		Onboarding:   onboardingSvc,
		Scheduler:    schedulerSvc,
		TreeControls: services.Office,
		Workspaces:   services.Office,
		Repo:         repo,
		GitManager:   gitMgr,
		KandevHome:   homeDir,
	}
}

// buildOfficeDashboardService constructs the dashboard service and wires
// all of its cross-service dependencies (governance, skill/routine listers,
// settings provider, coordinator-routine installer). Extracted from
// buildOfficeFeatureServices to keep the parent under funlen's 80-line cap.
func buildOfficeDashboardService(
	repo *officesqlite.Repository,
	log *logger.Logger,
	activity officeshared.ActivityLogger,
	agentSvc *officeagents.AgentService,
	costSvc *officecosts.CostService,
	skillSvc *officeskills.SkillService,
	routineSvc *officeroutines.RoutineService,
	approvalSvc *officeapprovals.ApprovalService,
	cfgLoader *configloader.ConfigLoader,
	cfgWriter *configloader.FileWriter,
) *officedashboard.DashboardService {
	dashboardSvc := officedashboard.NewDashboardService(repo, log, activity, agentSvc, costSvc)
	dashboardSvc.SetGovernanceStore(repo)
	dashboardSvc.SetSkillLister(skillSvc)
	dashboardSvc.SetRoutineLister(routineSvc)
	agentSvc.SetGovernanceSettings(dashboardSvc)
	agentSvc.SetGovernanceApproval(approvalSvc)
	// office-heartbeat-as-routine: every coordinator agent (onboarding or
	// post-onboarding via the agents API) gets a "Coordinator heartbeat"
	// routine on creation. The routines service is the canonical owner;
	// both creators delegate to it via a slim interface.
	agentSvc.SetCoordinatorRoutineInstaller(routineSvc)
	if cfgLoader != nil && cfgWriter != nil {
		dashboardSvc.SetSettingsProvider(&workspaceSettingsProviderAdapter{
			loader: cfgLoader,
			writer: cfgWriter,
		})
	}
	return dashboardSvc
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
	addCleanup func(func() error),
	repoCloner *repoclone.Cloner,
) *http.Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(httpmw.RequestLogger(log, "kandev"))
	router.Use(httpmw.OtelTracing("kandev"))
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())

	port := cfg.Server.Port
	if port == 0 {
		port = ports.Backend
	}

	registerRoutes(routeParams{
		router:                  router,
		gateway:                 gateway,
		taskSvc:                 services.Task,
		taskRepo:                repos.Task,
		officeRepo:              repos.Office,
		analyticsRepo:           repos.Analytics,
		orchestratorSvc:         orchestratorSvc,
		lifecycleMgr:            lifecycleMgr,
		hostUtilityMgr:          hostUtilityMgr,
		eventBus:                eventBus,
		services:                services,
		agentSettingsController: agentSettingsController,
		agentSettingsRepo:       repos.AgentSettings,
		agentList:               agentRegistry,
		agentRegistry:           agentRegistry,
		userCtrl:                usercontroller.NewController(services.User),
		notificationCtrl:        notificationCtrl,
		editorCtrl:              editorcontroller.NewController(services.Editor),
		promptCtrl:              promptcontroller.NewController(services.Prompts),
		utilityCtrl:             utilitycontroller.NewController(services.Utility),
		msgCreator:              msgCreator,
		secretsSvc:              secrets.NewService(repos.Secrets, log),
		secretStore:             repos.Secrets,
		mcpConfigSvc:            mcpconfig.NewService(repos.AgentSettings),
		addCleanup:              addCleanup,
		repoCloner:              repoCloner,
		version:                 Version,
		webInternalURL:          cfg.Server.WebInternalURL,
		devMode:                 cfg.Debug.DevMode || cfg.Debug.PprofEnabled,
		httpPort:                port,
		features:                cfg.Features,
		log:                     log,
	})

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
		_ = log.Close()
		os.Exit(1)
	}()

	log.Info("Received shutdown signal", zap.String("signal", sig.String()))
	runGracefulShutdown(server, orchestratorSvc, lifecycleMgr, runCleanups, log)
}
