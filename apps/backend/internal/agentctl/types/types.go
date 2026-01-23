// Package types provides shared types for agentctl packages.
// This breaks import cycles between adapter and acp packages.
//
// For stream protocol message types, see the streams subpackage.
package types

import (
	"context"
	"time"

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

	// Process stream types
	ProcessKind         = streams.ProcessKind
	ProcessStatus       = streams.ProcessStatus
	ProcessOutput       = streams.ProcessOutput
	ProcessStatusUpdate = streams.ProcessStatusUpdate
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

	// Process statuses
	ProcessStatusStarting = streams.ProcessStatusStarting
	ProcessStatusRunning  = streams.ProcessStatusRunning
	ProcessStatusExited   = streams.ProcessStatusExited
	ProcessStatusFailed   = streams.ProcessStatusFailed
	ProcessStatusStopped  = streams.ProcessStatusStopped
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

// WorkspaceMessageType represents the type of workspace stream message
type WorkspaceMessageType string

// McpServer represents an MCP server configuration.
// Supports both stdio (command+args) and SSE (url) transports.
type McpServer struct {
	Name    string   `json:"name"`
	Command string   `json:"command,omitempty"` // For stdio transport
	Args    []string `json:"args,omitempty"`    // For stdio transport
	URL     string   `json:"url,omitempty"`     // For SSE transport
	Type    string   `json:"type,omitempty"`    // "stdio" or "sse"
}

const (
	// Workspace stream message types
	WorkspaceMessageTypeShellOutput   WorkspaceMessageType = "shell_output"
	WorkspaceMessageTypeShellInput    WorkspaceMessageType = "shell_input"
	WorkspaceMessageTypeShellExit     WorkspaceMessageType = "shell_exit"
	WorkspaceMessageTypePing          WorkspaceMessageType = "ping"
	WorkspaceMessageTypePong          WorkspaceMessageType = "pong"
	WorkspaceMessageTypeGitStatus     WorkspaceMessageType = "git_status"
	WorkspaceMessageTypeFileChange    WorkspaceMessageType = "file_change"
	WorkspaceMessageTypeFileList      WorkspaceMessageType = "file_list"
	WorkspaceMessageTypeError         WorkspaceMessageType = "error"
	WorkspaceMessageTypeConnected     WorkspaceMessageType = "connected"
	WorkspaceMessageTypeShellResize   WorkspaceMessageType = "shell_resize"
	WorkspaceMessageTypeProcessOutput WorkspaceMessageType = "process_output"
	WorkspaceMessageTypeProcessStatus WorkspaceMessageType = "process_status"
)

// WorkspaceStreamMessage is the unified message format for the workspace stream.
// It carries all workspace events (shell I/O, git status, file changes) with
// message type differentiation.
type WorkspaceStreamMessage struct {
	Type      WorkspaceMessageType `json:"type"`
	Timestamp int64                `json:"timestamp"` // Unix milliseconds

	// Shell fields (for shell_output, shell_input, shell_exit)
	Data string `json:"data,omitempty"` // Shell output or input data
	Code int    `json:"code,omitempty"` // Exit code for shell_exit

	// Shell resize fields (for shell_resize)
	Cols int `json:"cols,omitempty"`
	Rows int `json:"rows,omitempty"`

	// Git status fields (for git_status)
	GitStatus *GitStatusUpdate `json:"git_status,omitempty"`

	// File change fields (for file_change)
	FileChange *FileChangeNotification `json:"file_change,omitempty"`

	// File list fields (for file_list)
	FileList *FileListUpdate `json:"file_list,omitempty"`

	// Process fields (for process_output, process_status)
	ProcessOutput *ProcessOutput       `json:"process_output,omitempty"`
	ProcessStatus *ProcessStatusUpdate `json:"process_status,omitempty"`

	// Error fields (for error)
	Error string `json:"error,omitempty"`
}

// NewWorkspaceShellOutput creates a shell output message
func NewWorkspaceShellOutput(data string) WorkspaceStreamMessage {
	return WorkspaceStreamMessage{
		Type:      WorkspaceMessageTypeShellOutput,
		Timestamp: timeNowUnixMilli(),
		Data:      data,
	}
}

// NewWorkspaceShellInput creates a shell input message
func NewWorkspaceShellInput(data string) WorkspaceStreamMessage {
	return WorkspaceStreamMessage{
		Type:      WorkspaceMessageTypeShellInput,
		Timestamp: timeNowUnixMilli(),
		Data:      data,
	}
}

// NewWorkspaceGitStatus creates a git status message
func NewWorkspaceGitStatus(status *GitStatusUpdate) WorkspaceStreamMessage {
	return WorkspaceStreamMessage{
		Type:      WorkspaceMessageTypeGitStatus,
		Timestamp: timeNowUnixMilli(),
		GitStatus: status,
	}
}

// NewWorkspaceFileChange creates a file change message
func NewWorkspaceFileChange(notification *FileChangeNotification) WorkspaceStreamMessage {
	return WorkspaceStreamMessage{
		Type:       WorkspaceMessageTypeFileChange,
		Timestamp:  timeNowUnixMilli(),
		FileChange: notification,
	}
}

// NewWorkspaceFileList creates a file list message
func NewWorkspaceFileList(update *FileListUpdate) WorkspaceStreamMessage {
	return WorkspaceStreamMessage{
		Type:      WorkspaceMessageTypeFileList,
		Timestamp: timeNowUnixMilli(),
		FileList:  update,
	}
}

// NewWorkspaceProcessOutput creates a process output message
func NewWorkspaceProcessOutput(output *ProcessOutput) WorkspaceStreamMessage {
	return WorkspaceStreamMessage{
		Type:          WorkspaceMessageTypeProcessOutput,
		Timestamp:     timeNowUnixMilli(),
		ProcessOutput: output,
	}
}

// NewWorkspaceProcessStatus creates a process status message
func NewWorkspaceProcessStatus(status *ProcessStatusUpdate) WorkspaceStreamMessage {
	return WorkspaceStreamMessage{
		Type:          WorkspaceMessageTypeProcessStatus,
		Timestamp:     timeNowUnixMilli(),
		ProcessStatus: status,
	}
}

// NewWorkspacePong creates a pong message
func NewWorkspacePong() WorkspaceStreamMessage {
	return WorkspaceStreamMessage{
		Type:      WorkspaceMessageTypePong,
		Timestamp: timeNowUnixMilli(),
	}
}

// NewWorkspaceConnected creates a connected message
func NewWorkspaceConnected() WorkspaceStreamMessage {
	return WorkspaceStreamMessage{
		Type:      WorkspaceMessageTypeConnected,
		Timestamp: timeNowUnixMilli(),
	}
}

// NewWorkspacePing creates a ping message
func NewWorkspacePing() WorkspaceStreamMessage {
	return WorkspaceStreamMessage{
		Type:      WorkspaceMessageTypePing,
		Timestamp: timeNowUnixMilli(),
	}
}

// NewWorkspaceShellResize creates a shell resize message
func NewWorkspaceShellResize(cols, rows int) WorkspaceStreamMessage {
	return WorkspaceStreamMessage{
		Type:      WorkspaceMessageTypeShellResize,
		Timestamp: timeNowUnixMilli(),
		Cols:      cols,
		Rows:      rows,
	}
}

// WorkspaceStreamSubscriber is a channel that receives unified workspace messages
type WorkspaceStreamSubscriber chan WorkspaceStreamMessage

// timeNowUnixMilli returns current time in unix milliseconds
func timeNowUnixMilli() int64 {
	return time.Now().UnixMilli()
}
