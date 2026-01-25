package main

import (
	"context"
	"time"

	agentctlclient "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/client/launcher"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// provideAgentctlLauncher starts the agentctl launcher for standalone runtime.
// agentctl is a core service that always runs - it's used by the Standalone runtime
// for agent execution on the host machine.
func provideAgentctlLauncher(ctx context.Context, cfg *config.Config, log *logger.Logger) (func() error, error) {
	_, cleanup, err := launcher.Provide(ctx, launcher.Config{
		Host: cfg.Agent.StandaloneHost,
		Port: cfg.Agent.StandalonePort,
	}, log)
	if err != nil {
		return nil, err
	}
	return cleanup, nil
}

// waitForAgentctlControlHealthy waits for the agentctl control server to be healthy.
// This is called during startup to ensure agentctl is ready before accepting requests.
func waitForAgentctlControlHealthy(ctx context.Context, cfg *config.Config, log *logger.Logger) {
	client := agentctlclient.NewControlClient(cfg.Agent.StandaloneHost, cfg.Agent.StandalonePort, log)
	healthCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		attemptCtx, attemptCancel := context.WithTimeout(healthCtx, 1*time.Second)
		err := client.Health(attemptCtx)
		attemptCancel()
		if err == nil {
			log.Info("agentctl control server is healthy")
			return
		}
		lastErr = err
		if healthCtx.Err() != nil {
			log.Warn("agentctl control server not ready; skipping resume wait", zap.Error(lastErr))
			return
		}
		<-ticker.C
	}
}
