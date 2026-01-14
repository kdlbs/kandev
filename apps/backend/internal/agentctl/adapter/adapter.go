// Package adapter provides protocol adapters for different agent communication protocols.
// This abstraction allows agentctl to work with agents using ACP, REST, MCP, or custom protocols.
package adapter

import (
	"context"
)

// SessionUpdate is a protocol-agnostic update from the agent.
// All protocol adapters normalize their updates to this format.
type SessionUpdate struct {
	// Type identifies the update type: "message_chunk", "tool_call", "tool_update", "plan", "complete", "error"
	Type string `json:"type"`

	// SessionID is the current session identifier
	SessionID string `json:"session_id,omitempty"`

	// Message fields (for "message_chunk" type)
	Text string `json:"text,omitempty"`

	// Tool call fields (for "tool_call" and "tool_update" types)
	ToolCallID string                 `json:"tool_call_id,omitempty"`
	ToolName   string                 `json:"tool_name,omitempty"`
	ToolTitle  string                 `json:"tool_title,omitempty"`
	ToolStatus string                 `json:"tool_status,omitempty"` // "started", "running", "completed", "error"
	ToolArgs   map[string]interface{} `json:"tool_args,omitempty"`
	ToolResult interface{}            `json:"tool_result,omitempty"`

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

// PermissionRequest represents a permission request from the agent
type PermissionRequest struct {
	SessionID  string             `json:"session_id"`
	ToolCallID string             `json:"tool_call_id"`
	Title      string             `json:"title"`
	Options    []PermissionOption `json:"options"`
}

// PermissionOption represents a permission option
type PermissionOption struct {
	OptionID string `json:"option_id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"` // "allow_once", "allow_always", "deny", etc.
}

// PermissionResponse is the response to a permission request
type PermissionResponse struct {
	OptionID  string `json:"option_id"`
	Cancelled bool   `json:"cancelled"`
}

// PermissionHandler is called when the agent requests permission for an action
type PermissionHandler func(ctx context.Context, req *PermissionRequest) (*PermissionResponse, error)

// AgentAdapter defines the interface for protocol adapters.
// Each adapter translates a specific protocol (ACP, REST, MCP, etc.) into the
// normalized SessionUpdate format that agentctl exposes via its HTTP API.
type AgentAdapter interface {
	// Initialize establishes the connection with the agent and exchanges capabilities.
	// For subprocess-based agents (ACP), this sends the initialize request.
	// For HTTP-based agents (REST), this might do a health check.
	Initialize(ctx context.Context) error

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

	// For subprocess-based adapters (ACP)
	Command []string // Command to run the agent

	// For HTTP-based adapters (REST)
	BaseURL    string            // Base URL of the agent's HTTP API
	AuthHeader string            // Optional auth header name
	AuthValue  string            // Optional auth header value
	Headers    map[string]string // Additional headers

	// Protocol-specific configuration
	Extra map[string]string
}

