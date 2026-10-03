package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
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

func reconcileProjectWorkspace(primaryRepositoryPath, contextPath, taskID, workspaceID, taskDirName string) (string, error) {
	if primaryRepositoryPath == "" || contextPath == "" || taskID == "" || taskDirName == "" {
		return "", fmt.Errorf("project workspace identity or path is incomplete")
	}
	taskRoot := filepath.Dir(primaryRepositoryPath)
	if err := validateProjectWorkspaceOwner(taskRoot, taskID, workspaceID, taskDirName); err != nil {
		return "", err
	}
	absContextPath, contextInfo, err := canonicalProjectContext(contextPath)
	if err != nil {
		return "", err
	}
	if err := validateProjectContextLink(taskRoot, absContextPath, contextInfo); err != nil {
		return "", err
	}
	if _, err := worktree.EnsureOwnedDirectoryLink(taskRoot, "context", absContextPath, worktree.OwnedDirectoryLinkOwner{
		TaskID: taskID, TaskDirName: taskDirName,
	}); err != nil {
		return "", fmt.Errorf("link project context into task workspace: %w", err)
	}
	return taskRoot, nil
}

func validateProjectWorkspaceOwner(taskRoot, taskID, workspaceID, taskDirName string) error {
	marker, found, err := storageworkspaces.ReadOwnershipMarker(taskRoot)
	if err != nil || !found || marker.TaskID != taskID || marker.TaskDirName != taskDirName ||
		(workspaceID != "" && marker.WorkspaceID != "" && marker.WorkspaceID != workspaceID) {
		return fmt.Errorf("project task workspace ownership could not be verified")
	}
	return nil
}

func canonicalProjectContext(contextPath string) (string, os.FileInfo, error) {
	absContextPath, err := filepath.Abs(contextPath)
	if err != nil {
		return "", nil, fmt.Errorf("resolve project context path: %w", err)
	}
	if filepath.Base(absContextPath) != "context" || filepath.Base(filepath.Dir(filepath.Dir(absContextPath))) != "agent-projects" {
		return "", nil, fmt.Errorf("project context path is outside the canonical agent-projects root")
	}
	for current := absContextPath; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || worktree.IsDirectoryLink(current) {
			return "", nil, fmt.Errorf("project context path contains a non-directory or link")
		}
		if filepath.Base(current) == "agent-projects" {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", nil, fmt.Errorf("project context path is outside the canonical agent-projects root")
		}
	}
	resolvedContextPath, err := filepath.EvalSymlinks(absContextPath)
	if err != nil || filepath.Clean(resolvedContextPath) != filepath.Clean(absContextPath) {
		return "", nil, fmt.Errorf("project context path is not canonical")
	}
	contextInfo, err := os.Lstat(absContextPath)
	if err != nil || !contextInfo.IsDir() || worktree.IsDirectoryLink(absContextPath) {
		return "", nil, fmt.Errorf("project context path is not a real directory")
	}
	return absContextPath, contextInfo, nil
}

func validateProjectContextLink(taskRoot, absContextPath string, contextInfo os.FileInfo) error {
	linkPath := filepath.Join(taskRoot, "context")
	_, linkErr := os.Lstat(linkPath)
	if linkErr == nil {
		if !worktree.IsDirectoryLink(linkPath) {
			return fmt.Errorf("project context entry is not a directory link")
		}
		linkedContext, statErr := os.Stat(linkPath)
		if statErr != nil {
			return fmt.Errorf("project context link target is unavailable: %w", statErr)
		}
		if !os.SameFile(linkedContext, contextInfo) {
			return fmt.Errorf("project context link points to an unexpected directory")
		}
	} else if !os.IsNotExist(linkErr) {
		return fmt.Errorf("inspect project context link: %w", linkErr)
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
			IntegrationRef: spec.IntegrationRef,
			BaseBranch:     spec.BaseBranch, DefaultBranch: spec.DefaultBranch, CheckoutBranch: spec.CheckoutBranch,
			PRNumber: spec.PRNumber, QualifiedPRBase: spec.QualifiedPRBase,
			ComparisonTarget: spec.ComparisonTarget,
			WorktreeID:       spec.WorktreeID, WorktreeBranchPrefix: spec.WorktreeBranchPrefix,
			WorktreeBranchTemplate: spec.WorktreeBranchTemplate, PullBeforeWorktree: spec.PullBeforeWorktree,
			RemoteSyncHandled: spec.RemoteSyncHandled,
			BranchSlug:        spec.BranchSlug, BranchIdentitySlug: spec.BranchIdentitySlug,
		})
	}
	return result
}

func workspaceSourceRoots(
	folders []WorkspaceFolderSpec,
	repositories []WorkspaceRepositorySpec,
	projectWorkspace *ProjectWorkspaceAccess,
) []string {
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
	if projectWorkspace != nil {
		add(projectWorkspace.ContextPath)
		for _, path := range projectWorkspace.RepositoryWorktreePaths {
			add(path)
		}
	} else {
		for _, repository := range repositories {
			add(repository.RepositoryPath)
		}
	}
	return roots
}

func projectWritableRoots(projectWorkspace *ProjectWorkspaceAccess) ([]string, error) {
	if projectWorkspace == nil {
		return nil, nil
	}
	if projectWorkspace.ContextPath == "" {
		return nil, fmt.Errorf("agent project context write root is unavailable")
	}
	if len(projectWorkspace.RepositoryWorktreePaths) == 0 {
		return nil, fmt.Errorf("agent project repository write roots are unavailable")
	}
	roots := make([]string, 0, len(projectWorkspace.RepositoryWorktreePaths)+1)
	seen := make(map[string]struct{}, cap(roots))
	for _, path := range append([]string{projectWorkspace.ContextPath}, projectWorkspace.RepositoryWorktreePaths...) {
		if path == "" {
			return nil, fmt.Errorf("agent project repository write root is unavailable")
		}
		resolved, err := filepath.EvalSymlinks(filepath.Clean(path))
		if err != nil {
			return nil, fmt.Errorf("resolve agent project write root %q: %w", path, err)
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return nil, fmt.Errorf("stat agent project write root %q: %w", path, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("agent project write root %q is not a directory", path)
		}
		if _, ok := seen[resolved]; ok {
			continue
		}
		seen[resolved] = struct{}{}
		roots = append(roots, resolved)
	}
	return roots, nil
}
