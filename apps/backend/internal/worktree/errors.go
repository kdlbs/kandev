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

	// ErrRemoteRefMissing is returned when Git confirms that the requested
	// remote ref is unavailable. It is narrower than ErrInvalidBaseBranch so
	// callers can distinguish a deleted branch from a failed fetch.
	ErrRemoteRefMissing = errors.New("remote ref does not exist")

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

	// ErrRemoteDefaultNetwork is returned when refreshing origin/HEAD fails
	// because the remote cannot be reached.
	ErrRemoteDefaultNetwork = errors.New("remote default branch network failure")

	// ErrRemoteDefaultUnresolved is returned when origin has no usable default
	// branch after the local ref and refresh paths have both been attempted.
	ErrRemoteDefaultUnresolved = errors.New("remote default branch unresolved")

	// ErrNonFastForward is returned when a fetch/pull is rejected due to non-fast-forward updates.
	ErrNonFastForward = errors.New("non-fast-forward update rejected")

	// ErrGitCryptFailed is returned when git-crypt unlock fails during worktree creation.
	ErrGitCryptFailed = errors.New("git-crypt unlock failed")

	// ErrTaskDirRequired is returned when a worktree create request is missing
	// TaskDirName or RepoName. Worktrees are always placed inside the per-task
	// directory (~/.kandev/tasks/{taskDir}/{repo}/), so callers must populate
	// both fields.
	ErrTaskDirRequired = errors.New("worktree create requires TaskDirName and RepoName")

	// ErrInvalidRepoName is returned when RepoName is set but contains no
	// characters that survive sanitization (e.g. "!@#$%"), so it cannot be
	// used as a directory segment.
	ErrInvalidRepoName = errors.New("repo name has no usable characters after sanitization")

	// ErrBranchUnrecoverable is returned by recreate when the worktree's
	// branch no longer exists locally (archive deletes it via `git branch
	// -D`) and could not be fetched from origin either. Callers can treat
	// this as "prior work is gone" and fall back to a fresh worktree.
	ErrBranchUnrecoverable = errors.New("worktree branch no longer exists locally or on origin")

	// ErrInvalidBranchSlug is returned when BranchSlug is set but contains no
	// usable characters after sanitization. Callers that pass an explicit slug
	// must populate something the filesystem can accept.
	ErrInvalidBranchSlug = errors.New("branch slug has no usable characters after sanitization")

	// ErrEnvironmentNotResolved is returned by the store when a worktree
	// cannot be persisted because the session has no task environment yet
	// (initial materialization). The manager treats it as a skip, not a
	// failure — the launch's environment persistence records the worktree.
	ErrEnvironmentNotResolved = errors.New("session has no task environment")

	// ErrTaskCleanupInProgress is returned by the store when the owning
	// task has an active lifecycle cleanup barrier. The caller compensates
	// the just-created physical worktree instead of admitting it after
	// cleanup inventory was captured.
	ErrTaskCleanupInProgress = errors.New("task cleanup in progress")

	// ErrReuseWorktreeUnavailable is returned when an attach-only launch cannot
	// find a valid canonical worktree. Callers must surface this as a workspace
	// reuse failure; they must never fall through to git worktree add.
	ErrReuseWorktreeUnavailable = errors.New("required worktree is unavailable for reuse")

	// ErrWorktreePathOwnedByAnotherTask is returned when a persisted worktree
	// record points at a task root whose ownership marker belongs to a
	// different task. Reusing or recreating that record could attach to or
	// delete the other task's live checkout.
	ErrWorktreePathOwnedByAnotherTask = errors.New("worktree path is owned by another task")
)

// BranchUnrecoverableError identifies the branch that could not be restored.
// It wraps ErrBranchUnrecoverable so callers can preserve the existing
// sentinel check while presenting a branch-specific recovery action.
type BranchUnrecoverableError struct {
	Branch string
}

type confirmedRemoteRefMissingError struct {
	detail string
}

func (e *confirmedRemoteRefMissingError) Error() string {
	if e == nil || e.detail == "" {
		return ErrRemoteRefMissing.Error()
	}
	return fmt.Sprintf("%s: %s", ErrRemoteRefMissing, e.detail)
}

func (e *confirmedRemoteRefMissingError) Unwrap() []error {
	return []error{ErrRemoteRefMissing, ErrInvalidBaseBranch}
}

func newConfirmedRemoteRefMissingError(detail string) error {
	return &confirmedRemoteRefMissingError{detail: strings.TrimSpace(detail)}
}

func (e *BranchUnrecoverableError) Error() string {
	if e == nil || e.Branch == "" {
		return ErrBranchUnrecoverable.Error()
	}
	return fmt.Sprintf("%s: %q", ErrBranchUnrecoverable, e.Branch)
}

func (e *BranchUnrecoverableError) Unwrap() error {
	return ErrBranchUnrecoverable
}

// BranchName returns the branch that was requested by the persisted worktree.
func (e *BranchUnrecoverableError) BranchName() string {
	if e == nil {
		return ""
	}
	return e.Branch
}

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

// isRemoteRefMissingError reports whether a fetch error indicates the
// requested ref genuinely does not exist on the remote (or there is no
// usable remote at all), as opposed to a transient network/auth failure
// where the branch may still exist and a retry could succeed.
func isRemoteRefMissingError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "couldn't find remote ref") ||
		strings.Contains(msg, "does not appear to be a git repository") ||
		strings.Contains(msg, "no such remote") ||
		strings.Contains(msg, "no remote configured") ||
		strings.Contains(msg, "cannot determine remote head") ||
		strings.Contains(msg, "remote head refers to nonexistent ref")
}

// isRemoteBranchMissingError reports the Git fetch result for a branch that
// does not exist on an otherwise usable remote. Transport and authentication
// failures must remain generic so callers do not mistake an unavailable
// remote for a deleted branch.
func isRemoteBranchMissingError(output string) bool {
	lowerOutput := strings.ToLower(output)
	return strings.Contains(lowerOutput, "couldn't find remote ref") ||
		(strings.Contains(lowerOutput, "remote ref") && strings.Contains(lowerOutput, "not found"))
}

// ClassifyGitError wraps a raw git error with a user-friendly sentinel error
// based on the command output.
func ClassifyGitError(output string, _ error) error {
	trimmed := strings.TrimSpace(output)
	lowerOutput := strings.ToLower(output)

	switch {
	case isBranchCheckedOutError(output):
		return fmt.Errorf("%w: %s", ErrBranchCheckedOut, trimmed)
	case containsAuthFailure(lowerOutput):
		return fmt.Errorf("%w: %s", ErrAuthFailed, trimmed)
	case strings.Contains(lowerOutput, "non-fast-forward"):
		return fmt.Errorf("%w: %s", ErrNonFastForward, trimmed)
	case strings.Contains(lowerOutput, "filename too long"):
		return fmt.Errorf(
			"%w: Git could not materialize the worktree and may have hit a path-length limit. "+
				"Kandev enabled Git long paths for this command; on Windows, enable Win32 long paths "+
				"or use a shorter checkout path. Git output: %s",
			ErrGitCommandFailed, trimmed,
		)
	default:
		return fmt.Errorf("%w: %s", ErrGitCommandFailed, trimmed)
	}
}
