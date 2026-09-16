package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/worktree"
)

const (
	workspaceLayoutTaskRoot        = "task_root"
	workspaceLayoutCurrentRoot     = "current_root"
	workspaceLayoutKandevDirectory = "kandev_directory"
)

func ownedDirectoryLinkOwner(taskID, taskDirName string) worktree.OwnedDirectoryLinkOwner {
	return worktree.OwnedDirectoryLinkOwner{TaskID: taskID, TaskDirName: taskDirName}
}

// reconcileWorkspaceSources recreates Kandev-owned links from durable source
// specs before a host launch or workspace-only resume.
func reconcileWorkspaceSources(_ context.Context, root string, folders []WorkspaceFolderSpec, owner worktree.OwnedDirectoryLinkOwner) error {
	return reconcileWorkspaceSourcesAtLayout(context.Background(), root, "", folders, owner)
}

func reconcileWorkspaceSourcesAtLayout(_ context.Context, root, workspaceLayout string, folders []WorkspaceFolderSpec, owner worktree.OwnedDirectoryLinkOwner) error {
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
		linkRoot, linkName, target := workspaceFolderDestination(root, workspaceLayout, folder)
		if target == "" || !isWorkspaceEntryName(linkName) {
			return fmt.Errorf("invalid durable workspace folder %q placement", folder.Name)
		}
		info, err := os.Stat(folder.LocalPath)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("workspace folder %q target is missing: %s", folder.Name, folder.LocalPath)
		}
		if sameDirectory(target, folder.LocalPath) {
			continue
		}
		if _, err := worktree.EnsureOwnedDirectoryLink(linkRoot, linkName, folder.LocalPath, owner); err != nil {
			return fmt.Errorf("link workspace folder %q: %w", folder.Name, err)
		}
	}
	return nil
}

// reconcileWorkspaceRepositories preserves the legacy call shape used by tests
// and older callers whose root is already the directory where links belong.
func reconcileWorkspaceRepositories(root string, repositories []WorkspaceRepositorySpec, log *logger.Logger, owner worktree.OwnedDirectoryLinkOwner) error {
	return reconcileWorkspaceRepositoriesAtLayout(root, "", repositories, log, owner)
}

// reconcileWorkspaceRepositoriesAtLayout recreates Kandev-owned repository
// links for a durable workspace. WorkspaceRelativePath is rooted at the task
// directory, so nested placements must be resolved from the task root before
// creating their link. A spec whose destination is already the repository is
// skipped: linking it would plant a self-referential junction/symlink inside
// the checkout. The comparison is by filesystem identity, not by index,
// because a host-materialized multi-repo local task roots the workspace at
// ~/.kandev/tasks/<taskDir> and the primary repository is then a sibling.
func reconcileWorkspaceRepositoriesAtLayout(workspacePath, workspaceLayout string, repositories []WorkspaceRepositorySpec, log *logger.Logger, owner worktree.OwnedDirectoryLinkOwner) error {
	if len(repositories) == 0 {
		return nil
	}
	if workspacePath == "" {
		return fmt.Errorf("workspace root is required for durable repositories")
	}
	for index, repository := range repositories {
		if !isWorkspaceEntryName(repository.RepoName) || repository.RepositoryPath == "" {
			return fmt.Errorf("invalid durable workspace repository")
		}
		linkRoot, linkName, target, err := workspaceRepositoryLinkTarget(workspacePath, workspaceLayout, repository, index)
		if err != nil {
			return err
		}
		if sameDirectory(target, repository.RepositoryPath) {
			warnSelfReferentialEntry(linkRoot, linkName, log)
			continue
		}
		info, err := os.Stat(repository.RepositoryPath)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("workspace repository %q target is missing: %s", repository.RepoName, repository.RepositoryPath)
		}
		if _, err := worktree.EnsureOwnedDirectoryLink(linkRoot, linkName, repository.RepositoryPath, owner); err != nil {
			return fmt.Errorf("link workspace repository %q: %w", linkName, err)
		}
	}
	return nil
}

func workspaceRepositoryLinkTarget(workspacePath, workspaceLayout string, repository WorkspaceRepositorySpec, index int) (string, string, string, error) {
	linkRoot := workspacePath
	linkName := workspaceRepositoryEntryName(repository.RepoName)
	switch {
	case repository.WorkspaceRelativePath != "":
		target := workspaceRepositoryCandidate(workspacePath, workspaceLayout, repository.RepoName, repository.WorkspaceRelativePath)
		if target == "" {
			return "", "", "", fmt.Errorf("invalid durable workspace repository %q placement", repository.RepoName)
		}
		return filepath.Dir(target), filepath.Base(target), target, nil
	case workspaceLayout == workspaceLayoutTaskRoot:
		return linkRoot, linkName, filepath.Join(workspacePath, linkName), nil
	case workspaceLayout == workspaceLayoutKandevDirectory:
		linkRoot = filepath.Join(workspacePath, "kandev")
		return linkRoot, linkName, filepath.Join(linkRoot, linkName), nil
	case index == 0:
		return linkRoot, linkName, workspacePath, nil
	default:
		return linkRoot, linkName, filepath.Join(linkRoot, linkName), nil
	}
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
			BaseBranch: spec.BaseBranch, DefaultBranch: spec.DefaultBranch, CheckoutBranch: spec.CheckoutBranch,
			ComparisonTarget: spec.ComparisonTarget,
			WorktreeID:       spec.WorktreeID, WorktreeBranchPrefix: spec.WorktreeBranchPrefix,
			WorktreeBranchTemplate: spec.WorktreeBranchTemplate, PullBeforeWorktree: spec.PullBeforeWorktree,
			RemoteSyncHandled: spec.RemoteSyncHandled,
			BranchSlug:        spec.BranchSlug, BranchIdentitySlug: spec.BranchIdentitySlug,
			WorkspaceRelativePath: spec.WorkspaceRelativePath,
		})
	}
	return result
}

// workspaceRepositoryCandidate resolves the physical checkout represented by
// one durable repository spec. WorkspaceRelativePath is rooted at the task
// directory. A repository-layout workspace exposes the primary checkout as
// WorkspacePath, so its task root is the parent of that path; a task-root
// workspace exposes the task directory itself.
func workspaceRepositoryCandidate(workspacePath, workspaceLayout, repoName, workspaceRelativePath string) string {
	if workspacePath == "" {
		return ""
	}
	if workspaceRelativePath != "" {
		root := workspacePath
		if workspaceLayout != workspaceLayoutTaskRoot && workspaceLayout != workspaceLayoutCurrentRoot && workspaceLayout != workspaceLayoutKandevDirectory {
			root = filepath.Dir(workspacePath)
		}
		relative := filepath.Clean(filepath.FromSlash(workspaceRelativePath))
		if relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return ""
		}
		candidate := filepath.Join(root, relative)
		contained, err := filepath.Rel(root, candidate)
		if err != nil || contained == ".." || strings.HasPrefix(contained, ".."+string(filepath.Separator)) {
			return ""
		}
		return candidate
	}
	if workspaceLayout == workspaceLayoutKandevDirectory {
		return filepath.Join(workspacePath, "kandev", workspaceRepositoryEntryName(repoName))
	}
	if workspaceLayout == workspaceLayoutTaskRoot {
		return filepath.Join(workspacePath, workspaceRepositoryEntryName(repoName))
	}
	return workspacePath
}

func workspaceRepositoryValidationCandidates(workspacePath, workspaceLayout string, repositories []WorkspaceRepositorySpec, index int) []string {
	if index < 0 || index >= len(repositories) {
		return nil
	}
	repository := repositories[index]
	candidate := workspaceRepositoryCandidate(workspacePath, workspaceLayout, repository.RepoName, repository.WorkspaceRelativePath)
	if repository.WorkspaceRelativePath != "" {
		return []string{candidate}
	}
	if workspaceLayout == workspaceLayoutKandevDirectory {
		return []string{candidate}
	}
	if index > 0 {
		return []string{filepath.Join(workspacePath, workspaceRepositoryEntryName(repository.RepoName))}
	}
	if workspaceLayout == workspaceLayoutCurrentRoot || len(repositories) > 1 {
		return []string{candidate, filepath.Join(workspacePath, workspaceRepositoryEntryName(repository.RepoName))}
	}
	return []string{candidate}
}

func workspaceFolderDestination(workspacePath, workspaceLayout string, folder WorkspaceFolderSpec) (string, string, string) {
	if folder.WorkspaceRelativePath != "" {
		relative := filepath.Clean(filepath.FromSlash(folder.WorkspaceRelativePath))
		if relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", "", ""
		}
		target := filepath.Join(workspacePath, relative)
		contained, err := filepath.Rel(workspacePath, target)
		if err != nil || contained == ".." || strings.HasPrefix(contained, ".."+string(filepath.Separator)) {
			return "", "", ""
		}
		return filepath.Dir(target), filepath.Base(target), target
	}
	if workspaceLayout == workspaceLayoutKandevDirectory {
		root := filepath.Join(workspacePath, "kandev")
		return root, folder.Name, filepath.Join(root, folder.Name)
	}
	return workspacePath, folder.Name, filepath.Join(workspacePath, folder.Name)
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
