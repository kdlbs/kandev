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
	return errors.Is(err, worktree.ErrDirtyWorktreeCleanup)
}
