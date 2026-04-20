package process

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

const (
	// maxDiffFileSize is the maximum file size for which we generate diffs.
	// Files larger than this are skipped with DiffSkipReason "too_large".
	maxDiffFileSize = 10 * 1024 * 1024 // 10 MB

	// maxDiffOutputSize is the maximum diff output size per file.
	// Diffs exceeding this are truncated with DiffSkipReason "truncated".
	maxDiffOutputSize = 256 * 1024 // 256 KB

	// maxTotalDiffBytes is the cumulative diff budget per GitStatusUpdate.
	// Once exceeded, remaining files are skipped with DiffSkipReason "budget_exceeded".
	maxTotalDiffBytes = 2 * 1024 * 1024 // 2 MB

	// binaryCheckSize is how many bytes to inspect for null bytes to detect binary files.
	binaryCheckSize = 8 * 1024 // 8 KB

	diffSkipReasonTooLarge       = "too_large"
	diffSkipReasonBinary         = "binary"
	diffSkipReasonTruncated      = "truncated"
	diffSkipReasonBudgetExceeded = "budget_exceeded"
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

// totalDiffBytes returns the cumulative size of all diff content in the update.
func totalDiffBytes(update *types.GitStatusUpdate) int64 {
	var total int64
	for _, fi := range update.Files {
		total += int64(len(fi.Diff))
	}
	return total
}

// capDiffOutput runs a git diff command and returns at most maxDiffOutputSize bytes.
// Returns the output string and whether it was truncated.
func capDiffOutput(ctx context.Context, workDir string, args ...string) (string, bool) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", false
	}
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		return "", false
	}

	limited := io.LimitReader(stdout, maxDiffOutputSize+1)
	data, _ := io.ReadAll(limited)
	truncated := len(data) > maxDiffOutputSize
	if truncated {
		data = data[:maxDiffOutputSize]
	}

	// Drain remaining stdout so the process doesn't hang on a full pipe.
	_, _ = io.Copy(io.Discard, stdout)
	_ = cmd.Wait()

	return string(data), truncated
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

		if totalDiffBytes(update) >= maxTotalDiffBytes {
			fileInfo.DiffSkipReason = diffSkipReasonBudgetExceeded
			update.Files[filePath] = fileInfo
			continue
		}

		diffOut, truncated := capDiffOutput(ctx, wt.workDir, "diff", baseRef, "--", filePath)
		if diffOut != "" {
			fileInfo.Diff = diffOut
			if truncated {
				fileInfo.DiffSkipReason = diffSkipReasonTruncated
			}
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
			if totalDiffBytes(update) >= maxTotalDiffBytes {
				fileInfo.DiffSkipReason = diffSkipReasonBudgetExceeded
				update.Files[filePath] = fileInfo
				continue
			}

			diffOut, truncated := capDiffOutput(ctx, wt.workDir, "diff", "--cached", baseRef, "--", filePath)
			if diffOut != "" {
				fileInfo.Diff = diffOut
				if truncated {
					fileInfo.DiffSkipReason = diffSkipReasonTruncated
				}
			}
		}
		update.Files[filePath] = fileInfo
	}
}

// isBinaryContent checks for null bytes in the data, same heuristic git uses.
func isBinaryContent(data []byte) bool {
	return bytes.IndexByte(data, 0) != -1
}

// enrichUntrackedFileDiffs builds a synthetic git diff for untracked files showing all
// lines as additions, so the diff viewer can display their full content.
func (wt *WorkspaceTracker) enrichUntrackedFileDiffs(ctx context.Context, update *types.GitStatusUpdate) {
	for filePath, fileInfo := range update.Files {
		if fileInfo.Status != fileStatusUntracked {
			continue
		}

		if totalDiffBytes(update) >= maxTotalDiffBytes {
			fileInfo.DiffSkipReason = diffSkipReasonBudgetExceeded
			update.Files[filePath] = fileInfo
			continue
		}

		safePath, err := wt.sanitizePath(filePath)
		if err != nil {
			continue
		}

		info, err := os.Stat(filepath.Clean(safePath))
		if err != nil {
			continue
		}
		if info.Size() > maxDiffFileSize {
			fileInfo.DiffSkipReason = diffSkipReasonTooLarge
			update.Files[filePath] = fileInfo
			continue
		}

		f, err := os.Open(filepath.Clean(safePath))
		if err != nil {
			continue
		}

		// Read first chunk to check for binary content.
		header := make([]byte, binaryCheckSize)
		n, _ := f.Read(header)
		if n > 0 && isBinaryContent(header[:n]) {
			_ = f.Close()
			fileInfo.DiffSkipReason = diffSkipReasonBinary
			update.Files[filePath] = fileInfo
			continue
		}

		// Read only enough content to fill maxDiffOutputSize of diff output.
		// No need to read the full file — any diff beyond maxDiffOutputSize gets truncated anyway.
		var buf bytes.Buffer
		buf.Write(header[:n])
		remaining := int64(maxDiffOutputSize) - int64(n)
		if remaining > 0 {
			_, _ = io.Copy(&buf, io.LimitReader(f, remaining))
		}
		_ = f.Close()

		content := buf.String()
		lines := strings.Split(content, "\n")
		// Trim trailing empty element from final newline so line count is accurate.
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		fileInfo.Additions = len(lines)
		fileInfo.Deletions = 0

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

		diffContent := diffBuilder.String()
		if len(diffContent) > maxDiffOutputSize {
			diffContent = diffContent[:maxDiffOutputSize]
			fileInfo.DiffSkipReason = diffSkipReasonTruncated
		}

		fileInfo.Diff = diffContent
		update.Files[filePath] = fileInfo
	}
}
