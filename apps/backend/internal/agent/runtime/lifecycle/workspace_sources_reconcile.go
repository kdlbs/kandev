package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/worktree"
)

func ownedDirectoryLinkOwner(taskID, taskDirName string) worktree.OwnedDirectoryLinkOwner {
	return worktree.OwnedDirectoryLinkOwner{TaskID: taskID, TaskDirName: taskDirName}
}

// reconcileWorkspaceSources recreates Kandev-owned links from durable source
// specs before a host launch or workspace-only resume.
func reconcileWorkspaceSources(_ context.Context, root string, folders []WorkspaceFolderSpec, owner worktree.OwnedDirectoryLinkOwner) error {
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
		if _, err := worktree.EnsureOwnedDirectoryLink(root, folder.Name, folder.LocalPath, owner); err != nil {
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
func reconcileWorkspaceRepositories(root string, repositories []WorkspaceRepositorySpec, log *logger.Logger, owner worktree.OwnedDirectoryLinkOwner) error {
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
			warnSelfReferentialEntry(root, repository.RepoName, log)
			continue
		}
		info, err := os.Stat(repository.RepositoryPath)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("workspace repository %q target is missing: %s", repository.RepoName, repository.RepositoryPath)
		}
		if _, err := worktree.EnsureOwnedDirectoryLink(root, repository.RepoName, repository.RepositoryPath, owner); err != nil {
			return fmt.Errorf("link workspace repository %q: %w", repository.RepoName, err)
		}
	}
	return nil
}

// selfReferentialEntryWarning is shared with the tests that assert the user is
// told about a stale entry, so the two cannot drift apart silently.
const selfReferentialEntryWarning = "workspace entry links to the workspace root; remove that entry to stop tools recursing into it"

// warnSelfReferentialEntry surfaces a link an earlier release planted inside
// the user's own repository, and deliberately leaves it in place.
//
// Kandev writes no ownership marker into user-owned sources, so such an entry
// cannot be shown to be ours: a user, or the repository itself, may keep a link
// of the same name and target on purpose, and deleting it would destroy content
// that is not ours. A stat-then-remove sequence could not close the window
// between the check and the unlink either — the entry can be swapped for an
// unrelated empty directory in between, which os.Remove would happily delete.
// Reporting it costs the user one command and risks nothing.
func warnSelfReferentialEntry(root, name string, log *logger.Logger) {
	if log == nil {
		return
	}
	selfLink, err := worktree.IsSelfReferentialDirectoryLink(root, name)
	if err != nil || !selfLink {
		return
	}
	// The guidance is deliberately shell-neutral. A ready-to-paste command built
	// from the entry path would need per-shell escaping, and a path carrying
	// $(), backticks or %VAR% would otherwise reach a shell that expands it.
	log.Warn(selfReferentialEntryWarning,
		zap.String("entry", filepath.Join(root, name)),
		zap.String("note", "the entry is a directory link, not a copy: removing it leaves the repository untouched"))
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
			Position:   spec.Position,
			BaseBranch: spec.BaseBranch, DefaultBranch: spec.DefaultBranch, CheckoutBranch: spec.CheckoutBranch,
			ComparisonTarget: spec.ComparisonTarget,
			WorktreeID:       spec.WorktreeID, WorktreeBranchPrefix: spec.WorktreeBranchPrefix,
			WorktreeBranchTemplate: spec.WorktreeBranchTemplate, PullBeforeWorktree: spec.PullBeforeWorktree,
			RemoteSyncHandled: spec.RemoteSyncHandled,
			BranchSlug:        spec.BranchSlug, BranchIdentitySlug: spec.BranchIdentitySlug,
		})
	}
	return result
}

func workspaceSourceRoots(folders []WorkspaceFolderSpec, repositories []WorkspaceRepositorySpec) []string {
	type sourceRoot struct {
		path     string
		position int
		ordinal  int
	}
	candidates := make([]sourceRoot, 0, len(folders)+len(repositories))
	for _, folder := range folders {
		candidates = append(candidates, sourceRoot{
			path: folder.LocalPath, position: folder.Position, ordinal: len(candidates),
		})
	}
	for _, repository := range repositories {
		candidates = append(candidates, sourceRoot{
			path: repository.RepositoryPath, position: repository.Position, ordinal: len(candidates),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].position != candidates[j].position {
			return candidates[i].position < candidates[j].position
		}
		return candidates[i].ordinal < candidates[j].ordinal
	})

	roots := make([]string, 0, len(candidates))
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
	for _, candidate := range candidates {
		add(candidate.path)
	}
	return roots
}
