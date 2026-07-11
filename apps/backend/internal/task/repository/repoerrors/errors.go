package repoerrors

import "errors"

// ErrWorkspaceNameMismatch reports that a confirmed workspace delete did not
// match the workspace row's current name.
var ErrWorkspaceNameMismatch = errors.New("workspace name mismatch")

// ErrWorkspaceNotFound reports that no workspace row matched the supplied id.
var ErrWorkspaceNotFound = errors.New("workspace not found")

// ErrTaskNotFound reports that no task row matched the supplied id.
var ErrTaskNotFound = errors.New("task not found")

// ErrTaskPlanNotFound reports that no task plan row matched the supplied task id.
var ErrTaskPlanNotFound = errors.New("task plan not found")

// ErrRepositoryNotFound reports that no live repository row matched the supplied id.
var ErrRepositoryNotFound = errors.New("repository not found")
