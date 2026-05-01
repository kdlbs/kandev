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

	// RepositoryName identifies which repository emitted this file change in
	// multi-repo task workspaces. Empty for single-repo. Carried through to
	// the frontend so per-repo views can scope refresh signals correctly.
	RepositoryName string `json:"repository_name,omitempty"`

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

	// RepositoryName identifies which repository this list belongs to in
	// multi-repo workspaces. Empty for single-repo. See GitStatusUpdate.
	RepositoryName string `json:"repository_name,omitempty"`

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

	// IsSymlink indicates this entry is a symbolic link.
	IsSymlink bool `json:"is_symlink,omitempty"`

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

	// Content is the file content (base64-encoded if IsBinary is true).
	Content string `json:"content"`

	// Size is the file size in bytes.
	Size int64 `json:"size"`

	// IsBinary indicates the file contains non-UTF-8 content.
	// When true, Content is base64-encoded.
	IsBinary bool `json:"is_binary,omitempty"`

	// ResolvedPath is the symlink target path (relative to workspace root).
	// Only set when the requested file is a symlink.
	ResolvedPath string `json:"resolved_path,omitempty"`

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

// FileUpdateRequest represents a request to update file content using a diff.
//
// HTTP endpoint: POST /api/v1/workspace/file/content
type FileUpdateRequest struct {
	// Path is the file path (relative to the per-repo subpath when Repo is
	// set, otherwise relative to the workspace root).
	Path string `json:"path"`

	// Repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo
	// workspaces. When set, Path is interpreted relative to <workDir>/<Repo>.
	Repo string `json:"repo,omitempty"`

	// Diff is the unified diff to apply to the file.
	Diff string `json:"diff"`

	// OriginalHash is the SHA256 hash of the original file content.
	// Used for conflict detection.
	OriginalHash string `json:"original_hash"`

	// DesiredContent is the full file content the user wants to save.
	// Used as a fallback when the diff cannot be applied (e.g., hash conflict).
	// Nil means no fallback; pointer to empty string means save as empty file.
	DesiredContent *string `json:"desired_content,omitempty"`
}

// FileUpdateResponse represents a response to a file update request.
type FileUpdateResponse struct {
	// RequestID identifies the request.
	RequestID string `json:"request_id"`

	// Path is the file path.
	Path string `json:"path"`

	// Success indicates if the update was successful.
	Success bool `json:"success"`

	// NewHash is the SHA256 hash of the updated file content.
	NewHash string `json:"new_hash,omitempty"`

	// Resolution indicates how the update was applied.
	// "applied" = normal diff apply, "overwritten" = full content write fallback.
	Resolution string `json:"resolution,omitempty"`

	// Error contains error message if the request failed.
	Error string `json:"error,omitempty"`
}

// FileCreateRequest represents a request to create a new file.
//
// HTTP endpoint: POST /api/v1/workspace/file/create
type FileCreateRequest struct {
	// Path is the file path (relative to the per-repo subpath when Repo is
	// set, otherwise relative to the workspace root).
	Path string `json:"path"`

	// Repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo
	// workspaces. When set, Path is interpreted relative to <workDir>/<Repo>.
	Repo string `json:"repo,omitempty"`
}

// FileCreateResponse represents a response to a file create request.
type FileCreateResponse struct {
	// Path is the file path.
	Path string `json:"path"`

	// Success indicates if the creation was successful.
	Success bool `json:"success"`

	// Error contains error message if the request failed.
	Error string `json:"error,omitempty"`
}

// FileDeleteRequest represents a request to delete a file or directory.
//
// HTTP endpoint: DELETE /api/v1/workspace/file
type FileDeleteRequest struct {
	// Path is the file path (relative to workspace root).
	Path string `json:"path"`
}

// FileDeleteResponse represents a response to a file delete request.
type FileDeleteResponse struct {
	// RequestID identifies the request.
	RequestID string `json:"request_id"`

	// Path is the file path.
	Path string `json:"path"`

	// Success indicates if the deletion was successful.
	Success bool `json:"success"`

	// Error contains error message if the request failed.
	Error string `json:"error,omitempty"`
}

// FileRenameRequest represents a request to rename a file or directory.
//
// HTTP endpoint: POST /api/v1/workspace/file/rename
type FileRenameRequest struct {
	// OldPath is the current path (relative to the per-repo subpath when
	// Repo is set, otherwise relative to the workspace root).
	OldPath string `json:"old_path"`

	// NewPath is the new path (same scoping rules as OldPath).
	NewPath string `json:"new_path"`

	// Repo is the multi-repo subpath (e.g. "kandev"); empty for single-repo
	// workspaces. Both OldPath and NewPath are scoped to this repo.
	Repo string `json:"repo,omitempty"`
}

// FileRenameResponse represents a response to a rename request.
type FileRenameResponse struct {
	// OldPath is the original path.
	OldPath string `json:"old_path"`

	// NewPath is the new path.
	NewPath string `json:"new_path"`

	// Success indicates if the rename was successful.
	Success bool `json:"success"`

	// Error contains error message if the request failed.
	Error string `json:"error,omitempty"`
}
