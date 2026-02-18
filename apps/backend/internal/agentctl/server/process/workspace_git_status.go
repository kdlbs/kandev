package process

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

// updateGitStatus updates the git status
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

// RefreshGitStatus forces a git status refresh and notifies subscribers.
// Useful after index-only changes (stage/unstage) that the file watcher won't detect.
func (wt *WorkspaceTracker) RefreshGitStatus(ctx context.Context) {
	wt.updateGitStatus(ctx)
}

// getGitStatus retrieves the current git status
func (wt *WorkspaceTracker) getGitStatus(ctx context.Context) (types.GitStatusUpdate, error) {
	update := types.GitStatusUpdate{
		Timestamp: time.Now(),
		Modified:  []string{},
		Added:     []string{},
		Deleted:   []string{},
		Untracked: []string{},
		Renamed:   []string{},
		Files:     make(map[string]types.FileInfo),
	}

	if err := wt.getGitBranchInfo(ctx, &update); err != nil {
		return update, err
	}

	wt.getAheadBehindCounts(ctx, &update)

	if err := wt.parseGitStatusOutput(ctx, &update); err != nil {
		return update, err
	}

	// Enrich file info with diff data (additions, deletions, and actual diff content)
	wt.enrichWithDiffData(ctx, &update)

	return update, nil
}

// getGitBranchInfo populates branch, remote branch, head commit, and base commit fields.
func (wt *WorkspaceTracker) getGitBranchInfo(ctx context.Context, update *types.GitStatusUpdate) error {
	// Get current branch
	branchCmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	branchCmd.Dir = wt.workDir
	branchOut, err := branchCmd.Output()
	if err != nil {
		return err
	}
	update.Branch = strings.TrimSpace(string(branchOut))

	// Get remote branch
	remoteCmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "@{upstream}")
	remoteCmd.Dir = wt.workDir
	if remoteOut, err := remoteCmd.Output(); err == nil {
		update.RemoteBranch = strings.TrimSpace(string(remoteOut))
	}

	// Get current HEAD commit SHA
	headCmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	headCmd.Dir = wt.workDir
	if headOut, err := headCmd.Output(); err == nil {
		update.HeadCommit = strings.TrimSpace(string(headOut))
	}

	// Get base commit SHA (base branch HEAD)
	// Use remote branch if available, fall back to main/master
	baseBranch := update.RemoteBranch
	if baseBranch == "" {
		// Try main first, then master
		baseBranch = "main"
		checkCmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "main")
		checkCmd.Dir = wt.workDir
		if err := checkCmd.Run(); err != nil {
			baseBranch = "master"
		}
	}
	baseCmd := exec.CommandContext(ctx, "git", "rev-parse", baseBranch)
	baseCmd.Dir = wt.workDir
	if baseOut, err := baseCmd.Output(); err == nil {
		update.BaseCommit = strings.TrimSpace(string(baseOut))
	}

	return nil
}

// getAheadBehindCounts populates the Ahead/Behind fields relative to the tracking or
// origin branch. Falls back to origin/main then origin/master when no upstream is set.
func (wt *WorkspaceTracker) getAheadBehindCounts(ctx context.Context, update *types.GitStatusUpdate) {
	// Use remote branch if available, otherwise fall back to origin/main or origin/master
	// This handles worktree branches that don't have an upstream tracking branch set
	compareRef := update.RemoteBranch
	if compareRef == "" {
		// Try origin/main first, then origin/master
		checkCmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "origin/main")
		checkCmd.Dir = wt.workDir
		if err := checkCmd.Run(); err == nil {
			compareRef = "origin/main"
		} else {
			checkCmd2 := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "origin/master")
			checkCmd2.Dir = wt.workDir
			if err := checkCmd2.Run(); err == nil {
				compareRef = "origin/master"
			}
		}
	}
	if compareRef == "" {
		return
	}
	countCmd := exec.CommandContext(ctx, "git", "rev-list", "--left-right", "--count", update.Branch+"..."+compareRef)
	countCmd.Dir = wt.workDir
	if countOut, err := countCmd.Output(); err == nil {
		parts := strings.Fields(string(countOut))
		if len(parts) == 2 {
			update.Ahead, _ = strconv.Atoi(parts[0])
			update.Behind, _ = strconv.Atoi(parts[1])
		}
	}
}

// parseGitStatusOutput runs git status --porcelain and populates the file lists and map.
func (wt *WorkspaceTracker) parseGitStatusOutput(ctx context.Context, update *types.GitStatusUpdate) error {
	// --untracked-files=all shows all files in untracked directories, not just the directory name
	statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain", "--untracked-files=all")
	statusCmd.Dir = wt.workDir
	statusOut, err := statusCmd.Output()
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

// applyPorcelainLine parses a single git status --porcelain line and updates the status update.
func (wt *WorkspaceTracker) applyPorcelainLine(line string, update *types.GitStatusUpdate) {
	indexStatus := line[0]    // Staged status (X)
	workTreeStatus := line[1] // Unstaged status (Y)
	filePath := strings.TrimSpace(line[3:])

	fileInfo := types.FileInfo{Path: filePath}

	// Determine staged status based on index and worktree status.
	// Prioritize worktree changes as they represent the current state.
	switch {
	case indexStatus == '?' && workTreeStatus == '?':
		fileInfo.Status = "untracked"
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
		// Renamed files have format "old -> new"
		if idx := strings.Index(filePath, " -> "); idx != -1 {
			fileInfo.OldPath = filePath[:idx]
			fileInfo.Path = filePath[idx+4:]
		}
		update.Renamed = append(update.Renamed, filePath)
	}

	update.Files[filePath] = fileInfo
}
