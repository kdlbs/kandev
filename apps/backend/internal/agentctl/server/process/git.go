// Package process provides git operation execution for agentctl.
package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// ErrOperationInProgress is returned when a git operation is already in progress.
var ErrOperationInProgress = errors.New("git operation already in progress")

// ErrInvalidBranchName is returned when a branch name contains invalid characters.
var ErrInvalidBranchName = errors.New("invalid branch name")

// validBranchNameRegex matches safe git branch names.
// Allows alphanumeric, hyphens, underscores, slashes, and dots.
// Disallows: spaces, shell metacharacters, and control characters.
var validBranchNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*$`)

// isValidBranchName validates that a branch name is safe to use in git commands.
func isValidBranchName(branch string) bool {
	if branch == "" || len(branch) > 255 {
		return false
	}
	// Disallow ".." to prevent path traversal
	if strings.Contains(branch, "..") {
		return false
	}
	// Disallow ending with ".lock"
	if strings.HasSuffix(branch, ".lock") {
		return false
	}
	return validBranchNameRegex.MatchString(branch)
}

// GitOperationResult represents the result of a git operation.
type GitOperationResult struct {
	Success       bool     `json:"success"`
	Operation     string   `json:"operation"`
	Output        string   `json:"output"`
	Error         string   `json:"error,omitempty"`
	ConflictFiles []string `json:"conflict_files,omitempty"`
}

// GitOperator executes git operations in a workspace directory.
type GitOperator struct {
	workDir          string
	logger           *logger.Logger
	workspaceTracker *WorkspaceTracker

	mu         sync.Mutex // Prevents concurrent git operations
	inProgress bool
	currentOp  string
}

// NewGitOperator creates a new GitOperator for the given workspace directory.
func NewGitOperator(workDir string, log *logger.Logger, workspaceTracker *WorkspaceTracker) *GitOperator {
	return &GitOperator{
		workDir:          workDir,
		logger:           log.WithFields(zap.String("component", "git-operator")),
		workspaceTracker: workspaceTracker,
	}
}

// runGitCommand executes a git command in the workDir
func (g *GitOperator) runGitCommand(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	g.logger.Debug("executing git command", zap.Strings("args", args))

	err := cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	if err != nil {
		return output, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return output, nil
}

// filterGitEnv removes GIT_DIR and GIT_WORK_TREE from the environment.
// This ensures that external tools like gh CLI correctly detect the repository
// from the working directory, which is essential for worktrees where these
// env vars could point to the wrong location.
func filterGitEnv(env []string) []string {
	result := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, "GIT_DIR=") || strings.HasPrefix(e, "GIT_WORK_TREE=") {
			continue
		}
		result = append(result, e)
	}
	return result
}

// triggerFsNotify creates and removes a temporary file to trigger fsnotify and refresh git status.
// This is OS-agnostic and reliably triggers filesystem events that the workspace tracker watches.
// Called after git operations like commit, push, pull, etc. to refresh the UI.
func (g *GitOperator) triggerFsNotify() {
	sentinelPath := filepath.Join(g.workDir, ".git-op-complete")
	f, err := os.Create(sentinelPath)
	if err != nil {
		g.logger.Debug("failed to create sentinel file", zap.Error(err))
		return
	}
	_ = f.Close()
	if err := os.Remove(sentinelPath); err != nil {
		g.logger.Debug("failed to remove sentinel file", zap.Error(err))
	}
}

// getCurrentBranch returns the current branch name
func (g *GitOperator) getCurrentBranch(ctx context.Context) (string, error) {
	output, err := g.runGitCommand(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// hasUncommittedChanges checks if there are uncommitted changes
func (g *GitOperator) hasUncommittedChanges(ctx context.Context) (bool, error) {
	output, err := g.runGitCommand(ctx, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("failed to check uncommitted changes: %w", err)
	}
	return strings.TrimSpace(output) != "", nil
}

// parseConflictFiles parses conflict file names from git output
func (g *GitOperator) parseConflictFiles(output string) []string {
	var conflicts []string
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for "CONFLICT" markers in git output
		if strings.HasPrefix(line, "CONFLICT") {
			// Extract file name from patterns like:
			// "CONFLICT (content): Merge conflict in <file>"
			// "CONFLICT (add/add): Merge conflict in <file>"
			if idx := strings.Index(line, "Merge conflict in "); idx != -1 {
				file := strings.TrimSpace(line[idx+len("Merge conflict in "):])
				if file != "" {
					conflicts = append(conflicts, file)
				}
			}
		}
	}

	return conflicts
}

// Pull performs a git pull operation.
func (g *GitOperator) Pull(ctx context.Context, rebase bool) (*GitOperationResult, error) {
	if !g.tryLock("pull") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{
		Operation: "pull",
	}

	branch, err := g.getCurrentBranch(ctx)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	var args []string
	if rebase {
		args = []string{"pull", "--rebase", "origin", branch}
	} else {
		args = []string{"pull", "origin", branch}
	}

	output, err := g.runGitCommand(ctx, args...)
	result.Output = output

	if err != nil {
		result.Error = err.Error()
		result.ConflictFiles = g.parseConflictFiles(output)

		// For rebase conflicts, auto-abort to restore clean state
		if rebase && len(result.ConflictFiles) > 0 {
			g.logger.Info("rebase conflict detected, aborting rebase")
			if _, abortErr := g.runGitCommand(ctx, "rebase", "--abort"); abortErr != nil {
				g.logger.Warn("failed to abort rebase", zap.Error(abortErr))
			}
		}
		return result, nil
	}

	result.Success = true
	g.logger.Info("pull completed", zap.String("branch", branch), zap.Bool("rebase", rebase))
	return result, nil
}

// Push performs a git push operation.
func (g *GitOperator) Push(ctx context.Context, force bool, setUpstream bool) (*GitOperationResult, error) {
	if !g.tryLock("push") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{
		Operation: "push",
	}

	branch, err := g.getCurrentBranch(ctx)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	args := []string{"push"}

	if setUpstream {
		args = append(args, "--set-upstream")
	}

	if force {
		// Use --force-with-lease for safer force push
		args = append(args, "--force-with-lease")
	}

	args = append(args, "origin", branch)

	output, err := g.runGitCommand(ctx, args...)
	result.Output = output

	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	result.Success = true
	g.logger.Info("push completed", zap.String("branch", branch), zap.Bool("force", force), zap.Bool("set_upstream", setUpstream))
	return result, nil
}

// Rebase performs a git rebase onto the specified base branch.
func (g *GitOperator) Rebase(ctx context.Context, baseBranch string) (*GitOperationResult, error) {
	// Validate branch name to prevent command injection
	if !isValidBranchName(baseBranch) {
		return nil, ErrInvalidBranchName
	}

	if !g.tryLock("rebase") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{
		Operation: "rebase",
	}

	// Fetch the base branch first
	fetchOutput, err := g.runGitCommand(ctx, "fetch", "origin", baseBranch)
	if err != nil {
		result.Error = fmt.Sprintf("failed to fetch base branch: %s", err.Error())
		result.Output = fetchOutput
		return result, nil
	}

	// Perform the rebase
	rebaseOutput, err := g.runGitCommand(ctx, "rebase", "origin/"+baseBranch)
	result.Output = fetchOutput + rebaseOutput

	if err != nil {
		result.Error = err.Error()
		result.ConflictFiles = g.parseConflictFiles(rebaseOutput)

		// Auto-abort rebase on conflicts to restore clean state
		if len(result.ConflictFiles) > 0 {
			g.logger.Info("rebase conflict detected, aborting rebase")
			if _, abortErr := g.runGitCommand(ctx, "rebase", "--abort"); abortErr != nil {
				g.logger.Warn("failed to abort rebase", zap.Error(abortErr))
			}
		}
		return result, nil
	}

	result.Success = true
	g.logger.Info("rebase completed", zap.String("base_branch", baseBranch))
	return result, nil
}

// Merge performs a git merge of the specified base branch.
func (g *GitOperator) Merge(ctx context.Context, baseBranch string) (*GitOperationResult, error) {
	// Validate branch name to prevent command injection
	if !isValidBranchName(baseBranch) {
		return nil, ErrInvalidBranchName
	}

	if !g.tryLock("merge") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{
		Operation: "merge",
	}

	// Fetch the base branch first
	fetchOutput, err := g.runGitCommand(ctx, "fetch", "origin", baseBranch)
	if err != nil {
		result.Error = fmt.Sprintf("failed to fetch base branch: %s", err.Error())
		result.Output = fetchOutput
		return result, nil
	}

	// Perform the merge
	mergeOutput, err := g.runGitCommand(ctx, "merge", "origin/"+baseBranch)
	result.Output = fetchOutput + mergeOutput

	if err != nil {
		result.Error = err.Error()
		result.ConflictFiles = g.parseConflictFiles(mergeOutput)
		// For merge conflicts, leave in place so user can resolve
		// Do NOT auto-abort like we do for rebase
		return result, nil
	}

	result.Success = true
	g.logger.Info("merge completed", zap.String("base_branch", baseBranch))
	return result, nil
}

// Commit creates a git commit with the specified message.
// If stageAll is true, it stages all changes before committing.
func (g *GitOperator) Commit(ctx context.Context, message string, stageAll bool) (*GitOperationResult, error) {
	if !g.tryLock("commit") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{
		Operation: "commit",
	}

	// Check if there are changes to commit
	hasChanges, err := g.hasUncommittedChanges(ctx)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	if !hasChanges {
		result.Error = "no changes to commit"
		return result, nil
	}

	// Stage all changes if requested
	if stageAll {
		stageOutput, err := g.runGitCommand(ctx, "add", "-A")
		if err != nil {
			result.Error = fmt.Sprintf("failed to stage changes: %s", err.Error())
			result.Output = stageOutput
			return result, nil
		}
		result.Output = stageOutput
	}

	// Create the commit
	commitOutput, err := g.runGitCommand(ctx, "commit", "-m", message)
	result.Output += commitOutput

	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	result.Success = true
	g.logger.Info("commit completed", zap.String("message", message))

	// Publish commit notification if we have a workspace tracker
	if g.workspaceTracker != nil {
		// Get commit details
		commitSHA, _ := g.runGitCommand(ctx, "rev-parse", "HEAD")
		parentSHA, _ := g.runGitCommand(ctx, "rev-parse", "HEAD~1")

		// Get commit info (author name|author email)
		authorInfo, _ := g.runGitCommand(ctx, "show", "-s", "--format=%an|%ae", "HEAD")
		authorParts := strings.Split(strings.TrimSpace(authorInfo), "|")
		authorName := ""
		authorEmail := ""
		if len(authorParts) >= 2 {
			authorName = authorParts[0]
			authorEmail = authorParts[1]
		}

		// Get commit stats
		filesChanged, insertions, deletions := g.getCommitStats(ctx, strings.TrimSpace(commitSHA))

		commit := &streams.GitCommitNotification{
			CommitSHA:    strings.TrimSpace(commitSHA),
			ParentSHA:    strings.TrimSpace(parentSHA),
			Message:      message,
			AuthorName:   authorName,
			AuthorEmail:  authorEmail,
			FilesChanged: filesChanged,
			Insertions:   insertions,
			Deletions:    deletions,
			CommittedAt:  time.Now().UTC(),
		}

		g.workspaceTracker.NotifyGitCommit(commit)
	}

	return result, nil
}

// Stage stages files for commit using git add.
// If paths is empty, stages all changes (git add -A).
func (g *GitOperator) Stage(ctx context.Context, paths []string) (*GitOperationResult, error) {
	if !g.tryLock("stage") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{
		Operation: "stage",
	}

	var args []string
	if len(paths) == 0 {
		// Stage all changes
		args = []string{"add", "-A"}
	} else {
		// Stage specific files
		args = append([]string{"add", "--"}, paths...)
	}

	output, err := g.runGitCommand(ctx, args...)
	result.Output = output

	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	result.Success = true
	g.logger.Info("stage completed", zap.Int("files", len(paths)))
	return result, nil
}

// Unstage unstages files from the index using git reset.
// If paths is empty, unstages all changes (git reset HEAD).
func (g *GitOperator) Unstage(ctx context.Context, paths []string) (*GitOperationResult, error) {
	if !g.tryLock("unstage") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{
		Operation: "unstage",
	}

	var args []string
	if len(paths) == 0 {
		// Unstage all changes
		args = []string{"reset", "HEAD"}
	} else {
		// Unstage specific files
		args = append([]string{"reset", "HEAD", "--"}, paths...)
	}

	output, err := g.runGitCommand(ctx, args...)
	result.Output = output

	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	result.Success = true
	g.logger.Info("unstage completed", zap.Int("files", len(paths)))
	return result, nil
}

// Abort aborts an in-progress merge or rebase operation.
func (g *GitOperator) Abort(ctx context.Context, operation string) (*GitOperationResult, error) {
	if !g.tryLock("abort") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{
		Operation: "abort",
	}

	var args []string
	switch operation {
	case "merge":
		args = []string{"merge", "--abort"}
	case "rebase":
		args = []string{"rebase", "--abort"}
	default:
		result.Error = fmt.Sprintf("unsupported operation to abort: %s (must be 'merge' or 'rebase')", operation)
		return result, nil
	}

	output, err := g.runGitCommand(ctx, args...)
	result.Output = output

	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	result.Success = true
	g.logger.Info("abort completed", zap.String("operation", operation))
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

// ShowCommit gets the diff for a specific commit using git show.
func (g *GitOperator) ShowCommit(ctx context.Context, commitSHA string) (*CommitDiffResult, error) {
	result := &CommitDiffResult{
		CommitSHA: commitSHA,
	}

	// Validate commit SHA (basic validation - alphanumeric only)
	if commitSHA == "" || len(commitSHA) > 64 {
		result.Error = "invalid commit SHA"
		return result, nil
	}
	for _, c := range commitSHA {
		isDigit := c >= '0' && c <= '9'
		isLowerHex := c >= 'a' && c <= 'f'
		isUpperHex := c >= 'A' && c <= 'F'
		if !isDigit && !isLowerHex && !isUpperHex {
			result.Error = "invalid commit SHA: must be hexadecimal"
			return result, nil
		}
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

	// Calculate total insertions/deletions
	for _, fileInfo := range result.Files {
		if fi, ok := fileInfo.(map[string]interface{}); ok {
			if additions, ok := fi["additions"].(int); ok {
				result.Insertions += additions
			}
			if deletions, ok := fi["deletions"].(int); ok {
				result.Deletions += deletions
			}
		}
	}

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
		// Extract path from "diff --git a/path b/path"
		headerParts := strings.Split(header, " ")
		if len(headerParts) < 4 {
			continue
		}

		// Get the b/path part and remove the "b/" prefix
		bPath := headerParts[len(headerParts)-1]
		filePath := strings.TrimPrefix(bPath, "b/")

		// Determine file status from diff content
		status := "modified"
		if strings.Contains(diffContent, "new file mode") {
			status = "added"
		} else if strings.Contains(diffContent, "deleted file mode") {
			status = "deleted"
		} else if strings.Contains(diffContent, "rename from") {
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

// tryLock attempts to acquire the operation lock without blocking.
// Returns true if the lock was acquired, false if an operation is in progress.
func (g *GitOperator) tryLock(opName string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inProgress {
		return false
	}
	g.inProgress = true
	g.currentOp = opName
	return true
}

// unlock releases the operation lock and triggers a git status refresh.
func (g *GitOperator) unlock() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inProgress = false
	g.currentOp = ""

	// Trigger fsnotify to refresh git status in the workspace tracker.
	// This is called after every git operation completes.
	g.triggerFsNotify()
}

// PRCreateResult represents the result of a PR creation operation.
type PRCreateResult struct {
	Success bool   `json:"success"`
	PRURL   string `json:"pr_url,omitempty"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

// CreatePR creates a pull request using the gh CLI.
// It first pushes the current branch to the remote, then creates the PR.
func (g *GitOperator) CreatePR(ctx context.Context, title, body, baseBranch string) (*PRCreateResult, error) {
	if !g.tryLock("create-pr") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &PRCreateResult{}

	// Get current branch name for --head flag
	branch, err := g.getCurrentBranch(ctx)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get current branch: %s", err.Error())
		return result, nil
	}
	g.logger.Debug("current branch", zap.String("branch", branch))

	// First, push the branch to remote (with --set-upstream in case it's a new branch)
	pushOutput, err := g.runGitCommand(ctx, "push", "--set-upstream", "origin", "HEAD")
	if err != nil {
		result.Error = fmt.Sprintf("failed to push branch: %s", pushOutput)
		result.Output = pushOutput
		return result, nil
	}
	g.logger.Debug("pushed branch to remote", zap.String("output", pushOutput))

	// Now create the PR
	// Use --head to explicitly specify the branch (helps gh in worktree scenarios)
	args := []string{"pr", "create", "--title", title, "--body", body, "--head", branch}

	// Strip remote prefix (e.g., "origin/main" -> "main") since gh expects just the branch name
	cleanBaseBranch := strings.TrimPrefix(baseBranch, "origin/")
	if cleanBaseBranch != "" {
		args = append(args, "--base", cleanBaseBranch)
	}

	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = g.workDir

	// Clear GIT_DIR and GIT_WORK_TREE to ensure gh uses the worktree's .git file
	// and correctly detects the current branch. Inherited env vars can confuse gh
	// when running from a worktree.
	env := filterGitEnv(os.Environ())
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	g.logger.Debug("executing gh command",
		zap.Strings("args", args),
		zap.String("workDir", g.workDir))

	err = cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}
	result.Output = output

	if err != nil {
		result.Error = fmt.Sprintf("%s: %s", err.Error(), strings.TrimSpace(stderr.String()))
		return result, nil
	}

	// Extract PR URL from output (gh pr create outputs the URL on success)
	result.PRURL = strings.TrimSpace(stdout.String())
	result.Success = true
	g.logger.Info("PR created", zap.String("url", result.PRURL))
	return result, nil
}

// getCommitStats returns the number of files changed, insertions, and deletions for a commit
func (g *GitOperator) getCommitStats(ctx context.Context, commitSHA string) (filesChanged, insertions, deletions int) {
	// git show --stat --format="" HEAD gives us the stat summary
	output, err := g.runGitCommand(ctx, "show", "--stat", "--format=", commitSHA)
	if err != nil {
		return 0, 0, 0
	}

	// The last non-empty line contains the summary like:
	// " 3 files changed, 10 insertions(+), 5 deletions(-)"
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return 0, 0, 0
	}

	summary := lines[len(lines)-1]
	// Parse: "N files changed, M insertions(+), K deletions(-)"
	// Some variants may be missing insertions or deletions

	// Match files changed
	if idx := strings.Index(summary, " file"); idx > 0 {
		part := strings.TrimSpace(summary[:idx])
		// Get just the number
		parts := strings.Fields(part)
		if len(parts) > 0 {
			_, _ = fmt.Sscanf(parts[len(parts)-1], "%d", &filesChanged)
		}
	}

	// Match insertions
	if idx := strings.Index(summary, " insertion"); idx > 0 {
		// Find the number before " insertion"
		start := strings.LastIndex(summary[:idx], " ") + 1
		if start > 0 && start < idx {
			_, _ = fmt.Sscanf(summary[start:idx], "%d", &insertions)
		}
	}

	// Match deletions
	if idx := strings.Index(summary, " deletion"); idx > 0 {
		// Find the number before " deletion"
		start := strings.LastIndex(summary[:idx], " ") + 1
		if start > 0 && start < idx {
			_, _ = fmt.Sscanf(summary[start:idx], "%d", &deletions)
		}
	}

	return filesChanged, insertions, deletions
}
