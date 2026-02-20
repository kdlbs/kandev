package shared

import "time"

// DefaultPermissionTimeout is the default timeout for permission requests (5 minutes).
const DefaultPermissionTimeout = 5 * time.Minute

// Config holds configuration for creating transport adapters.
// This is passed to transport adapters by the factory.
type Config struct {
	// WorkDir is the working directory for the agent
	WorkDir string

	// AutoApprove automatically approves permission requests
	AutoApprove bool

	// ApprovalPolicy controls when the agent requests approval.
	// Valid values: "untrusted" (always), "on-failure", "on-request", "never".
	// Defaults to "on-request" if empty.
	ApprovalPolicy string

	// PermissionPolicy controls the permission mode: "autonomous", "supervised", "plan".
	// Used to determine hook registration and --permission-mode flag.
	PermissionPolicy string

	// PermissionTimeout is the maximum time to wait for a permission response.
	// After timeout, the request is auto-denied with interrupt. Defaults to DefaultPermissionTimeout.
	PermissionTimeout time.Duration

	// McpServers is a list of MCP servers to configure for the agent
	McpServers []McpServerConfig

	// AgentID is the agent identifier from the registry (e.g., "auggie", "amp", "claude-code").
	// Used for logging and debug capture. Adapters should use this instead of hardcoded names.
	AgentID string

	// AgentName is the human-readable agent name (e.g., "Auggie", "AMP", "Claude Code").
	// Used for display purposes.
	AgentName string

	// For HTTP-based adapters (REST)
	BaseURL    string            // Base URL of the agent's HTTP API
	AuthHeader string            // Optional auth header name
	AuthValue  string            // Optional auth header value
	Headers    map[string]string // Additional headers

	// Protocol-specific configuration
	Extra map[string]string
}

// GetPermissionTimeout returns the configured permission timeout or the default.
func (c *Config) GetPermissionTimeout() time.Duration {
	if c.PermissionTimeout > 0 {
		return c.PermissionTimeout
	}
	return DefaultPermissionTimeout
}

// McpServerConfig holds configuration for an MCP server.
type McpServerConfig struct {
	// Name is the human-readable name of the MCP server
	Name string `json:"name"`
	// URL is the URL for HTTP/SSE transport
	URL string `json:"url,omitempty"`
	// Type is the transport type: "sse" or "http"
	Type string `json:"type,omitempty"`
	// Command is the command for stdio transport
	Command string `json:"command,omitempty"`
	// Args are the arguments for stdio transport
	Args []string `json:"args,omitempty"`
}
