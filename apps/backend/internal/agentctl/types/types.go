// Package types provides shared types for agentctl packages.
// This breaks import cycles between adapter and acp packages.
package types

import (
	"context"
	"time"
)

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

// GitStatusUpdate represents a git status update
type GitStatusUpdate struct {
	Timestamp    time.Time           `json:"timestamp"`
	Modified     []string            `json:"modified"`
	Added        []string            `json:"added"`
	Deleted      []string            `json:"deleted"`
	Untracked    []string            `json:"untracked"`
	Renamed      []string            `json:"renamed"`
	Ahead        int                 `json:"ahead"`
	Behind       int                 `json:"behind"`
	Branch       string              `json:"branch"`
	RemoteBranch string              `json:"remote_branch,omitempty"`
	Files        map[string]FileInfo `json:"files,omitempty"`
}

// FileInfo represents information about a file
type FileInfo struct {
	Path      string `json:"path"`
	Status    string `json:"status"` // modified, added, deleted, untracked, renamed
	Additions int    `json:"additions,omitempty"`
	Deletions int    `json:"deletions,omitempty"`
	OldPath   string `json:"old_path,omitempty"` // For renamed files
	Diff      string `json:"diff,omitempty"`
}

// FileListUpdate represents a file listing update
type FileListUpdate struct {
	Timestamp time.Time   `json:"timestamp"`
	Files     []FileEntry `json:"files"`
}

// FileEntry represents a file in the workspace
type FileEntry struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size,omitempty"`
}

// FileTreeNode represents a node in the file tree
type FileTreeNode struct {
	Name     string          `json:"name"`
	Path     string          `json:"path"`
	IsDir    bool            `json:"is_dir"`
	Size     int64           `json:"size,omitempty"`
	Children []*FileTreeNode `json:"children,omitempty"`
}

// FileTreeRequest represents a request for file tree
type FileTreeRequest struct {
	Path  string `json:"path"`  // Path to get tree for (relative to workspace root)
	Depth int    `json:"depth"` // Depth to traverse (0 = unlimited, 1 = immediate children only)
}

// FileTreeResponse represents a response with file tree
type FileTreeResponse struct {
	RequestID string        `json:"request_id"`
	Root      *FileTreeNode `json:"root"`
	Error     string        `json:"error,omitempty"`
}

// FileContentRequest represents a request for file content
type FileContentRequest struct {
	Path string `json:"path"` // Path to file (relative to workspace root)
}

// FileContentResponse represents a response with file content
type FileContentResponse struct {
	RequestID string `json:"request_id"`
	Path      string `json:"path"`
	Content   string `json:"content"`
	Size      int64  `json:"size"`
	Error     string `json:"error,omitempty"`
}

// FileChangeNotification represents a filesystem change notification
type FileChangeNotification struct {
	Timestamp time.Time `json:"timestamp"`
	Path      string    `json:"path"`
	Operation string    `json:"operation"` // create, write, remove, rename, chmod, refresh
}

// GitStatusSubscriber is a channel that receives git status updates
type GitStatusSubscriber chan GitStatusUpdate

// FilesSubscriber is a channel that receives file listing updates
type FilesSubscriber chan FileListUpdate

// FileChangeSubscriber is a channel that receives file change notifications
type FileChangeSubscriber chan FileChangeNotification

// WorkspaceMessageType represents the type of workspace stream message
type WorkspaceMessageType string

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

// NewWorkspaceShellExit creates a shell exit message
func NewWorkspaceShellExit(code int) WorkspaceStreamMessage {
	return WorkspaceStreamMessage{
		Type:      WorkspaceMessageTypeShellExit,
		Timestamp: timeNowUnixMilli(),
		Code:      code,
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

// NewWorkspaceError creates an error message
func NewWorkspaceError(err string) WorkspaceStreamMessage {
	return WorkspaceStreamMessage{
		Type:      WorkspaceMessageTypeError,
		Timestamp: timeNowUnixMilli(),
		Error:     err,
	}
}

// WorkspaceStreamSubscriber is a channel that receives unified workspace messages
type WorkspaceStreamSubscriber chan WorkspaceStreamMessage

// timeNowUnixMilli returns current time in unix milliseconds
func timeNowUnixMilli() int64 {
	return time.Now().UnixMilli()
}

