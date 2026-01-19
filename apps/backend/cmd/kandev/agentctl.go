package main

import (
	"context"

	"github.com/kandev/kandev/internal/agentctl/client/launcher"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
)

func provideAgentctlLauncher(ctx context.Context, cfg *config.Config, log *logger.Logger) (func() error, error) {
	if cfg.Agent.Runtime != "standalone" {
		return nil, nil
	}
	_, cleanup, err := launcher.Provide(ctx, launcher.Config{
		Host: cfg.Agent.StandaloneHost,
		Port: cfg.Agent.StandalonePort,
	}, log)
	if err != nil {
		return nil, err
	}
	return cleanup, nil
}
