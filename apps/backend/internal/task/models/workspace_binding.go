package models

import (
	"errors"
	"fmt"
)

// Workspace binding errors are returned before a new session is persisted.
// They deliberately describe state rather than filesystem details: callers can
// offer a bounded retry without exposing a checkout path or branch name.
var (
	ErrWorkspacePreparing        = errors.New("workspace is preparing")
	ErrWorkspaceReuseUnsafe      = errors.New("workspace reuse is unsafe")
	ErrWorkspaceReuseUnsupported = errors.New("workspace reuse is unsupported")
	// ErrTaskEnvironmentResetRequired is the reuse refusal for a failed task
	// environment that still holds workspace resources. A start cannot
	// re-materialize it; resetting the task environment clears it.
	ErrTaskEnvironmentResetRequired = fmt.Errorf(
		"%w: the task environment failed with workspace resources; reset the task environment to start again",
		ErrWorkspaceReuseUnsafe,
	)
)

const (
	WorkspaceReuseUnsupportedRetry   = "create a task with workspace_mode=\"new_workspace\""
	WorkspaceReuseUnsupportedMessage = "the selected executor cannot reuse another task's workspace; " + WorkspaceReuseUnsupportedRetry
)

// WorkspacePreparingError is ErrWorkspacePreparing for a binding that lost the
// workspace election. EnvironmentID names the environment being materialized.
// In a shared workspace group it belongs to another member task, so a caller
// that wants to wait must use this ID rather than look up its own task.
type WorkspacePreparingError struct {
	EnvironmentID string
	Reason        string
}

func (e *WorkspacePreparingError) Error() string {
	return fmt.Sprintf("%s: %s", ErrWorkspacePreparing, e.Reason)
}

func (e *WorkspacePreparingError) Unwrap() error { return ErrWorkspacePreparing }

// DescribeInheritedEnvironmentUnavailable builds the reason clause for an
// inherit_parent task whose inherited task_environments row cannot be used
// (missing, or belonging to an archived parent). It is shared by
// internal/orchestrator and internal/orchestrator/executor — both packages
// already hold the parent *Task from their own GetTask lookups, and neither
// may import the other, so this lives in the neutral models package both
// import. parent may be nil when the parent task itself could not be loaded.
func DescribeInheritedEnvironmentUnavailable(parentID string, parent *Task) string {
	if parent != nil && parent.ArchivedAt != nil {
		return fmt.Sprintf("inherited task environment is unavailable: parent task %s was archived and its workspace was removed", parentID)
	}
	return fmt.Sprintf("inherited task environment is unavailable: parent task %s workspace could not be resolved", parentID)
}
