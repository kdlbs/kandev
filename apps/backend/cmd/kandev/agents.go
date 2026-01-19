package main

import (
	"context"
	"os"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/credentials"
	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
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
	agentSettingsRepo *settingsstore.SQLiteRepository,
) (*lifecycle.Manager, *registry.Registry, error) {
	agentManagerEnabled := (cfg.Agent.Runtime == "docker" && dockerClient != nil) ||
		cfg.Agent.Runtime == "standalone"
	if !agentManagerEnabled {
		log.Info("Agent Manager disabled (no Docker and not in standalone mode)")
		return nil, nil, nil
	}

	log.Info("Initializing Agent Manager...", zap.String("runtime", cfg.Agent.Runtime))

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

	profileResolver := lifecycle.NewStoreProfileResolver(agentSettingsRepo)

	var agentRuntime lifecycle.Runtime
	var containerMgr *lifecycle.ContainerManager
	switch cfg.Agent.Runtime {
	case "standalone":
		controlClient := agentctl.NewControlClient(
			cfg.Agent.StandaloneHost,
			cfg.Agent.StandalonePort,
			log,
		)
		agentRuntime = lifecycle.NewStandaloneRuntime(
			controlClient,
			cfg.Agent.StandaloneHost,
			cfg.Agent.StandalonePort,
			log,
		)
		log.Info("Using standalone runtime",
			zap.String("host", cfg.Agent.StandaloneHost),
			zap.Int("port", cfg.Agent.StandalonePort))
	default:
		if dockerClient != nil {
			containerMgr = lifecycle.NewContainerManager(dockerClient, "", log)
			agentRuntime = lifecycle.NewDockerRuntime(dockerClient, log)
			log.Info("Using Docker runtime")
		}
	}

	lifecycleMgr := lifecycle.NewManager(agentRegistry, eventBus, agentRuntime, containerMgr, credsMgr, profileResolver, log)
	if err := lifecycleMgr.Start(ctx); err != nil {
		return nil, nil, err
	}

	log.Info("Agent Manager initialized",
		zap.String("runtime", cfg.Agent.Runtime),
		zap.Int("agent_types", len(agentRegistry.List())))
	return lifecycleMgr, agentRegistry, nil
}
