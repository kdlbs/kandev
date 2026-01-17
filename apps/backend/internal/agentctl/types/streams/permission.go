package streams

import "time"

// Permission action types categorize the kind of action requiring approval.
const (
	// ActionTypeCommand indicates shell command execution.
	ActionTypeCommand = "command"

	// ActionTypeFileWrite indicates file modification or creation.
	ActionTypeFileWrite = "file_write"

	// ActionTypeFileRead indicates file read (for sensitive files).
	ActionTypeFileRead = "file_read"

	// ActionTypeNetwork indicates network access.
	ActionTypeNetwork = "network"

	// ActionTypeMCPTool indicates MCP tool invocation.
	ActionTypeMCPTool = "mcp_tool"

	// ActionTypeOther indicates other/unknown action type.
	ActionTypeOther = "other"
)

// PermissionNotification is the message type streamed via the permissions stream.
// Received when the agent requests permission for an action.
//
// Stream endpoint: ws://.../api/v1/acp/permissions/stream
type PermissionNotification struct {
	// PendingID uniquely identifies this pending permission request.
	PendingID string `json:"pending_id"`

	// SessionID is the session making the request.
	SessionID string `json:"session_id"`

	// ToolCallID is the tool call that triggered this permission request.
	ToolCallID string `json:"tool_call_id"`

	// Title is a human-readable description of the action.
	Title string `json:"title"`

	// Options contains the available permission choices.
	Options []PermissionOption `json:"options"`

	// ActionType categorizes the action requiring approval.
	// Use ActionType* constants: "command", "file_write", "file_read", "network", "mcp_tool", "other".
	ActionType string `json:"action_type,omitempty"`

	// ActionDetails contains structured details about the action.
	// For commands: {"command": ["ls", "-la"], "cwd": "/path"}
	// For files: {"path": "/file.go", "diff": "..."}
	// For MCP tools: {"server": "...", "tool": "...", "arguments": {...}}
	ActionDetails map[string]interface{} `json:"action_details,omitempty"`

	// CreatedAt is when the permission request was created.
	CreatedAt time.Time `json:"created_at"`
}

// PermissionOption represents a permission choice presented to the user.
type PermissionOption struct {
	// OptionID uniquely identifies this option.
	OptionID string `json:"option_id"`

	// Name is a human-readable name for the option.
	Name string `json:"name"`

	// Kind indicates the type of permission: "allow_once", "allow_always",
	// "reject_once", "reject_always".
	Kind string `json:"kind"`

	// Metadata contains protocol-specific option data.
	// For Codex: {"for_session": true} for session-wide approvals.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// PermissionRespondRequest is sent to respond to a permission request.
//
// HTTP endpoint: POST /api/v1/acp/permissions/respond
type PermissionRespondRequest struct {
	// PendingID is the ID of the permission request to respond to.
	PendingID string `json:"pending_id"`

	// OptionID is the selected option ID.
	OptionID string `json:"option_id,omitempty"`

	// Cancelled indicates if the request was cancelled.
	Cancelled bool `json:"cancelled,omitempty"`

	// ResponseMetadata contains protocol-specific response data.
	// For Codex: {"accept_settings": {"for_session": true}}.
	ResponseMetadata map[string]interface{} `json:"response_metadata,omitempty"`
}

// PermissionRespondResponse is the response from the permission respond endpoint.
type PermissionRespondResponse struct {
	// Success indicates if the response was accepted.
	Success bool `json:"success"`

	// Error contains error message if Success is false.
	Error string `json:"error,omitempty"`
}

