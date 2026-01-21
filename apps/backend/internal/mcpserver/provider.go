// Package mcpserver provides the MCP server for Kandev task management.
package mcpserver

import (
	"context"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Port:      9090,
		KandevURL: "http://localhost:8080",
	}
}

// NewWithLogger creates a new MCP server with the given configuration and logger.
// This is useful for integration with dependency injection frameworks.
func NewWithLogger(cfg Config, log *logger.Logger) *Server {
	srv := New(cfg)
	srv.logger = log.WithFields(zap.String("component", "mcp-server"))
	return srv
}

// Provide starts the MCP server and returns a cleanup function to stop it.
// This is useful for integration with dependency injection frameworks.
func Provide(ctx context.Context, cfg Config, log *logger.Logger) (*Server, func() error, error) {
	srv := NewWithLogger(cfg, log)
	if err := srv.Start(ctx); err != nil {
		return nil, nil, err
	}

	var stopOnce sync.Once
	cleanup := func() error {
		var stopErr error
		stopOnce.Do(func() {
			stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			stopErr = srv.Stop(stopCtx)
		})
		return stopErr
	}

	return srv, cleanup, nil
}

