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

// checkoutBranchExistsAnywhere returns true when the named branch is present
// either locally or as origin/<branch>. Used by createInTaskDir to decide
// whether to treat req.CheckoutBranch as "fetch this existing ref" or as
// "create a new branch with this name". A timeout / fs stall counts as
// "present" so we don't accidentally clobber a working branch by creating a
// duplicate when the probe couldn't complete.
func (m *Manager) checkoutBranchExistsAnywhere(ctx context.Context, repoPath, branch string) bool {
	local, err := m.branchExists(ctx, repoPath, branch)
	if err != nil {
		return true
	}
	if local {
		return true
	}
	remote, err := m.branchExists(ctx, repoPath, "refs/remotes/origin/"+branch)
	if err != nil {
		return true
	}
	return remote
}

func (m *Manager) currentBranch(ctx context.Context, repoPath string) string {
	inspectCtx, cancel := context.WithTimeout(ctx, m.inspectTimeout)
	defer cancel()
	cmd := m.newNonInteractiveGitCmd(inspectCtx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
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

	// Fetch branch from origin in non-interactive mode.
	fetchCtx, cancelFetch := context.WithTimeout(ctx, m.fetchTimeout)
	defer cancelFetch()

	fetchArgs := []string{"fetch", gitNoTags, "origin"}
	if localBranch != "" {
		fetchArgs = append(fetchArgs, localBranch)
	}
	fetchCmd := m.newNonInteractiveGitCmd(fetchCtx, repoPath, fetchArgs...)
	if output, err := fetchCmd.CombinedOutput(); err != nil {
		return m.handleFetchFallback(baseBranch, stepName, onProgress, fetchCtx.Err(), output, err)
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
	pullCtx, cancelPull := context.WithTimeout(ctx, m.pullTimeout)
	defer cancelPull()

	pullCmd := m.newNonInteractiveGitCmd(pullCtx, repoPath, "pull", "--ff-only", "origin", baseBranch)
	if output, err := pullCmd.CombinedOutput(); err != nil {
		reason := classifyGitFallbackReason(err, string(output), pullCtx.Err())
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
