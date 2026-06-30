package repoerrors

import "errors"

// ErrWorkspaceNameMismatch reports that a confirmed workspace delete did not
// match the workspace row's current name.
var ErrWorkspaceNameMismatch = errors.New("workspace name mismatch")

// ErrWorkspaceNotFound reports that no workspace row matched the supplied id.
var ErrWorkspaceNotFound = errors.New("workspace not found")
