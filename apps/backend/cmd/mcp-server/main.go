// Package main is the entry point for the standalone MCP server binary.
// mcp-server provides a Model Context Protocol server that exposes Kandev
// task management tools to MCP-compatible clients (Claude Desktop, Cursor, Codex, etc.)
//
// The server supports two transports:
//   - SSE (Server-Sent Events) at /sse for Claude Desktop, Cursor
//   - Streamable HTTP at /mcp for Codex
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/mcpserver"
	"go.uber.org/zap"
)

// Command-line flags
var (
	portFlag      = flag.Int("port", 9090, "MCP server port")
	kandevURLFlag = flag.String("kandev-url", "http://localhost:8080", "Kandev API URL")
	logLevelFlag  = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	logFormatFlag = flag.String("log-format", "console", "Log format (console, json)")
)

func main() {
	flag.Parse()

	// Build configuration from flags and environment
	cfg := mcpserver.Config{
		Port:      getEnvIntOrFlag("MCP_PORT", *portFlag),
		KandevURL: getEnvOrFlag("KANDEV_API_URL", *kandevURLFlag),
	}

	// Initialize logger
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      getEnvOrFlag("MCP_LOG_LEVEL", *logLevelFlag),
		Format:     getEnvOrFlag("MCP_LOG_FORMAT", *logFormatFlag),
		OutputPath: "stdout",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	log.Info("starting mcp-server",
		zap.Int("port", cfg.Port),
		zap.String("kandev_url", cfg.KandevURL))

	run(cfg, log)
}

// run starts the MCP server and waits for shutdown.
func run(cfg mcpserver.Config, log *logger.Logger) {
	ctx := context.Background()
	srv, cleanup, err := mcpserver.Provide(ctx, cfg, log)
	if err != nil {
		log.Error("failed to start MCP server", zap.Error(err))
		os.Exit(1)
	}

	log.Info("MCP server started",
		zap.String("sse_endpoint", srv.SSEEndpoint()),
		zap.String("streamable_http_endpoint", srv.StreamableHTTPEndpoint()))

	fmt.Printf("Kandev MCP server running on :%d\n", cfg.Port)
	fmt.Printf("Kandev API URL: %s\n", cfg.KandevURL)
	fmt.Printf("SSE endpoint: %s (for Claude Desktop, Cursor)\n", srv.SSEEndpoint())
	fmt.Printf("Streamable HTTP endpoint: %s (for Codex)\n", srv.StreamableHTTPEndpoint())

	// Wait for shutdown signal
	waitForShutdown(log, func(ctx context.Context) {
		if err := cleanup(); err != nil {
			log.Error("error during shutdown", zap.Error(err))
		}
	})
}

// waitForShutdown waits for shutdown signal and calls cleanup
func waitForShutdown(log *logger.Logger, cleanup func(ctx context.Context)) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down mcp-server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cleanup(ctx)

	log.Info("mcp-server stopped")
}

// getEnvOrFlag returns the environment variable value if set, otherwise the flag value.
func getEnvOrFlag(envKey, flagValue string) string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return flagValue
}

// getEnvIntOrFlag returns the environment variable value as int if set, otherwise the flag value.
func getEnvIntOrFlag(envKey string, flagValue int) int {
	if v := os.Getenv(envKey); v != "" {
		var i int
		if _, err := fmt.Sscanf(v, "%d", &i); err == nil {
			return i
		}
	}
	return flagValue
}

