package main

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/credentials"
	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/registry"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

func provideLifecycleManager(
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	eventBus bus.EventBus,
	dockerClient *docker.Client,
	agentSettingsRepo settingsstore.Repository,
) (*lifecycle.Manager, *registry.Registry, error) {
	log.Info("Initializing Agent Manager...")

	// Create runtime registry to manage multiple runtimes
	runtimeRegistry := lifecycle.NewRuntimeRegistry(log)

	// Standalone runtime is always available (agentctl is a core service)
	controlClient := agentctl.NewControlClient(
		cfg.Agent.StandaloneHost,
		cfg.Agent.StandalonePort,
		log,
	)
	standaloneRuntime := lifecycle.NewStandaloneRuntime(
		controlClient,
		cfg.Agent.StandaloneHost,
		cfg.Agent.StandalonePort,
		log,
	)

	// Create InteractiveRunner for passthrough mode (no WorkspaceTracker, uses callbacks)
	interactiveRunner := process.NewInteractiveRunner(nil, log, 2*1024*1024) // 2MB buffer
	standaloneRuntime.SetInteractiveRunner(interactiveRunner)

	runtimeRegistry.Register(standaloneRuntime)
	log.Info("Standalone runtime registered with passthrough support",
		zap.String("host", cfg.Agent.StandaloneHost),
		zap.Int("port", cfg.Agent.StandalonePort))

	// Register Docker runtime if enabled and Docker client is available
	var containerMgr *lifecycle.ContainerManager
	if cfg.Docker.Enabled && dockerClient != nil {
		containerMgr = lifecycle.NewContainerManager(dockerClient, "", log)
		dockerRuntime := lifecycle.NewDockerRuntime(dockerClient, log)
		runtimeRegistry.Register(dockerRuntime)
		log.Info("Docker runtime registered")
	} else if cfg.Docker.Enabled && dockerClient == nil {
		log.Warn("Docker runtime enabled but Docker client not available")
	}

	agentRegistry, _, err := registry.Provide(log)
	if err != nil {
		return nil, nil, err
	}

	credsMgr := credentials.NewManager(log)
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
		runtimeRegistry,
		containerMgr,
		credsMgr,
		profileResolver,
		mcpService,
		lifecycle.RuntimeFallbackWarn,
		log,
	)

	// Set backend WS URL for MCP tunneling
	// agentctl instances will connect back to this URL to forward MCP tool calls
	backendWsURL := fmt.Sprintf("ws://%s:%d/ws", cfg.Server.Host, cfg.Server.Port)
	lifecycleMgr.SetBackendWsURL(backendWsURL)
	log.Info("Backend WS URL configured for MCP tunneling", zap.String("url", backendWsURL))

	if err := lifecycleMgr.Start(ctx); err != nil {
		return nil, nil, err
	}

	log.Info("Agent Manager initialized",
		zap.Int("runtimes", len(runtimeRegistry.List())),
		zap.Int("agent_types", len(agentRegistry.List())))
	return lifecycleMgr, agentRegistry, nil
}
