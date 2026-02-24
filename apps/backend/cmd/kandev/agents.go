package main

import (
	"context"
	"os"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/credentials"
	"github.com/kandev/kandev/internal/agent/docker"
	agentexecutor "github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/registry"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/secrets"
)

func provideLifecycleManager(
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	eventBus bus.EventBus,
	dockerClient *docker.Client,
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
	)
	standaloneExec := lifecycle.NewStandaloneExecutor(
		controlClient,
		cfg.Agent.StandaloneHost,
		cfg.Agent.StandalonePort,
		log,
	)

	// Create InteractiveRunner for passthrough mode (no WorkspaceTracker, uses callbacks)
	interactiveRunner := process.NewInteractiveRunner(nil, log, 2*1024*1024) // 2MB buffer
	standaloneExec.SetInteractiveRunner(interactiveRunner)

	executorRegistry.Register(standaloneExec)
	log.Info("Standalone runtime registered with passthrough support",
		zap.String("host", cfg.Agent.StandaloneHost),
		zap.Int("port", cfg.Agent.StandalonePort))

	// Register Docker runtime if enabled and Docker client is available
	var containerMgr *lifecycle.ContainerManager
	if cfg.Docker.Enabled && dockerClient != nil {
		containerMgr = lifecycle.NewContainerManager(dockerClient, "", log)
		dockerExec := lifecycle.NewDockerExecutor(dockerClient, log)
		executorRegistry.Register(dockerExec)
		log.Info("Docker runtime registered")
	} else if cfg.Docker.Enabled && dockerClient == nil {
		log.Debug("Docker runtime enabled but Docker client not available")
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
		containerMgr,
		credsMgr,
		profileResolver,
		mcpService,
		lifecycle.ExecutorFallbackWarn,
		log,
	)

	// Register environment preparers
	preparerRegistry := lifecycle.NewPreparerRegistry(log)
	preparerRegistry.Register(agentexecutor.NameStandalone, lifecycle.NewLocalPreparer(log))
	preparerRegistry.Register(agentexecutor.NameDocker, lifecycle.NewDockerPreparer(log))
	preparerRegistry.Register(agentexecutor.NameSprites, lifecycle.NewSpritesPreparer(log))
	lifecycleMgr.SetPreparerRegistry(preparerRegistry)

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
