package service

import (
	"errors"

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
