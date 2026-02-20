// Package main is the entry point for the agentctl binary.
// agentctl is a sidecar process that manages agent subprocess communication
// via HTTP API, bridging the agent's ACP protocol with the kandev backend.
//
// agentctl is runtime-agnostic - it behaves identically whether running
// inside a Docker container or directly on the host machine. The caller
// (kandev backend) handles any Docker vs standalone differences.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/server/api"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	mcpserver "github.com/kandev/kandev/internal/agentctl/server/mcp"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
	"go.uber.org/zap"
)

// Command-line flags
var (
	protocolFlag     = flag.String("protocol", "", "Protocol for agent communication (acp, codex, mcp)")
	agentCommandFlag = flag.String("agent-command", "", "Command to run the agent")
	workDirFlag      = flag.String("workdir", "", "Working directory for the agent")
	portFlag         = flag.Int("port", 0, "HTTP server port")
)

func main() {
	flag.Parse()

	// Load configuration
	cfg := config.Load()

	// Override with CLI flags if provided
	if *protocolFlag != "" {
		cfg.Defaults.Protocol = agent.Protocol(*protocolFlag)
	}
	if *agentCommandFlag != "" {
		cfg.Defaults.AgentCommand = *agentCommandFlag
	}
	if *workDirFlag != "" {
		cfg.Defaults.WorkDir = *workDirFlag
	}
	if *portFlag != 0 {
		cfg.Port = *portFlag
	}

	// Initialize logger
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      cfg.LogLevel,
		Format:     cfg.LogFormat,
		OutputPath: "stdout",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = log.Sync()
	}()

	log.Info("starting agentctl",
		zap.Int("port", cfg.Port),
		zap.String("log_level", cfg.LogLevel))

	run(cfg, log)
}

// run starts the agentctl server.
// All instances are managed through the instance management API.
func run(cfg *config.Config, log *logger.Logger) {
	// Create instance manager
	instMgr := instance.NewManager(cfg, log)

	// Set the server factory to create API servers for each instance
	instMgr.SetServerFactory(func(instCfg *config.InstanceConfig, procMgr *process.Manager, instLog *logger.Logger) http.Handler {
		// Create MCP backend client for bidirectional communication through agent stream
		// MCP requests from agents are sent through the agent stream WebSocket to the backend
		mcpBackendClient := mcpserver.NewChannelBackendClient()

		// Create MCP server using the channel-based backend client
		mcpSrv := mcpserver.New(mcpBackendClient, instCfg.SessionID, instCfg.Port, instLog, cfg.McpLogFile)
		instLog.Info("MCP server enabled (channel-based)",
			zap.String("session_id", instCfg.SessionID))

		return api.NewServer(instCfg, procMgr, mcpSrv, mcpBackendClient, instLog).Router()
	})

	// Create control server
	controlServer := api.NewControlServer(cfg, instMgr, log)

	// Start HTTP server
	addr := fmt.Sprintf(":%d", cfg.Port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: controlServer.Router(),
	}

	go func() {
		log.Info("HTTP server starting", zap.String("address", addr))
		// Try to listen first so we get a clear error before serving
		ln, listenErr := net.Listen("tcp", addr)
		if listenErr != nil {
			log.Error("HTTP server failed to bind",
				zap.String("address", addr),
				zap.Error(listenErr))
			os.Exit(1)
		}
		log.Info("HTTP server bound successfully", zap.String("address", ln.Addr().String()))
		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Error("HTTP server error", zap.Error(err))
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	waitForShutdown(log, func(ctx context.Context) {
		// Flush pending traces before stopping instances
		if err := shared.ShutdownTracing(ctx); err != nil {
			log.Error("error shutting down tracing", zap.Error(err))
		}
		// Shutdown all instances
		if err := instMgr.Shutdown(ctx); err != nil {
			log.Error("error shutting down instance manager", zap.Error(err))
		}
		if err := httpServer.Shutdown(ctx); err != nil {
			log.Error("error shutting down HTTP server", zap.Error(err))
		}
	})
}

// waitForShutdown waits for shutdown signal and calls cleanup
func waitForShutdown(log *logger.Logger, cleanup func(ctx context.Context)) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down agentctl...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cleanup(ctx)

	log.Info("agentctl stopped")
}
