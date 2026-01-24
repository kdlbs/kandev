// Package adapter provides protocol adapters for different agent communication protocols.
// This abstraction allows agentctl to work with agents using ACP, REST, MCP, or custom protocols.
package adapter

import (
	"context"
	"io"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// Re-export permission types from the shared types package for convenience.
// This allows users of the adapter package to access these types without
// importing the types package directly.
type (
	PermissionRequest  = types.PermissionRequest
	PermissionResponse = types.PermissionResponse
	PermissionOption   = streams.PermissionOption
	PermissionHandler  = types.PermissionHandler
)

// Re-export stream types for convenience.
type (
	AgentEvent = streams.AgentEvent
	PlanEntry  = streams.PlanEntry
)

// Re-export agent event type constants from streams package.
const (
	EventTypeMessageChunk      = streams.EventTypeMessageChunk
	EventTypeReasoning         = streams.EventTypeReasoning
	EventTypeToolCall          = streams.EventTypeToolCall
	EventTypeToolUpdate        = streams.EventTypeToolUpdate
	EventTypePlan              = streams.EventTypePlan
	EventTypeComplete          = streams.EventTypeComplete
	EventTypeError             = streams.EventTypeError
	EventTypePermissionRequest = streams.EventTypePermissionRequest
	EventTypeContextWindow     = streams.EventTypeContextWindow
	EventTypeSessionStatus     = streams.EventTypeSessionStatus
)

// StderrProvider provides access to recent stderr output for error context.
// This is used by adapters to include stderr in error events when the agent
// reports an error without a detailed message (e.g., rate limit errors).
type StderrProvider interface {
	// GetRecentStderr returns the most recent stderr lines from the agent process.
	GetRecentStderr() []string
}

// AgentInfo contains information about the connected agent.
type AgentInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// AgentAdapter defines the interface for protocol adapters.
// Each adapter translates a specific protocol (ACP, REST, MCP, etc.) into the
// normalized AgentEvent format that agentctl exposes via its HTTP API.
//
// Lifecycle:
//  1. Create adapter with New*Adapter(cfg, logger)
//  2. Call PrepareEnvironment() before starting the agent process
//  3. Start the agent process and obtain stdin/stdout pipes
//  4. Call Connect(stdin, stdout) to wire up communication
//  5. Call Initialize() to establish protocol handshake
//  6. Use NewSession/LoadSession/Prompt/Cancel for agent interactions
//  7. Call Close() when done
type AgentAdapter interface {
	// PrepareEnvironment performs protocol-specific setup before the agent process starts.
	// For Codex, this writes MCP servers to ~/.codex/config.toml.
	// For ACP, this is a no-op (MCP servers are passed through the protocol).
	// Must be called before the agent subprocess is started.
	PrepareEnvironment() error

	// Connect wires up the stdin/stdout pipes from the running agent subprocess.
	// Must be called after the subprocess is started and before Initialize.
	Connect(stdin io.Writer, stdout io.Reader) error

	// Initialize establishes the connection with the agent and exchanges capabilities.
	// For subprocess-based agents (ACP), this sends the initialize request.
	// For HTTP-based agents (REST), this might do a health check.
	Initialize(ctx context.Context) error

	// GetAgentInfo returns information about the connected agent.
	// Returns nil if Initialize has not been called yet.
	GetAgentInfo() *AgentInfo

	// NewSession creates a new agent session and returns the session ID.
	NewSession(ctx context.Context, mcpServers []types.McpServer) (string, error)

	// LoadSession resumes an existing session by ID.
	LoadSession(ctx context.Context, sessionID string) error

	// Prompt sends a prompt to the agent.
	// The agent's responses are streamed via the Updates channel.
	Prompt(ctx context.Context, message string) error

	// Cancel cancels the current operation.
	Cancel(ctx context.Context) error

	// Updates returns a channel that receives agent events.
	// The channel is closed when the adapter is closed.
	Updates() <-chan AgentEvent

	// GetSessionID returns the current session ID.
	GetSessionID() string

	// GetOperationID returns the current operation/turn ID.
	// Returns empty string if no operation is in progress or not supported by the protocol.
	// For Codex this is the turn ID, for ACP this may be empty.
	GetOperationID() string

	// SetPermissionHandler sets the handler for permission requests.
	SetPermissionHandler(handler PermissionHandler)

	// Close releases resources held by the adapter.
	Close() error
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

// Config holds configuration for creating adapters
type Config struct {
	// WorkDir is the working directory for the agent
	WorkDir string

	// AutoApprove automatically approves permission requests
	AutoApprove bool

	// ApprovalPolicy controls when the agent requests approval.
	// Valid values: "untrusted" (always), "on-failure", "on-request", "never".
	// Defaults to "on-request" if empty.
	ApprovalPolicy string

	// McpServers is a list of MCP servers to configure for the agent
	McpServers []McpServerConfig

	// For HTTP-based adapters (REST)
	BaseURL    string            // Base URL of the agent's HTTP API
	AuthHeader string            // Optional auth header name
	AuthValue  string            // Optional auth header value
	Headers    map[string]string // Additional headers

	// Protocol-specific configuration
	Extra map[string]string
}
