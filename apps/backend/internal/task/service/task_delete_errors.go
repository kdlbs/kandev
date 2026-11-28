package service

import (
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/worktree"
)

const TaskDeleteDirtyWorktreeErrorCode = "task_delete_dirty_worktree"

// TaskDeleteDirtyWorktreeError is returned before task deletion when one or
// more owned worktrees contain local changes and the caller did not grant
// discard consent.
type TaskDeleteDirtyWorktreeError struct {
	DirtyWorktrees []worktree.DirtyWorktree
}

func (e *TaskDeleteDirtyWorktreeError) Error() string {
	return "task worktree contains local changes"
}

// TaskDeleteChildWorkspaceConflictErrorCode is the machine-readable code for
// TaskDeleteChildWorkspaceConflictError.
const TaskDeleteChildWorkspaceConflictErrorCode = "task_delete_child_workspace_changed"

// TaskDeleteChildConflictReason says which part of a direct child changed
// between the non-cascade delete's read and its guarded workspace write.
type TaskDeleteChildConflictReason string

const (
	// TaskDeleteChildDeleted: the child row is gone.
	TaskDeleteChildDeleted TaskDeleteChildConflictReason = "child_deleted"
	// TaskDeleteChildReparented: the child now has a different parent.
	TaskDeleteChildReparented TaskDeleteChildConflictReason = "child_reparented"
	// TaskDeleteChildWorkspaceChanged: the child's workspace mode or orphan
	// marker changed.
	TaskDeleteChildWorkspaceChanged TaskDeleteChildConflictReason = "child_workspace_changed"
)

// TaskDeleteChildWorkspaceConflictError is returned when a non-cascade delete
// cannot normalize a direct inherit_parent child's workspace mode because the
// child changed concurrently. Nothing has been deleted and the caller can
// retry with a fresh read. It wraps errWorkspaceMetadataChangedConcurrently.
type TaskDeleteChildWorkspaceConflictError struct {
	ChildID string
	Reason  TaskDeleteChildConflictReason
}

func (e *TaskDeleteChildWorkspaceConflictError) Error() string {
	return fmt.Sprintf("normalize workspace mode for child %s before delete: %s (%s)",
		e.ChildID, errWorkspaceMetadataChangedConcurrently, e.Reason)
}

func (e *TaskDeleteChildWorkspaceConflictError) Unwrap() error {
	return errWorkspaceMetadataChangedConcurrently
}

func newTaskDeleteDirtyWorktreeError(dirty []worktree.DirtyWorktree) error {
	if len(dirty) == 0 {
		return nil
	}
	copyDirty := make([]worktree.DirtyWorktree, len(dirty))
	copy(copyDirty, dirty)
	for i := range copyDirty {
		copyDirty[i].DirtyFiles = append([]string(nil), copyDirty[i].DirtyFiles...)
	}
	return &TaskDeleteDirtyWorktreeError{DirtyWorktrees: copyDirty}
}

func isDirtyWorktreeCleanupError(err error) bool {
	// A dirty refusal is terminal only when it is the sole cleanup failure.
	// Joined errors can also contain retryable environment or remote cleanup
	// failures that must keep their retry schedule.
	if err == nil {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		causes := joined.Unwrap()
		return len(causes) == 1 && isDirtyWorktreeCleanupError(causes[0])
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return isDirtyWorktreeCleanupError(wrapped.Unwrap())
	}
	return errors.Is(err, worktree.ErrDirtyWorktreeCleanup)
}
