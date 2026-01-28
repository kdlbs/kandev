// Package worktree provides Git worktree management for concurrent agent execution.
package worktree

import "errors"

var (
	// ErrWorktreeExists is returned when attempting to create a worktree that already exists.
	ErrWorktreeExists = errors.New("worktree already exists for task")

	// ErrWorktreeNotFound is returned when the requested worktree does not exist.
	ErrWorktreeNotFound = errors.New("worktree not found")

	// ErrRepoNotGit is returned when the repository path is not a Git repository.
	ErrRepoNotGit = errors.New("repository is not a git repository")

	// ErrBranchExists is returned when the branch name already exists in the repository.
	ErrBranchExists = errors.New("branch already exists")

	// ErrWorktreeLocked is returned when the worktree is locked by another process.
	ErrWorktreeLocked = errors.New("worktree is locked by another process")

	// ErrInvalidBaseBranch is returned when the base branch does not exist.
	ErrInvalidBaseBranch = errors.New("base branch does not exist")

	// ErrWorktreeCorrupted is returned when the worktree directory is corrupted or invalid.
	ErrWorktreeCorrupted = errors.New("worktree directory is corrupted")

	// ErrGitCommandFailed is returned when a git command fails to execute.
	ErrGitCommandFailed = errors.New("git command failed")

	// ErrInvalidSession is returned when the session ID is invalid or empty.
	ErrInvalidSession = errors.New("invalid or empty session ID")
)

