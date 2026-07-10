package process

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/subproc"
	"go.uber.org/zap"
)

// sha1HexPattern matches a full-length git SHA-1 (40 lowercase or
// uppercase hex characters). Used inline at the few sinks that consume
// `update.BaseCommit` so CodeQL's `go/command-injection` taint tracker
// sees the allowlist barrier co-located with the subprocess call — the
// taint flowed in through the merge-base/rev-parse call upstream whose
// argument list contained a (sanitised) user-controlled branch ref, and
// the analyser otherwise treats that command's output as still tainted.
var sha1HexPattern = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

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

// enrichWithDiffData adds diff information (additions, deletions, diff content)
// to file info. The prior status is threaded through so per-file diff data can
// be carried forward when a git command times out — without it a single bad
// poll would wipe the diffs panel for files that genuinely still have changes.
//
// Carry-forward runs once at the end, after every enrichment phase has had a
// chance to populate fresh data. Running it inside enrichWithUnstagedDiff would
// pre-fill fi.Diff for staged-only files, and enrichWithStagedDiff's
// `if fileInfo.Diff == ""` guard would then skip the fresh staged diff fetch.
func (wt *WorkspaceTracker) enrichWithDiffData(ctx context.Context, update *types.GitStatusUpdate, prior types.GitStatusUpdate) {
	// Always diff against HEAD for unstaged/staged content so that files committed
	// locally (but not yet pushed) show only their uncommitted changes rather than
	// the entire file as new. The remote branch is only relevant for ahead/behind counts.
	wt.enrichWithUnstagedDiff(ctx, update, "HEAD", prior)
	wt.enrichWithStagedDiff(ctx, update, "HEAD", prior)
	wt.enrichUntrackedFileDiffs(ctx, update)
	carryForwardFileDiffs(update, prior)
}

// enrichWithBranchDiff computes the total additions/deletions for the entire branch
// vs the merge-base, covering committed + staged + unstaged changes in one pass.
// Untracked file line counts (already computed) are added on top.
// The result is stored in BranchAdditions/BranchDeletions for the sidebar display.
//
// On numstat failure the totals are carried forward from prior when HEAD
// hasn't moved, mirroring the per-command carry-forward used elsewhere in
// getGitStatus to avoid a transient git timeout clearing the sidebar count.
func (wt *WorkspaceTracker) enrichWithBranchDiff(ctx context.Context, update *types.GitStatusUpdate, prior types.GitStatusUpdate) {
	if update.BaseCommit == "" {
		return
	}

	// BaseCommit was produced by an earlier `git merge-base / rev-parse`
	// call whose argument list contained a user-controlled branch ref —
	// CodeQL's `go/command-injection` taint tracker treats the output of
	// such a command as still tainted. Inline a SHA-shape allowlist check
	// here so the sanitiser barrier sits in the same function as the
	// downstream `wt.runGitOutput` call and the value flowing into the
	// next `git` invocation is provably a hex SHA, not user input.
	if !sha1HexPattern.MatchString(update.BaseCommit) {
		return
	}

	// git diff --numstat <merge-base> covers committed + staged + unstaged changes.
	numstatOut, err := wt.runGitOutput(ctx, "diff", "--numstat", update.BaseCommit)
	if err != nil {
		wt.logger.Debug("enrichWithBranchDiff: numstat failed, carrying forward", zap.Error(err))
		carryBranchDiff(update, prior)
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

// carryBranchDiff copies prior.BranchAdditions/BranchDeletions onto update when
// neither HEAD nor BaseCommit has moved. A new HEAD or a re-pointed merge-base
// invalidates the cached totals — both are inputs to `git diff --numstat
// <merge-base>`, so a change in either makes the prior count stale.
func carryBranchDiff(update *types.GitStatusUpdate, prior types.GitStatusUpdate) {
	if prior.HeadCommit == "" || prior.HeadCommit != update.HeadCommit {
		return
	}
	if prior.BaseCommit != update.BaseCommit {
		return
	}
	update.BranchAdditions = prior.BranchAdditions
	update.BranchDeletions = prior.BranchDeletions
}

// carryForwardFileDiffs copies per-file diff content, additions, deletions, and
// skip reason from prior.Files into update.Files for files still present in
// update.Files. Used when a `git diff --numstat` call (the gate that drives
// per-file diff enrichment) fails: the porcelain status already reported which
// files changed; we just can't compute fresh diffs this poll, so we keep the
// last known diff data visible until the next successful poll. HEAD is
// required to be unchanged so we don't show stale diffs after a reset/commit.
func carryForwardFileDiffs(update *types.GitStatusUpdate, prior types.GitStatusUpdate) {
	if prior.HeadCommit == "" || prior.HeadCommit != update.HeadCommit {
		return
	}
	for path, fi := range update.Files {
		// A non-empty Diff means an earlier enrichment pass already populated
		// this entry (e.g. unstaged numstat succeeded, then staged numstat
		// failed). Leave the freshly-computed entry alone — only fill in
		// completely-missing diff data.
		if fi.Diff != "" {
			continue
		}
		// A skip reason set this poll (budget_exceeded, too_large, binary)
		// is an intentional decision to not surface diff content; restoring
		// a prior diff would defeat that skip and could show stale content
		// for a file that has since grown past the size or budget cap.
		if fi.DiffSkipReason != "" {
			continue
		}
		priorFi, ok := prior.Files[path]
		if !ok || priorFi.Diff == "" {
			continue
		}
		fi.Diff = priorFi.Diff
		fi.DiffSkipReason = priorFi.DiffSkipReason
		if fi.Additions == 0 {
			fi.Additions = priorFi.Additions
		}
		if fi.Deletions == 0 {
			fi.Deletions = priorFi.Deletions
		}
		update.Files[path] = fi
	}
}

// carryForwardFileDiff fills fi.Diff/DiffSkipReason from prior for a single
// file when HEAD matches and prior had diff content. Used when numstat
// succeeded (so Additions/Deletions are fresh) but the per-file capDiffOutput
// call returned empty — typically a 10s gitCommandTimeout on a large diff,
// throttle starvation, or a broken pipe. Without this, a single slow file
// would blank the diff content in the UI even though we have a usable cached
// copy. Returns the (possibly updated) FileInfo so callers can re-store it.
func carryForwardFileDiff(fi types.FileInfo, filePath string, update *types.GitStatusUpdate, prior types.GitStatusUpdate) types.FileInfo {
	if prior.HeadCommit == "" || prior.HeadCommit != update.HeadCommit {
		return fi
	}
	priorFi, ok := prior.Files[filePath]
	if !ok || priorFi.Diff == "" {
		return fi
	}
	fi.Diff = priorFi.Diff
	fi.DiffSkipReason = priorFi.DiffSkipReason
	return fi
}

// capDiffOutput runs a git diff command and returns at most maxDiffOutputSize bytes.
// Returns the output string and whether it was truncated.
//
// Holds a git throttle slot for the full Start → Wait lifetime so streaming
// diffs count against the same global cap as Output/CombinedOutput callers
// — otherwise the throttle could be silently bypassed by switching to
// pipe-based reads. Slot is acquired before Start; if Start fails we
// release immediately, else release runs after Wait.
func capDiffOutput(ctx context.Context, workDir string, args ...string) (string, bool) {
	cctx, cancel := context.WithTimeout(ctx, gitCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "git", args...)
	cmd.Dir = workDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", false
	}
	release, err := subproc.Git().Acquire(cctx)
	if err != nil {
		_ = stdout.Close()
		return "", false
	}
	// release() is idempotent (sync.Once inside the throttle), so a defer
	// covers every return path — including ones added later between Start
	// and the explicit end-of-function release.
	defer release()
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

// resolveNumstatPath resolves a numstat path that may contain rename notation
// (e.g. "old.txt => new.txt" or "{old => new}/file.txt") to the new file path.
// For non-rename paths, returns the input unchanged.
func resolveNumstatPath(numstatPath string) string {
	arrowIdx := strings.Index(numstatPath, " => ")
	if arrowIdx == -1 {
		return numstatPath
	}

	// Check for brace-style rename: {old => new}/suffix or prefix/{old => new}/suffix
	braceOpen := strings.LastIndex(numstatPath[:arrowIdx], "{")
	braceClose := strings.Index(numstatPath[arrowIdx:], "}")
	if braceOpen != -1 && braceClose != -1 {
		prefix := numstatPath[:braceOpen]
		newPart := numstatPath[arrowIdx+4 : arrowIdx+braceClose]
		suffix := numstatPath[arrowIdx+braceClose+1:]
		return prefix + newPart + suffix
	}

	// Simple rename: "old.txt => new.txt"
	return numstatPath[arrowIdx+4:]
}

// enrichWithUnstagedDiff populates additions/deletions and diff content for files
// with unstaged changes by comparing the worktree against baseRef. On numstat
// failure the function returns early and leaves the carry-forward to
// enrichWithDiffData — preventing a single git timeout from blanking the diffs
// panel for files whose porcelain status still shows them as changed, while
// keeping the staged phase free to populate fresh data for staged-only files.
func (wt *WorkspaceTracker) enrichWithUnstagedDiff(ctx context.Context, update *types.GitStatusUpdate, baseRef string, prior types.GitStatusUpdate) {
	numstatOut, err := wt.runGitOutput(ctx, "diff", "--numstat", baseRef)
	if err != nil {
		// Carry-forward happens once at the end of enrichWithDiffData so the
		// staged phase can still populate fresh diffs for staged-only files.
		wt.logger.Debug("enrichWithUnstagedDiff: numstat failed", zap.Error(err))
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
		numstatPath := parts[2]

		// Resolve rename notation (e.g. "old => new") to the new path,
		// which is the key used in the Files map.
		filePath := resolveNumstatPath(numstatPath)

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
		} else {
			fileInfo = carryForwardFileDiff(fileInfo, filePath, update, prior)
		}

		update.Files[filePath] = fileInfo
	}
}

// enrichWithStagedDiff populates additions/deletions and diff content for staged files
// that have no additional unstaged changes, using git diff --cached. On --cached
// numstat failure the function returns early; carry-forward happens once at the
// end of enrichWithDiffData (same rationale as enrichWithUnstagedDiff).
func (wt *WorkspaceTracker) enrichWithStagedDiff(ctx context.Context, update *types.GitStatusUpdate, baseRef string, prior types.GitStatusUpdate) {
	// For staged files that don't have unstaged changes, we need to get the diff from the index.
	// The first diff (git diff baseRef) shows worktree vs baseRef, but if a file is staged
	// and has no additional unstaged changes, its diff won't appear there.
	stagedOut, err := wt.runGitOutput(ctx, "diff", "--cached", "--numstat", baseRef)
	if err != nil {
		// Carry-forward happens at the end of enrichWithDiffData; running it
		// here would mask the unstaged phase's fresh data.
		wt.logger.Debug("enrichWithStagedDiff: --cached numstat failed", zap.Error(err))
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
		numstatPath := parts[2]

		// Resolve rename notation (e.g. "old => new") to the new path,
		// which is the key used in the Files map.
		filePath := resolveNumstatPath(numstatPath)

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
			} else {
				fileInfo = carryForwardFileDiff(fileInfo, filePath, update, prior)
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
