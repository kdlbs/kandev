// Package worktree provides Git worktree management for concurrent agent execution.
package worktree

import (
	"errors"
	"fmt"
	"strings"
)

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

	// ErrBranchCheckedOut is returned when a branch is already checked out in another worktree.
	ErrBranchCheckedOut = errors.New("branch is already checked out in another worktree")

	// ErrAuthFailed is returned when git authentication fails (e.g. no credentials in non-interactive mode).
	ErrAuthFailed = errors.New("git authentication failed")

	// ErrNonFastForward is returned when a fetch/pull is rejected due to non-fast-forward updates.
	ErrNonFastForward = errors.New("non-fast-forward update rejected")

	// ErrGitCryptFailed is returned when git-crypt unlock fails during worktree creation.
	ErrGitCryptFailed = errors.New("git-crypt unlock failed")
)

// containsAuthFailure checks if git output indicates an authentication failure.
func containsAuthFailure(lowerOutput string) bool {
	return strings.Contains(lowerOutput, "authentication failed") ||
		strings.Contains(lowerOutput, "terminal prompts disabled") ||
		strings.Contains(lowerOutput, "could not read username") ||
		strings.Contains(lowerOutput, "username for 'https://") ||
		strings.Contains(lowerOutput, "askpass")
}

// isBranchCheckedOutError checks if git output indicates a branch is already
// checked out in another worktree. Different git versions use different messages:
// "is already checked out at" or "is already used by worktree at".
func isBranchCheckedOutError(output string) bool {
	out := strings.ToLower(output)
	return strings.Contains(out, "is already checked out at") ||
		strings.Contains(out, "is already used by worktree at")
}

// isFetchRefusedCheckedOut checks if a git fetch error is caused by the branch
// being checked out in another worktree. Git refuses to update a local branch
// ref that is currently checked out.
func isFetchRefusedCheckedOut(output string) bool {
	return strings.Contains(strings.ToLower(output), "refusing to fetch into branch")
}

// ClassifyGitError wraps a raw git error with a user-friendly sentinel error
// based on the command output.
func ClassifyGitError(output string, _ error) error {
	trimmed := strings.TrimSpace(output)

	switch {
	case isBranchCheckedOutError(output):
		return fmt.Errorf("%w: %s", ErrBranchCheckedOut, trimmed)
	case containsAuthFailure(strings.ToLower(output)):
		return fmt.Errorf("%w: %s", ErrAuthFailed, trimmed)
	case strings.Contains(strings.ToLower(output), "non-fast-forward"):
		return fmt.Errorf("%w: %s", ErrNonFastForward, trimmed)
	default:
		return fmt.Errorf("%w: %s", ErrGitCommandFailed, trimmed)
	}
}
