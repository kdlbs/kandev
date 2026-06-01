package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/subproc"
)

// isGitRepo checks if a path is a Git repository.
func (m *Manager) isGitRepo(path string) bool {
	gitDir := filepath.Join(path, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return false
	}
	// .git can be either a directory (regular repo) or a file (worktree)
	return info.IsDir() || info.Mode().IsRegular()
}

// branchExists checks if a branch exists in the repository.
// Bounded by m.inspectTimeout so a hung git (credential prompt, stuck filter,
// filesystem stall) cannot deadlock the caller while holding repoLock.
//
// Returns:
//   - (true, nil)  branch exists
//   - (false, nil) git ran and reported the branch absent
//   - (false, err) check could not be completed (timeout, fs stall); err
//     carries the underlying ctx error so callers can distinguish a real
//     "missing branch" from a "could not tell" and avoid surfacing a
//     misleading ErrInvalidBaseBranch.
func (m *Manager) branchExists(ctx context.Context, repoPath, branch string) (bool, error) {
	// Acquire the throttle slot FIRST, then start the inspectTimeout
	// timer. Building inspectCtx before Acquire (as we did originally)
	// let throttle queue time eat through the 10s budget under load —
	// see the 70s-lock-held / signal:killed cascade investigated in
	// PR #1216. With this ordering the 10s timer starts the moment git
	// is about to run, so we get an accurate "could not tell" only when
	// the inspect itself is the slow part.
	release, err := subproc.Git().Acquire(ctx)
	if err != nil {
		m.logger.Warn("branchExists bounded by context",
			zap.String("repository_path", repoPath),
			zap.String("branch", branch),
			zap.Error(err))
		return false, fmt.Errorf("branch check timed out for %q before throttle acquire: %w", branch, err)
	}
	defer release()
	inspectCtx, cancel := context.WithTimeout(ctx, m.inspectTimeout)
	defer cancel()
	cmd := m.newNonInteractiveGitCmd(inspectCtx, repoPath, "rev-parse", "--verify", branch)
	if err := cmd.Run(); err != nil {
		if ctxErr := inspectCtx.Err(); ctxErr != nil {
			m.logger.Warn("branchExists bounded by context",
				zap.String("repository_path", repoPath),
				zap.String("branch", branch),
				zap.Error(ctxErr))
			return false, fmt.Errorf("branch check timed out for %q after %s: %w", branch, m.inspectTimeout, ctxErr)
		}
		return false, nil
	}
	return true, nil
}

func (m *Manager) currentBranch(ctx context.Context, repoPath string) string {
	// Same Acquire-then-build-execCtx ordering as branchExists.
	release, err := subproc.Git().Acquire(ctx)
	if err != nil {
		return ""
	}
	defer release()
	inspectCtx, cancel := context.WithTimeout(ctx, m.inspectTimeout)
	defer cancel()
	cmd := m.newNonInteractiveGitCmd(inspectCtx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	output, runErr := cmd.Output()
	if runErr != nil {
		if ctxErr := inspectCtx.Err(); ctxErr != nil {
			m.logger.Warn("currentBranch bounded by context",
				zap.String("repository_path", repoPath),
				zap.Error(ctxErr))
		}
		return ""
	}
	return strings.TrimSpace(string(output))
}

func (m *Manager) newNonInteractiveGitCmd(ctx context.Context, repoPath string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=Never",
		"GIT_ASKPASS=echo",
		"SSH_ASKPASS=/bin/false",
		"GIT_SSH_COMMAND=ssh -oBatchMode=yes",
	)
	// After the context cancels and the process is killed, child processes
	// (e.g. credential helpers) may still hold stdout/stderr pipes open.
	// WaitDelay bounds how long CombinedOutput waits for those pipes to close.
	cmd.WaitDelay = 500 * time.Millisecond
	return cmd
}

func classifyGitFallbackReason(cmdErr error, cmdOutput string, ctxErr error) string {
	if errors.Is(ctxErr, context.DeadlineExceeded) || errors.Is(cmdErr, context.DeadlineExceeded) {
		return "timeout"
	}

	if containsAuthFailure(strings.ToLower(cmdOutput)) {
		return "non_interactive_auth_failed"
	}

	return "git_command_failed"
}

// pullBaseBranch fetches the latest changes from origin and returns the best ref to use
// for creating a new worktree. The function handles three scenarios:
//
//  1. baseBranch is already a remote ref (e.g., "origin/main"): fetch and use it directly
//  2. baseBranch is a local branch and we're currently on it: pull --ff-only to update
//  3. baseBranch is a local branch but we're on a different branch: use origin/<branch> instead
//
// On fetch/pull failure, errors are logged but the function continues with the best available ref.
func (m *Manager) pullBaseBranch(ctx context.Context, repoPath, baseBranch string, onProgress SyncProgressCallback) string {
	localBranch := strings.TrimPrefix(baseBranch, "origin/")
	isRemoteRef := localBranch != baseBranch
	stepName := "Sync base branch"

	m.reportSyncProgress(onProgress, SyncProgressEvent{
		StepName: stepName,
		Status:   SyncProgressRunning,
		Output:   fmt.Sprintf("Fetching latest changes for %s", baseBranch),
	})

	// Acquire the git throttle slot first, then start the fetch timer.
	// Order matters: the previous "build fetchCtx, then runGitCmd" shape
	// let throttle queue time burn the fetch budget while we waited for a
	// slot, and the cmd was killed with `signal: killed` the moment it
	// got one — see the 70s-lock-held trace in PR #1216.
	fetchArgs := []string{"fetch", gitNoTags, "origin"}
	if localBranch != "" {
		fetchArgs = append(fetchArgs, localBranch)
	}
	output, err, execCtxErr := m.runGitCombinedAfterAcquire(ctx, m.fetchTimeout, repoPath, fetchArgs...)
	if err != nil {
		return m.handleFetchFallback(baseBranch, stepName, onProgress, execCtxErr, output, err)
	}

	if isRemoteRef {
		resolved := "origin/" + localBranch
		m.reportSyncCompleted(stepName, onProgress, fmt.Sprintf("Synced and using %s", resolved), "")
		return resolved
	}

	return m.resolveLocalBaseRef(ctx, repoPath, baseBranch, localBranch, stepName, onProgress)
}

func (m *Manager) reportSyncProgress(cb SyncProgressCallback, event SyncProgressEvent) {
	if cb != nil {
		cb(event)
	}
}

func (m *Manager) reportSyncCompleted(stepName string, onProgress SyncProgressCallback, output, errOutput string) {
	m.reportSyncProgress(onProgress, SyncProgressEvent{
		StepName: stepName,
		Status:   SyncProgressCompleted,
		Output:   output,
		Error:    strings.TrimSpace(errOutput),
	})
}

func (m *Manager) handleFetchFallback(baseBranch, stepName string, onProgress SyncProgressCallback, ctxErr error, output []byte, cmdErr error) string {
	reason := classifyGitFallbackReason(cmdErr, string(output), ctxErr)
	m.logger.Warn("git fetch failed before worktree creation; continuing with fallback ref",
		zap.String("branch", baseBranch),
		zap.String("reason", reason),
		zap.String("fallback_ref", baseBranch),
		zap.String("output", string(output)),
		zap.Error(cmdErr))
	m.reportSyncCompleted(stepName, onProgress, fmt.Sprintf("Fetch %s; using fallback ref %s", reason, baseBranch), string(output))
	return baseBranch
}

func (m *Manager) resolveLocalBaseRef(
	ctx context.Context, repoPath, baseBranch, localBranch, stepName string,
	onProgress SyncProgressCallback,
) string {
	remoteRef := "origin/" + localBranch
	if m.currentBranch(ctx, repoPath) == baseBranch {
		return m.pullCurrentBranchOrFallback(ctx, repoPath, baseBranch, remoteRef, stepName, onProgress)
	}
	// Best-effort sync: a timeout / error here is treated the same as a
	// missing remote ref — caller falls back to the local baseBranch.
	if exists, _ := m.branchExists(ctx, repoPath, remoteRef); exists {
		m.reportSyncCompleted(stepName, onProgress, fmt.Sprintf("Synced and using %s", remoteRef), "")
		return remoteRef
	}
	m.reportSyncCompleted(stepName, onProgress, fmt.Sprintf("Remote ref not found; using %s", baseBranch), "")
	return baseBranch
}

func (m *Manager) pullCurrentBranchOrFallback(
	ctx context.Context, repoPath, baseBranch, remoteRef, stepName string,
	onProgress SyncProgressCallback,
) string {
	// Same Acquire-then-build-execCtx ordering as the fetch path.
	output, err, execCtxErr := m.runGitCombinedAfterAcquire(ctx, m.pullTimeout, repoPath, "pull", "--ff-only", "origin", baseBranch)
	if err != nil {
		reason := classifyGitFallbackReason(err, string(output), execCtxErr)
		m.logger.Warn("git pull failed before worktree creation; continuing with remote ref",
			zap.String("branch", baseBranch),
			zap.String("reason", reason),
			zap.String("remote_ref", remoteRef),
			zap.String("output", string(output)),
			zap.Error(err))
		m.reportSyncCompleted(stepName, onProgress, fmt.Sprintf("Pull %s; using %s", reason, remoteRef), string(output))
		return remoteRef
	}
	m.reportSyncCompleted(stepName, onProgress, fmt.Sprintf("Synced and using %s", baseBranch), "")
	return baseBranch
}

// runGitCombinedAfterAcquire acquires the backend git throttle slot,
// then constructs a child context with execTimeout and runs the
// non-interactive git command with CombinedOutput. The exec timer
// starts only AFTER Acquire returns so throttle queue time cannot
// burn the budget — see PR #1216 for the failure mode this prevents.
// Returns (combined output, run error, exec-ctx error). The exec-ctx
// error lets callers tell a context-driven kill (timeout) from a
// regular git failure when classifying fallbacks.
func (m *Manager) runGitCombinedAfterAcquire(
	ctx context.Context, execTimeout time.Duration, repoPath string, args ...string,
) ([]byte, error, error) {
	release, err := subproc.Git().Acquire(ctx)
	if err != nil {
		return nil, err, ctx.Err()
	}
	defer release()
	execCtx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	cmd := m.newNonInteractiveGitCmd(execCtx, repoPath, args...)
	out, runErr := cmd.CombinedOutput()
	return out, runErr, execCtx.Err()
}
