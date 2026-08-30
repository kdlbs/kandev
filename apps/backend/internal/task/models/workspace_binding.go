package models

import "errors"

// Workspace binding errors are returned before a new session is persisted.
// They deliberately describe state rather than filesystem details: callers can
// offer a bounded retry without exposing a checkout path or branch name.
var (
	ErrWorkspacePreparing   = errors.New("workspace is preparing")
	ErrWorkspaceReuseUnsafe = errors.New("workspace reuse is unsafe")
)
