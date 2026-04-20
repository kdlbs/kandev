// Package config provides unified configuration for agentctl.
//
// agentctl is runtime-agnostic - it behaves identically whether running
// inside a Docker container or directly on the host. The caller (kandev backend)
// handles any Docker vs standalone differences.
//
// Configuration hierarchy:
//   - Config: Global server settings (ports, logging, instance limits)
//   - Config.Defaults: Default values for new instances
//   - InstanceConfig: Per-instance settings (derived from Defaults + overrides)
package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/kandev/kandev/pkg/agent"
)

// Config is the agentctl configuration.
// agentctl always exposes the same instance management API regardless of
// deployment context (Docker container or host machine).
type Config struct {
	// Port is the control/API server port
	Port int

	// Ports configures the port range for instance allocation
	Ports PortConfig

	// Defaults provides default values for new instances
	Defaults InstanceDefaults

	// Shell configuration
	ShellEnabled bool // Enable auto-shell feature (default: true)

	// Logging configuration
	LogLevel   string
	LogFormat  string
	McpLogFile string // Optional file path for MCP debug logs

	// VS Code server configuration
	VscodeCommand string // Command to run code-server (default: "code-server")
}

// PortConfig defines port allocation for instances
type PortConfig struct {
	// Base is the starting port for instance allocation (multi-instance mode)
	Base int
	// Max is the maximum port for instance allocation
	Max int
}

// InstanceDefaults provides default values for new instances.
// These can be overridden when creating an instance.
type InstanceDefaults struct {
	// Protocol for agent communication (acp, codex, mcp)
	Protocol agent.Protocol

	// AgentCommand is the command to run the agent (e.g., "auggie --acp")
	AgentCommand string

	// WorkDir is the default working directory
	WorkDir string

	// AutoStart starts the agent automatically when the instance is created
	AutoStart bool

	// AutoApprovePermissions auto-approves permission requests (for testing/CI)
	AutoApprovePermissions bool

	// HealthCheckInterval is the interval in seconds for health checks
	HealthCheckInterval int

	// ProcessBufferMaxBytes is the max bytes per process output buffer (default 2MB)
	ProcessBufferMaxBytes int64
}

// McpServerConfig holds configuration for an MCP server.
type McpServerConfig struct {
	// Name is the human-readable name of the MCP server
	Name string `json:"name"`
	// URL is the URL for HTTP/SSE transport
	URL string `json:"url,omitempty"`
	// Type is the transport type: "stdio", "sse", "http", or "streamable_http"
	Type string `json:"type,omitempty"`
	// Command is the command for stdio transport
	Command string `json:"command,omitempty"`
	// Args are the arguments for stdio transport
	Args []string `json:"args,omitempty"`
	// Env holds environment variables for stdio transport
	Env map[string]string `json:"env,omitempty"`
	// Headers holds HTTP headers for SSE/HTTP transport
	Headers map[string]string `json:"headers,omitempty"`
}

// InstanceConfig holds configuration for a single agent instance.
// This is passed to the process manager and API server.
type InstanceConfig struct {
	// Port is the HTTP server port for this instance
	Port int

	// Protocol for agent communication
	Protocol agent.Protocol

	// AgentCommand is the command to run the agent
	AgentCommand string

	// AgentArgs is the parsed command (derived from AgentCommand)
	AgentArgs []string

	// WorkDir is the working directory for the agent process
	WorkDir string

	// AgentEnv is the environment variables to pass to the agent
	AgentEnv []string

	// AutoStart starts the agent automatically
	AutoStart bool

	// AutoApprovePermissions auto-approves permission requests
	AutoApprovePermissions bool

	// ApprovalPolicy controls when the agent requests approval.
	// Valid values: "untrusted" (always), "on-failure", "on-request", "never".
	// Defaults to "on-request" if empty.
	ApprovalPolicy string

	// ShellEnabled enables auto-shell feature
	ShellEnabled bool

	// LogLevel for this instance
	LogLevel string

	// LogFormat for this instance
	LogFormat string

	// AgentType identifies the agent (e.g., "auggie", "codex", "claude")
	// Used for agent-specific adapter selection
	AgentType string

	// McpServers is a list of MCP servers to configure for the agent
	McpServers []McpServerConfig

	// ProcessBufferMaxBytes caps per-process output buffer size
	ProcessBufferMaxBytes int64

	// SessionID is the session ID for this agent instance (used in MCP tool calls)
	SessionID string

	// TaskID is the task ID for this agent instance (used in MCP plan tool calls)
	TaskID string

	// ContinueCommand is the command template for follow-up prompts in one-shot agents.
	// When set, the adapter spawns a new process per prompt using this command for
	// continuation (thread ID appended at runtime). Only used by Amp.
	ContinueCommand string

	// VscodeCommand is the command to run the VS Code server (e.g., "code-server")
	VscodeCommand string

	// DisableAskQuestion disables the ask_user_question MCP tool.
	// Used for TUI/passthrough agents that don't need clarification tools.
	DisableAskQuestion bool

	// AssumeMcpSse overrides MCP capability filtering to assume the agent supports SSE.
	AssumeMcpSse bool

	// McpMode controls which MCP tools are registered for this instance.
	// "task" (default) registers kanban/plan tools; "config" registers config tools.
	McpMode string
}

// Load loads the configuration from environment variables.
func Load() *Config {
	return &Config{
		Port: getEnvInt("AGENTCTL_PORT", 9999),
		Ports: PortConfig{
			Base: getEnvInt("AGENTCTL_INSTANCE_PORT_BASE", 10001),
			Max:  getEnvInt("AGENTCTL_INSTANCE_PORT_MAX", 10100),
		},
		Defaults: InstanceDefaults{
			Protocol:               agent.Protocol(getEnv("AGENTCTL_PROTOCOL", string(agent.ProtocolACP))),
			AgentCommand:           getEnv("AGENTCTL_AGENT_COMMAND", "auggie --acp"),
			WorkDir:                getEnv("AGENTCTL_WORKDIR", "/workspace"),
			AutoStart:              getEnvBool("AGENTCTL_AUTO_START", false),
			AutoApprovePermissions: getEnvBool("AGENTCTL_AUTO_APPROVE_PERMISSIONS", false),
			HealthCheckInterval:    getEnvInt("AGENTCTL_HEALTH_CHECK_INTERVAL", 5),
			ProcessBufferMaxBytes:  getEnvInt64("AGENTCTL_PROCESS_BUFFER_MAX_BYTES", 2*1024*1024),
		},
		ShellEnabled:  getEnvBool("AGENTCTL_SHELL_ENABLED", true),
		LogLevel:      getEnvWithFallback("AGENTCTL_LOG_LEVEL", "KANDEV_LOG_LEVEL", "info"),
		LogFormat:     getEnv("AGENTCTL_LOG_FORMAT", "json"),
		McpLogFile:    getEnv("KANDEV_MCP_LOG_FILE", ""),
		VscodeCommand: getEnv("AGENTCTL_VSCODE_COMMAND", "code-server"),
	}
}

// NewInstanceConfig creates an InstanceConfig from defaults with optional overrides.
// If port is 0, it should be allocated by the caller.
func (c *Config) NewInstanceConfig(port int, overrides *InstanceOverrides) *InstanceConfig {
	cfg := &InstanceConfig{
		Port:                   port,
		Protocol:               c.Defaults.Protocol,
		AgentCommand:           c.Defaults.AgentCommand,
		WorkDir:                c.Defaults.WorkDir,
		AutoStart:              c.Defaults.AutoStart,
		AutoApprovePermissions: c.Defaults.AutoApprovePermissions,
		ShellEnabled:           c.ShellEnabled,
		LogLevel:               c.LogLevel,
		LogFormat:              c.LogFormat,
		ProcessBufferMaxBytes:  c.Defaults.ProcessBufferMaxBytes,
		VscodeCommand:          c.VscodeCommand,
		McpMode:                "task",
	}

	applyOverrides(cfg, overrides)

	// Inject local kandev MCP server for MCP tunneling through the agent stream
	// This ensures the kandev MCP server is available for protocols that read MCP config
	// at startup time (e.g., Codex via -c flags). The MCP server uses the agent stream
	// WebSocket connection (bidirectional) to forward tool calls to the backend.
	if port > 0 {
		cfg.McpServers = injectKandevMcpServer(cfg.McpServers, port)
	}

	// Parse agent command into args
	cfg.AgentArgs = ParseCommand(cfg.AgentCommand)

	// Collect environment if not explicitly set
	if cfg.AgentEnv == nil {
		cfg.AgentEnv = CollectAgentEnv(nil)
	}

	return cfg
}

// applyOverrides applies non-zero fields from overrides to cfg.
func applyOverrides(cfg *InstanceConfig, overrides *InstanceOverrides) {
	if overrides == nil {
		return
	}
	if overrides.Protocol != "" {
		cfg.Protocol = overrides.Protocol
	}
	if overrides.AgentCommand != "" {
		cfg.AgentCommand = overrides.AgentCommand
	}
	if overrides.WorkDir != "" {
		cfg.WorkDir = overrides.WorkDir
	}
	if overrides.AutoStart != nil {
		cfg.AutoStart = *overrides.AutoStart
	}
	if overrides.Env != nil {
		cfg.AgentEnv = overrides.Env
	}
	if overrides.ApprovalPolicy != "" {
		cfg.ApprovalPolicy = overrides.ApprovalPolicy
	}
	if overrides.AgentType != "" {
		cfg.AgentType = overrides.AgentType
	}
	if len(overrides.McpServers) > 0 {
		cfg.McpServers = overrides.McpServers
	}
	if overrides.SessionID != "" {
		cfg.SessionID = overrides.SessionID
	}
	if overrides.TaskID != "" {
		cfg.TaskID = overrides.TaskID
	}
	if overrides.DisableAskQuestion {
		cfg.DisableAskQuestion = true
	}
	if overrides.AssumeMcpSse {
		cfg.AssumeMcpSse = true
	}
	if overrides.McpMode != "" {
		cfg.McpMode = overrides.McpMode
	}
}

// InstanceOverrides allows overriding default values when creating an instance
type InstanceOverrides struct {
	Protocol           agent.Protocol
	AgentCommand       string
	WorkDir            string
	AutoStart          *bool
	Env                []string
	ApprovalPolicy     string
	AgentType          string
	McpServers         []McpServerConfig
	SessionID          string
	TaskID             string
	DisableAskQuestion bool
	AssumeMcpSse       bool
	McpMode            string
}

// ParseCommand splits a command string into arguments
func ParseCommand(cmd string) []string {
	return strings.Fields(cmd)
}

// CollectAgentEnv collects environment variables to pass to the agent.
// It filters out AGENTCTL_* variables and optionally merges additional env vars.
func CollectAgentEnv(additional map[string]string) []string {
	envMap := make(map[string]string)

	// Start with current environment, excluding AGENTCTL_* and npm_config_* vars.
	// npm_config_* vars from the parent pnpm process cause "Unknown env config"
	// warnings when the agent is launched via npx.
	for _, e := range os.Environ() {
		if idx := strings.Index(e, "="); idx > 0 {
			key := e[:idx]
			if !strings.HasPrefix(key, "AGENTCTL_") && !isNpmEnvVar(key) {
				envMap[key] = e[idx+1:]
			}
		}
	}

	// Merge additional env vars
	for k, v := range additional {
		envMap[k] = v
	}

	// Convert back to slice
	result := make([]string, 0, len(envMap))
	for k, v := range envMap {
		result = append(result, k+"="+v)
	}
	return result
}

// isNpmEnvVar returns true if the key is an npm-related environment variable
// that should be filtered out to prevent warnings in npx commands.
func isNpmEnvVar(key string) bool {
	return strings.HasPrefix(key, "npm_config_") ||
		strings.HasPrefix(key, "npm_package_") ||
		strings.HasPrefix(key, "npm_lifecycle_") ||
		strings.HasPrefix(key, "npm_execpath") ||
		strings.HasPrefix(key, "npm_node_execpath")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvWithFallback checks the primary key, then the fallback key, then returns defaultValue.
func getEnvWithFallback(primary, fallback, defaultValue string) string {
	if value := os.Getenv(primary); value != "" {
		return value
	}
	if value := os.Getenv(fallback); value != "" {
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

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
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

// kandevMcpServerName is the name used for the local kandev MCP server.
const kandevMcpServerName = "kandev"

// injectKandevMcpServer prepends the local kandev MCP server to the list of MCP servers.
// This replaces any existing kandev server to avoid duplicates.
// The kandev MCP server provides tools like ask_user_question to the agent.
// Both SSE and HTTP variants are injected - agent capability filtering will select the appropriate one.
func injectKandevMcpServer(servers []McpServerConfig, port int) []McpServerConfig {
	portStr := strconv.Itoa(port)
	kandevMcpSse := McpServerConfig{
		Name: kandevMcpServerName,
		Type: "sse",
		URL:  "http://localhost:" + portStr + "/sse",
	}
	kandevMcpHttp := McpServerConfig{
		Name: kandevMcpServerName,
		Type: "http",
		URL:  "http://localhost:" + portStr + "/mcp",
	}

	// Filter out any existing kandev server and prepend the local ones
	result := make([]McpServerConfig, 0, len(servers)+2)
	result = append(result, kandevMcpSse, kandevMcpHttp)
	for _, srv := range servers {
		if srv.Name != kandevMcpServerName {
			result = append(result, srv)
		}
	}
	return result
}
