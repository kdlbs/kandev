package process

import (
	"context"
	"fmt"
	"strings"
)

// GitLogResult represents the result of a git log operation.
type GitLogResult struct {
	Success bool             `json:"success"`
	Commits []*GitCommitInfo `json:"commits"`
	Error   string           `json:"error,omitempty"`
}

// GitCommitInfo represents a single commit in the log.
// Field names match GitCommitData and SessionCommit for frontend consistency.
type GitCommitInfo struct {
	CommitSHA     string `json:"commit_sha"`
	ParentSHA     string `json:"parent_sha"`
	CommitMessage string `json:"commit_message"`
	AuthorName    string `json:"author_name"`
	AuthorEmail   string `json:"author_email"`
	CommittedAt   string `json:"committed_at"`
	FilesChanged  int    `json:"files_changed"`
	Insertions    int    `json:"insertions"`
	Deletions     int    `json:"deletions"`
}

// CumulativeDiffResult represents the cumulative diff from base commit to HEAD.
type CumulativeDiffResult struct {
	Success      bool                   `json:"success"`
	BaseCommit   string                 `json:"base_commit"`
	HeadCommit   string                 `json:"head_commit"`
	TotalCommits int                    `json:"total_commits"`
	Files        map[string]interface{} `json:"files"`
	Error        string                 `json:"error,omitempty"`
}

// Field and record separators for git log parsing.
// Using non-printable ASCII separators to avoid collision with commit message content.
const (
	fieldSep  = "\x1f" // Unit Separator (ASCII 31)
	recordSep = "\x1e" // Record Separator (ASCII 30)
)

// GetLog returns commits from baseCommit (exclusive) to HEAD (inclusive).
// If baseCommit is empty, returns recent commits (limited by limit parameter).
// Stats (files changed, insertions, deletions) are fetched in-band via --shortstat
// to avoid an N+1 git-show call per commit.
func (g *GitOperator) GetLog(ctx context.Context, baseCommit string, limit int) (*GitLogResult, error) {
	result := &GitLogResult{
		Commits: make([]*GitCommitInfo, 0),
	}

	// Build the git log command with non-printable separators and --shortstat.
	// The record separator (%x1e) is placed BEFORE the fields so that --shortstat
	// output (appended after the format) stays within the same record:
	//   \x1e<fields>\n <stat summary>\n\x1e<fields>\n <stat summary>\n...
	// Splitting on recordSep groups each commit's fields + stats together.
	args := []string{"log", "--format=%x1e%H%x1f%P%x1f%an%x1f%ae%x1f%s%x1f%aI", "--shortstat"}

	switch {
	case baseCommit != "":
		args = append(args, baseCommit+"..HEAD")
	case limit > 0:
		args = append(args, fmt.Sprintf("-n%d", limit))
	default:
		// Default to last 50 commits if no base and no limit
		args = append(args, "-n50")
	}

	output, err := g.runGitCommand(ctx, args...)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get git log: %s", err.Error())
		return result, nil
	}

	output = strings.TrimSpace(output)
	if output == "" {
		result.Success = true
		return result, nil
	}

	// Split by record separator. The first element is empty (separator is at the start).
	records := strings.Split(output, recordSep)
	for _, record := range records {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}

		// The record may contain a trailing shortstat line after the commit fields.
		// Split on newline: first line has the fields, remaining lines may have the stat summary.
		lines := strings.SplitN(record, "\n", 2)
		fieldLine := strings.TrimSpace(lines[0])

		parts := strings.Split(fieldLine, fieldSep)
		if len(parts) < 6 {
			continue
		}

		sha := parts[0]
		parentSHA := parts[1]
		if idx := strings.Index(parentSHA, " "); idx > 0 {
			parentSHA = parentSHA[:idx]
		}

		// Parse inline shortstat if present
		var filesChanged, insertions, deletions int
		if len(lines) > 1 {
			statLine := strings.TrimSpace(lines[1])
			if statLine != "" {
				filesChanged, insertions, deletions = parseStatSummary(statLine)
			}
		}

		result.Commits = append(result.Commits, &GitCommitInfo{
			CommitSHA:     sha,
			ParentSHA:     parentSHA,
			AuthorName:    parts[2],
			AuthorEmail:   parts[3],
			CommitMessage: parts[4],
			CommittedAt:   parts[5],
			FilesChanged:  filesChanged,
			Insertions:    insertions,
			Deletions:     deletions,
		})
	}

	result.Success = true
	return result, nil
}

// GetCumulativeDiff returns the cumulative diff from baseCommit to the working tree
// (including uncommitted/unstaged changes).
func (g *GitOperator) GetCumulativeDiff(ctx context.Context, baseCommit string) (*CumulativeDiffResult, error) {
	result := &CumulativeDiffResult{
		Files: make(map[string]interface{}),
	}

	if baseCommit == "" {
		result.Error = "base_commit is required"
		return result, nil
	}

	result.BaseCommit = baseCommit

	// Get current HEAD
	headOutput, err := g.runGitCommand(ctx, "rev-parse", "HEAD")
	if err != nil {
		result.Error = fmt.Sprintf("failed to get HEAD: %s", err.Error())
		return result, nil
	}
	result.HeadCommit = strings.TrimSpace(headOutput)

	// Count commits between base and HEAD
	countOutput, err := g.runGitCommand(ctx, "rev-list", "--count", baseCommit+"..HEAD")
	if err == nil {
		_, _ = fmt.Sscanf(strings.TrimSpace(countOutput), "%d", &result.TotalCommits)
	}

	// Get the cumulative diff (base → working tree, includes uncommitted changes)
	diffOutput, err := g.runGitCommand(ctx, "diff", baseCommit)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get diff: %s", err.Error())
		return result, nil
	}

	// Parse the diff output into files
	result.Files = g.parseCommitDiff(diffOutput)

	result.Success = true
	return result, nil
}

// CommitDiffResult represents the result of getting a commit's diff.
type CommitDiffResult struct {
	Success      bool                   `json:"success"`
	CommitSHA    string                 `json:"commit_sha"`
	Message      string                 `json:"message"`
	Author       string                 `json:"author"`
	Date         string                 `json:"date"`
	Files        map[string]interface{} `json:"files"` // FileInfo objects with diff content
	FilesChanged int                    `json:"files_changed"`
	Insertions   int                    `json:"insertions"`
	Deletions    int                    `json:"deletions"`
	Error        string                 `json:"error,omitempty"`
}

// isHexChar reports whether r is a valid hexadecimal digit (0-9, a-f, A-F).
func isHexChar(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

// validateCommitSHA validates a commit SHA.
// Returns an error message if invalid, empty string if valid.
func validateCommitSHA(sha string) string {
	if sha == "" {
		return "commit SHA is required"
	}

	// Must be between 4 and 40 hex characters
	if len(sha) < 4 || len(sha) > 40 {
		return "commit SHA must be between 4 and 40 characters"
	}

	for _, r := range sha {
		if !isHexChar(r) {
			return "commit SHA contains invalid characters"
		}
	}
	return ""
}

// sumFileDiffStats sums additions and deletions from a file diff map.
func sumFileDiffStats(files map[string]interface{}) (insertions, deletions int) {
	for _, f := range files {
		fi, ok := f.(map[string]interface{})
		if !ok {
			continue
		}
		if additions, ok := fi["additions"].(int); ok {
			insertions += additions
		}
		if dels, ok := fi["deletions"].(int); ok {
			deletions += dels
		}
	}
	return insertions, deletions
}

// ShowCommit gets the diff for a specific commit using git show.
func (g *GitOperator) ShowCommit(ctx context.Context, commitSHA string) (*CommitDiffResult, error) {
	result := &CommitDiffResult{
		CommitSHA: commitSHA,
	}

	// Validate commit SHA (basic validation - alphanumeric only)
	if errMsg := validateCommitSHA(commitSHA); errMsg != "" {
		result.Error = errMsg
		return result, nil
	}

	// Get commit metadata
	formatOutput, err := g.runGitCommand(ctx, "show", "--no-patch", "--format=%H%n%s%n%an <%ae>%n%aI", commitSHA)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get commit info: %s", err.Error())
		return result, nil
	}

	lines := strings.Split(strings.TrimSpace(formatOutput), "\n")
	if len(lines) >= 4 {
		result.CommitSHA = lines[0]
		result.Message = lines[1]
		result.Author = lines[2]
		result.Date = lines[3]
	}

	// Get the diff with stats
	diffOutput, err := g.runGitCommand(ctx, "show", "--format=", "--stat", "--numstat", "-p", commitSHA)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get commit diff: %s", err.Error())
		return result, nil
	}

	// Parse the diff output into files
	result.Files = g.parseCommitDiff(diffOutput)
	result.FilesChanged = len(result.Files)
	result.Insertions, result.Deletions = sumFileDiffStats(result.Files)

	result.Success = true
	return result, nil
}

// parseCommitDiff parses git show output into file info map
func (g *GitOperator) parseCommitDiff(output string) map[string]interface{} {
	files := make(map[string]interface{})

	// Split by "diff --git" to get individual file diffs
	parts := strings.Split(output, "diff --git ")
	if len(parts) <= 1 {
		return files
	}

	for _, part := range parts[1:] {
		if part == "" {
			continue
		}

		// Re-add the "diff --git " prefix
		diffContent := "diff --git " + part

		// Extract file path from the diff header
		// Format: diff --git a/path/to/file b/path/to/file
		lines := strings.SplitN(diffContent, "\n", 2)
		if len(lines) == 0 {
			continue
		}

		header := lines[0]
		// Extract path from "diff --git a/<path> b/<path>".
		// We cannot split by space because paths may contain spaces.
		// Instead, strip the known prefix and find the " b/" separator.
		pathsPart := strings.TrimPrefix(header, "diff --git ")
		bIdx := strings.Index(pathsPart, " b/")
		if bIdx == -1 {
			continue
		}
		filePath := pathsPart[bIdx+3:]

		// Determine file status from diff content
		status := fileStatusModified
		switch {
		case strings.Contains(diffContent, "new file mode"):
			status = "added"
		case strings.Contains(diffContent, "deleted file mode"):
			status = fileStatusDeleted
		case strings.Contains(diffContent, "rename from"):
			status = "renamed"
		}

		// Count additions and deletions
		additions := 0
		deletions := 0
		for _, line := range strings.Split(diffContent, "\n") {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				additions++
			} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
				deletions++
			}
		}

		files[filePath] = map[string]interface{}{
			"status":    status,
			"staged":    false,
			"additions": additions,
			"deletions": deletions,
			"diff":      diffContent,
		}
	}

	return files
}

// GetMergeBase returns the merge-base commit SHA between two refs (e.g., HEAD and origin/main).
// This is used to determine the common ancestor for filtering commits.
func (g *GitOperator) GetMergeBase(ctx context.Context, ref1, ref2 string) (string, error) {
	output, err := g.runGitCommand(ctx, "merge-base", ref1, ref2)
	if err != nil {
		return "", fmt.Errorf("failed to compute merge-base: %w", err)
	}
	return strings.TrimSpace(output), nil
}
