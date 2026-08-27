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
	// let throttle queue time eat through the 10s budget under load,
	// producing 70s-lock-held / signal:killed cascades under git-pool
	// contention. With this ordering the 10s timer starts the moment
	// git is about to run, so we get an accurate "could not tell" only
	// when the inspect itself is the slow part.
	release, err := subproc.AcquireGit(ctx, subproc.GitLifecycle)
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

// runBoundedGitInspect runs a non-interactive local git inspection after
// acquiring the lifecycle throttle. The timeout starts after admission so
// queue wait does not consume the command's inspection budget.
func (m *Manager) runBoundedGitInspect(ctx context.Context, repoPath string, args ...string) (string, error) {
	release, err := subproc.AcquireGit(ctx, subproc.GitLifecycle)
	if err != nil {
		return "", err
	}
	defer release()

	inspectCtx, cancel := context.WithTimeout(ctx, m.inspectTimeout)
	defer cancel()
	cmd := m.newNonInteractiveGitCmd(inspectCtx, repoPath, args...)
	output, runErr := cmd.CombinedOutput()
	if ctxErr := inspectCtx.Err(); ctxErr != nil {
		return string(output), fmt.Errorf("git inspection timed out: %w", ctxErr)
	}
	return string(output), runErr
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

func (m *Manager) preferRefreshedRemoteRef(ctx context.Context, repoPath, branch string) (string, error) {
	localBranch := strings.TrimPrefix(branch, "origin/")
	if localBranch == "" {
		return branch, nil
	}
	remoteRef := "origin/" + localBranch
	return m.selectContainingRef(ctx, repoPath, branch, remoteRef)
}

// prepareCheckoutFromRefreshedOrigin verifies a refreshed remote branch and
// returns whether a usable local or remote ref exists without contacting
// origin. It returns false when neither ref exists, preserving the existing
// "create a new branch from base" behavior.
func (m *Manager) prepareCheckoutFromRefreshedOrigin(ctx context.Context, repoPath, branch string) (bool, error) {
	selected, err := m.prepareBranchFromRefreshedOrigin(ctx, repoPath, branch, branch, 0)
	return selected != "", err
}

// prepareBranchFromRefreshedOrigin selects a provider-refreshed source branch
// without contacting origin. A PR number selects the dedicated origin/pr/<N>
// ref, which is also how fork PR heads are kept available after the
// authenticated refresh. When both refs exist, the selected ref is the one
// that contains the other. A local-only ref is preserved, a refreshed remote
// ref is returned to the caller as the worktree start point, and divergence or
// an unverified relationship fails closed.
func (m *Manager) prepareBranchFromRefreshedOrigin(
	ctx context.Context, repoPath, localBranch, sourceBranch string, prNumber int,
) (string, error) {
	remoteRef := "origin/" + sourceBranch
	if prNumber > 0 {
		remoteRef = fmt.Sprintf("origin/pr/%d", prNumber)
	}
	localExists, err := m.branchExists(ctx, repoPath, localBranch)
	if err != nil {
		return "", err
	}
	remoteExists, err := m.branchExists(ctx, repoPath, remoteRef)
	if err != nil {
		return "", err
	}
	if !localExists && !remoteExists {
		return "", nil
	}
	if !remoteExists {
		return "", fmt.Errorf("required fetched remote ref %q is missing", remoteRef)
	}
	if !localExists {
		release, err := subproc.AcquireGit(ctx, subproc.GitLifecycle)
		if err != nil {
			return "", err
		}
		defer release()
		cmd := m.newNonInteractiveGitCmd(ctx, repoPath, "branch", "--track", localBranch, remoteRef)
		if output, runErr := cmd.CombinedOutput(); runErr != nil {
			return "", fmt.Errorf("create local branch %q from refreshed origin: %s: %w",
				localBranch, strings.TrimSpace(string(output)), runErr)
		}
		return localBranch, nil
	}

	return m.selectContainingRef(ctx, repoPath, localBranch, remoteRef)
}

// Branch recovery statuses reported by BranchRecoveryStatus.
const (
	BranchStatusLocal   = "local"
	BranchStatusRemote  = "remote"
	BranchStatusMissing = "missing"
)

// BranchRecoveryStatus reports where a worktree branch still exists:
// "local" when refs/heads/<branch> resolves, "remote" when only the
// remote-tracking ref refs/remotes/origin/<branch> does, "missing"
// otherwise. Offline-friendly best-effort probe — it inspects local refs
// only (no network), so a remote-tracking ref that was deleted upstream
// but not yet pruned still reports "remote"; the recreate-time fetch is
// the authoritative check.
func (m *Manager) BranchRecoveryStatus(ctx context.Context, repoPath, branch string) string {
	if repoPath == "" || branch == "" {
		return BranchStatusMissing
	}
	if exists, err := m.branchExists(ctx, repoPath, "refs/heads/"+branch); err == nil && exists {
		return BranchStatusLocal
	}
	if exists, err := m.branchExists(ctx, repoPath, "refs/remotes/origin/"+branch); err == nil && exists {
		return BranchStatusRemote
	}
	return BranchStatusMissing
}

// refContains reports whether container already includes every commit in
// contained, i.e. `git merge-base --is-ancestor contained container`.
// A non-zero ancestry result is distinct from a failed probe: the former is a
// proven negative, while the latter must stop required refresh preparation.
func (m *Manager) refContains(ctx context.Context, repoPath, container, contained string) (bool, error) {
	// Same Acquire-then-build-execCtx ordering as branchExists.
	release, err := subproc.AcquireGit(ctx, subproc.GitLifecycle)
	if err != nil {
		m.logger.Warn("refContains bounded by context before throttle acquire",
			zap.String("repository_path", repoPath),
			zap.String("container", container),
			zap.String("contained", contained),
			zap.Error(err))
		return false, fmt.Errorf("acquire Git ancestry check: %w", err)
	}
	defer release()
	inspectCtx, cancel := context.WithTimeout(ctx, m.inspectTimeout)
	defer cancel()
	cmd := m.newNonInteractiveGitCmd(inspectCtx, repoPath, "merge-base", "--is-ancestor", contained, container)
	runErr := cmd.Run()
	if ctxErr := inspectCtx.Err(); ctxErr != nil {
		return false, fmt.Errorf("git ancestry check timed out: %w", ctxErr)
	}
	if runErr == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("git ancestry check failed for %q and %q: %w", contained, container, runErr)
}

func (m *Manager) currentBranch(ctx context.Context, repoPath string) string {
	// Same Acquire-then-build-execCtx ordering as branchExists.
	release, err := subproc.AcquireGit(ctx, subproc.GitLifecycle)
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
	cmd := newGitCommand(ctx, args...)
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

// pullBaseBranch fetches the latest changes from origin and returns the safest
// ref to use for creating a new worktree. The function handles three scenarios:
//
//  1. baseBranch is already a remote ref (e.g., "origin/main"): fetch and use it directly
//  2. baseBranch is a local branch and we're currently on it: pull --ff-only to update
//  3. baseBranch is a local branch but we're on a different branch: use origin/<branch> instead
//
// A failed required fetch or an unprovable base relationship returns an error.
func (m *Manager) pullBaseBranch(
	ctx context.Context, repoPath, baseBranch string, onProgress SyncProgressCallback,
) (string, error) {
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
	// got one (70s-lock-held trace under contention).
	fetchArgs := []string{"fetch", gitNoTags, "origin"}
	if localBranch != "" {
		fetchArgs = append(fetchArgs, localBranch)
	}
	output, err, execCtxErr := m.runGitCombinedAfterAcquire(ctx, m.fetchTimeout, repoPath, fetchArgs...)
	if err != nil {
		reason := classifyGitFallbackReason(err, string(output), execCtxErr)
		return "", m.failRequiredSync(
			stepName, baseBranch, onProgress, reason,
			fmt.Errorf("required refresh of %q failed (%s): %w", baseBranch, reason, syncFailureCause(reason, err, execCtxErr)),
		)
	}

	if isRemoteRef {
		resolved := "origin/" + localBranch
		if exists, branchErr := m.branchExists(ctx, repoPath, resolved); branchErr != nil {
			return "", m.failRequiredSync(stepName, baseBranch, onProgress, "base_ref_unverified", branchErr)
		} else if !exists {
			return "", m.failRequiredSync(
				stepName, baseBranch, onProgress, "missing_remote_ref",
				fmt.Errorf("required fetched remote ref %q is missing", resolved),
			)
		}
		m.reportSyncCompleted(stepName, onProgress, fmt.Sprintf("Synced and using %s", resolved), "")
		return resolved, nil
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

func (m *Manager) failRequiredSync(
	stepName, branch string, onProgress SyncProgressCallback, reason string, err error,
) error {
	m.logger.Warn("required git refresh failed before worktree creation",
		zap.String("branch", branch),
		zap.String("reason", reason),
		zap.Error(err))
	m.reportSyncProgress(onProgress, SyncProgressEvent{
		StepName: stepName,
		Status:   SyncProgressFailed,
		Output:   fmt.Sprintf("Required refresh failed for %s (%s)", branch, reason),
		Error:    reason,
	})
	return err
}

func (m *Manager) resolveLocalBaseRef(
	ctx context.Context, repoPath, baseBranch, localBranch, stepName string,
	onProgress SyncProgressCallback,
) (string, error) {
	remoteRef := "origin/" + localBranch
	if m.currentBranch(ctx, repoPath) == baseBranch {
		return m.pullCurrentBranchOrFallback(ctx, repoPath, baseBranch, remoteRef, stepName, onProgress)
	}
	resolved, err := m.selectContainingRef(ctx, repoPath, baseBranch, remoteRef)
	if err != nil {
		return "", m.failRequiredSync(stepName, baseBranch, onProgress, "base_ref_unverified", err)
	}
	m.reportSyncCompleted(stepName, onProgress, fmt.Sprintf("Synced and using %s", resolved), "")
	return resolved, nil
}

func (m *Manager) pullCurrentBranchOrFallback(
	ctx context.Context, repoPath, baseBranch, remoteRef, stepName string,
	onProgress SyncProgressCallback,
) (string, error) {
	// Same Acquire-then-build-execCtx ordering as the fetch path.
	output, err, execCtxErr := m.runGitCombinedAfterAcquire(ctx, m.pullTimeout, repoPath, "pull", "--ff-only", "origin", baseBranch)
	if err != nil {
		reason := classifyGitFallbackReason(err, string(output), execCtxErr)
		resolved, selectErr := m.selectContainingRef(ctx, repoPath, baseBranch, remoteRef)
		if selectErr != nil {
			return "", m.failRequiredSync(stepName, baseBranch, onProgress, reason, selectErr)
		}
		m.reportSyncCompleted(stepName, onProgress, fmt.Sprintf("Pull %s; using %s", reason, resolved), "")
		return resolved, nil
	}
	m.reportSyncCompleted(stepName, onProgress, fmt.Sprintf("Synced and using %s", baseBranch), "")
	return baseBranch, nil
}

func (m *Manager) selectContainingRef(
	ctx context.Context, repoPath, localRef, remoteRef string,
) (string, error) {
	remoteExists, err := m.branchExists(ctx, repoPath, remoteRef)
	if err != nil {
		return "", fmt.Errorf("could not verify fetched remote ref %q: %w", remoteRef, err)
	}
	if !remoteExists {
		return "", fmt.Errorf("required fetched remote ref %q is missing", remoteRef)
	}
	if localRef == remoteRef {
		return remoteRef, nil
	}
	localExists, err := m.branchExists(ctx, repoPath, localRef)
	if err != nil {
		return "", fmt.Errorf("could not verify local base ref %q: %w", localRef, err)
	}
	if !localExists {
		return remoteRef, nil
	}
	remoteContainsLocal, err := m.refContains(ctx, repoPath, remoteRef, localRef)
	if err != nil {
		return "", err
	}
	if remoteContainsLocal {
		return remoteRef, nil
	}
	localContainsRemote, err := m.refContains(ctx, repoPath, localRef, remoteRef)
	if err != nil {
		return "", err
	}
	if localContainsRemote {
		return localRef, nil
	}
	return "", fmt.Errorf("base refs %q and %q diverged", localRef, remoteRef)
}

// syncFailureCause intentionally suppresses cmdErr because Git output can
// contain credentials. Callers expose only a bounded failure class and keep
// raw command output in internal logs where the existing redaction policy
// applies.
func syncFailureCause(reason string, _ error, contextErr error) error {
	if contextErr != nil {
		return contextErr
	}
	switch reason {
	case "timeout":
		return context.DeadlineExceeded
	case "non_interactive_auth_failed":
		return ErrAuthFailed
	default:
		return ErrGitCommandFailed
	}
}

// runGitCombinedAfterAcquire acquires the backend git throttle slot,
// then constructs a child context with execTimeout and runs the
// non-interactive git command with CombinedOutput. The exec timer
// starts only AFTER Acquire returns so throttle queue time cannot
// burn the budget (otherwise the cmd gets killed with `signal: killed`
// the moment it acquires a slot under contention).
// Returns (combined output, run error, exec-ctx error). The exec-ctx
// error lets callers tell a context-driven kill (timeout) from a
// regular git failure when classifying fallbacks.
func (m *Manager) runGitCombinedAfterAcquire(
	ctx context.Context, execTimeout time.Duration, repoPath string, args ...string,
) ([]byte, error, error) {
	release, err := subproc.AcquireGit(ctx, subproc.GitLifecycle)
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
