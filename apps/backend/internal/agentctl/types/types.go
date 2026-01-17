// Package types provides shared types for agentctl packages.
// This breaks import cycles between adapter and acp packages.
//
// For stream protocol message types, see the streams subpackage.
package types

import (
	"context"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// Re-export stream types for convenience.
// New code may import from the streams package directly.
type (
	// Agent event stream types
	AgentEvent = streams.AgentEvent
	PlanEntry  = streams.PlanEntry

	// Permission stream types
	PermissionNotification    = streams.PermissionNotification
	PermissionOption          = streams.PermissionOption
	PermissionRespondRequest  = streams.PermissionRespondRequest
	PermissionRespondResponse = streams.PermissionRespondResponse

	// Git stream types
	GitStatusUpdate = streams.GitStatusUpdate
	FileInfo        = streams.FileInfo

	// File stream types
	FileChangeNotification = streams.FileChangeNotification
	FileListUpdate         = streams.FileListUpdate
	FileEntry              = streams.FileEntry
	FileTreeNode           = streams.FileTreeNode
	FileTreeRequest        = streams.FileTreeRequest
	FileTreeResponse       = streams.FileTreeResponse
	FileContentRequest     = streams.FileContentRequest
	FileContentResponse    = streams.FileContentResponse

	// Shell stream types
	ShellMessage        = streams.ShellMessage
	ShellStatusResponse = streams.ShellStatusResponse
	ShellBufferResponse = streams.ShellBufferResponse
)

// Re-export stream constants for convenience.
const (
	// Agent event types (preferred)
	EventTypeMessageChunk = streams.EventTypeMessageChunk
	EventTypeReasoning    = streams.EventTypeReasoning
	EventTypeToolCall     = streams.EventTypeToolCall
	EventTypeToolUpdate   = streams.EventTypeToolUpdate
	EventTypePlan         = streams.EventTypePlan
	EventTypeComplete     = streams.EventTypeComplete
	EventTypeError        = streams.EventTypeError

	// Permission action types
	ActionTypeCommand   = streams.ActionTypeCommand
	ActionTypeFileWrite = streams.ActionTypeFileWrite
	ActionTypeFileRead  = streams.ActionTypeFileRead
	ActionTypeNetwork   = streams.ActionTypeNetwork
	ActionTypeMCPTool   = streams.ActionTypeMCPTool
	ActionTypeOther     = streams.ActionTypeOther

	// File operation types
	FileOpCreate  = streams.FileOpCreate
	FileOpWrite   = streams.FileOpWrite
	FileOpRemove  = streams.FileOpRemove
	FileOpRename  = streams.FileOpRename
	FileOpChmod   = streams.FileOpChmod
	FileOpRefresh = streams.FileOpRefresh

	// Shell message types
	ShellMsgTypeInput  = streams.ShellMsgTypeInput
	ShellMsgTypeOutput = streams.ShellMsgTypeOutput
	ShellMsgTypePing   = streams.ShellMsgTypePing
	ShellMsgTypePong   = streams.ShellMsgTypePong
	ShellMsgTypeExit   = streams.ShellMsgTypeExit
)

// PermissionRequest represents a permission request from the agent.
// This is used internally by adapters and is not sent over streams directly.
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

// PermissionResponse is the user's response to a permission request.
// This is used internally by adapters.
type PermissionResponse struct {
	OptionID  string `json:"option_id,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`

	// ResponseMetadata contains protocol-specific response data.
	// For Codex: {"accept_settings": {"for_session": true}}
	ResponseMetadata map[string]interface{} `json:"response_metadata,omitempty"`
}

// PermissionHandler is called when the agent requests permission for an action.
type PermissionHandler func(ctx context.Context, req *PermissionRequest) (*PermissionResponse, error)

// Subscriber types for internal use.
type (
	// GitStatusSubscriber is a channel that receives git status updates.
	GitStatusSubscriber chan GitStatusUpdate

	// FilesSubscriber is a channel that receives file listing updates.
	FilesSubscriber chan FileListUpdate

	// FileChangeSubscriber is a channel that receives file change notifications.
	FileChangeSubscriber chan FileChangeNotification
)
