package main

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/mcpserver"
)

// provideMcpServer starts the embedded MCP server if enabled.
// Returns the SSE endpoint URL and a cleanup function.
func provideMcpServer(ctx context.Context, cfg *config.Config, log *logger.Logger) (string, func() error, error) {
	if !cfg.Agent.McpServerEnabled {
		return "", nil, nil
	}

	mcpCfg := mcpserver.Config{
		Port:      cfg.Agent.McpServerPort,
		KandevURL: fmt.Sprintf("http://localhost:%d", cfg.Server.Port),
	}

	srv, cleanup, err := mcpserver.Provide(ctx, mcpCfg, log)
	if err != nil {
		return "", nil, fmt.Errorf("failed to start MCP server: %w", err)
	}

	return srv.SSEEndpoint(), cleanup, nil
}

