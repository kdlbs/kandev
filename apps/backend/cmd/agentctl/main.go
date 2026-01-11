// Package main is the entry point for the agentctl binary
// agentctl is a sidecar process that manages agent subprocess communication
// via HTTP API, bridging the agent's ACP protocol with the kandev backend.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kandev/kandev/internal/agentctl/api"
	"github.com/kandev/kandev/internal/agentctl/config"
	"github.com/kandev/kandev/internal/agentctl/process"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

func main() {
	// Load configuration from environment
	cfg := config.Load()

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
	defer log.Sync()

	log.Info("starting agentctl",
		zap.Int("port", cfg.Port),
		zap.String("agent_command", cfg.AgentCommand),
		zap.String("workdir", cfg.WorkDir),
		zap.Bool("auto_start", cfg.AutoStart))

	// Create process manager
	procMgr := process.NewManager(cfg, log)

	// Create HTTP API server
	server := api.NewServer(cfg, procMgr, log)

	// Start HTTP server
	addr := fmt.Sprintf(":%d", cfg.Port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: server.Router(),
	}

	// Start server in goroutine
	go func() {
		log.Info("HTTP server starting", zap.String("address", addr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("HTTP server error", zap.Error(err))
			os.Exit(1)
		}
	}()

	// Auto-start agent if configured
	if cfg.AutoStart {
		log.Info("auto-starting agent process")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := procMgr.Start(ctx); err != nil {
			log.Error("failed to auto-start agent", zap.Error(err))
		}
		cancel()
	}

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down agentctl...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop agent process
	if err := procMgr.Stop(ctx); err != nil {
		log.Error("error stopping agent process", zap.Error(err))
	}

	// Shutdown HTTP server
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error("error shutting down HTTP server", zap.Error(err))
	}

	log.Info("agentctl stopped")
}

