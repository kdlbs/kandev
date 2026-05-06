package configloader

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/logger"

	"go.uber.org/zap"
)

const gitCmdTimeout = 30 * time.Second

// GitStatus holds the git status for a workspace directory.
type GitStatus struct {
	Branch      string `json:"branch"`
	IsDirty     bool   `json:"is_dirty"`
	HasRemote   bool   `json:"has_remote"`
	Ahead       int    `json:"ahead"`
	Behind      int    `json:"behind"`
	CommitCount int    `json:"commit_count"`
}

// GitManager provides git operations for workspace directories.
type GitManager struct {
	basePath string
	loader   *ConfigLoader
	logger   *logger.Logger
}

// NewGitManager creates a new git manager for workspace directories.
func NewGitManager(basePath string, loader *ConfigLoader, log *logger.Logger) *GitManager {
	return &GitManager{
		basePath: basePath,
		loader:   loader,
		logger:   log.WithFields(zap.String("component", "git-manager")),
	}
}

// workspacePath returns the absolute path for a workspace directory.
func (g *GitManager) workspacePath(workspaceName string) string {
	return filepath.Join(g.basePath, "workspaces", workspaceName)
}

// IsGitWorkspace checks whether the workspace directory contains a .git directory.
func (g *GitManager) IsGitWorkspace(workspaceName string) bool {
	gitDir := filepath.Join(g.workspacePath(workspaceName), ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// CloneWorkspace clones a git repository into the workspace directory.
// If the directory already exists and is a git repo, it pulls instead.
func (g *GitManager) CloneWorkspace(ctx context.Context, repoURL, branch, workspaceName string) error {
	wsPath := g.workspacePath(workspaceName)

	if g.IsGitWorkspace(workspaceName) {
		g.logger.Info("workspace already cloned, pulling instead",
			zap.String("workspace", workspaceName))
		return g.PullWorkspace(ctx, workspaceName)
	}

	if !isAllowedRepoURL(repoURL) {
		return fmt.Errorf("git clone: disallowed repository URL scheme: %s", repoURL)
	}

	args := []string{"clone"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, "--", repoURL, wsPath)

	if _, err := g.runGit(ctx, "", args...); err != nil {
		return fmt.Errorf("git clone: %w", err)
	}

	g.logger.Info("workspace cloned",
		zap.String("workspace", workspaceName),
		zap.String("repo", repoURL))
	return g.loader.Reload(workspaceName)
}

// PullWorkspace runs git pull in the workspace directory.
func (g *GitManager) PullWorkspace(ctx context.Context, workspaceName string) error {
	wsPath := g.workspacePath(workspaceName)
	if !g.IsGitWorkspace(workspaceName) {
		return fmt.Errorf("workspace %q is not a git repository", workspaceName)
	}

	output, err := g.runGit(ctx, wsPath, "pull")
	if err != nil {
		return fmt.Errorf("git pull: %w\n%s", err, output)
	}

	g.logger.Info("workspace pulled", zap.String("workspace", workspaceName))
	return g.loader.Reload(workspaceName)
}

// PushWorkspace stages all changes, commits with the given message, and pushes.
// Returns nil if there is nothing to commit.
func (g *GitManager) PushWorkspace(ctx context.Context, workspaceName, message string) error {
	wsPath := g.workspacePath(workspaceName)
	if !g.IsGitWorkspace(workspaceName) {
		return fmt.Errorf("workspace %q is not a git repository", workspaceName)
	}

	if err := g.stageAndCommit(ctx, wsPath, message); err != nil {
		return err
	}

	if _, err := g.runGit(ctx, wsPath, "push"); err != nil {
		return fmt.Errorf("git push: %w", err)
	}

	g.logger.Info("workspace pushed", zap.String("workspace", workspaceName))
	return nil
}

// stageAndCommit stages all changes and commits. Returns nil if nothing to commit.
func (g *GitManager) stageAndCommit(ctx context.Context, wsPath, message string) error {
	if _, err := g.runGit(ctx, wsPath, "add", "-A"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	statusOut, err := g.runGit(ctx, wsPath, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if strings.TrimSpace(statusOut) == "" {
		return nil // nothing to commit
	}

	if _, err := g.runGit(ctx, wsPath, "commit", "-m", message); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	return nil
}

// GetWorkspaceGitStatus returns the git status for a workspace.
func (g *GitManager) GetWorkspaceGitStatus(ctx context.Context, workspaceName string) (*GitStatus, error) {
	wsPath := g.workspacePath(workspaceName)
	if !g.IsGitWorkspace(workspaceName) {
		return nil, fmt.Errorf("workspace %q is not a git repository", workspaceName)
	}

	status := &GitStatus{}

	branch, err := g.readBranch(ctx, wsPath)
	if err != nil {
		return nil, err
	}
	status.Branch = branch
	status.IsDirty = g.checkDirty(ctx, wsPath)
	status.HasRemote = g.checkHasRemote(ctx, wsPath)

	if status.HasRemote {
		_ = g.fetchSilent(ctx, wsPath)
		ahead, behind := g.readAheadBehind(ctx, wsPath)
		status.Ahead = ahead
		status.Behind = behind
	}

	status.CommitCount = g.readCommitCount(ctx, wsPath)
	return status, nil
}

// readBranch returns the current branch name.
func (g *GitManager) readBranch(ctx context.Context, wsPath string) (string, error) {
	out, err := g.runGit(ctx, wsPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("read branch: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// checkDirty returns true if the working tree has uncommitted changes.
func (g *GitManager) checkDirty(ctx context.Context, wsPath string) bool {
	out, err := g.runGit(ctx, wsPath, "status", "--porcelain")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) != ""
}

// checkHasRemote returns true if the repository has a remote configured.
func (g *GitManager) checkHasRemote(ctx context.Context, wsPath string) bool {
	out, err := g.runGit(ctx, wsPath, "remote")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) != ""
}

// fetchSilent fetches from all remotes without merging.
func (g *GitManager) fetchSilent(ctx context.Context, wsPath string) error {
	_, err := g.runGit(ctx, wsPath, "fetch", "--all", "--quiet")
	return err
}

// readAheadBehind returns the number of commits ahead and behind the upstream.
func (g *GitManager) readAheadBehind(ctx context.Context, wsPath string) (ahead, behind int) {
	out, err := g.runGit(ctx, wsPath, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		return 0, 0
	}
	parts := strings.Fields(strings.TrimSpace(out))
	if len(parts) == 2 {
		ahead, _ = strconv.Atoi(parts[0])
		behind, _ = strconv.Atoi(parts[1])
	}
	return ahead, behind
}

// readCommitCount returns the total number of commits in the repository.
func (g *GitManager) readCommitCount(ctx context.Context, wsPath string) int {
	out, err := g.runGit(ctx, wsPath, "rev-list", "--count", "HEAD")
	if err != nil {
		return 0
	}
	count, _ := strconv.Atoi(strings.TrimSpace(out))
	return count
}

// isAllowedRepoURL validates that the repository URL uses a known safe scheme,
// preventing argument injection via URLs that start with "--".
func isAllowedRepoURL(u string) bool {
	for _, prefix := range []string{"https://", "git://", "ssh://", "git@", "file://", "/"} {
		if strings.HasPrefix(u, prefix) {
			return true
		}
	}
	return false
}

// runGit executes a git command and returns combined stdout. If dir is empty,
// the command runs in the current directory.
func (g *GitManager) runGit(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitCmdTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = stdout.String()
		}
		return "", fmt.Errorf("%s: %s", err, strings.TrimSpace(errMsg))
	}
	return stdout.String(), nil
}
