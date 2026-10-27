package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/common/subproc"
)

// localPoolWorktreeBranch is the branch of one environment generation: the
// workspace directory name (g<generation>) scopes the environment branch.
func localPoolWorktreeBranch(environmentBranch, workspacePath string) string {
	return environmentBranch + "-" + filepath.Base(workspacePath)
}

// localPoolWorktreeBranchForWorkspace derives the branch from a pool workspace
// path of the form <root>/environments/<environment>/g<generation>.
func localPoolWorktreeBranchForWorkspace(workspacePath string) string {
	return localPoolWorktreeBranch(nomad.PoolWorktreeBranch(filepath.Base(filepath.Dir(workspacePath))), workspacePath)
}

func poolWorktreeGitOutput(ctx context.Context, args ...string) (string, error) {
	cmd := subproc.NewGitCommand(ctx, args...)
	out, err := subproc.RunGitOutputClass(ctx, subproc.GitLifecycle, cmd)
	return strings.TrimSpace(string(out)), err
}

func resolvedLocalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

// verifyLocalPoolWorktree requires path to be a worktree of the repository at
// repoPath, rooted at path, with branch checked out.
func verifyLocalPoolWorktree(ctx context.Context, repoPath, path, branch string) error {
	source, err := poolWorktreeGit(ctx, repoPath, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return fmt.Errorf("read source repository git dir: %w", err)
	}
	common, err := poolWorktreeGit(ctx, path, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return fmt.Errorf("read worktree git dir: %w", err)
	}
	if resolvedLocalPath(source) != resolvedLocalPath(common) {
		return fmt.Errorf("worktree %s belongs to a different repository", path)
	}
	top, err := poolWorktreeGit(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil || resolvedLocalPath(top) != resolvedLocalPath(path) {
		return fmt.Errorf("worktree %s is not rooted at its path", path)
	}
	head, err := poolWorktreeGit(ctx, path, "symbolic-ref", "--short", "HEAD")
	if err != nil || head != branch {
		return fmt.Errorf("worktree %s is on %q, expected %q", path, head, branch)
	}
	return nil
}

func poolWorktreeGit(ctx context.Context, dir string, args ...string) (string, error) {
	return poolWorktreeGitOutput(ctx, append([]string{"-C", dir}, args...)...)
}

// localPoolWorktreePresent reports whether destination under workspacePath is a
// git worktree (a .git file) rather than a directory checkout.
func localPoolWorktreePresent(workspacePath, destination string) bool {
	info, err := os.Lstat(filepath.Join(workspacePath, destination, ".git"))
	return err == nil && info.Mode().IsRegular()
}
