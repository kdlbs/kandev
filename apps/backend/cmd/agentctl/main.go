// Package main is the entry point for agentctl - a control process that runs
// inside Docker containers to manage AI agent processes.
//
// agentctl provides:
// - HTTP REST API for control operations (start, stop, pause, resume)
// - WebSocket streaming of agent output
// - Process management for the underlying agent (e.g., auggie --acp)
// - ACP message relay between HTTP API and agent subprocess
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
	// Load configuration
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
		zap.String("version", "0.1.0"),
		zap.Int("port", cfg.Port),
		zap.String("agent_command", cfg.AgentCommand),
	)

	// Create process manager
	procMgr := process.NewManager(cfg, log)

	// Create API server
	apiServer := api.NewServer(cfg, procMgr, log)

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      apiServer.Router(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Info("HTTP server listening", zap.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("HTTP server error", zap.Error(err))
		}
	}()

	// Auto-start agent if configured
	if cfg.AutoStart {
		log.Info("auto-starting agent process")
		ctx := context.Background()
		if err := procMgr.Start(ctx); err != nil {
			log.Error("failed to auto-start agent", zap.Error(err))
		}
	}

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down agentctl")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop the agent process
	if err := procMgr.Stop(ctx); err != nil {
		log.Error("failed to stop agent process", zap.Error(err))
	}

	// Shutdown HTTP server
	if err := server.Shutdown(ctx); err != nil {
		log.Error("HTTP server shutdown error", zap.Error(err))
	}

	log.Info("agentctl stopped")
}

