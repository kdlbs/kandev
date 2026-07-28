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
		if !isWorkspaceEntryName(folder.Name) || folder.LocalPath == "" {
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

// reconcileWorkspaceRepositories recreates Kandev-owned repository links below
// root. A spec whose repository IS the workspace root is skipped: for the local
// executor the primary repository is the workspace root itself, so linking it
// would plant a self-referential junction/symlink inside the user's own
// checkout. This mirrors buildRemoteWorkspaceRepositories, which skips the
// primary for the same reason. The comparison is by filesystem identity, not
// by index, because a host-materialized multi-repo local task roots the
// workspace at ~/.kandev/tasks/<taskDir> — there repositories[0] is a real
// sibling and must keep its link.
func reconcileWorkspaceRepositories(root string, repositories []WorkspaceRepositorySpec) error {
	if len(repositories) == 0 {
		return nil
	}
	if root == "" {
		return fmt.Errorf("workspace root is required for durable repositories")
	}
	for _, repository := range repositories {
		if !isWorkspaceEntryName(repository.RepoName) || repository.RepositoryPath == "" {
			return fmt.Errorf("invalid durable workspace repository")
		}
		if sameDirectory(root, repository.RepositoryPath) {
			// Best effort: a self-link planted by an earlier release must not
			// block the launch when it cannot be removed (open handle, an agent
			// shell whose cwd is inside it). The next reconcile retries.
			_, _ = worktree.RemoveSelfReferentialDirectoryLink(root, repository.RepoName)
			continue
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

// isWorkspaceEntryName reports whether name is usable as a single entry below
// an owned workspace root. "." and ".." survive a filepath.Base round-trip, so
// they are rejected explicitly here rather than deeper in the worktree helpers,
// which would surface them as a confusing "owned link entry already exists".
func isWorkspaceEntryName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if filepath.Base(name) != name {
		return false
	}
	// A bare root survives the Base round-trip too — filepath.Base("/") is "/"
	// on Unix and filepath.Base(`\`) is `\` on Windows — and joining it resolves
	// back to the root itself rather than to an entry below it.
	return name != "/" && name != string(filepath.Separator) && filepath.VolumeName(name) == ""
}

// sameDirectory reports whether two paths name the same directory on disk.
// The comparison is filesystem identity rather than path text: os.Stat follows
// junctions and Unix symlinks alike, and os.SameFile compares volume and file
// index, which also absorbs 8.3 short paths and path case. A canonical-path
// comparison would not do — filepath.EvalSymlinks does not traverse a Windows
// junction, it returns the link's own path.
//
// A path that cannot be stat'ed is not the same directory: the workspace root
// may not exist yet, and the caller must then fall through to link creation.
// Both sides must be directories, so that a repository path replaced by a
// regular file falls through to the caller's IsDir validation instead of being
// skipped as "already the root".
func sameDirectory(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	leftInfo, err := os.Stat(left)
	if err != nil || !leftInfo.IsDir() {
		return false
	}
	rightInfo, err := os.Stat(right)
	if err != nil || !rightInfo.IsDir() {
		return false
	}
	return os.SameFile(leftInfo, rightInfo)
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
