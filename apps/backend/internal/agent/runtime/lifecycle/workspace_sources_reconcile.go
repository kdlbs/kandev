package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kandev/kandev/internal/worktree"
)

// reconcileWorkspaceSources recreates Kandev-owned links from durable source
// specs before a host launch or workspace-only resume.
func reconcileWorkspaceSources(_ context.Context, root string, folders []WorkspaceFolderSpec) error {
	if len(folders) == 0 {
		return nil
	}
	if root == "" {
		return fmt.Errorf("workspace root is required for durable folders")
	}
	for _, folder := range folders {
		if folder.Name == "" || filepath.Base(folder.Name) != folder.Name || folder.LocalPath == "" {
			return fmt.Errorf("invalid durable workspace folder")
		}
		info, err := os.Stat(folder.LocalPath)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("workspace folder %q target is missing: %s", folder.Name, folder.LocalPath)
		}
		if _, _, err := worktree.EnsureOwnedDirectoryLink(root, folder.Name, folder.LocalPath); err != nil {
			return fmt.Errorf("link workspace folder %q: %w", folder.Name, err)
		}
	}
	return nil
}

func reconcileWorkspaceRepositories(root string, repositories []WorkspaceRepositorySpec) error {
	if len(repositories) == 0 {
		return nil
	}
	if root == "" {
		return fmt.Errorf("workspace root is required for durable repositories")
	}
	for _, repository := range repositories {
		if repository.RepoName == "" || filepath.Base(repository.RepoName) != repository.RepoName || repository.RepositoryPath == "" {
			return fmt.Errorf("invalid durable workspace repository")
		}
		info, err := os.Stat(repository.RepositoryPath)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("workspace repository %q target is missing: %s", repository.RepoName, repository.RepositoryPath)
		}
		if _, _, err := worktree.EnsureOwnedDirectoryLink(root, repository.RepoName, repository.RepositoryPath); err != nil {
			return fmt.Errorf("link workspace repository %q: %w", repository.RepoName, err)
		}
	}
	return nil
}

func workspaceRepositorySpecsFromLaunch(req *LaunchRequest) []WorkspaceRepositorySpec {
	if req == nil {
		return nil
	}
	specs := req.RepoSpecs()
	result := make([]WorkspaceRepositorySpec, 0, len(specs))
	for _, spec := range specs {
		result = append(result, WorkspaceRepositorySpec{
			RepositoryID: spec.RepositoryID, RepositoryPath: spec.RepositoryPath, RepoName: spec.RepoName,
			BaseBranch: spec.BaseBranch, DefaultBranch: spec.DefaultBranch, CheckoutBranch: spec.CheckoutBranch,
			WorktreeID: spec.WorktreeID, WorktreeBranchPrefix: spec.WorktreeBranchPrefix,
			WorktreeBranchTemplate: spec.WorktreeBranchTemplate, PullBeforeWorktree: spec.PullBeforeWorktree,
			BranchSlug: spec.BranchSlug, BranchIdentitySlug: spec.BranchIdentitySlug,
		})
	}
	return result
}

func workspaceSourceRoots(folders []WorkspaceFolderSpec, repositories []WorkspaceRepositorySpec) []string {
	roots := make([]string, 0, len(folders)+len(repositories))
	seen := make(map[string]struct{}, cap(roots))
	add := func(path string) {
		resolved, err := filepath.EvalSymlinks(filepath.Clean(path))
		if err != nil {
			return
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() {
			return
		}
		if _, ok := seen[resolved]; ok {
			return
		}
		seen[resolved] = struct{}{}
		roots = append(roots, resolved)
	}
	for _, folder := range folders {
		add(folder.LocalPath)
	}
	for _, repository := range repositories {
		add(repository.RepositoryPath)
	}
	return roots
}
