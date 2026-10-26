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
)

// provisionLocalPoolWorktrees creates a git worktree of the Kandev checkout for
// every repository of a launch placed on a local pool node, on the instance's
// per-environment branch, and returns the projection without them. Other
// repositories (no local checkout) and every remote node keep the fresh
// checkout done by agentctl materialization. A failure removes the worktrees
// this call created.
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
		local[destination] = spec
	}
	remaining := make([]WorkspaceRepositoryMaterialization, 0, len(projection))
	var created []string
	for _, item := range projection {
		spec, ok := local[item.Destination]
		if !ok {
			remaining = append(remaining, item)
			continue
		}
		path := filepath.Join(instance.WorkspacePath, item.Destination)
		added, err := addLocalPoolWorktree(ctx, spec, instance.LocalWorktreeBranch, path)
		if err != nil {
			return nil, errors.Join(err, removeLocalPoolWorktrees(ctx, created))
		}
		if added {
			created = append(created, path)
		}
	}
	return remaining, nil
}

// addLocalPoolWorktree reports whether it created the worktree; an existing
// worktree on the same branch is reused.
func addLocalPoolWorktree(ctx context.Context, spec RepoLaunchSpec, branch, path string) (bool, error) {
	if info, err := os.Lstat(filepath.Join(path, ".git")); err == nil && info.Mode().IsRegular() {
		return false, nil
	}
	base := spec.BaseBranch
	if base == "" {
		base = spec.DefaultBranch
	}
	if !securityutil.IsValidBranchName(base) || !securityutil.IsValidBranchName(branch) {
		return false, fmt.Errorf("local worktree for %q: invalid branch", spec.RepoName)
	}
	cmd := subproc.NewGitCommand(ctx, "-C", spec.RepositoryPath, "worktree", "add", "-b", branch, path, base)
	if out, err := subproc.RunGitCombinedOutputClass(ctx, subproc.GitLifecycle, cmd); err != nil {
		return false, fmt.Errorf("local worktree for %q: git worktree add: %w: %s", spec.RepoName, err, strings.TrimSpace(string(out)))
	}
	return true, nil
}

func removeLocalPoolWorktrees(ctx context.Context, paths []string) error {
	var errs []error
	for _, path := range paths {
		cmd := subproc.NewGitCommand(context.WithoutCancel(ctx), "-C", path, "worktree", "remove", "--force", path)
		if _, err := subproc.RunGitCombinedOutputClass(ctx, subproc.GitLifecycle, cmd); err != nil {
			errs = append(errs, fmt.Errorf("remove local worktree %s: %w", path, err))
		}
	}
	return errors.Join(errs...)
}
