package streams

import "time"

// File change operation constants.
const (
	// FileOpCreate indicates a file was created.
	FileOpCreate = "create"

	// FileOpWrite indicates a file was written/modified.
	FileOpWrite = "write"

	// FileOpRemove indicates a file was removed.
	FileOpRemove = "remove"

	// FileOpRename indicates a file was renamed.
	FileOpRename = "rename"

	// FileOpChmod indicates file permissions changed.
	FileOpChmod = "chmod"

	// FileOpRefresh indicates a refresh/rescan of the file.
	FileOpRefresh = "refresh"
)

// FileChangeNotification is the message type streamed via the file changes stream.
// Represents a filesystem change notification.
//
// Stream endpoint: ws://.../api/v1/workspace/file-changes/stream
type FileChangeNotification struct {
	// Timestamp is when the change was detected.
	Timestamp time.Time `json:"timestamp"`

	// Path is the file path relative to workspace root.
	Path string `json:"path"`

	// Operation indicates the type of change. Use FileOp* constants:
	// "create", "write", "remove", "rename", "chmod", "refresh".
	Operation string `json:"operation"`
}

// FileListUpdate represents a file listing update.
// Used for initial sync or full refresh of workspace files.
type FileListUpdate struct {
	// Timestamp is when the listing was captured.
	Timestamp time.Time `json:"timestamp"`

	// Files contains the list of files in the workspace.
	Files []FileEntry `json:"files"`
}

// FileEntry represents a file in the workspace.
type FileEntry struct {
	// Path is the file path relative to workspace root.
	Path string `json:"path"`

	// IsDir indicates if this is a directory.
	IsDir bool `json:"is_dir"`

	// Size is the file size in bytes (for files only).
	Size int64 `json:"size,omitempty"`
}

// FileTreeNode represents a node in the file tree.
type FileTreeNode struct {
	// Name is the file or directory name.
	Name string `json:"name"`

	// Path is the full path relative to workspace root.
	Path string `json:"path"`

	// IsDir indicates if this is a directory.
	IsDir bool `json:"is_dir"`

	// Size is the file size in bytes (for files only).
	Size int64 `json:"size,omitempty"`

	// Children contains child nodes (for directories only).
	Children []*FileTreeNode `json:"children,omitempty"`
}

// FileTreeRequest represents a request for file tree.
//
// HTTP endpoint: GET /api/v1/workspace/tree
type FileTreeRequest struct {
	// Path is the path to get tree for (relative to workspace root).
	Path string `json:"path"`

	// Depth is the depth to traverse (0 = unlimited, 1 = immediate children only).
	Depth int `json:"depth"`
}

// FileTreeResponse represents a response with file tree.
type FileTreeResponse struct {
	// RequestID identifies the request.
	RequestID string `json:"request_id"`

	// Root is the root node of the file tree.
	Root *FileTreeNode `json:"root"`

	// Error contains error message if the request failed.
	Error string `json:"error,omitempty"`
}

// FileContentRequest represents a request for file content.
//
// HTTP endpoint: GET /api/v1/workspace/file/content
type FileContentRequest struct {
	// Path is the file path (relative to workspace root).
	Path string `json:"path"`
}

// FileContentResponse represents a response with file content.
type FileContentResponse struct {
	// RequestID identifies the request.
	RequestID string `json:"request_id"`

	// Path is the file path.
	Path string `json:"path"`

	// Content is the file content.
	Content string `json:"content"`

	// Size is the file size in bytes.
	Size int64 `json:"size"`

	// Error contains error message if the request failed.
	Error string `json:"error,omitempty"`
}

// FileSearchRequest represents a request to search for files.
//
// HTTP endpoint: GET /api/v1/workspace/search
type FileSearchRequest struct {
	// Query is the search query (partial filename or path).
	Query string `json:"query"`

	// Limit is the maximum number of results to return.
	Limit int `json:"limit"`
}

// FileSearchResponse represents a response with matching files.
type FileSearchResponse struct {
	// Files is the list of matching file paths.
	Files []string `json:"files"`

	// Error contains error message if the request failed.
	Error string `json:"error,omitempty"`
}

