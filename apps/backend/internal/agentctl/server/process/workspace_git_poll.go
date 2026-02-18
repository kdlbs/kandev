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

	ticker := time.NewTicker(wt.gitPollInterval)
	defer ticker.Stop()

	// Initialize cached HEAD SHA and index hash
	wt.gitStateMu.Lock()
	wt.cachedHeadSHA = wt.getHeadSHA(ctx)
	wt.cachedIndexHash = wt.getGitStatusHash(ctx)
	wt.gitStateMu.Unlock()

	wt.logger.Info("git polling started",
		zap.Duration("interval", wt.gitPollInterval),
		zap.String("initial_head", wt.cachedHeadSHA))

	for {
		select {
		case <-ctx.Done():
			return
		case <-wt.stopCh:
			return
		case <-ticker.C:
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
	currentIndexHash := wt.getGitStatusHash(ctx)

	wt.gitStateMu.RLock()
	previousHead := wt.cachedHeadSHA
	previousIndexHash := wt.cachedIndexHash
	wt.gitStateMu.RUnlock()

	headChanged := currentHead != "" && currentHead != previousHead
	indexChanged := currentIndexHash != "" && currentIndexHash != previousIndexHash

	// If neither HEAD nor index changed, nothing to do
	if !headChanged && !indexChanged {
		return
	}

	// Update cached index hash (HEAD will be updated below if it changed)
	if indexChanged && !headChanged {
		wt.gitStateMu.Lock()
		wt.cachedIndexHash = currentIndexHash
		wt.gitStateMu.Unlock()

		wt.logger.Debug("git index changed (staging/unstaging detected)")
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

	// Update cached HEAD and index hash
	wt.gitStateMu.Lock()
	wt.cachedHeadSHA = currentHead
	wt.cachedIndexHash = currentIndexHash
	wt.gitStateMu.Unlock()

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
