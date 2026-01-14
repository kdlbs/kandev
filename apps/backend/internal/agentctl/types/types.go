// Package types provides shared types for agentctl packages.
// This breaks import cycles between adapter and acp packages.
package types

import "context"

// Permission action types - categorize the kind of action requiring approval
const (
	ActionTypeCommand   = "command"    // Shell command execution
	ActionTypeFileWrite = "file_write" // File modification/creation
	ActionTypeFileRead  = "file_read"  // File read (for sensitive files)
	ActionTypeNetwork   = "network"    // Network access
	ActionTypeMCPTool   = "mcp_tool"   // MCP tool invocation
	ActionTypeOther     = "other"      // Other/unknown action type
)

// PermissionRequest represents a permission request from the agent
type PermissionRequest struct {
	SessionID  string             `json:"session_id"`
	ToolCallID string             `json:"tool_call_id"`
	Title      string             `json:"title"`
	Options    []PermissionOption `json:"options"`

	// ActionType categorizes the action requiring approval.
	// Use ActionType* constants: "command", "file_write", "network", "mcp_tool", etc.
	ActionType string `json:"action_type,omitempty"`

	// ActionDetails contains structured details about the action.
	// For commands: {"command": ["ls", "-la"], "cwd": "/path"}
	// For files: {"path": "/file.go", "diff": "..."}
	// For MCP tools: {"server": "...", "tool": "...", "arguments": {...}}
	ActionDetails map[string]interface{} `json:"action_details,omitempty"`
}

// PermissionOption represents a permission choice
type PermissionOption struct {
	OptionID string `json:"option_id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"` // allow_once, allow_always, reject_once, reject_always

	// Metadata contains protocol-specific option data.
	// For Codex: {"for_session": true} for session-wide approvals
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// PermissionResponse is the user's response to a permission request
type PermissionResponse struct {
	OptionID  string `json:"option_id,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`

	// ResponseMetadata contains protocol-specific response data.
	// For Codex: {"accept_settings": {"for_session": true}}
	ResponseMetadata map[string]interface{} `json:"response_metadata,omitempty"`
}

// PermissionHandler is called when the agent requests permission for an action
type PermissionHandler func(ctx context.Context, req *PermissionRequest) (*PermissionResponse, error)

