package types

import "errors"

// ErrWorkspaceUploadConflict reports that an upload destination already exists
// and the caller supplied no resolution.
//
// It lives here rather than beside the agentctl client so that higher-level
// callers can distinguish a conflict from a failure without importing the
// runtime-tier client package directly (ARCH-RUNTIME-IMPORT).
var ErrWorkspaceUploadConflict = errors.New("workspace upload destination already exists")

// WorkspaceUploadConflict is one upload destination that already exists.
type WorkspaceUploadConflict struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

// UploadedWorkspaceFile is the path agentctl actually wrote. It differs from the
// requested path when the caller asked to keep both copies.
type UploadedWorkspaceFile struct {
	Path              string `json:"path"`
	SizeBytes         int64  `json:"size_bytes"`
	ResolutionApplied string `json:"resolution_applied"`
}

// WorkspaceUploadParams describes one file destined for a task workspace.
// Content is supplied separately so this stays a plain data type.
type WorkspaceUploadParams struct {
	Dir          string
	Repo         string
	RelativePath string
	Resolution   string
	SizeBytes    int64
}
