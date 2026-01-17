package streams

import "time"

// GitStatusUpdate is the message type streamed via the git status stream.
// Represents the current git state of the workspace.
//
// Stream endpoint: ws://.../api/v1/workspace/git-status/stream
type GitStatusUpdate struct {
	// Timestamp is when this status was captured.
	Timestamp time.Time `json:"timestamp"`

	// Modified contains paths of modified files.
	Modified []string `json:"modified"`

	// Added contains paths of added/staged files.
	Added []string `json:"added"`

	// Deleted contains paths of deleted files.
	Deleted []string `json:"deleted"`

	// Untracked contains paths of untracked files.
	Untracked []string `json:"untracked"`

	// Renamed contains paths of renamed files.
	Renamed []string `json:"renamed"`

	// Ahead is the number of commits ahead of the remote branch.
	Ahead int `json:"ahead"`

	// Behind is the number of commits behind the remote branch.
	Behind int `json:"behind"`

	// Branch is the current local branch name.
	Branch string `json:"branch"`

	// RemoteBranch is the tracked remote branch (e.g., "origin/main").
	RemoteBranch string `json:"remote_branch,omitempty"`

	// Files contains detailed information about each changed file.
	Files map[string]FileInfo `json:"files,omitempty"`
}

// FileInfo represents detailed information about a file's git status.
type FileInfo struct {
	// Path is the file path relative to workspace root.
	Path string `json:"path"`

	// Status indicates the file status: "modified", "added", "deleted", "untracked", "renamed".
	Status string `json:"status"`

	// Additions is the number of added lines.
	Additions int `json:"additions,omitempty"`

	// Deletions is the number of deleted lines.
	Deletions int `json:"deletions,omitempty"`

	// OldPath is the original path for renamed files.
	OldPath string `json:"old_path,omitempty"`

	// Diff contains the unified diff content for this file.
	Diff string `json:"diff,omitempty"`
}

