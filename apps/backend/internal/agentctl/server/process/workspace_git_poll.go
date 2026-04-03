package process

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os/exec"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

// pollGitChanges periodically checks for git changes (commits, branch switches, staging)
// This catches manual git operations done via shell that file watching might miss
func (wt *WorkspaceTracker) pollGitChanges(ctx context.Context) {
	defer wt.wg.Done()

	// If no valid git index was found at startup, git commands will fail.
	// Exit immediately to avoid repeated rev-parse probes on a non-repo directory.
	if wt.gitIndexPath == "" {
		wt.logger.Warn("no valid git repository found, git polling not started",
			zap.String("workDir", wt.workDir))
		return
	}

	ticker := time.NewTicker(wt.gitPollInterval)
	defer ticker.Stop()

	// Initialize cached HEAD SHA, branch name, and index hash
	wt.gitStateMu.Lock()
	wt.cachedHeadSHA = wt.getHeadSHA(ctx)
	wt.cachedBranchName = wt.getCurrentBranchName(ctx)
	wt.cachedIndexHash = wt.getGitStatusHash(ctx)
	wt.gitStateMu.Unlock()

	wt.logger.Info("git polling started",
		zap.Duration("interval", wt.gitPollInterval),
		zap.String("initial_head", wt.cachedHeadSHA),
		zap.String("initial_branch", wt.cachedBranchName))

	var consecutiveFailures int

	for {
		select {
		case <-ctx.Done():
			return
		case <-wt.stopCh:
			return
		case <-ticker.C:
			// Stop polling if the work directory was deleted (worktree removed)
			if !wt.workDirExists() {
				wt.logger.Warn("work directory no longer exists, stopping git polling",
					zap.String("workDir", wt.workDir))
				return
			}

			// Quick git health check before running full change detection.
			// If git is broken (e.g., worktree .git reference points to deleted gitdir),
			// stop after maxConsecutiveGitFailures to avoid wasting CPU.
			probeCmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir")
			probeCmd.Dir = wt.workDir
			if err := probeCmd.Run(); err != nil {
				consecutiveFailures++
				if consecutiveFailures >= maxConsecutiveGitFailures {
					wt.logger.Error("git not functional, stopping git polling",
						zap.String("workDir", wt.workDir),
						zap.Int("consecutiveFailures", consecutiveFailures),
						zap.Error(err))
					return
				}
				continue
			}
			consecutiveFailures = 0

			wt.checkGitChanges(ctx)
		}
	}
}

// getHeadSHA returns the current HEAD commit SHA
func (wt *WorkspaceTracker) getHeadSHA(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = wt.workDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// getCurrentBranchName returns the current branch name.
// Returns empty string if in detached HEAD state (e.g., during rebase or when a commit/tag is checked out).
func (wt *WorkspaceTracker) getCurrentBranchName(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, "git", "symbolic-ref", "--short", "-q", "HEAD")
	cmd.Dir = wt.workDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// getGitStatusHash returns a hash of the git status porcelain output.
// This is used to detect changes to the git index (staging/unstaging) that don't
// change HEAD. The hash includes both the status codes and file paths.
func (wt *WorkspaceTracker) getGitStatusHash(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain", "--untracked-files=all")
	cmd.Dir = wt.workDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(out)
	return hex.EncodeToString(hash[:])
}

// checkGitChanges checks if HEAD or git index has changed and processes changes
func (wt *WorkspaceTracker) checkGitChanges(ctx context.Context) {
	currentHead := wt.getHeadSHA(ctx)
	currentBranch := wt.getCurrentBranchName(ctx)
	currentIndexHash := wt.getGitStatusHash(ctx)

	wt.gitStateMu.RLock()
	previousHead := wt.cachedHeadSHA
	previousBranch := wt.cachedBranchName
	previousIndexHash := wt.cachedIndexHash
	wt.gitStateMu.RUnlock()

	headChanged := currentHead != "" && currentHead != previousHead
	branchChanged := currentBranch != previousBranch // Track all transitions including to/from detached HEAD
	indexChanged := currentIndexHash != "" && currentIndexHash != previousIndexHash

	// If nothing changed, nothing to do
	if !headChanged && !branchChanged && !indexChanged {
		return
	}

	// Update cached index hash (HEAD will be updated below if it changed)
	if indexChanged && !headChanged && !branchChanged {
		wt.gitStateMu.Lock()
		wt.cachedIndexHash = currentIndexHash
		wt.gitStateMu.Unlock()

		wt.logger.Debug("git index changed (staging/unstaging detected)")
		wt.updateGitStatus(ctx)
		return
	}

	// Branch changed without HEAD change - this can happen when:
	// 1. Switching between branches that point to the same commit
	// 2. Transitioning to/from detached HEAD state (git switch --detach, rebase end)
	if branchChanged && !headChanged {
		wt.gitStateMu.Lock()
		wt.cachedBranchName = currentBranch
		wt.cachedIndexHash = currentIndexHash
		wt.gitStateMu.Unlock()

		// Only handle branch switch if we're not in detached HEAD state
		// Suppress notification when currentBranch is empty (detached HEAD)
		if currentBranch != "" {
			wt.handleBranchSwitch(ctx, previousBranch, currentBranch, currentHead)
			return
		}

		// In detached HEAD state, just update git status
		wt.updateGitStatus(ctx)
		return
	}

	// HEAD changed - handle normally
	if !headChanged {
		return
	}

	wt.logger.Info("git HEAD changed, syncing",
		zap.String("previous", previousHead),
		zap.String("current", currentHead))

	// Update cached HEAD, branch, and index hash
	wt.gitStateMu.Lock()
	wt.cachedHeadSHA = currentHead
	wt.cachedBranchName = currentBranch
	wt.cachedIndexHash = currentIndexHash
	wt.gitStateMu.Unlock()

	// Check if this is a branch switch (branch name changed)
	// Branch switches need special handling to update the base commit
	// Only handle if we're not in detached HEAD state (currentBranch != "")
	if branchChanged && currentBranch != "" {
		wt.handleBranchSwitch(ctx, previousBranch, currentBranch, currentHead)
		return
	}

	// Check if history was rewritten (reset, rebase, amend, etc.)
	// There are three cases:
	// 1. HEAD moved backward: currentHead is an ancestor of previousHead (e.g., git reset HEAD~1)
	// 2. History rewritten: previousHead is NOT reachable from currentHead (e.g., git rebase -i, git commit --amend)
	// 3. HEAD moved forward: previousHead IS an ancestor of currentHead (normal commits)
	if previousHead != "" {
		switch {
		case wt.isAncestor(ctx, currentHead, previousHead):
			// Case 1: HEAD moved backward - emit reset notification
			wt.logger.Info("detected git reset (HEAD moved backward)",
				zap.String("previous", previousHead),
				zap.String("current", currentHead))
			wt.notifyWorkspaceStreamGitReset(&types.GitResetNotification{
				Timestamp:    time.Now(),
				PreviousHead: previousHead,
				CurrentHead:  currentHead,
			})
		case !wt.isAncestor(ctx, previousHead, currentHead):
			// Case 2: History was rewritten - previousHead is not reachable from currentHead
			// This happens with interactive rebase, commit amend, etc.
			wt.logger.Info("detected git history rewrite (previous HEAD not reachable)",
				zap.String("previous", previousHead),
				zap.String("current", currentHead))
			wt.notifyWorkspaceStreamGitReset(&types.GitResetNotification{
				Timestamp:    time.Now(),
				PreviousHead: previousHead,
				CurrentHead:  currentHead,
			})
		default:
			// Case 3: HEAD moved forward normally - get new commits
			commits := wt.getCommitsSince(ctx, previousHead)

			// Filter out commits that are already on remote branches.
			// This prevents recording upstream commits as session commits when
			// the user pulls/rebases onto a remote branch (e.g., git reset --hard main).
			localCommits := wt.filterLocalCommits(ctx, commits)

			for _, commit := range localCommits {
				wt.notifyWorkspaceStreamGitCommit(commit)
			}
			if len(localCommits) > 0 {
				wt.logger.Info("detected new commits via polling",
					zap.Int("count", len(localCommits)))
			}
		}
	}

	// Update and broadcast git status
	wt.updateGitStatus(ctx)
}

// handleBranchSwitch handles a branch switch event by calculating the new base commit
// and notifying subscribers. This allows the session to update its base commit to reflect
// the new branch's merge-base with the target branch (e.g., main).
func (wt *WorkspaceTracker) handleBranchSwitch(ctx context.Context, previousBranch, currentBranch, currentHead string) {
	wt.logger.Info("detected branch switch",
		zap.String("previous", previousBranch),
		zap.String("current", currentBranch),
		zap.String("head", currentHead))

	// Calculate the new base commit for the current branch
	// Use the sampled currentHead to avoid race conditions
	baseCommit := wt.getBaseCommitForBranch(ctx, currentBranch, currentHead)

	// Only notify if we successfully resolved a base commit
	// Skip notification if base commit resolution failed to avoid incomplete updates
	if baseCommit != "" {
		wt.notifyWorkspaceStreamBranchSwitch(&types.GitBranchSwitchNotification{
			Timestamp:      time.Now(),
			PreviousBranch: previousBranch,
			CurrentBranch:  currentBranch,
			CurrentHead:    currentHead,
			BaseCommit:     baseCommit,
		})
	} else {
		wt.logger.Warn("failed to resolve base commit for branch switch, skipping notification",
			zap.String("branch", currentBranch))
	}

	// Update and broadcast git status to reflect the new branch
	wt.updateGitStatus(ctx)
}

// getBaseCommitForBranch calculates the base commit (merge-base) for a given branch.
// This is the common ancestor between the branch and the integration branch (e.g., main).
// Prioritizes integration branches (origin/main, origin/master, main, master) over upstream
// to ensure the base commit represents the branch-off point from the main development line.
// Uses the provided head SHA to avoid race conditions with concurrent git operations.
func (wt *WorkspaceTracker) getBaseCommitForBranch(ctx context.Context, branch, head string) string {
	var baseBranch string

	// Try integration branch candidates first (origin/main, origin/master, main, master)
	// This ensures we get the branch-off point from the main development line
	for _, candidate := range []string{"origin/main", "origin/master", "main", "master"} {
		checkCmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", candidate)
		checkCmd.Dir = wt.workDir
		if err := checkCmd.Run(); err == nil {
			baseBranch = candidate
			break
		}
	}

	// Fall back to upstream tracking branch if no integration branch exists
	if baseBranch == "" {
		upstreamCmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", branch+"@{upstream}")
		upstreamCmd.Dir = wt.workDir
		upstreamOut, err := upstreamCmd.Output()
		if err == nil && len(upstreamOut) > 0 {
			baseBranch = strings.TrimSpace(string(upstreamOut))
		}
	}

	// Calculate merge-base if we found a base branch
	// Use the sampled head SHA instead of live HEAD to avoid race conditions
	if baseBranch != "" && head != "" {
		mergeBaseCmd := exec.CommandContext(ctx, "git", "merge-base", baseBranch, head)
		mergeBaseCmd.Dir = wt.workDir
		if mergeBaseOut, err := mergeBaseCmd.Output(); err == nil {
			return strings.TrimSpace(string(mergeBaseOut))
		}
	}

	return ""
}

// isAncestor checks if commit1 is an ancestor of commit2
func (wt *WorkspaceTracker) isAncestor(ctx context.Context, commit1, commit2 string) bool {
	cmd := exec.CommandContext(ctx, "git", "merge-base", "--is-ancestor", commit1, commit2)
	cmd.Dir = wt.workDir
	err := cmd.Run()
	// Exit code 0 means commit1 IS an ancestor of commit2
	// Exit code 1 means commit1 is NOT an ancestor of commit2
	return err == nil
}

// isOnRemote checks if a commit is reachable from any remote tracking branch.
// This is used to filter out upstream commits that came from a pull/fetch,
// as opposed to commits made locally in the session.
func (wt *WorkspaceTracker) isOnRemote(ctx context.Context, commitSHA string) bool {
	// Use git branch -r --contains to check if commit is on any remote branch
	cmd := exec.CommandContext(ctx, "git", "branch", "-r", "--contains", commitSHA)
	cmd.Dir = wt.workDir
	out, err := cmd.Output()
	if err != nil {
		// If the command fails, assume it's not on remote (safer default)
		return false
	}
	// If output is non-empty, commit is reachable from at least one remote branch
	return strings.TrimSpace(string(out)) != ""
}

// filterLocalCommits filters out commits that are already on remote branches.
// This ensures we only report commits made locally in the session, not upstream commits
// that came from pulling/rebasing onto a remote branch.
func (wt *WorkspaceTracker) filterLocalCommits(ctx context.Context, commits []*types.GitCommitNotification) []*types.GitCommitNotification {
	if len(commits) == 0 {
		return commits
	}

	localCommits := make([]*types.GitCommitNotification, 0, len(commits))
	for _, commit := range commits {
		if !wt.isOnRemote(ctx, commit.CommitSHA) {
			localCommits = append(localCommits, commit)
		} else {
			wt.logger.Debug("skipping upstream commit (already on remote)",
				zap.String("sha", commit.CommitSHA),
				zap.String("message", commit.Message))
		}
	}

	if skipped := len(commits) - len(localCommits); skipped > 0 {
		wt.logger.Info("filtered upstream commits",
			zap.Int("total", len(commits)),
			zap.Int("skipped", skipped),
			zap.Int("local", len(localCommits)))
	}

	return localCommits
}
