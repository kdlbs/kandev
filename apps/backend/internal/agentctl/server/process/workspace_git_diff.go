package process

import (
	"context"
	"os/exec"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

// enrichWithDiffData adds diff information (additions, deletions, diff content) to file info
func (wt *WorkspaceTracker) enrichWithDiffData(ctx context.Context, update *types.GitStatusUpdate) {
	// Use remote branch if available, otherwise use HEAD
	baseRef := "HEAD"
	if update.RemoteBranch != "" {
		baseRef = update.RemoteBranch
	}

	wt.enrichWithUnstagedDiff(ctx, update, baseRef)
	wt.enrichWithStagedDiff(ctx, update, baseRef)
	wt.enrichUntrackedFileDiffs(ctx, update)
}

// enrichWithUnstagedDiff populates additions/deletions and diff content for files
// with unstaged changes by comparing the worktree against baseRef.
func (wt *WorkspaceTracker) enrichWithUnstagedDiff(ctx context.Context, update *types.GitStatusUpdate, baseRef string) {
	numstatCmd := exec.CommandContext(ctx, "git", "diff", "--numstat", baseRef)
	numstatCmd.Dir = wt.workDir
	numstatOut, err := numstatCmd.Output()
	if err != nil {
		wt.logger.Debug("failed to get numstat", zap.Error(err))
		return
	}

	lines := strings.Split(string(numstatOut), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		additions, _ := strconv.Atoi(parts[0])
		deletions, _ := strconv.Atoi(parts[1])
		filePath := parts[2]

		// Only update file info if it exists in status (uncommitted changes).
		// Files that appear in diff but not in status are committed changes - we don't
		// add them to the Files map as that would make git status show already-committed files.
		fileInfo, exists := update.Files[filePath]
		if !exists {
			continue
		}
		fileInfo.Additions = additions
		fileInfo.Deletions = deletions

		// Get the actual diff content for this file (compare against base branch)
		diffCmd := exec.CommandContext(ctx, "git", "diff", baseRef, "--", filePath)
		diffCmd.Dir = wt.workDir
		if diffOut, err := diffCmd.Output(); err == nil {
			fileInfo.Diff = string(diffOut)
		}

		update.Files[filePath] = fileInfo
	}
}

// enrichWithStagedDiff populates additions/deletions and diff content for staged files
// that have no additional unstaged changes, using git diff --cached.
func (wt *WorkspaceTracker) enrichWithStagedDiff(ctx context.Context, update *types.GitStatusUpdate, baseRef string) {
	// For staged files that don't have unstaged changes, we need to get the diff from the index.
	// The first diff (git diff baseRef) shows worktree vs baseRef, but if a file is staged
	// and has no additional unstaged changes, its diff won't appear there.
	stagedCmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--numstat", baseRef)
	stagedCmd.Dir = wt.workDir
	stagedOut, err := stagedCmd.Output()
	if err != nil {
		return
	}

	lines := strings.Split(string(stagedOut), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		additions, _ := strconv.Atoi(parts[0])
		deletions, _ := strconv.Atoi(parts[1])
		filePath := parts[2]

		fileInfo, exists := update.Files[filePath]
		if !exists {
			continue
		}
		// Only set additions/deletions if they weren't already set by the unstaged diff.
		// This prevents double-counting when changes appear in both diffs.
		if fileInfo.Additions == 0 && fileInfo.Deletions == 0 {
			fileInfo.Additions = additions
			fileInfo.Deletions = deletions
		}
		// Get the staged diff content if we don't have diff content yet
		if fileInfo.Diff == "" {
			diffCmd := exec.CommandContext(ctx, "git", "diff", "--cached", baseRef, "--", filePath)
			diffCmd.Dir = wt.workDir
			if diffOut, err := diffCmd.Output(); err == nil {
				fileInfo.Diff = string(diffOut)
			}
		}
		update.Files[filePath] = fileInfo
	}
}

// enrichUntrackedFileDiffs builds a synthetic git diff for untracked files showing all
// lines as additions, so the diff viewer can display their full content.
func (wt *WorkspaceTracker) enrichUntrackedFileDiffs(ctx context.Context, update *types.GitStatusUpdate) {
	for filePath, fileInfo := range update.Files {
		if fileInfo.Status != "untracked" {
			continue
		}
		catCmd := exec.CommandContext(ctx, "cat", filePath)
		catCmd.Dir = wt.workDir
		catOut, err := catCmd.Output()
		if err != nil {
			continue
		}
		content := string(catOut)
		lines := strings.Split(content, "\n")
		fileInfo.Additions = len(lines)
		fileInfo.Deletions = 0

		// Format as a proper git diff with all required headers.
		// The @git-diff-view/react library requires the full git diff format.
		var diffBuilder strings.Builder
		diffBuilder.WriteString("diff --git a/" + filePath + " b/" + filePath + "\n")
		diffBuilder.WriteString("new file mode 100644\n")
		diffBuilder.WriteString("index 0000000..0000000\n")
		diffBuilder.WriteString("--- /dev/null\n")
		diffBuilder.WriteString("+++ b/" + filePath + "\n")
		diffBuilder.WriteString("@@ -0,0 +1," + strconv.Itoa(len(lines)) + " @@\n")
		for _, line := range lines {
			diffBuilder.WriteString("+" + line + "\n")
		}
		fileInfo.Diff = diffBuilder.String()
		update.Files[filePath] = fileInfo
	}
}
