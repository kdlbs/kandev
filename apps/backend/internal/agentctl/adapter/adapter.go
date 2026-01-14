// Package adapter provides protocol adapters for different agent communication protocols.
// This abstraction allows agentctl to work with agents using ACP, REST, MCP, or custom protocols.
package adapter

import (
	"context"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// Re-export permission types from the shared types package for convenience.
// This allows users of the adapter package to access these types without
// importing the types package directly.
type (
	PermissionRequest  = types.PermissionRequest
	PermissionResponse = types.PermissionResponse
	PermissionOption   = types.PermissionOption
	PermissionHandler  = types.PermissionHandler
)

// SessionUpdate type constants
const (
	UpdateTypeMessageChunk = "message_chunk" // Agent text streaming
	UpdateTypeReasoning    = "reasoning"     // Chain-of-thought/thinking
	UpdateTypeToolCall     = "tool_call"     // Tool invocation started
	UpdateTypeToolUpdate   = "tool_update"   // Tool status update
	UpdateTypePlan         = "plan"          // Agent plan updates
	UpdateTypeComplete     = "complete"      // Turn/session complete
	UpdateTypeError        = "error"         // Error occurred
)

// SessionUpdate is a protocol-agnostic update from the agent.
// All protocol adapters normalize their updates to this format.
type SessionUpdate struct {
	// Type identifies the update type. Use UpdateType* constants.
	Type string `json:"type"`

	// SessionID is the current session identifier
	SessionID string `json:"session_id,omitempty"`

	// OperationID identifies the current in-flight operation (turn, prompt, etc.)
	// Used to target specific operations for cancellation or status updates.
	// For Codex this is the turn ID, for other protocols it may be empty.
	OperationID string `json:"operation_id,omitempty"`

	// Message fields (for "message_chunk" type)
	Text string `json:"text,omitempty"`

	// Reasoning fields (for "reasoning" type)
	// Used for chain-of-thought, thinking traces, etc.
	ReasoningText    string `json:"reasoning_text,omitempty"`    // Full reasoning content
	ReasoningSummary string `json:"reasoning_summary,omitempty"` // Summarized version (if available)

	// Tool call fields (for "tool_call" and "tool_update" types)
	ToolCallID string                 `json:"tool_call_id,omitempty"`
	ToolName   string                 `json:"tool_name,omitempty"`
	ToolTitle  string                 `json:"tool_title,omitempty"`
	ToolStatus string                 `json:"tool_status,omitempty"` // "started", "running", "completed", "error"
	ToolArgs   map[string]interface{} `json:"tool_args,omitempty"`
	ToolResult interface{}            `json:"tool_result,omitempty"`

	// Diff contains unified diff content for file changes.
	// Populated when tools modify files, providing the aggregated diff.
	Diff string `json:"diff,omitempty"`

	// Plan fields (for "plan" type)
	PlanEntries []PlanEntry `json:"plan_entries,omitempty"`

	// Error fields (for "error" type)
	Error string `json:"error,omitempty"`

	// Raw data for protocol-specific extensions
	Data map[string]interface{} `json:"data,omitempty"`
}

// PlanEntry represents an entry in the agent's execution plan
type PlanEntry struct {
	Description string `json:"description,omitempty"` // Content/description of the task
	Status      string `json:"status,omitempty"`      // "pending", "in_progress", "completed", "failed"
	Priority    string `json:"priority,omitempty"`    // Relative importance
}

// AgentInfo contains information about the connected agent.
type AgentInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// AgentAdapter defines the interface for protocol adapters.
// Each adapter translates a specific protocol (ACP, REST, MCP, etc.) into the
// normalized SessionUpdate format that agentctl exposes via its HTTP API.
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

	// Updates returns a channel that receives session updates.
	// The channel is closed when the adapter is closed.
	Updates() <-chan SessionUpdate

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

	// For HTTP-based adapters (REST)
	BaseURL    string            // Base URL of the agent's HTTP API
	AuthHeader string            // Optional auth header name
	AuthValue  string            // Optional auth header value
	Headers    map[string]string // Additional headers

	// Protocol-specific configuration
	Extra map[string]string
}

