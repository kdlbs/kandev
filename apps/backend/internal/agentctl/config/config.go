// Package config provides configuration for agentctl
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds the agentctl configuration
type Config struct {
	// HTTP server port
	Port int

	// Agent command to execute (e.g., "auggie --acp")
	AgentCommand string

	// Agent arguments (parsed from AgentCommand)
	AgentArgs []string

	// Working directory for the agent process
	WorkDir string

	// Environment variables to pass to the agent
	AgentEnv []string

	// Auto-start the agent on agentctl startup
	AutoStart bool

	// Auto-approve permission requests (for testing/CI)
	AutoApprovePermissions bool

	// Logging configuration
	LogLevel  string
	LogFormat string

	// Buffer size for agent output (in lines)
	OutputBufferSize int

	// Health check interval
	HealthCheckInterval int
}

// Load loads configuration from environment variables
func Load() *Config {
	workDir := getEnv("AGENTCTL_WORKDIR", "/workspace")
	defaultCmd := "auggie --acp --workspace-root " + workDir

	cfg := &Config{
		Port:                   getEnvInt("AGENTCTL_PORT", 9999),
		AgentCommand:           getEnv("AGENTCTL_AGENT_COMMAND", defaultCmd),
		WorkDir:                workDir,
		AutoStart:              getEnvBool("AGENTCTL_AUTO_START", false),
		AutoApprovePermissions: getEnvBool("AGENTCTL_AUTO_APPROVE_PERMISSIONS", false),
		LogLevel:               getEnv("AGENTCTL_LOG_LEVEL", "info"),
		LogFormat:              getEnv("AGENTCTL_LOG_FORMAT", "json"),
		OutputBufferSize:       getEnvInt("AGENTCTL_OUTPUT_BUFFER_SIZE", 1000),
		HealthCheckInterval:    getEnvInt("AGENTCTL_HEALTH_CHECK_INTERVAL", 5),
	}

	// Parse agent command into args
	cfg.AgentArgs = parseCommand(cfg.AgentCommand)

	// Collect agent environment variables (pass through most env vars)
	cfg.AgentEnv = collectAgentEnv()

	return cfg
}

// parseCommand splits a command string into arguments
func parseCommand(cmd string) []string {
	// Simple split by spaces (doesn't handle quotes, but good enough for now)
	return strings.Fields(cmd)
}

// collectAgentEnv collects environment variables to pass to the agent
func collectAgentEnv() []string {
	// Pass through all environment variables except AGENTCTL_* ones
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "AGENTCTL_") {
			env = append(env, e)
		}
	}
	return env
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return strings.ToLower(value) == "true" || value == "1"
	}
	return defaultValue
}

