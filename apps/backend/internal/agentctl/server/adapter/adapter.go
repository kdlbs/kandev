// Package adapter provides protocol adapters for different agent communication protocols.
// This abstraction allows agentctl to work with agents using ACP, REST, MCP, or custom protocols.
package adapter

import (
	"context"

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
	EventTypeMessageChunk       = streams.EventTypeMessageChunk
	EventTypeReasoning          = streams.EventTypeReasoning
	EventTypeToolCall           = streams.EventTypeToolCall
	EventTypeToolUpdate         = streams.EventTypeToolUpdate
	EventTypePlan               = streams.EventTypePlan
	EventTypeComplete           = streams.EventTypeComplete
	EventTypeError              = streams.EventTypeError
	EventTypePermissionRequest  = streams.EventTypePermissionRequest
)

// AgentInfo contains information about the connected agent.
type AgentInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// AgentAdapter defines the interface for protocol adapters.
// Each adapter translates a specific protocol (ACP, REST, MCP, etc.) into the
// normalized AgentEvent format that agentctl exposes via its HTTP API.
type AgentAdapter interface {
	// Initialize establishes the connection with the agent and exchanges capabilities.
	// For subprocess-based agents (ACP), this sends the initialize request.
	// For HTTP-based agents (REST), this might do a health check.
	Initialize(ctx context.Context) error

	// GetAgentInfo returns information about the connected agent.
	// Returns nil if Initialize has not been called yet.
	GetAgentInfo() *AgentInfo

	// NewSession creates a new agent session and returns the session ID.
	NewSession(ctx context.Context) (string, error)

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

	// For HTTP-based adapters (REST)
	BaseURL    string            // Base URL of the agent's HTTP API
	AuthHeader string            // Optional auth header name
	AuthValue  string            // Optional auth header value
	Headers    map[string]string // Additional headers

	// Protocol-specific configuration
	Extra map[string]string
}

