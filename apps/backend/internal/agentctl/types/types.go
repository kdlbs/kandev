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

