// Package main is the entry point for the agentctl binary
// agentctl is a sidecar process that manages agent subprocess communication
// via HTTP API, bridging the agent's ACP protocol with the kandev backend.
//
// It supports two modes:
// - Docker mode: Single instance, used when running inside a container
// - Standalone mode: Multiple instances, used when running directly on host machine
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
	"github.com/kandev/kandev/internal/agentctl/instance"
	"github.com/kandev/kandev/internal/agentctl/process"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

func main() {
	// Load multi-instance configuration
	cfg := config.LoadMulti()

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

	// Determine mode
	mode := detectMode(cfg.Mode)

	log.Info("starting agentctl",
		zap.String("mode", mode),
		zap.Int("control_port", cfg.ControlPort),
		zap.Int("max_instances", cfg.MaxInstances))

	if mode == "docker" {
		runDockerMode(cfg, log)
	} else {
		runStandaloneMode(cfg, log)
	}
}

// detectMode determines the operating mode based on config and environment
func detectMode(configMode string) string {
	if configMode == "docker" || configMode == "standalone" {
		return configMode
	}
	// Auto-detect: check for Docker container indicators
	if isRunningInDocker() {
		return "docker"
	}
	return "standalone"
}

// isRunningInDocker checks if we're running inside a Docker container
func isRunningInDocker() bool {
	// Check for /.dockerenv file
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	// Check for /proc/1/cgroup containing "docker"
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		if len(data) > 0 {
			content := string(data)
			if len(content) > 0 && (contains(content, "docker") || contains(content, "kubepods")) {
				return true
			}
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// runDockerMode runs agentctl in Docker mode (single instance, same as legacy behavior)
func runDockerMode(cfg *config.MultiConfig, log *logger.Logger) {
	// Load full config from environment (including AutoApprovePermissions)
	singleCfg := config.Load()
	// Override with multi-config values if they differ from defaults
	singleCfg.Port = cfg.ControlPort
	if cfg.DefaultAgentCommand != "" {
		singleCfg.AgentCommand = cfg.DefaultAgentCommand
		singleCfg.AgentArgs = parseCommand(cfg.DefaultAgentCommand)
	}
	if cfg.DefaultWorkDir != "" {
		singleCfg.WorkDir = cfg.DefaultWorkDir
	}
	// Parse agent args
	singleCfg.AgentArgs = parseCommand(singleCfg.AgentCommand)
	singleCfg.AgentEnv = collectAgentEnv()

	log.Info("running in Docker mode (single instance)",
		zap.Int("port", singleCfg.Port),
		zap.String("agent_command", singleCfg.AgentCommand),
		zap.String("workdir", singleCfg.WorkDir),
		zap.Bool("auto_approve_permissions", singleCfg.AutoApprovePermissions))

	// Create process manager
	procMgr := process.NewManager(singleCfg, log)

	// Create HTTP API server
	server := api.NewServer(singleCfg, procMgr, log)

	// Start HTTP server
	addr := fmt.Sprintf(":%d", singleCfg.Port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: server.Router(),
	}

	go func() {
		log.Info("HTTP server starting", zap.String("address", addr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("HTTP server error", zap.Error(err))
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	waitForShutdown(log, func(ctx context.Context) {
		if err := procMgr.Stop(ctx); err != nil {
			log.Error("error stopping agent process", zap.Error(err))
		}
		if err := httpServer.Shutdown(ctx); err != nil {
			log.Error("error shutting down HTTP server", zap.Error(err))
		}
	})
}

// runStandaloneMode runs agentctl in standalone mode (multiple instances)
func runStandaloneMode(cfg *config.MultiConfig, log *logger.Logger) {
	log.Info("running in standalone mode (multi-instance)",
		zap.Int("control_port", cfg.ControlPort),
		zap.Int("instance_port_base", cfg.InstancePortBase),
		zap.Int("instance_port_max", cfg.InstancePortMax),
		zap.Int("max_instances", cfg.MaxInstances))

	// Create instance manager
	instMgr := instance.NewManager(cfg, log)

	// Set the server factory to create API servers for each instance
	instMgr.SetServerFactory(func(instCfg *config.Config, procMgr *process.Manager, instLog *logger.Logger) http.Handler {
		return api.NewServer(instCfg, procMgr, instLog).Router()
	})

	// Create control server
	controlServer := api.NewControlServer(cfg, instMgr, log)

	// Start control server on control port
	addr := fmt.Sprintf(":%d", cfg.ControlPort)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: controlServer.Router(),
	}

	go func() {
		log.Info("control server starting", zap.String("address", addr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("control server error", zap.Error(err))
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	waitForShutdown(log, func(ctx context.Context) {
		if err := instMgr.Shutdown(ctx); err != nil {
			log.Error("error shutting down instance manager", zap.Error(err))
		}
		if err := httpServer.Shutdown(ctx); err != nil {
			log.Error("error shutting down control server", zap.Error(err))
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

// parseCommand splits a command string into arguments
func parseCommand(cmd string) []string {
	var args []string
	for _, part := range splitFields(cmd) {
		if part != "" {
			args = append(args, part)
		}
	}
	return args
}

func splitFields(s string) []string {
	var result []string
	var current string
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(r)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

// collectAgentEnv collects environment variables to pass to the agent
func collectAgentEnv() []string {
	var env []string
	for _, e := range os.Environ() {
		// Exclude AGENTCTL_* variables
		if len(e) < 9 || e[:9] != "AGENTCTL_" {
			env = append(env, e)
		}
	}
	return env
}

