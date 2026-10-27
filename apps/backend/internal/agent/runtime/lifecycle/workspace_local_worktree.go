package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/common/securityutil"
	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/task/models"
)

// localPoolWorktree is one worktree a launch created from a Kandev checkout.
type localPoolWorktree struct {
	repoPath string
	path     string
	branch   string
}

// provisionLocalPoolWorktrees creates a git worktree of the Kandev checkout for
// every repository of a launch placed on a local pool node, on a per-generation
// branch of the environment, and returns the projection without them. Other
// repositories (no local checkout) and every remote node keep the fresh
// checkout done by agentctl materialization. A repository whose options a
// plain worktree cannot honour is refused before any worktree is created. A
// failure removes the worktrees and branches this call created.
func provisionLocalPoolWorktrees(
	ctx context.Context,
	req *LaunchRequest,
	instance *ExecutorInstance,
	projection []WorkspaceRepositoryMaterialization,
) ([]WorkspaceRepositoryMaterialization, error) {
	if instance == nil || instance.LocalWorktreeBranch == "" {
		return projection, nil
	}
	local := map[string]RepoLaunchSpec{}
	for _, spec := range req.RepoSpecs() {
		if spec.RepositoryPath == "" {
			continue
		}
		destination, err := remoteWorkspaceRepositoryDestination(spec)
		if err != nil {
			return nil, fmt.Errorf("local worktree for %q: %w", spec.RepoName, err)
		}
		if option := unsupportedLocalWorktreeOption(spec); option != "" {
			return nil, fmt.Errorf("local worktree for %q: unsupported option: %s", spec.RepoName, option)
		}
		local[destination] = spec
	}
	branch := localPoolWorktreeBranch(instance.LocalWorktreeBranch, instance.WorkspacePath)
	remaining := make([]WorkspaceRepositoryMaterialization, 0, len(projection))
	var created []localPoolWorktree
	for _, item := range projection {
		spec, ok := local[item.Destination]
		if !ok {
			remaining = append(remaining, item)
			continue
		}
		path := filepath.Join(instance.WorkspacePath, item.Destination)
		added, err := addLocalPoolWorktree(ctx, spec, branch, path)
		if err != nil {
			return nil, errors.Join(err, removeLocalPoolWorktrees(ctx, created))
		}
		if added {
			created = append(created, localPoolWorktree{repoPath: spec.RepositoryPath, path: path, branch: branch})
		}
	}
	return remaining, nil
}

// unsupportedLocalWorktreeOption names the first launch option that a worktree
// of the local base branch does not honour; the empty string means none.
func unsupportedLocalWorktreeOption(spec RepoLaunchSpec) string {
	switch {
	case spec.CheckoutBranch != "":
		return "checkout branch"
	case spec.PRNumber > 0:
		return "pull request"
	case spec.QualifiedPRBase != nil:
		return "qualified base"
	case spec.RemoteContribution != nil || spec.ContributionDestination != nil:
		return "contribution"
	case spec.CheckoutOptions != nil && len(spec.CheckoutOptions.SparseDirectories) != 0:
		return "sparse checkout"
	case spec.CheckoutOptions != nil && spec.CheckoutOptions.DownloadMode != models.DownloadStandard:
		return "on-demand download"
	}
	return ""
}

// addLocalPoolWorktree reports whether it created the worktree; an existing
// worktree is reused only when it belongs to the same repository, sits at path
// and is on the expected branch.
func addLocalPoolWorktree(ctx context.Context, spec RepoLaunchSpec, branch, path string) (bool, error) {
	if info, err := os.Lstat(filepath.Join(path, ".git")); err == nil {
		if !info.Mode().IsRegular() {
			return false, fmt.Errorf("local worktree for %q: %s is not a git worktree", spec.RepoName, path)
		}
		if err := verifyLocalPoolWorktree(ctx, spec.RepositoryPath, path, branch); err != nil {
			return false, fmt.Errorf("local worktree for %q: %w", spec.RepoName, err)
		}
		return false, nil
	}
	base := spec.BaseBranch
	if base == "" {
		base = spec.DefaultBranch
	}
	if !securityutil.IsValidBranchName(base) || !securityutil.IsValidBranchName(branch) {
		return false, fmt.Errorf("local worktree for %q: invalid branch", spec.RepoName)
	}
	cmd := subproc.NewGitCommand(ctx, "-C", spec.RepositoryPath, gitWorktreeCmd, "add", "-b", branch, path, base)
	if out, err := subproc.RunGitCombinedOutputClass(ctx, subproc.GitLifecycle, cmd); err != nil {
		return false, fmt.Errorf("local worktree for %q: git worktree add: %w: %s", spec.RepoName, err, strings.TrimSpace(string(out)))
	}
	return true, nil
}

// removeLocalPoolWorktrees removes each worktree and the branch created for it.
func removeLocalPoolWorktrees(ctx context.Context, worktrees []localPoolWorktree) error {
	ctx = context.WithoutCancel(ctx)
	var errs []error
	for _, w := range worktrees {
		for _, args := range [][]string{
			{"-C", w.repoPath, gitWorktreeCmd, "remove", "--force", w.path},
			{"-C", w.repoPath, "branch", "-D", w.branch},
		} {
			cmd := subproc.NewGitCommand(ctx, args...)
			if out, err := subproc.RunGitCombinedOutputClass(ctx, subproc.GitLifecycle, cmd); err != nil {
				errs = append(errs, fmt.Errorf("git %s: %w: %s", strings.Join(args[2:], " "), err, strings.TrimSpace(string(out))))
			}
		}
	}
	return errors.Join(errs...)
}

// gitWorktreeCmd is the git subcommand for linked worktrees.
const gitWorktreeCmd = "worktree"
