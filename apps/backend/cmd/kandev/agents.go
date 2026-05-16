package main

import (
	"context"
	"os"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/credentials"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/registry"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

func provideLifecycleManager(
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	eventBus bus.EventBus,
	agentSettingsRepo settingsstore.Repository,
	agentRegistry *registry.Registry,
	secretStore secrets.SecretStore,
) (*lifecycle.Manager, error) {
	log.Info("Initializing Agent Manager...")

	// Create runtime registry to manage multiple runtimes
	executorRegistry := lifecycle.NewExecutorRegistry(log)

	// Standalone runtime is always available (agentctl is a core service)
	controlClient := agentctl.NewControlClient(
		cfg.Agent.StandaloneHost,
		cfg.Agent.StandalonePort,
		log,
		agentctl.WithControlAuthToken(cfg.Agent.StandaloneAuthToken),
	)
	standaloneExec := lifecycle.NewStandaloneExecutor(
		controlClient,
		cfg.Agent.StandaloneHost,
		cfg.Agent.StandalonePort,
		log,
	)
	standaloneExec.SetAuthToken(cfg.Agent.StandaloneAuthToken)

	// Create InteractiveRunner for passthrough mode (no WorkspaceTracker, uses callbacks)
	interactiveRunner := process.NewInteractiveRunner(nil, log, 2*1024*1024) // 2MB buffer
	standaloneExec.SetInteractiveRunner(interactiveRunner)

	executorRegistry.Register(standaloneExec)
	log.Info("Standalone runtime registered with passthrough support",
		zap.String("host", cfg.Agent.StandaloneHost),
		zap.Int("port", cfg.Agent.StandalonePort))

	// Register Docker runtime if enabled (client is created lazily on first use)
	if cfg.Docker.Enabled {
		dockerExec := lifecycle.NewDockerExecutor(cfg.Docker, cfg.ResolvedHomeDir(), log)
		executorRegistry.Register(dockerExec)
		log.Info("Docker runtime registered (lazy initialization)")
	}

	// Register Remote Docker runtime (always available, instances are created lazily per host)
	remoteDockerExec := lifecycle.NewRemoteDockerExecutor(log)
	executorRegistry.Register(remoteDockerExec)
	log.Info("Remote Docker runtime registered")

	// Register Sprites runtime (remote sandboxes via Sprites.dev)
	agentctlResolver := lifecycle.NewAgentctlResolver(log)
	spritesExec := lifecycle.NewSpritesExecutor(secretStore, agentRegistry, agentctlResolver, 8765, log)
	executorRegistry.Register(spritesExec)
	log.Info("Sprites runtime registered")

	credsMgr := credentials.NewManager(log)
	if secretStore != nil {
		credsMgr.AddProvider(secrets.NewSecretStoreProvider(secretStore))
	}
	credsMgr.AddProvider(credentials.NewEnvProvider("KANDEV_"))
	credsMgr.AddProvider(credentials.NewAugmentSessionProvider())
	if credsFile := os.Getenv("KANDEV_CREDENTIALS_FILE"); credsFile != "" {
		credsMgr.AddProvider(credentials.NewFileProvider(credsFile))
	}

	profileResolver := lifecycle.NewStoreProfileResolver(agentSettingsRepo, agentRegistry)
	mcpService := mcpconfig.NewService(agentSettingsRepo)

	lifecycleMgr := lifecycle.NewManager(
		agentRegistry,
		eventBus,
		executorRegistry,
		credsMgr,
		profileResolver,
		mcpService,
		lifecycle.ExecutorFallbackWarn,
		cfg.ResolvedHomeDir(),
		log,
	)

	// Register environment preparers (keyed by ExecutorType — the
	// "local"/"worktree"/"local_docker"/"sprites" taxonomy, not Runtime).
	// The Worktree preparer is registered separately in
	// Manager.SetWorktreeManager once a worktree.Manager is wired.
	preparerRegistry := lifecycle.NewPreparerRegistry(log)
	localPreparer := lifecycle.NewLocalPreparer(log)
	preparerRegistry.Register(models.ExecutorTypeLocal, localPreparer)
	preparerRegistry.Register(models.ExecutorTypeMockRemote, localPreparer)
	preparerRegistry.Register(models.ExecutorTypeLocalDocker, lifecycle.NewDockerPreparer(log))
	preparerRegistry.Register(models.ExecutorTypeSprites, lifecycle.NewSpritesPreparer(log))
	lifecycleMgr.SetPreparerRegistry(preparerRegistry)
	lifecycleMgr.SetSecretStore(secretStore)
	// Wire the agent_profiles reader so the launch-prep skill deploy hook
	// (ADR 0005 Wave A) can resolve full profile rows including the office
	// enrichment fields. Without a wired SkillDeployer this is a no-op,
	// but the reader still lets future Wave-B/C consumers light up.
	lifecycleMgr.SetAgentProfileReader(agentSettingsRepo)

	// MCP handler is set later in main.go after MCP handlers are registered
	// via lifecycleMgr.SetMCPHandler(gateway.Dispatcher)

	if err := lifecycleMgr.Start(ctx); err != nil {
		return nil, err
	}

	log.Info("Agent Manager initialized",
		zap.Int("runtimes", len(executorRegistry.List())),
		zap.Int("agent_types", len(agentRegistry.List())))
	return lifecycleMgr, nil
}
