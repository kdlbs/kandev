package process

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

// updateGitStatus updates the git status. Callers must coordinate access
// via updateMu — use tryUpdateGitStatus for polling loops, RefreshGitStatus
// for user-triggered operations.
func (wt *WorkspaceTracker) updateGitStatus(ctx context.Context) {
	status, err := wt.getGitStatus(ctx)
	if err != nil {
		wt.logger.Warn("updateGitStatus: getGitStatus failed", zap.Error(err))
		return
	}

	wt.mu.Lock()
	wt.currentStatus = status
	wt.mu.Unlock()

	// Notify workspace stream subscribers
	wt.notifyWorkspaceStreamGitStatus(status)
}

// tryUpdateGitStatus attempts a non-blocking git status update. If another
// update is already in progress (from the other polling loop or an explicit
// refresh), the call is skipped — the running update will produce the same result.
func (wt *WorkspaceTracker) tryUpdateGitStatus(ctx context.Context) {
	if !wt.updateMu.TryLock() {
		return
	}
	defer wt.updateMu.Unlock()
	wt.updateGitStatus(ctx)
}

// RefreshGitStatus forces a git status refresh and notifies subscribers.
// Useful after index-only changes (stage/unstage) that the file watcher won't detect.
// Uses a blocking lock so user-triggered operations always complete.
func (wt *WorkspaceTracker) RefreshGitStatus(ctx context.Context) {
	wt.updateMu.Lock()
	defer wt.updateMu.Unlock()
	wt.updateGitStatus(ctx)
}

// GetCurrentGitStatus returns the current cached git status.
// If no status has been cached yet, it fetches fresh status.
func (wt *WorkspaceTracker) GetCurrentGitStatus(ctx context.Context) (types.GitStatusUpdate, error) {
	return wt.GetGitStatus(ctx, false)
}

// GetGitStatus returns git status. When fresh is false, the cached value is
// returned (with a fresh fallback if no cache exists). When fresh is true, a
// new git query is always executed, bypassing the cache. Callers that need
// up-to-date data after the agent just committed changes should pass fresh=true.
func (wt *WorkspaceTracker) GetGitStatus(ctx context.Context, fresh bool) (types.GitStatusUpdate, error) {
	if fresh {
		return wt.getGitStatus(ctx)
	}

	wt.mu.RLock()
	status := wt.currentStatus
	wt.mu.RUnlock()

	if status.Timestamp.IsZero() {
		return wt.getGitStatus(ctx)
	}

	return status, nil
}

// getGitStatus retrieves the current git status. Secondary commands
// (ahead/behind counts, diff enrichment) that fail or time out carry their
// values forward from the prior cached status rather than overwriting them
// with zero. The two primary commands (branch info and `git status
// --porcelain`) still propagate errors upward; on those, updateGitStatus
// skips the cache write entirely so the prior state stays visible to the UI.
func (wt *WorkspaceTracker) getGitStatus(ctx context.Context) (types.GitStatusUpdate, error) {
	update := types.GitStatusUpdate{
		Timestamp:      time.Now(),
		RepositoryName: wt.repositoryName,
		Modified:       []string{},
		Added:          []string{},
		Deleted:        []string{},
		Untracked:      []string{},
		Renamed:        []string{},
		Files:          make(map[string]types.FileInfo),
	}

	// Bare trackers (multi-repo task roots) sit on a directory that isn't
	// itself a git repo. Without this guard, `git status` would ascend the
	// directory tree until it found a `.git` — for tasks nested inside a
	// developer's own kandev checkout that lands on the OUTER worktree and
	// silently emits its branch/ahead/behind as if it were the task. Bail
	// out early to keep the bare tracker's `currentStatus` zero-valued.
	if wt.gitIndexPath == "" {
		return update, nil
	}

	// Snapshot the prior cache once so per-command carry-forward sees a
	// consistent view even if a concurrent RefreshGitStatus replaces
	// currentStatus mid-poll.
	wt.mu.RLock()
	prior := wt.currentStatus
	wt.mu.RUnlock()

	if err := wt.getGitBranchInfo(ctx, &update); err != nil {
		return update, err
	}

	wt.getAheadBehindCounts(ctx, &update, prior)

	if err := wt.parseGitStatusOutput(ctx, &update); err != nil {
		return update, err
	}

	// Enrich file info with diff data (additions, deletions, and actual diff content)
	wt.enrichWithDiffData(ctx, &update, prior)

	// Compute full branch totals vs merge-base (committed + staged + unstaged + untracked)
	wt.enrichWithBranchDiff(ctx, &update, prior)

	return update, nil
}

// getGitBranchInfo populates branch, remote branch, head commit, and base commit fields.
// Each command runs under a per-command timeout via the runGit* helpers so a
// single wedged git invocation cannot pin the shared throttle slot.
func (wt *WorkspaceTracker) getGitBranchInfo(ctx context.Context, update *types.GitStatusUpdate) error {
	// Get current branch
	branchOut, err := wt.runGitOutput(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	update.Branch = strings.TrimSpace(string(branchOut))

	// Get remote branch
	if remoteOut, err := wt.runGitOutput(ctx, "rev-parse", "--abbrev-ref", "@{upstream}"); err == nil {
		update.RemoteBranch = strings.TrimSpace(string(remoteOut))
	}

	// Get current HEAD commit SHA
	if headOut, err := wt.runGitOutput(ctx, "rev-parse", "HEAD"); err == nil {
		update.HeadCommit = strings.TrimSpace(string(headOut))
	}

	// Get base commit SHA using merge-base between current branch and the integration branch.
	// Always use the integration branch (origin/main, etc.) rather than the tracking branch,
	// so we show all changes this branch introduces compared to the main development line.
	var baseBranch string
	for _, candidate := range []string{"origin/main", "origin/master", "main", "master"} {
		if err := wt.runGit(ctx, "rev-parse", "--verify", candidate); err == nil {
			baseBranch = candidate
			break
		}
	}
	if baseBranch != "" {
		// Use merge-base to find common ancestor between current branch and base branch.
		// This is correct even when the branch has diverged from main.
		if mergeBaseOut, err := wt.runGitOutput(ctx, "merge-base", baseBranch, "HEAD"); err == nil {
			update.BaseCommit = strings.TrimSpace(string(mergeBaseOut))
		}
	}

	return nil
}

// getAheadBehindCounts populates the Ahead/Behind fields relative to the base
// branch (origin/main or origin/master). Always compares against the base
// branch rather than the remote tracking branch, because after a rebase the
// tracking branch has stale commit SHAs that produce inflated counts.
//
// If either the compareRef lookup or the count command fails (e.g. timeout
// under throttle saturation, transient FS hang), Ahead/Behind are carried
// forward from prior when HEAD hasn't moved. Otherwise a single bad poll
// would silently overwrite the cached counts with 0/0 and hide a legitimate
// "Pull N" / "Push N" indicator in the UI.
func (wt *WorkspaceTracker) getAheadBehindCounts(ctx context.Context, update *types.GitStatusUpdate, prior types.GitStatusUpdate) {
	// Always compare against the base branch (origin/main or origin/master).
	// Using the remote tracking branch (origin/<feature-branch>) gives wrong counts
	// after rebase because rebased commits have new SHAs.
	var compareRef string
	for _, candidate := range []string{"origin/main", "origin/master"} {
		if err := wt.runGit(ctx, "rev-parse", "--verify", candidate); err == nil {
			compareRef = candidate
			break
		}
	}
	if compareRef == "" {
		carryAheadBehind(update, prior)
		return
	}
	countOut, err := wt.runGitOutput(ctx, "rev-list", "--left-right", "--count", update.Branch+"..."+compareRef)
	if err != nil {
		wt.logger.Debug("getAheadBehindCounts: rev-list failed, carrying forward", zap.Error(err))
		carryAheadBehind(update, prior)
		return
	}
	parts := strings.Fields(string(countOut))
	if len(parts) != 2 {
		carryAheadBehind(update, prior)
		return
	}
	update.Ahead, _ = strconv.Atoi(parts[0])
	update.Behind, _ = strconv.Atoi(parts[1])
}

// carryAheadBehind copies prior.Ahead/Behind onto update when HEAD hasn't
// moved. A new HEAD invalidates prior counts (the user committed, pulled, or
// reset), so we leave update zeroed in that case.
func carryAheadBehind(update *types.GitStatusUpdate, prior types.GitStatusUpdate) {
	if prior.HeadCommit == "" || prior.HeadCommit != update.HeadCommit {
		return
	}
	update.Ahead = prior.Ahead
	update.Behind = prior.Behind
}

// parseGitStatusOutput runs git status --porcelain and populates the file lists and map.
func (wt *WorkspaceTracker) parseGitStatusOutput(ctx context.Context, update *types.GitStatusUpdate) error {
	// --untracked-files=all shows all files in untracked directories, not just the directory name.
	// GIT_OPTIONAL_LOCKS=0 (via runPollingGitOutput) prevents the background poll loop from
	// taking .git/index.lock, which would race with concurrent user-initiated git operations.
	statusOut, err := wt.runPollingGitOutput(ctx, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return err
	}

	// Git status --porcelain format: XY filename
	// X = index (staged) status, Y = working tree (unstaged) status
	// ' ' = unmodified, M = modified, A = added, D = deleted, R = renamed, ? = untracked
	lines := strings.Split(string(statusOut), "\n")
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		wt.applyPorcelainLine(line, update)
	}
	return nil
}

// unquoteGitPath strips git's C-style quoting from paths that contain spaces or
// special characters. Git wraps such paths in double quotes in porcelain output
// (e.g. "path with spaces/file.md"). Go's strconv.Unquote handles the same
// escaping rules (backslash sequences for tabs, newlines, quotes, etc.).
func unquoteGitPath(p string) string {
	if len(p) >= 2 && p[0] == '"' && p[len(p)-1] == '"' {
		if unquoted, err := strconv.Unquote(p); err == nil {
			return unquoted
		}
	}
	return p
}

// applyPorcelainLine parses a single git status --porcelain line and updates the status update.
func (wt *WorkspaceTracker) applyPorcelainLine(line string, update *types.GitStatusUpdate) {
	indexStatus := line[0]    // Staged status (X)
	workTreeStatus := line[1] // Unstaged status (Y)
	rawPath := strings.TrimSpace(line[3:])

	// For renames the format is "old -> new" (each part may be independently
	// quoted), so we must split first and unquote each part separately.
	filePath := rawPath
	if indexStatus != 'R' {
		filePath = unquoteGitPath(rawPath)
	}

	fileInfo := types.FileInfo{Path: filePath}

	// Determine staged status based on index and worktree status.
	// Prioritize worktree changes as they represent the current state.
	switch {
	case indexStatus == '?' && workTreeStatus == '?':
		fileInfo.Status = fileStatusUntracked
		fileInfo.Staged = false
		update.Untracked = append(update.Untracked, filePath)
	case workTreeStatus == 'D':
		// File deleted in worktree - this is an unstaged deletion
		fileInfo.Status = fileStatusDeleted
		fileInfo.Staged = false
		update.Deleted = append(update.Deleted, filePath)
	case indexStatus == 'D':
		// File deleted and staged
		fileInfo.Status = fileStatusDeleted
		fileInfo.Staged = true
		update.Deleted = append(update.Deleted, filePath)
	case workTreeStatus == 'M':
		// Modified in worktree - unstaged modification
		fileInfo.Status = fileStatusModified
		fileInfo.Staged = false
		update.Modified = append(update.Modified, filePath)
	case indexStatus == 'M':
		// Modified and staged (no worktree changes)
		fileInfo.Status = fileStatusModified
		fileInfo.Staged = true
		update.Modified = append(update.Modified, filePath)
	case indexStatus == 'A':
		// Added and staged
		fileInfo.Status = "added"
		fileInfo.Staged = true
		update.Added = append(update.Added, filePath)
	case indexStatus == 'R':
		fileInfo.Status = "renamed"
		fileInfo.Staged = true
		// Renamed files have format "old -> new"; each part may be quoted independently.
		if idx := strings.Index(rawPath, " -> "); idx != -1 {
			fileInfo.OldPath = unquoteGitPath(rawPath[:idx])
			filePath = unquoteGitPath(rawPath[idx+4:])
			fileInfo.Path = filePath
		}
		update.Renamed = append(update.Renamed, filePath)
	}

	update.Files[filePath] = fileInfo
}
