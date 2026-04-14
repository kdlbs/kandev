package process

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

const (
	// maxUntrackedFileSize is the maximum size of an untracked file to read for
	// diff generation. Files larger than this are skipped to prevent excessive
	// memory usage (the workspace tracker polls every few seconds).
	maxUntrackedFileSize = 10 << 20 // 10 MB

	// binaryDetectBytes is the number of bytes read from the beginning of a file
	// to detect whether it is binary (contains null bytes).
	binaryDetectBytes = 8192
)

// enrichWithDiffData adds diff information (additions, deletions, diff content) to file info
func (wt *WorkspaceTracker) enrichWithDiffData(ctx context.Context, update *types.GitStatusUpdate) {
	// Always diff against HEAD for unstaged/staged content so that files committed
	// locally (but not yet pushed) show only their uncommitted changes rather than
	// the entire file as new. The remote branch is only relevant for ahead/behind counts.
	wt.enrichWithUnstagedDiff(ctx, update, "HEAD")
	wt.enrichWithStagedDiff(ctx, update, "HEAD")
	wt.enrichUntrackedFileDiffs(ctx, update)
}

// enrichWithBranchDiff computes the total additions/deletions for the entire branch
// vs the merge-base, covering committed + staged + unstaged changes in one pass.
// Untracked file line counts (already computed) are added on top.
// The result is stored in BranchAdditions/BranchDeletions for the sidebar display.
func (wt *WorkspaceTracker) enrichWithBranchDiff(ctx context.Context, update *types.GitStatusUpdate) {
	if update.BaseCommit == "" {
		return
	}

	// git diff --numstat <merge-base> covers committed + staged + unstaged changes.
	numstatCmd := exec.CommandContext(ctx, "git", "diff", "--numstat", update.BaseCommit)
	numstatCmd.Dir = wt.workDir
	numstatOut, err := numstatCmd.Output()
	if err != nil {
		wt.logger.Debug("enrichWithBranchDiff: numstat failed", zap.Error(err))
		return
	}

	var additions, deletions int
	for _, line := range strings.Split(string(numstatOut), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		a, _ := strconv.Atoi(parts[0])
		d, _ := strconv.Atoi(parts[1])
		additions += a
		deletions += d
	}

	// Add untracked file line counts (not included in git diff output).
	for _, fileInfo := range update.Files {
		if fileInfo.Status == fileStatusUntracked {
			additions += fileInfo.Additions
		}
	}

	update.BranchAdditions = additions
	update.BranchDeletions = deletions
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
		// numstat uses tab-separated values: <added>\t<deleted>\t<path>
		// Split by tab (not whitespace) to preserve spaces in file paths.
		parts := strings.SplitN(line, "\t", 3)
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
		// numstat uses tab-separated values: <added>\t<deleted>\t<path>
		parts := strings.SplitN(line, "\t", 3)
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
//
// Safety guards to prevent OOM (the workspace tracker polls every few seconds):
//   - Files larger than maxUntrackedFileSize are skipped.
//   - Binary files (containing null bytes) are skipped.
func (wt *WorkspaceTracker) enrichUntrackedFileDiffs(_ context.Context, update *types.GitStatusUpdate) {
	for filePath, fileInfo := range update.Files {
		if fileInfo.Status != fileStatusUntracked {
			continue
		}

		absPath := filepath.Join(wt.workDir, filePath)

		// Check file size before reading.
		info, err := os.Stat(absPath)
		if err != nil {
			continue
		}
		if info.Size() > maxUntrackedFileSize {
			wt.logger.Debug("skipping large untracked file for diff",
				zap.String("path", filePath),
				zap.Int64("size_bytes", info.Size()))
			fileInfo.DiffSkipReason = "too_large"
			update.Files[filePath] = fileInfo
			continue
		}

		content, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}

		// Skip binary files (contain null bytes in the first few KB).
		detectLen := len(content)
		if detectLen > binaryDetectBytes {
			detectLen = binaryDetectBytes
		}
		if bytes.ContainsRune(content[:detectLen], 0) {
			wt.logger.Debug("skipping binary untracked file for diff",
				zap.String("path", filePath))
			fileInfo.DiffSkipReason = "binary"
			update.Files[filePath] = fileInfo
			continue
		}

		lines := strings.Split(string(content), "\n")
		fileInfo.Additions = len(lines)
		fileInfo.Deletions = 0

		// Format as a proper git diff with all required headers.
		// The @git-diff-view/react library requires the full git diff format.
		var diffBuilder strings.Builder
		// Pre-grow to avoid repeated allocations.
		diffBuilder.Grow(len(content) + len(lines) + 256)
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
