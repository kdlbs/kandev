package streams

import "time"

const (
	PermissionStatusPending   = "pending"
	PermissionStatusResolving = "resolving"

	PermissionErrorNotFound         = "permission_not_found"
	PermissionErrorStale            = "permission_stale"
	PermissionErrorAlreadyResolved  = "permission_already_resolved"
	PermissionErrorInProgress       = "permission_resolution_in_progress"
	PermissionErrorOptionNotOffered = "permission_option_not_offered"
	PermissionErrorDeliveryFailed   = "permission_delivery_failed"
)

// PermissionActionType categorizes the kind of action requiring approval.
// Values are stable wire-format strings shared across agentctl, the backend
// orchestrator, and the frontend.
type PermissionActionType string

func (t PermissionActionType) String() string { return string(t) }

// Permission action types categorize the kind of action requiring approval.
const (
	// ActionTypeCommand indicates shell command execution.
	ActionTypeCommand PermissionActionType = "command"

	// ActionTypeFileWrite indicates file modification or creation.
	ActionTypeFileWrite PermissionActionType = "file_write"

	// ActionTypeFileRead indicates file read (for sensitive files).
	ActionTypeFileRead PermissionActionType = "file_read"

	// ActionTypeNetwork indicates network access.
	ActionTypeNetwork PermissionActionType = "network"

	// ActionTypeMCPTool indicates MCP tool invocation.
	ActionTypeMCPTool PermissionActionType = "mcp_tool"

	// ActionTypeOther indicates other/unknown action type.
	ActionTypeOther PermissionActionType = "other"
)

// PermissionOptionKind identifies the semantics of a permission option
// presented to the user. Values are stable wire-format strings shared with
// the frontend.
type PermissionOptionKind string

func (k PermissionOptionKind) String() string { return string(k) }

// Permission option kinds describe the semantics of each available choice.
const (
	// PermissionOptionKindAllowOnce approves the current request only.
	PermissionOptionKindAllowOnce PermissionOptionKind = "allow_once"

	// PermissionOptionKindAllowAlways approves and remembers the decision.
	PermissionOptionKindAllowAlways PermissionOptionKind = "allow_always"

	// PermissionOptionKindRejectOnce rejects the current request only.
	PermissionOptionKindRejectOnce PermissionOptionKind = "reject_once"

	// PermissionOptionKindRejectAlways rejects and remembers the decision.
	PermissionOptionKindRejectAlways PermissionOptionKind = "reject_always"
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
	ActionType PermissionActionType `json:"action_type,omitempty"`

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
	Kind PermissionOptionKind `json:"kind"`

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

// PermissionChoice is the immutable public identity of one provider-offered
// option. Provider metadata is intentionally excluded.
type PermissionChoice struct {
	OptionID string               `json:"option_id"`
	Name     string               `json:"name"`
	Kind     PermissionOptionKind `json:"kind"`
}

// PermissionAction is an allowlisted presentation of the requested action.
type PermissionAction struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Command     string `json:"command,omitempty"`
	CWD         string `json:"cwd,omitempty"`
	Path        string `json:"path,omitempty"`
	Destination string `json:"destination,omitempty"`
	Server      string `json:"server,omitempty"`
	Tool        string `json:"tool,omitempty"`
	Redacted    bool   `json:"redacted"`
}

// PendingAgentPermission is the safe immutable snapshot of one live request.
type PendingAgentPermission struct {
	TaskID     string             `json:"task_id"`
	SessionID  string             `json:"session_id"`
	RequestID  string             `json:"request_id"`
	PendingID  string             `json:"pending_id"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
	Title      string             `json:"title"`
	Action     PermissionAction   `json:"action"`
	Options    []PermissionChoice `json:"options"`
	CreatedAt  time.Time          `json:"created_at"`
	Status     string             `json:"status"`
}

type PermissionListResponse struct {
	Permissions []PendingAgentPermission `json:"permissions"`
	Total       int                      `json:"total"`
}

type PermissionResolveRequest struct {
	RequestID string `json:"request_id"`
	PendingID string `json:"pending_id"`
	OptionID  string `json:"option_id"`
}

type PermissionResolveResponse struct {
	RequestID  string               `json:"request_id"`
	PendingID  string               `json:"pending_id"`
	OptionID   string               `json:"option_id"`
	OptionKind PermissionOptionKind `json:"option_kind"`
	Status     string               `json:"status"`
}

type PermissionCancelRequest struct {
	RequestID string `json:"request_id"`
	PendingID string `json:"pending_id"`
}

type PermissionCancelResponse struct {
	RequestID string `json:"request_id"`
	PendingID string `json:"pending_id"`
	Status    string `json:"status"`
}
