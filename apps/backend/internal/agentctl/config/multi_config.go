// Package config provides configuration for agentctl.
// This file contains multi-instance configuration for running multiple agent instances.
package config

import "github.com/kandev/kandev/pkg/agent"

// MultiConfig holds configuration for multi-instance mode.
// It extends the base configuration with settings for managing multiple concurrent agent instances.
type MultiConfig struct {
	// ControlPort is the main control API port
	ControlPort int

	// InstancePortBase is the starting port for instances
	InstancePortBase int

	// InstancePortMax is the maximum port for instances
	InstancePortMax int

	// MaxInstances is the maximum number of concurrent instances
	MaxInstances int

	// DefaultProtocol is the default protocol for agents
	DefaultProtocol agent.Protocol

	// DefaultAgentCommand is the default command for agents
	DefaultAgentCommand string

	// DefaultWorkDir is the default working directory for agents
	DefaultWorkDir string

	// AutoApprovePermissions auto-approves permission requests (for testing/CI)
	AutoApprovePermissions bool

	// ShellEnabled enables auto-shell feature for each instance (default: true)
	ShellEnabled bool

	// LogLevel is the logging level (debug, info, warn, error)
	LogLevel string

	// LogFormat is the logging format (json, text)
	LogFormat string

	// Mode determines how instances are run: "standalone", "docker", or "auto"
	Mode string
}

// LoadMulti loads multi-instance configuration from environment variables.
// Environment variables use the AGENTCTL_ prefix.
func LoadMulti() *MultiConfig {
	return &MultiConfig{
		ControlPort:            getEnvInt("AGENTCTL_CONTROL_PORT", 9999),
		InstancePortBase:       getEnvInt("AGENTCTL_INSTANCE_PORT_BASE", 10001),
		InstancePortMax:        getEnvInt("AGENTCTL_INSTANCE_PORT_MAX", 10100),
		MaxInstances:           getEnvInt("AGENTCTL_MAX_INSTANCES", 10),
		DefaultProtocol:        agent.Protocol(getEnv("AGENTCTL_PROTOCOL", string(agent.ProtocolACP))),
		DefaultAgentCommand:    getEnv("AGENTCTL_AGENT_COMMAND", "auggie --acp"),
		DefaultWorkDir:         getEnv("AGENTCTL_WORKDIR", "/workspace"),
		AutoApprovePermissions: getEnvBool("AGENTCTL_AUTO_APPROVE_PERMISSIONS", false),
		ShellEnabled:           getEnvBool("AGENTCTL_SHELL_ENABLED", true),
		LogLevel:               getEnv("AGENTCTL_LOG_LEVEL", "info"),
		LogFormat:              getEnv("AGENTCTL_LOG_FORMAT", "json"),
		Mode:                   getEnv("AGENTCTL_MODE", "auto"),
	}
}

