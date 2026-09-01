// Package backendapp runs the Kandev backend server.
//
//revive:disable:file-length-limit
package backendapp

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth"
	authhttpmw "github.com/kandev/kandev/internal/auth/httpmw"
	"github.com/kandev/kandev/internal/common/httpmw"
	"github.com/kandev/kandev/internal/entityrefs"
	"go.uber.org/zap"

	// Common packages
	"github.com/kandev/kandev/internal/backendapp/ownershiplock"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/profiles"

	// Event bus
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"

	// GitHub integration
	azuredevopspkg "github.com/kandev/kandev/internal/azuredevops"
	githubpkg "github.com/kandev/kandev/internal/github"
	gitlabpkg "github.com/kandev/kandev/internal/gitlab"

	// JIRA integration
	jirapkg "github.com/kandev/kandev/internal/jira"
	linearpkg "github.com/kandev/kandev/internal/linear"
	sentrypkg "github.com/kandev/kandev/internal/sentry"
	workflowsyncpkg "github.com/kandev/kandev/internal/workflowsync"

	// Agent infrastructure
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/registry"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	runtimeskill "github.com/kandev/kandev/internal/agent/runtime/lifecycle/skill"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	agentctltracing "github.com/kandev/kandev/internal/agentctl/tracing"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/utility/profilebinding"

	// WebSocket gateway
	gateways "github.com/kandev/kandev/internal/gateway/websocket"

	editorcontroller "github.com/kandev/kandev/internal/editors/controller"
	notificationcontroller "github.com/kandev/kandev/internal/notifications/controller"
	promptcontroller "github.com/kandev/kandev/internal/prompts/controller"
	usercontroller "github.com/kandev/kandev/internal/user/controller"
	userservice "github.com/kandev/kandev/internal/user/service"
	userstore "github.com/kandev/kandev/internal/user/store"
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
	schedulercron "github.com/kandev/kandev/internal/scheduler/cron"

	// Workflow engine adapters (Phase 3.2 of task-model-unification)
	officeengineadapters "github.com/kandev/kandev/internal/office/engine_adapters"
	officeenginedispatcher "github.com/kandev/kandev/internal/office/engine_dispatcher"
	workflowadapters "github.com/kandev/kandev/internal/workflow/adapters"
	workflowengine "github.com/kandev/kandev/internal/workflow/engine"

	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	repoerrors "github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"

	// Repository cloning
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/runtimeflags"

	// Secrets
	"github.com/kandev/kandev/internal/secrets"

	// System pages (status / database / backups / logs / updates / about)
	systemsvc "github.com/kandev/kandev/internal/system"
	"github.com/kandev/kandev/internal/system/storage/tempartifacts"

	// Database
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/delivery"

	"github.com/kandev/kandev/internal/common/ports"
)

// Build-time variables are set by cmd/kandev before Run is called. Defaults
// apply when running un-stamped builds.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// BuildInfo contains build metadata injected into the top-level command.
type BuildInfo struct {
	Version   string
	Commit    string
	BuildTime string
}

type backendFlags struct {
	Port     int
	LogLevel string
	Help     bool
	Version  bool
}

// parseBackendFlags parses the `kandev __backend` command-line flags into a
// backendFlags struct, returning the parsed flags, a usage printer, and any
// parse error.
func parseBackendFlags(args []string) (backendFlags, func(), error) {
	flags := flag.NewFlagSet("kandev __backend", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	out := backendFlags{}
	flags.IntVar(&out.Port, "port", 0, fmt.Sprintf("HTTP server port (default: %d)", ports.Backend))
	flags.StringVar(&out.LogLevel, "log-level", "", "Log level: debug, info, warn, error")
	flags.BoolVar(&out.Help, "help", false, "Show help message")
	flags.BoolVar(&out.Version, versionFieldKey, false, "Show version information")
	flags.Usage = func() {
		_, _ = fmt.Fprintf(flags.Output(), "Usage: kandev __backend [options]\n\n")
		_, _ = fmt.Fprintf(flags.Output(), "Kandev backend server. This mode is normally started by the launcher.\n\n")
		_, _ = fmt.Fprintf(flags.Output(), "Options:\n")
		flags.PrintDefaults()
	}
	return out, flags.Usage, flags.Parse(args)
}

// ready is the readiness flag consulted by the GET /ready handler. Until it
// flips true, /ready returns 503 — so callers polling for readiness
// (Playwright fixtures, container orchestrators, manual curl loops) keep
// waiting instead of racing ahead of route registration. GET /health is a
// liveness probe and is unaffected by this flag: it answers 200 as soon as
// the listener is bound (see bindBootstrapListeners), because the launcher's
// healthcheck must succeed regardless of startup progress — making it depend
// on readiness would bring back the crash loop docs/specs/startup-listener-
// before-recovery/spec.md exists to fix. startGatewayAndServe flips this flag
// before swapping the bootstrap handler for the fully wired router (in that
// order — see the comment at the call site), so a request racing the swap
// never observes the real router with ready still false.
var ready atomic.Bool

// Run contains all startup logic and returns 0 on success or 1 on fatal error.
// Deferred cleanup is registered here so it always executes before Run returns.
func Run(args []string, build BuildInfo) int {
	setBuildInfo(build)
	ready.Store(false)

	parsedFlags, usage, err := parseBackendFlags(args)
	if err != nil {
		return 1
	}
	if parsedFlags.Help {
		usage()
		return 0
	}
	if parsedFlags.Version {
		fmt.Printf("kandev version %s (commit %s, built %s)\n", Version, Commit, BuildTime)
		return 0
	}

	// 1. Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		return 1
	}
	if _, _, err := profiles.ApplyProfile(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to apply profile defaults: %v\n", err)
		return 1
	}
	constants.ApplyPreparationTimeout(cfg.Tasks.PreparationTimeout)
	subproc.ConfigureCaps(cfg.Limits.GHMaxConcurrent, cfg.Limits.GitMaxConcurrent)

	// Apply command-line flag overrides (flags take precedence over config/env)
	if parsedFlags.Port > 0 {
		cfg.Server.Port = parsedFlags.Port
	}
	if parsedFlags.LogLevel != "" {
		cfg.Logging.Level = parsedFlags.LogLevel
	}
	agentctltracing.ConfigureEndpoint(cfg.Observability.OTLPEndpoint)

	// Acquire runtime-state ownership before any shared-state initialization.
	// The lock remains held through logger and service cleanup, so a second
	// backend cannot reconcile or migrate the live home before its bind fails.
	owner, err := acquireRuntimeStateOwnership(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"Failed to acquire backend runtime-state ownership: %v; use a separate KANDEV_HOME_DIR for an intentional second instance\n",
			err)
		return 1
	}
	defer func() {
		if closeErr := owner.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Failed to release backend runtime-state ownership: %v\n", closeErr)
		}
	}()

	// Initialize logger only after runtime-state ownership is secured.
	log, err := logger.NewBackendLogger(logger.BackendLoggingConfig{
		HomeDir:      cfg.ResolvedHomeDir(),
		Level:        cfg.Logging.Level,
		Format:       cfg.Logging.Format,
		ConsoleLevel: os.Getenv("KANDEV_CONSOLE_LOG_LEVEL"),
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

// acquireRuntimeStateOwnership acquires the advisory ownership lock over the
// backend's runtime state (home dir and database), preventing a second backend
// instance from acting on the same state.
func acquireRuntimeStateOwnership(cfg *config.Config) (*ownershiplock.Owner, error) {
	targets, err := ownershiplock.Targets(cfg.ResolvedHomeDir(), cfg.Database.Driver, cfg.Database.Path)
	if err != nil {
		return nil, fmt.Errorf("resolve backend runtime-state ownership: %w", err)
	}
	return ownershiplock.Acquire(targets)
}

// setBuildInfo stamps the package-level build variables with the provided
// build metadata, skipping fields that were not populated.
func setBuildInfo(build BuildInfo) {
	if build.Version != "" {
		Version = build.Version
	}
	if build.Commit != "" {
		Commit = build.Commit
	}
	if build.BuildTime != "" {
		BuildTime = build.BuildTime
	}
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

	return startServices(ctx, cfg, log, addCleanup, eventBus, runCleanups, cancel)
}

// applyStartupRuntimeFlags resolves persisted runtime-flag overrides and
// applies them to the config, returning false if resolution fails. It is a
// no-op when no runtime-flag repository is configured.
func applyStartupRuntimeFlags(ctx context.Context, cfg *config.Config, repos *Repositories, log *logger.Logger) bool {
	if repos.RuntimeFlags == nil {
		return true
	}
	svc := runtimeflags.NewService(repos.RuntimeFlags, runtimeflags.OptionsFromConfig(cfg))
	states, err := svc.ListStates(ctx)
	if err != nil {
		log.Error("Failed to resolve runtime flag overrides", zap.Error(err))
		return false
	}
	runtimeflags.ApplyStatesToConfig(cfg, states)
	return true
}

// startServices initializes task-level services and all downstream infrastructure.
func startServices( //nolint:cyclop
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	addCleanup func(func() error),
	eventBus bus.EventBus,
	runCleanups func(),
	cancelContext context.CancelFunc,
) bool {
	// ============================================
	// TASK SERVICE
	// ============================================
	log.Info("Initializing Task Service...")

	dbPool, repos, repoCleanups, err := provideRepositories(ctx, cfg, log, Version)
	if err != nil {
		log.Error("Failed to initialize repositories", zap.Error(err))
		return false
	}
	for _, c := range repoCleanups {
		addCleanup(c)
	}

	runtimeFlagDefaults := runtimeflags.OptionsFromConfig(cfg).DefaultValues
	if !applyStartupRuntimeFlags(ctx, cfg, repos, log) {
		return false
	}

	agentRegistry, _, err := registry.Provide(log)
	if err != nil {
		log.Error("Failed to initialize agent registry", zap.Error(err))
		return false
	}

	services, agentSettingsController, err := provideServices(cfg, log, repos, dbPool, eventBus, agentRegistry, Version)
	if err != nil {
		log.Error("Failed to initialize services", zap.Error(err))
		return false
	}
	agentRegistry.SetManagedRuntimeSelectionStore(services.ManagedRuntimeSelections)
	if services.Workflow != nil {
		addCleanup(services.Workflow.Close)
	}
	if services.GitLabCleanup != nil {
		addCleanup(services.GitLabCleanup)
	}
	services.RuntimeFlags = runtimeflags.NewService(
		repos.RuntimeFlags,
		runtimeflags.RuntimeOptionsFromAppliedConfig(runtimeFlagDefaults, cfg),
	)
	log.Info("Task Service initialized")

	services.Auth, err = provideAuthService(ctx, cfg, dbPool, repos, log)
	if err != nil {
		log.Error("Failed to initialize auth service", zap.Error(err))
		return false
	}
	warnIfExposedWithoutAuth(cfg, services.Auth, log)

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
	agentRuntimeAvailability := agentctlclient.NewAvailability(eventBus, log)
	agentctlResult, err := provideAgentctlLauncher(ctx, cfg, log, agentRuntimeAvailability)
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

	return startAgentInfrastructure(ctx, cfg, log, addCleanup, eventBus, agentRuntimeAvailability,
		dbPool, repos, services, agentSettingsController, agentRegistry, agentctlBinaryPath, runCleanups, cancelContext)
}

// startAgentInfrastructure initializes the agent lifecycle manager, worktree, orchestrator,
// gateway, and HTTP server.
//
//nolint:funlen // Moved legacy backend startup orchestration; split after launcher migration settles.
func startAgentInfrastructure(
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	addCleanup func(func() error),
	eventBus bus.EventBus,
	agentRuntimeAvailability *agentctlclient.Availability,
	dbPool *db.Pool,
	repos *Repositories,
	services *Services,
	agentSettingsController *agentsettingscontroller.Controller,
	agentRegistry *registry.Registry,
	agentctlBinaryPath string,
	runCleanups func(),
	cancelContext context.CancelFunc,
) bool {
	restoreCleanups := make([]func() error, 0)
	var databaseQuiesce func() error
	addRuntimeCleanup := func(fn func() error) func() error {
		if fn == nil {
			return nil
		}
		var stopOnce sync.Once
		var stopErr error
		stop := func() error {
			stopOnce.Do(func() { stopErr = fn() })
			return stopErr
		}
		addCleanup(stop)
		restoreCleanups = append(restoreCleanups, stop)
		return stop
	}
	userSecretStore := secrets.NewUserVisibleStore(repos.Secrets)
	mcpScopeResolver := mcpscope.NewResolver(
		repos.Task,
		services.Auth,
		func() bool { return services.Auth != nil && services.Auth.Mode() != auth.ModeDisabled },
		log,
	)
	// ============================================
	// AGENT MANAGER
	// ============================================
	lifecycleMgr, err := provideLifecycleManager(
		ctx,
		cfg,
		log,
		eventBus,
		repos.AgentSettings,
		agentRegistry,
		repos.Secrets,
		services.Task.TaskBaseBranches,
		services.Task.TaskComparisonTargets,
		services.ManagedRuntimeSelections,
		mcpScopeResolver.Scope,
		mcpScopeResolver.ScopePrincipal,
	)
	if err != nil {
		log.Error("Failed to initialize agent manager", zap.Error(err))
		return false
	}

	// ============================================
	// WORKTREE MANAGER
	// ============================================
	log.Info("Initializing Worktree Manager...")

	worktreeMgr, _, worktreeCleanup, err := provideWorktreeManager(dbPool, cfg, log, lifecycleMgr, services.Task)
	if err != nil {
		log.Error("Failed to initialize worktree manager", zap.Error(err))
		return false
	}
	services.WorktreeMgr = worktreeMgr
	addRuntimeCleanup(worktreeCleanup)
	log.Info("Worktree Manager initialized",
		zap.Bool("enabled", cfg.Worktree.Enabled))

	services.Task.SetBranchMaterializer(newBranchMaterializer(repos.Task, worktreeMgr, lifecycleMgr, log))
	workspaceSourceMaterializer := newWorkspaceSourceMaterializer(repos.Task, worktreeMgr, lifecycleMgr, log)
	services.Task.SetWorkspaceSourceMaterializer(workspaceSourceMaterializer)
	services.Task.SetWorkspaceSourceProviderRefresher(newTaskMCPProviderRefresher(repos.Task, lifecycleMgr, log))
	services.Task.SetAgentBaseBranchPusher(lifecycleMgr)
	services.Task.SetAgentComparisonTargetPusher(lifecycleMgr)

	lifecycleMgr.SetWorkspaceInfoProvider(services.Task)
	// Session/environment-scoped HTTP surfaces (shell, files, ports, vscode,
	// LSP, terminals) enforce per-user workspace scoping (opt-in auth). The
	// GetOrEnsure* execution paths run these checks internally; the vscode and
	// port reverse proxies (bare lookup + cache) call CheckSessionAccess at
	// the handler, and the SSR terminal-list routes call CheckTaskAccess /
	// CheckEnvironmentAccess / CheckTaskEnvironmentAccess in a route guard.
	wireLifecycleAccessCheckers(lifecycleMgr, services.Task)
	log.Info("Workspace info provider configured for session recovery")

	// TODO(task-model-unification Phase 2, ADR 0004): wire agentruntime.New(lifecycleMgr)
	// once a real consumer (workflow-engine / cron-driven trigger handlers) exists.
	// Allocating the facade in Phase 1 without a caller is dead code.

	// Persistence writer for executors_running. This makes the lifecycle manager
	// the sole writer of agent_execution_id / container_id / runtime / status —
	// the structural fix for the agent-execution-id divergence bug. Must be set
	// before any Launch / EnsureWorkspaceExecutionForSession can run.
	lifecycleMgr.SetExecutorRunningWriter(repos.Task)

	// Lets user shell terminals export the executor profile's env vars, so the
	// terminal sees the same variables the agent subprocess and the repository
	// setup script get.
	lifecycleMgr.SetExecutorProfileReader(repos.Task)

	// Configure quick-chat workspace cleanup
	if homeDir := cfg.ResolvedHomeDir(); homeDir != "" {
		quickChatDir := filepath.Join(homeDir, "quick-chat")
		services.Task.SetQuickChatDir(quickChatDir)
		log.Info("Quick-chat workspace cleanup configured", zap.String("quick_chat_dir", quickChatDir))
	}

	// ============================================
	// REPO CLONER
	// ============================================
	repoCloner := repoclone.NewClonerWithProtocolResolver(repoclone.Config{
		BasePath: cfg.RepoClone.BasePath,
	}, repoclone.NewGitProtocolResolver(), cfg.ResolvedHomeDir(), log)
	if services.GitHub != nil || services.Plugins != nil {
		repoCloner.SetGitCredentialProvider(
			newRepositoryCloneCredentialProvider(services.GitHub, services.Plugins),
		)
	}
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
		lifecycleMgr, agentRegistry, services.Workflow, userSecretStore, repoCloner, services.Prompts, services.GitHub, services.GitCredentials)
	if err != nil {
		log.Error("Failed to initialize orchestrator", zap.Error(err))
		return false
	}
	orchestratorSvc.SetAgentctlBinaryPath(agentctlBinaryPath)
	orchestratorSvc.SetRouteActionHandler(dynamicRouteActionHandler(
		repos.Task,
		repos.AgentSettings,
		services.DynamicProfileResolver,
		orchestratorSvc.LaunchDynamicRouteAction,
	))
	orchestratorSvc.SetProfileExecutionResolver(services.DynamicProfileResolver)

	// Wire the soft-deleted-profile pre-flight into the watcher dispatch.
	// Orphan watchers (their agent profile was soft-deleted by the
	// reconciler when its agent type left the registry) self-heal on the
	// next poll instead of looping on "profile not found" forever.
	orchestratorSvc.SetProfileLookup(&profileLookupAdapter{store: repos.AgentSettings})
	// Watcher dispatch self-heals a binding whose repository was soft-deleted
	// after the watch was configured, instead of creating an orphan task row.
	orchestratorSvc.SetRepositoryChecker(&repositoryLookupAdapter{svc: services.Task})

	// Wire the watcher-dependency enumerator into the agent settings
	// controller so the profile-delete UI can surface "this will also
	// disable N watchers" before the user confirms.
	agentSettingsController.SetWatcherDependencyChecker(&watcherDepsAdapter{
		linear: services.Linear,
		jira:   services.Jira,
		github: services.GitHub,
		log:    log,
	})
	agentSettingsController.SetUtilityDependencyChecker(&utilityDepsAdapter{svc: services.Utility, userSvc: services.User})
	agentSettingsController.SetRoutingTierDependencyChecker(&routingTierDepsAdapter{
		repo: repos.Office,
	})
	// An enabled automation is a standing instruction to launch against a
	// profile. Nothing is running, so it never reaches the active-session list,
	// but deleting the profile would leave the schedule firing into nothing —
	// quietly, hours later. Name them in the confirmation instead.
	if services.Automation != nil {
		agentSettingsController.SetAutomationDependencyChecker(&automationDepsAdapter{
			store: services.Automation.Service.Store(),
		})
	}

	// Wire automation service into orchestrator for trigger-based task creation.
	// The service is constructed and wired here, but starts after the orchestrator
	// has subscribed to automation.triggered in startGatewayAndServe.
	// The Automation subsystem is independent of the Office feature flag — it
	// has its own cron scheduler, GitHub poller, webhook handler, and creates
	// tasks via the task service directly.
	if services.Automation != nil {
		orchestratorSvc.SetAutomationService(services.Automation.Service)
		services.Automation.Service.SetRunStopper(orchestratorSvc)
		services.Automation.Service.SetRunLivenessChecker(orchestratorSvc)
	}

	// Wire GitHub service into orchestrator for PR auto-detection on push
	if services.GitHub != nil {
		orchestratorSvc.SetGitHubService(services.GitHub)
		services.GitHub.SetTaskDeleter(&taskDeleterAdapter{svc: services.Task})
		taskStoreAdapter := githubTaskIssueStoreAdapter{svc: services.Task}
		services.GitHub.SetTaskIssueStore(taskStoreAdapter)
		services.GitHub.SetTaskRepositoryBaseBranchUpdater(taskStoreAdapter)
		services.GitHub.SetTaskSessionChecker(&taskSessionCheckerAdapter{repo: repos.Task})
		services.GitHub.SetWorkspaceGroupOwnerResolver(repos.Task)
		log.Info("GitHub service configured for orchestrator (PR auto-detection enabled)")

	}

	// Start GitLab background poller + wire the service into the
	// orchestrator so review/issue watch events get turned into tasks.
	if services.GitLab != nil {
		orchestratorSvc.SetGitLabService(services.GitLab)
		orchestratorSvc.SetGitLabMRLinkService(services.GitLab)
		orchestratorSvc.SetGitLabCredentialResolver(services.GitLab)
		orchestratorSvc.SetGitLabMRAutomationService(services.GitLab)
		services.GitLab.SetTaskDeleter(&taskDeleterAdapter{svc: services.Task})
		services.GitLab.SetTaskSessionChecker(&taskSessionCheckerAdapter{repo: repos.Task})
		services.GitLab.SetTaskAuthorizer(services.Task)
		glPoller := gitlabpkg.NewPoller(services.GitLab, eventBus, log)
		glPoller.Start(ctx)
		addRuntimeCleanup(func() error { glPoller.Stop(); return nil })
		log.Info("GitLab poller started")
	}
	// Bind only the path-returning orchestrator seam after its clone pipeline
	// and workspace-scoped GitLab credential resolver are both configured.
	workspaceSourceMaterializer.SetHostRepositoryCloner(orchestratorSvc)

	// Azure DevOps owns connection-health and work-item/pull-request watcher
	// polling. Watch matches flow through the shared orchestrator coordinator.
	if services.AzureDevOps != nil {
		orchestratorSvc.SetAzureDevOpsService(services.AzureDevOps)
		services.AzureDevOps.SetTaskSessionChecker(&taskSessionCheckerAdapter{repo: repos.Task})
		azureLifecycle, lifecycleErr := azuredevopspkg.RegisterLifecycleCleanup(eventBus, services.AzureDevOps)
		if lifecycleErr != nil {
			log.Warn("Azure DevOps lifecycle cleanup unavailable", zap.Error(lifecycleErr))
		} else {
			addRuntimeCleanup(azureLifecycle.Close)
		}
		azurePoller := azuredevopspkg.NewPoller(services.AzureDevOps, log)
		azurePoller.Start(ctx)
		addRuntimeCleanup(func() error { azurePoller.Stop(); return nil })
		log.Info("Azure DevOps auth poller started")
	}

	// Start JIRA poller. Drives two background loops sharing one service: an
	// auth-health probe (so the UI can show connect status without polling
	// JIRA itself) and an issue-watch loop that runs configured JQL queries
	// and emits NewJiraIssueEvent for the orchestrator to turn into tasks.
	if services.Jira != nil {
		orchestratorSvc.SetJiraService(&jiraServiceAdapter{svc: services.Jira})
		jiraPoller := jirapkg.NewPoller(services.Jira, log)
		jiraPoller.Start(ctx)
		addRuntimeCleanup(func() error { jiraPoller.Stop(); return nil })
	}

	// Start Linear poller. Mirrors the Jira shape: auth-health probe plus an
	// issue-watch loop that runs configured filters and emits
	// NewLinearIssueEvent for the orchestrator to turn into tasks.
	if services.Linear != nil {
		orchestratorSvc.SetLinearService(&linearServiceAdapter{svc: services.Linear})
		linearPoller := linearpkg.NewPoller(services.Linear, log)
		linearPoller.Start(ctx)
		addRuntimeCleanup(func() error { linearPoller.Stop(); return nil })
	}

	// Start Sentry poller: an auth-health probe plus an issue-watch loop that
	// runs configured filters and emits NewSentryIssueEvent. The dedup adapter
	// lets the orchestrator turn matching Sentry issues into kandev tasks.
	if services.Sentry != nil {
		orchestratorSvc.SetSentryService(&sentryServiceAdapter{svc: services.Sentry})
		sentryPoller := sentrypkg.NewPoller(services.Sentry, log)
		sentryPoller.Start(ctx)
		addRuntimeCleanup(func() error { sentryPoller.Stop(); return nil })
	}

	// Start workflow-sync poller: periodically pulls workflow definition
	// files from each workspace's configured GitHub repo and reconciles the
	// workspace's synced workflows with them.
	if services.WorkflowSync != nil {
		workflowSyncPoller := workflowsyncpkg.NewPoller(services.WorkflowSync, log)
		workflowSyncPoller.Start(ctx)
		addRuntimeCleanup(func() error { workflowSyncPoller.Stop(); return nil })
		log.Info("Workflow sync poller started")
	}

	// Start the plugin system's event delivery and health monitor
	// background loops.
	if services.Plugins != nil {
		startPluginsSubsystems(ctx, services.Plugins, lifecycleMgr, eventBus, log,
			func(fn func() error) { addRuntimeCleanup(fn) })
	}

	// Start the task delivery ledger sweep. Must run after task/repository
	// tables exist (already true here, provided in provideRepositories) —
	// the ledger's foreign keys require them present at CREATE TABLE time
	// on PostgreSQL. services.Task satisfies delivery.CheckoutResolver.
	if _, deliveryCleanup, err := delivery.Provide(dbPool.Writer(), dbPool.Reader(), services.Task, log); err != nil {
		log.Warn("delivery ledger sweep unavailable", zap.Error(err))
	} else {
		// Must be addRuntimeCleanup, not addCleanup: RestoreQuiesce only
		// stops workers registered here, and a restore checkpoints, closes,
		// and replaces the shared database pool. A five-minute sweep pass
		// overlapping that would race the pool swap.
		databaseQuiesce = addRuntimeCleanup(deliveryCleanup)
	}

	return startGatewayAndServe(ctx, cfg, log, eventBus, agentRuntimeAvailability, dbPool, repos, services,
		agentSettingsController, lifecycleMgr, agentRegistry, orchestratorSvc, msgCreator, repoCloner, agentctlBinaryPath,
		func(fn func() error) { addRuntimeCleanup(fn) }, runCleanups, cancelContext, restoreCleanups, databaseQuiesce)
}

// startOrchestratorAndAutomationConsumers establishes the startup chain in
// dependency order: the HTTP listener must be bound (so the launcher's
// liveness probe can succeed) before the orchestrator runs its startup
// recovery sweeps, which can run long. The GitHub poller performs an
// immediate sweep on start, so both downstream consumers must be ready
// before it is started.
func startOrchestratorAndAutomationConsumers(
	bindListeners func() error,
	startOrchestrator func() error,
	startAutomation func(),
	startGitHubPoller func(),
) error {
	if err := bindListeners(); err != nil {
		return err
	}
	if err := startOrchestrator(); err != nil {
		return err
	}
	startAutomation()
	startGitHubPoller()
	return nil
}

// closeBoundListeners releases the HTTP listener(s) bound by bindListeners
// when a later startup step fails and startGatewayAndServe returns early.
// listeners.Stop only halts the background bind-retry loop; the bound TCP
// sockets are released by the shared http.Server (see serverListeners's doc
// comment), so an early return that skips this leaves the port held by this
// process even though the caller sees a start failure. server is non-nil
// whenever listeners is, since bindListeners assigns both together.
func closeBoundListeners(server *http.Server, listeners *serverListeners, log *logger.Logger) {
	if listeners == nil {
		return
	}
	listeners.Stop()
	if err := server.Close(); err != nil {
		log.Warn("failed to close HTTP listeners after startup failure", zap.Error(err))
	}
}

// startGatewayAndServe sets up the WebSocket gateway, HTTP routes, starts the server,
// and blocks until a shutdown signal.
//
//nolint:funlen // Moved legacy backend startup orchestration; split after launcher migration settles.
func startGatewayAndServe(
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	eventBus bus.EventBus,
	agentRuntimeAvailability *agentctlclient.Availability,
	dbPool *db.Pool,
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
	cancelContext context.CancelFunc,
	restoreCleanups []func() error,
	databaseQuiesce func() error,
) bool {
	// ============================================
	// WEBSOCKET GATEWAY
	// ============================================
	log.Info("Initializing WebSocket Gateway...")
	var referenceValidator entityrefs.SubmissionValidator
	if services.Mentions != nil {
		referenceValidator = services.Mentions.Submission
	}
	gateway, notificationSvc, notificationCtrl, terminalSvc, err := provideGateway(
		ctx, log, eventBus, services.Task, services.User,
		orchestratorSvc, lifecycleMgr, agentRegistry,
		repos.Notification, repos.Task, repos.Terminal, services.GitHub, services.GitLab,
		referenceValidator,
		// Notifications drop rather than redirect when a task's owner cannot
		// be resolved while authentication is enforced.
		services.Auth,
		cfg.ResolvedHomeDir(),
		cfg.Limits.LSPMaxConnections,
	)
	if terminalSvc != nil {
		services.Terminal = terminalSvc
	}
	if err != nil {
		log.Error("Failed to initialize WebSocket gateway", zap.Error(err))
		return false
	}

	gateways.RegisterSessionStreamNotifications(ctx, eventBus, gateway.Hub, log)
	gateway.Hub.SetSessionDataProvider(buildSessionDataProvider(repos.Task, lifecycleMgr, orchestratorSvc, log))
	gateway.Hub.SetSessionGitDataProvider(buildSessionGitDataProvider(repos.Task, lifecycleMgr, log))
	log.Info("Session data provider configured for session subscriptions (git status from snapshots)")

	// WS gateway per-user scoping (opt-in auth): connection auth on upgrade
	// and proxy routes, subscription visibility checks, and workspace-owner
	// broadcast routing. Must be installed before SetupRoutes runs in
	// buildHTTPServer.
	gateway.SetAuthPolicy(gatewayAuthPolicy(services.Auth, services.Task, repos.Task, repos.Office))

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
	hostUtilityMgr.SetProfileResolver(profilebinding.New(repos.AgentSettings, func(agentID string) bool {
		_, ok := agentRegistry.GetInferenceAgent(agentID)
		return ok
	}))
	hostUtilityMgr.SetManagedRuntimeSelectionStore(services.ManagedRuntimeSelections)
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
		if migrated, err := services.Utility.MigrateLegacyBindings(ctx); err != nil {
			log.Warn("utility profile migration failed", zap.Error(err))
		} else if migrated > 0 {
			log.Info("migrated utility profile bindings", zap.Int("updated", migrated))
		}
		migrateDefaultUtilityProfile(ctx, services.User, repos.AgentSettings, agentRegistry, log)
	}()
	addCleanup(func() error {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		hostUtilityMgr.Stop(stopCtx)
		return nil
	})

	// Wire Host.InvokeUtilityAgent (ADR 0048): plugins delegate one-shot LLM
	// calls to the utility agent selected in each plugin's configuration and
	// runs them through the sessionless host-utility tier, at the first point
	// where hostUtilityMgr is live.
	if services.Plugins != nil && services.Utility != nil {
		services.Plugins.SetUtilityAgent(pluginsUtilityAgentAdapter{svc: services.Utility, userSvc: services.User}, pluginsHostUtilityAdapter{mgr: hostUtilityMgr})
	}

	var (
		handler   *handlerSwitch
		server    *http.Server
		listeners *serverListeners
	)
	bindListeners := func() error {
		h, s, l, err := bindBootstrapListeners(cfg, log, Version)
		if err != nil {
			return err
		}
		handler, server, listeners = h, s, l
		return nil
	}

	if err := startOrchestratorAndAutomationConsumers(
		bindListeners,
		func() error { return orchestratorSvc.Start(ctx) },
		func() {
			if services.Automation == nil {
				return
			}
			services.Automation.Start(ctx)
			addCleanup(func() error { services.Automation.Stop(); return nil })
			log.Info("Automation scheduler and evaluator started")
		},
		func() {
			if services.GitHub == nil {
				return
			}
			ghPoller := githubpkg.NewPoller(services.GitHub, eventBus, log)
			ghPoller.SetTaskBranchProvider(orchestratorSvc)
			ghPoller.Start(ctx)
			addCleanup(func() error { ghPoller.Stop(); return nil })
			log.Info("GitHub poller started")
		},
	); err != nil {
		if !errors.Is(err, errServerBindFailed) {
			log.Error("Failed to start orchestrator", zap.Error(err))
		}
		closeBoundListeners(server, listeners, log)
		return false
	}
	log.Info("Orchestrator initialized")

	// Wire the Host data API's late write dependencies (ADR 0043 phase 2): the
	// task-message delivery path backs SendMessage (api_write:messages), and
	// the orchestrator backs CreateTask's start_agent. The orchestrator is
	// constructed after StartActivePlugins spawns boot-active plugins, so the
	// plugins service reads these live rather than snapshotting (see
	// SetWriteDeps).
	//
	// Deliberately wired here rather than inside initOfficeServices: that
	// function returns early when features.office=false (the production
	// default), while plugins start whenever services.Plugins is non-nil.
	// Wiring it there would leave every default production backend with
	// Unimplemented SendMessage and a silently no-op start_agent.
	if services.Plugins != nil {
		messenger := pluginsTaskMessengerAdapter{tasks: services.Task, orch: orchestratorSvc, log: log}
		services.Plugins.SetWriteDeps(messenger, pluginsTaskStarterAdapter{orch: orchestratorSvc, log: log})
	}

	// ============================================
	// OFFICE FEATURES + GLOBAL RUN SCHEDULING
	// ============================================
	runProcessorSvc, ok := initOfficeServices(ctx, cfg, log, repos, services, orchestratorSvc, eventBus, agentctlBinaryPath, addCleanup, lifecycleMgr, agentRegistry)
	if !ok {
		closeBoundListeners(server, listeners, log)
		return false
	}
	scheduling := startSchedulingRuntime(
		ctx, repos, services, eventBus, orchestratorSvc, runProcessorSvc, log,
		runsscheduler.TickIntervalFromConfig(cfg.Office.SchedulerTickMs),
	)
	addCleanup(scheduling.Stop)
	var restoreQuiesceOnce sync.Once
	var restoreQuiesceErr error
	restoreQuiesce := func() error {
		restoreQuiesceOnce.Do(func() {
			workers := make([]func() error, 0, len(restoreCleanups))
			for i := len(restoreCleanups) - 1; i >= 0; i-- {
				workers = append(workers, restoreCleanups[i])
			}
			restoreQuiesceErr = quiesceForRestore(
				cancelContext,
				scheduling.Stop,
				orchestratorSvc.Stop,
				func() error { return stopLifecycleManager(lifecycleMgr, log) },
				workers,
			)
		})
		return restoreQuiesceErr
	}

	// Wire subscription usage provider into the office agents service so the
	// /agents/:id/utilization endpoint can fetch live utilization data.
	// Skipped when the Office feature flag is off (services.OfficeSvcs is nil).
	if services.OfficeSvcs != nil && services.OfficeSvcs.Agents != nil {
		usageAdapter := newUsageProviderAdapter(repos.AgentSettings, agentRegistry)
		services.OfficeSvcs.Agents.SetUsageProvider(usageAdapter)
	}

	services.Task.StartAutoArchiveLoop(ctx)
	services.Task.StartArchivedSessionReconciliationLoop(ctx)
	services.Task.StartQuickChatExpirationLoop(ctx)

	// ============================================
	// SYSTEM PAGES
	// ============================================
	// Composed before HTTP routes so the registration pass below can mount
	// the /api/v1/system/* group; started before the listener so the
	// updates poller is alive as soon as we accept connections.
	systemSvc := systemsvc.Provide(cfg, log, dbPool, eventBus, systemsvc.BuildInfo{
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
	}, systemsvc.Wiring{
		OrchestratorShutdown: func() { _ = orchestratorSvc.Stop() },
		DatabaseQuiesce:      databaseQuiesce,
		RestoreQuiesce:       restoreQuiesce,
		MessageQueue:         orchestratorSvc.GetMessageQueue(),
		MessageQueueConfig:   queueConfiguration(cfg),
		TaskSessions:         repos.Task,
	})
	storageComposition, err := provideStorageComposition(
		cfg, dbPool, systemSvc.Jobs, lifecycleMgr, services.WorktreeMgr, services.Task,
		log,
		func(message string, err error) { log.Error(message, zap.Error(err)) },
	)
	if err != nil {
		log.Error("Failed to initialize storage maintenance", zap.Error(err))
		closeBoundListeners(server, listeners, log)
		return false
	}
	hostUtilityMgr.SetTemporaryArtifactRegistry(storageComposition.tempArtifacts)
	hostUtilityCtx, hostUtilityCancel := context.WithCancel(ctx)
	var hostUtilityWG sync.WaitGroup
	hostUtilityWG.Add(1)
	go func() {
		defer hostUtilityWG.Done()
		if err := hostUtilityMgr.Start(hostUtilityCtx); err != nil {
			log.Warn("host utility manager bootstrap error", zap.Error(err))
		}
		// Reconcile profiles against fresh probe results — seeds defaults for
		// newly probed agents, heals stale profile models/modes, cleans up
		// orphans referencing removed agents.
		if err := profileReconciler.Run(hostUtilityCtx); err != nil {
			log.Warn("profile reconciler error", zap.Error(err))
		}
	}()
	addCleanup(func() error {
		hostUtilityCancel()
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		hostUtilityMgr.Stop(stopCtx)
		hostUtilityWG.Wait()
		return nil
	})
	systemSvc.Storage = storageComposition.handler
	systemSvc.StorageRuntime = storageComposition.runtime
	if systemSvc.LogBundles != nil {
		systemSvc.LogBundles.SetNotifier(gateway.Hub)
		systemSvc.LogBundles.SetSessionProvider(newDiagnosticSessionProvider(services.Task))
		systemSvc.LogBundles.SetACPExporter(newDiagnosticACPExporter(lifecycleMgr))
	}
	if systemSvc.Metrics != nil {
		systemSvc.Metrics.SetBroadcaster(gateway.Hub.BroadcastToSystemMetrics)
		gateway.Hub.SetSystemMetricsInterestTracker(systemSvc.Metrics)
		systemSvc.Metrics.SetExecutionProvider(lifecycleMetricProvider{manager: lifecycleMgr})
	}
	if systemSvc.Updates != nil {
		systemSvc.Updates.SetNotifier(notificationSvc)
		gateway.Hub.AddUserSubscriptionListener(func(userID string) {
			if userID != userstore.DefaultUserID {
				return
			}
			if err := systemSvc.Updates.ReplayCachedUpdate(ctx); err != nil {
				log.Warn("failed to replay cached update notification", zap.Error(err))
			}
		})
	}
	systemSvc.StartBackground(ctx)
	addCleanup(func() error { systemSvc.StopBackground(); return nil })
	gateways.RegisterSystemNotifications(ctx, eventBus, gateway.Hub, log)
	gateways.RegisterAgentRuntimeNotifications(ctx, eventBus, gateway.Hub, func() (any, bool) {
		if agentRuntimeAvailability == nil {
			return nil, false
		}
		snapshot, ok := agentRuntimeAvailability.Snapshot()
		return snapshot, ok
	}, log)

	// ============================================
	// HTTP SERVER
	// ============================================
	// The listener was already bound (and is serving the bootstrap handler)
	// by bindListeners above, before orchestratorSvc.Start ran its startup
	// recovery sweeps. Build the real router now and swap it in on the same,
	// already-bound handler and listeners — no second bind, no window where
	// the socket is closed and reopened.
	builtServer, err := buildHTTPServer(cfg, log, gateway, repos, services, agentSettingsController,
		lifecycleMgr, eventBus, orchestratorSvc, notificationCtrl, msgCreator, agentRegistry, hostUtilityMgr,
		addCleanup, repoCloner, systemSvc, storageComposition.workspaceRestorer,
		storageComposition.tempArtifacts, dbPool, agentRuntimeAvailability)
	if err != nil {
		log.Error("Failed to build HTTP server", zap.Error(err))
		closeBoundListeners(server, listeners, log)
		return false
	}

	log.Info("API configured",
		zap.String("websocket", "/ws"),
		zap.String("health", "/health"),
		zap.String("http", "/api/v1"),
	)

	// Flip readiness before swapping in the fully wired router — see
	// publishReadiness for why the order matters and
	// TestPublishReadinessFlipsReadyBeforeSwappingHandler for the regression
	// test pinning it.
	publishReadiness(func() { ready.Store(true) }, func() { handler.Store(builtServer.Handler) })

	awaitShutdown(server, listeners, scheduling, orchestratorSvc, lifecycleMgr, runCleanups, log)
	return true
}

// publishReadiness flips readiness and then swaps in the fully wired router,
// in that order — never the reverse. A request landing between the two must
// never observe the real router while ready is still false: readyHandler
// gates on ready.Load(), so the reverse order would 503 a client on a router
// that is otherwise already fully up, recreating the exact flap
// docs/specs/startup-listener-before-recovery/spec.md exists to prevent.
// Flipping ready first means any request in that window still hits the
// bootstrap handler, whose /health branch is unconditionally 200 and whose
// every other path (including /ready) is a deterministic 503 "starting" —
// never a stale ready=false read through the real router.
func publishReadiness(markReady func(), swapHandler func()) {
	markReady()
	swapHandler()
}

// serverListenAddr formats the host and port into a listen address, binding
// all interfaces when host is empty.
func serverListenAddr(host string, port int) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Sprintf(":%d", port)
	}
	return net.JoinHostPort(host, fmt.Sprint(port))
}

// initOfficeServices constructs the run processor service for every backend and
// adds Office-only services, reconciliation, and subscribers when the feature
// is enabled. Global run and cron scheduling starts separately so this feature
// gate cannot disable workflow-generic queue_run dispatch.
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
) (*officeservice.Service, bool) {
	configBasePath := cfg.ResolvedHomeDir()
	var cfgLoader *configloader.ConfigLoader
	var cfgWriter *configloader.FileWriter
	if cfg.Features.Office {
		cfgLoader, cfgWriter = initOfficeConfigLoader(configBasePath, log, addCleanup)
	}

	runProcessorSvc := newRunProcessorService(
		cfg, repos, services, orchestratorSvc, eventBus,
		agentctlBinaryPath, cfgLoader, cfgWriter, log,
	)

	// Task dependencies are a core Kanban relationship, not an Office feature.
	// The task_blockers table physically lives in the Office repository's DDL but
	// sits in the same database, so wire the store BEFORE the Office early return
	// below. Wiring it after left a Kanban-only install with no dependency store
	// at all: blocked_by on create failed and list_related_tasks reported nothing.
	services.Task.SetBlockerRepository(repos.Office)

	// office-costs Wave B: lazy models.dev pricing lookup. The Client
	// allocates no resources at startup — the first non-claude-acp cost
	// event triggers a disk read; the first missing cache file triggers
	// a background fetch. Workspaces running only claude-acp stay
	// untouched because Layer A handles every event before lookup.
	//
	// Constructed here, above the Office early return, because
	// task_usage_events's ledger writer (docs/specs/task-cost-ledger/spec.md
	// AC-10, AC-26) needs it in every install, not just Office-enabled ones.
	// SetPricingLookup/SetModelInfoLookup below stay gated - the Office
	// service and orchestrator model-info surface those Office features
	// widen only apply when the feature is on.
	modelsdevCachePath := filepath.Join(cfg.ResolvedHomeDir(), "cache", "models-dev.json")
	pricingLookup := officemodelsdev.New(officemodelsdev.Config{
		CachePath: modelsdevCachePath,
	}, log)

	// The ledger writer is the sole writer of task_sessions' usage rollup
	// columns (AC-10) and must run in every install, so it too is
	// constructed and started before the Office early return.
	if err := startTaskUsageWriter(repos.Task, usagePricingAdapter{lookup: pricingLookup}, eventBus, log, addCleanup); err != nil {
		log.Error("Failed to subscribe task usage ledger writer", zap.Error(err))
		return nil, false
	}

	if !cfg.Features.Office {
		log.Info("Office feature disabled; Office services skipped while global run scheduling remains enabled")
		return runProcessorSvc, true
	}

	services.Office = runProcessorSvc
	log.Info("Office service constructed with all dependencies")

	services.Office.SetPricingLookup(pricingLookup)
	orchestratorSvc.SetModelInfoLookup(pricingLookup)

	// ADR 0005 Wave E: plug the runtime-tier skill deployer into the
	// lifecycle manager. The deployer reads office's skills repo +
	// instructions repo via small adapters; office no longer ships
	// its own delivery code.
	wireRuntimeSkillDeployer(lifecycleMgr, agentRegistry, repos.Office, services.Office, cfg.ResolvedHomeDir(), log)

	// Wire office-owned repositories into the task service for cross-package
	// operations. The blocker repository is wired above, before the Office gate,
	// because task dependencies are not Office-only.
	services.Task.SetCommentRepository(repos.Office)

	// Build feature-package services and wire all inter-service dependencies.
	services.OfficeSvcs = buildOfficeFeatureServices(
		repos.Office, repos.Task, repos.AgentSettings, cfgLoader, cfgWriter, configBasePath,
		agentRegistry, log, services, cfg.Office.JWTSigningKey,
	)
	wireOfficeSvcsDependencies(services, repos, eventBus, orchestratorSvc, agentRegistry)
	services.OfficeSvcs.Dashboard.SetOfficeSessionIdentity(cfg.Features.OfficeSessionIdentity)

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

	// Register Office-only event subscribers. Global scheduling starts after
	// this initializer returns, regardless of the feature flag.
	if err := services.Office.RegisterEventSubscribers(eventBus); err != nil {
		log.Error("Failed to register office event subscribers", zap.Error(err))
		return nil, false
	}

	return runProcessorSvc, true
}

// newRunProcessorService constructs the office run-processor service, wiring
// its workspace, task, PR, and task-starter adapters to the backend services.
func newRunProcessorService(
	cfg *config.Config,
	repos *Repositories,
	services *Services,
	orchestratorSvc *orchestrator.Service,
	eventBus bus.EventBus,
	agentctlBinaryPath string,
	cfgLoader *configloader.ConfigLoader,
	cfgWriter *configloader.FileWriter,
	log *logger.Logger,
) *officeservice.Service {
	apiPort := cfg.Server.Port
	if apiPort == 0 {
		apiPort = ports.Backend
	}
	return officeservice.NewService(officeservice.ServiceOptions{
		Repo:               repos.Office,
		Logger:             log,
		CfgLoader:          cfgLoader,
		CfgWriter:          cfgWriter,
		WorkspaceCreator:   &taskWorkspaceCreatorAdapter{taskSvc: services.Task},
		TaskWorkspace:      services.Task,
		TaskCreator:        &taskCreatorAdapter{taskSvc: services.Task},
		TaskPRs:            &taskPRListerAdapter{gh: services.GitHub},
		APIBaseURL:         fmt.Sprintf("http://localhost:%d/api/v1", apiPort),
		TaskStarter:        newOfficeTaskStarter(orchestratorSvc),
		TaskCanceller:      orchestratorSvc,
		AgentctlBinaryPath: agentctlBinaryPath,
		EventBus:           eventBus,
	})
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
	// Wire the office service as the dashboard's run resolver so the
	// synchronous task_status_changed activity write (UpdateTaskStatus)
	// tags its row with the originating run id, matching the async
	// subscriber it replaced.
	services.OfficeSvcs.Dashboard.SetRunResolver(services.Office)
	// Wire the Office activity projection before task.state_changed events
	// reach the WebSocket broadcaster, so workflow moves have durable timeline
	// data when the frontend refetches the task detail.
	services.Task.SetTaskStateActivityLogger(services.OfficeSvcs.Dashboard)
	// Wire the office service as the retry canceller for task reassignment.
	services.OfficeSvcs.Dashboard.SetRetryCanceller(services.Office)
	// Wire the office service as the task canceller for status→cancelled hard-cancels.
	services.OfficeSvcs.Dashboard.SetTaskCanceller(services.Office)
	// Route the Office "No parent" mutation through the canonical task detach
	// operation so inherited workspace sharing remains valid.
	services.OfficeSvcs.Dashboard.SetTaskDetacher(services.Task)
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
	resolver.SetExecutionProfileStore(repos.AgentSettings, agentRegistry)
	scheduler.SetResolver(resolver)
	scheduler.SetTaskStarter(&schedulerTaskStarterAdapter{orch: orchestratorSvc})
	scheduler.SetEventBus(eventBus)
	services.Office.SetRoutingDispatcher(scheduler)

	provider := routing.NewProvider(repos.Office, agentRegistry, resolver, scheduler)
	provider.SetExecutionProfileStore(repos.AgentSettings)
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

// GetWorkspaceRouting returns the routing configuration for a workspace.
func (a *officeRoutingRepoAdapter) GetWorkspaceRouting(
	ctx context.Context, workspaceID string,
) (*routing.WorkspaceConfig, error) {
	return a.repo.GetWorkspaceRouting(ctx, workspaceID)
}

// ListProviderHealth returns the provider health statuses for a workspace.
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

// StartTask starts a task on the orchestrator with the given launch parameters.
func (a *schedulerTaskStarterAdapter) StartTask(
	ctx context.Context,
	taskID, agentProfileID, executorID, executorProfileID string,
	priority, prompt, workflowStepID string,
	planMode bool, attachments []interface{},
) error {
	_, err := a.orch.StartTask(ctx, taskID, agentProfileID,
		executorID, executorProfileID, priority, prompt,
		workflowStepID, planMode, false, nil)
	return err
}

// StartTaskWithRoute starts a task on the orchestrator with an explicit launch
// context and route override.
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
			ExecutionProfileID: route.ExecutionProfileID,
			ProviderID:         route.ProviderID,
			Model:              route.Model,
			Tier:               route.Tier,
			Mode:               route.Mode,
			Flags:              route.Flags,
			Env:                route.Env,
		})
}

// startSchedulingRuntime wires the backend-wide runs service, workflow engine
// dispatcher, runs scheduler, and shared cron loop. Office recovery is attached
// only when Office feature services were initialized.
func startSchedulingRuntime(
	ctx context.Context,
	repos *Repositories,
	services *Services,
	eventBus bus.EventBus,
	orchestratorSvc *orchestrator.Service,
	runProcessorSvc *officeservice.Service,
	log *logger.Logger,
	tickInterval time.Duration,
) *schedulingRuntime {
	log.Info("Global run processor wired to orchestrator StartTask")
	orchScheduler := officeservice.NewSchedulerIntegration(
		runProcessorSvc, tickInterval,
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
	runProcessorSvc.SetRunsService(runsSvc)
	// Phase 4 (ADR-0004): wire the workflow engine's dependencies and a
	// dispatcher so office event subscribers route through the engine
	// unconditionally.
	engineDispatcher := wireWorkflowEngineForOffice(
		orchestratorSvc, runProcessorSvc, services.Task, services.Workflow, repos, runsSvc, log,
	)
	if services.OfficeSvcs != nil {
		services.OfficeSvcs.Dashboard.SetWorkflowEngineDispatcher(engineDispatcher)
	}
	// Start the runs scheduler (tick + signal listener). It drives
	// orchScheduler.Tick on both periodic ticks and event-driven signals.
	runScheduler := runsscheduler.New(
		orchScheduler, runsSvc.SubscribeSignal(),
		tickInterval, log,
	)
	runScheduler.Start(ctx)
	log.Info("Runs scheduler started",
		zap.Duration("tick", tickInterval))
	// Phase 5 (ADR-0004): start the shared cron loop. The routines handler
	// degrades to a no-op when routineSvc is nil, so omitting Office's
	// scheduler is safe when features.office is off.
	var officeRoutines *officeroutines.RoutineService
	if services.OfficeSvcs != nil {
		officeRoutines = services.OfficeSvcs.Routines
	}
	var officeRecovery schedulercron.Handler
	var parentWakeReconciler schedulercron.Handler
	if services.Office != nil {
		officeRecovery = officeservice.NewOfficeRecoveryHandler(orchScheduler)
		parentWakeReconciler = officeservice.NewParentWakeReconciler(orchScheduler)
	}
	cronLoop := startCronScheduler(
		ctx, repos, engineDispatcher, officeRoutines, officeRecovery, parentWakeReconciler, log,
	)
	return &schedulingRuntime{runs: runScheduler, cron: cronLoop}
}

// wireWorkflowEngineForOffice composes the Phase 2 (ADR-0004)
// dependencies the workflow engine needs to evaluate office triggers,
// then builds an engine-dispatcher and hands it to the office service.
//
// Engine options wired here:
//   - RunQueueAdapter        — runs service (Phase 3.1)
//   - ParticipantStore       — workflow_step_participants
//   - DecisionStore          — workflow_step_decisions
//   - PrimaryAgentResolver   — current task runner / workflow_steps.agent_profile_id
//   - CEOAgentResolver       — agent_profiles WHERE role='ceo' AND workspace_id != ”
//   - ParticipantSeatWriter  — workflow_step_participants (REQ-OFFICE-REVIEW-SEATS-001)
//   - ParticipantSeatCaster  — casting resolution over workspace CEO agents (REQ-OFFICE-REVIEW-SEATS-002)
//   - AgentProfileResolver   — drops a required seat whose agent profile was deleted since casting (REQ-OFFICE-REVIEW-SEATS-004.3)
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
	// REQ-OFFICE-REVIEW-SEATS-001/-002: the writer persists seats; the
	// caster resolves who fills them via the casting resolution algorithm
	// (system design "Casting resolution") over the workspace's CEO agents,
	// falling back to the task's runner when none are eligible.
	orchestratorSvc.SetEngineParticipantSeatWriter(workflowadapters.NewParticipantSeatWriterAdapter(repos.Workflow))
	orchestratorSvc.SetEngineParticipantSeatCaster(officeengineadapters.NewSeatCasterAdapter(repos.Office, repos.Workflow))
	// REQ-OFFICE-REVIEW-SEATS-004.3: drop a required seat whose agent profile
	// was deleted after casting, rather than waiting forever on it.
	orchestratorSvc.SetEngineAgentProfileResolver(officeengineadapters.NewAgentProfileResolverAdapter(repos.Office))
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

	repos.Task.SetStepEntryDispatcher(&engineStepEntryDispatcherAdapter{engineProvider: orchestratorSvc, log: log})
	log.Info("step entry dispatcher wired for workflow engine")

	return dispatcher
}

// workflowEngineProvider is the seam engineStepEntryDispatcherAdapter reads
// the engine through. orchestrator.Service.WorkflowEngine satisfies it
// structurally. Declared as an interface (rather than holding a
// *workflowengine.Engine field directly) so the adapter always reads the
// engine that is current *at dispatch time*, not whatever engine existed at
// boot-wiring time — see DispatchStepEntry.
type workflowEngineProvider interface {
	WorkflowEngine() *workflowengine.Engine
}

// engineStepEntryDispatcherAdapter adapts (*engine.Engine).DispatchStepEntry
// to tasksqlite.StepEntryDispatcher, the seam every registered
// step-transition writer calls synchronously after its own commit. See
// docs/specs/office/system-design/step-entry-sequence-execution.md.
type engineStepEntryDispatcherAdapter struct {
	engineProvider workflowEngineProvider
	log            *logger.Logger
}

// DispatchStepEntry runs the step's session-independent on_enter sequence and
// logs the entry identity, step, attempted action kinds, and any failure
// reason (design's Observability section). Excluded (session-shaped) kinds
// are not logged here — DispatchStepEntry itself skips them silently, which
// is the contract, not an anomaly to report.
//
// The engine is resolved from engineProvider on every call, not captured at
// construction time: wireWorkflowEngineForOffice runs from
// startSchedulingRuntime, which executes before later boot wiring (e.g.
// SetReviewRunner) calls orchestrator.Service.reinitWorkflowEngine, which
// REPLACES s.workflowEngine with a new *engine.Engine rather than mutating
// the existing one. A captured pointer would permanently miss any callback
// registered by a Set* call that runs after this adapter is built —
// concretely, run_code_review would silently no-op on every step-entry
// dispatch, since buildWorkflowCallbacks only registers
// ActionRunCodeReview once SetReviewRunner has been called. This mirrors
// switchWorkflowDispatcher's existing lazy read of svc.workflowEngine
// (workflow_callbacks.go).
func (a *engineStepEntryDispatcherAdapter) DispatchStepEntry(ctx context.Context, taskID, workflowID, stepID, entryID string) {
	eng := a.engineProvider.WorkflowEngine()
	if eng == nil {
		a.log.Warn("step entry dispatch skipped: workflow engine not initialised",
			zap.String("task_id", taskID),
			zap.String("workflow_id", workflowID),
			zap.String("step_id", stepID),
			zap.String("entry_id", entryID))
		return
	}
	results := eng.DispatchStepEntry(ctx, taskID, workflowID, stepID, entryID)
	for _, result := range results {
		fields := []zap.Field{
			zap.String("task_id", taskID),
			zap.String("workflow_id", workflowID),
			zap.String("step_id", stepID),
			zap.String("entry_id", entryID),
			zap.String("action_kind", string(result.Kind)),
		}
		if result.Err != nil {
			a.log.Warn("step entry action failed", append(fields, zap.Error(result.Err))...)
			continue
		}
		a.log.Debug("step entry action dispatched", fields...)
	}
}

// runsServiceEngineAdapter bridges runs/service.Service.QueueRun (which
// takes runs/service.QueueRunRequest) to engine.RunQueueAdapter (which
// takes engine.QueueRunRequest). The two structs have identical fields
// — they are intentionally duplicated so neither package imports the
// other — so this adapter is a field-by-field copy.
type runsServiceEngineAdapter struct {
	svc *runsservice.Service
}

// QueueRun enqueues a run, translating the engine's QueueRunRequest into the
// runs-service request shape.
func (a *runsServiceEngineAdapter) QueueRun(
	ctx context.Context, req workflowengine.QueueRunRequest,
) (workflowengine.QueueOutcome, error) {
	outcome, err := a.svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: req.AgentProfileID,
		TaskID:         req.TaskID,
		WorkflowStepID: req.WorkflowStepID,
		Reason:         req.Reason,
		IdempotencyKey: req.IdempotencyKey,
		Payload:        req.Payload,
	})
	return workflowengine.QueueOutcome(outcome), err
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

// newOfficeFailureInboxAdapter wraps an office service in the dashboard
// failure-inbox adapter so its rows can be served through the dashboard API.
func newOfficeFailureInboxAdapter(svc *officeservice.Service) *officeFailureInboxAdapter {
	return &officeFailureInboxAdapter{svc: svc}
}

// ListFailedRunInboxRows returns the failed-run inbox rows for a workspace and
// user, mapped into the dashboard failure-inbox row type.
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

// ListPausedAgentInboxRows returns the paused-agent inbox rows for a workspace
// and user, mapped into the dashboard failure-inbox row type.
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
				workflowStepID, planMode, false, attachments, env)
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
	agentSvc.SetProfileStore(settingsRepo)
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
var newInterimSettingsInterlockToken = httpmw.NewInterimSettingsInterlockToken

// resolvedHTTPPort returns the configured HTTP server port, falling back to
// the default backend port when unset.
func resolvedHTTPPort(cfg *config.Config) int {
	if cfg.Server.Port != 0 {
		return cfg.Server.Port
	}
	return ports.Backend
}

// buildHTTPServer creates the HTTP server with all middleware and routes
// registered against the gateway and service layer.
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
	systemSvc *systemsvc.Service,
	workspaceRestorer taskhandlers.WorkspaceQuarantineRestorer,
	temporaryArtifacts *tempartifacts.Registry,
	dbPool *db.Pool,
	agentRuntimeAvailability *agentctlclient.Availability,
) (*http.Server, error) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	// Trusted-proxy configuration for X-Forwarded-For via KANDEV_TRUSTED_PROXIES
	// (comma-separated IPs/CIDRs). gin trusts all proxies out of the box,
	// which would let a directly-reachable backend accept a spoofed client IP
	// and defeat the login rate limiter (keyed on ClientIP). The default is no
	// trusted proxies: ClientIP() falls back to the real peer RemoteAddr and
	// forwarded headers are ignored. Deployments behind a real proxy set the
	// env var to the proxy's IPs/CIDRs; a directly-reachable backend with the
	// var set can have X-Forwarded-For spoofed, which also defeats the
	// ClientIP-keyed login rate limiter.
	// X-Forwarded-Host feeds the port-scoped cookie-name resolver; honor it
	// only from the same trusted proxies that may rewrite it (an untrusted
	// value is stripped with a warning, so the resolver falls back to Host).
	trusted := configureTrustedProxies(router, log, cfg.Server.TrustedProxies)
	router.Use(authhttpmw.StripUntrustedForwardedHost(trusted, log))
	router.Use(httpmw.RequestLogger(log, kandevName))
	router.Use(httpmw.OtelTracing(kandevName))
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	// Generate the interim-settings interlock token before touching any
	// service deps, so a failure here aborts early (the test path passes nil
	// services to exercise exactly this).
	interimSettingsInterlockToken, err := newInterimSettingsInterlockToken()
	if err != nil {
		return nil, fmt.Errorf("generate interim settings interlock token: %w", err)
	}
	userSecretStore := secrets.NewUserVisibleStore(repos.Secrets)
	// The login PTY manager is shared by agent-login dialogs and Quick
	// Terminal tabs. The latter uses an owner-aware exit callback to keep its
	// durable descriptor accurate even while no browser is attached.
	loginMgr, quickTerminalSvc := buildLoginPTYServices(
		log,
		repos.QuickTerminal,
		services.Task,
		agentSettingsController,
		addCleanup,
	)

	// Opt-in authentication. Runs after CORS; in disabled mode it only
	// injects the synthetic single-user identity (behavior unchanged).
	router.Use(authhttpmw.Middleware(services.Auth))
	// Per-user workspace ownership on the third-party integration route
	// groups (jira/gitlab/github/...), which resolve a caller-supplied
	// workspace_id with no gate of their own. No-op when auth is disabled.
	router.Use(integrationWorkspaceScopeMiddleware(services.Auth, services.Task))

	secretsSvc := secrets.NewService(userSecretStore, log)
	// Workspace classification happens here, at the wiring boundary, where both
	// packages are importable: the task service's not-found sentinel becomes
	// the secrets sentinel (404), while raw lookup/storage errors pass through
	// unclassified (sanitized 500).
	secretsSvc.SetWorkspaceAuthorizer(func(ctx context.Context, workspaceID string) error {
		err := services.Task.AuthorizeWorkspaceAccess(ctx, workspaceID)
		if errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
			return secrets.ErrWorkspaceAccessDenied
		}
		return err
	})
	secretsSvc.SetWorkspaceExistenceChecker(func(ctx context.Context, workspaceID string) error {
		_, err := services.Task.GetWorkspace(ctx, workspaceID)
		if errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
			return secrets.ErrWorkspaceAccessDenied
		}
		return err
	})
	registerRoutes(routeParams{
		router:                        router,
		gateway:                       gateway,
		taskSvc:                       services.Task,
		taskRepo:                      repos.Task,
		officeRepo:                    repos.Office,
		analyticsRepo:                 repos.Analytics,
		orchestratorSvc:               orchestratorSvc,
		lifecycleMgr:                  lifecycleMgr,
		loginMgr:                      loginMgr,
		quickTerminalSvc:              quickTerminalSvc,
		hostUtilityMgr:                hostUtilityMgr,
		eventBus:                      eventBus,
		services:                      services,
		systemSvc:                     systemSvc,
		workspaceRestorer:             workspaceRestorer,
		temporaryArtifacts:            temporaryArtifacts,
		runtimeFlagsSvc:               services.RuntimeFlags,
		dbPool:                        dbPool,
		agentSettingsController:       agentSettingsController,
		agentSettingsRepo:             repos.AgentSettings,
		agentList:                     agentRegistry,
		agentRegistry:                 agentRegistry,
		userCtrl:                      usercontroller.NewController(services.User),
		notificationCtrl:              notificationCtrl,
		editorCtrl:                    editorcontroller.NewController(services.Editor),
		promptCtrl:                    promptcontroller.NewController(services.Prompts),
		utilityCtrl:                   utilitycontroller.NewController(services.Utility),
		msgCreator:                    msgCreator,
		secretsSvc:                    secretsSvc,
		secretStore:                   userSecretStore,
		mcpConfigSvc:                  mcpconfig.NewService(repos.AgentSettings),
		authSvc:                       services.Auth,
		agentRuntimeAvailability:      agentRuntimeAvailability,
		addCleanup:                    addCleanup,
		repoCloner:                    repoCloner,
		version:                       Version,
		webInternalURL:                cfg.Server.WebInternalURL,
		webTitlePrefix:                cfg.Server.WebTitlePrefix,
		devMode:                       cfg.Debug.DevMode || cfg.Debug.PprofEnabled,
		httpPort:                      resolvedHTTPPort(cfg),
		features:                      cfg.Features,
		planCoalesceWindow:            time.Duration(cfg.Planning.CoalesceWindowMs) * time.Millisecond,
		planCoalesceWindowConfigured:  true,
		homeDir:                       cfg.ResolvedHomeDir(),
		interimSettingsInterlockToken: interimSettingsInterlockToken,
		log:                           log,
	})

	// Addr is intentionally left unset: bind addresses are resolved from
	// cfg.Server.ResolvedBinds() and served via startHTTPServers, which may
	// create several listeners on one shared handler. server.Shutdown closes
	// all of them regardless of Addr.
	return &http.Server{
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeoutDuration(),
		WriteTimeout: cfg.Server.WriteTimeoutDuration(),
	}, nil
}

// awaitShutdown waits for an OS signal then performs graceful shutdown.
func awaitShutdown(
	server *http.Server,
	listeners *serverListeners,
	scheduling *schedulingRuntime,
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
	log.Debug("shutdown signal handler armed",
		zap.Int("pid", os.Getpid()),
		zap.Int("ppid", os.Getppid()))
	sig := <-quit

	// If we get a second signal, exit immediately.
	go func() {
		second := <-quit
		log.Warn("Received second shutdown signal, forcing exit", zap.String("signal", second.String()))
		_ = log.Close()
		os.Exit(1)
	}()

	log.Info("Received shutdown signal",
		zap.String("signal", sig.String()),
		zap.Int("pid", os.Getpid()))
	runGracefulShutdown(server, listeners, scheduling, orchestratorSvc, lifecycleMgr, runCleanups, log)
}

// migrateDefaultUtilityProfile upgrades the portable user's legacy default
// pair only when it identifies one eligible profile. Ambiguous and missing
// values remain untouched for a later retry or user repair.
func migrateDefaultUtilityProfile(
	ctx context.Context,
	userSvc *userservice.Service,
	profiles settingsstore.Repository,
	agentRegistry *registry.Registry,
	log *logger.Logger,
) {
	settings, err := userSvc.GetUserSettings(ctx)
	if err != nil || settings.DefaultUtilityAgentProfileID != "" || (settings.DefaultUtilityAgentID == "" && settings.DefaultUtilityModel == "") {
		return
	}
	resolver := profilebinding.New(profiles, func(agentID string) bool {
		_, ok := agentRegistry.GetInferenceAgent(agentID)
		return ok
	})
	profile, err := resolver.MatchLegacy(ctx, settings.DefaultUtilityAgentID, settings.DefaultUtilityModel)
	if err != nil || profile == nil {
		return
	}
	profileID := profile.ID
	if _, err := userSvc.UpdateUserSettings(ctx, &userservice.UpdateUserSettingsRequest{DefaultUtilityAgentProfileID: &profileID}); err != nil {
		log.Warn("failed to persist migrated default utility profile", zap.Error(err))
	}
}
