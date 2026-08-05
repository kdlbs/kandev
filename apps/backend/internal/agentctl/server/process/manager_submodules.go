package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/subproc"
	"go.uber.org/zap"
)

const submoduleDiscoveryTimeout = 10 * time.Second

func (m *Manager) hasBareMultiRepoTrackerGraph() bool {
	root, trackers := m.snapshotTrackers()
	return root != nil && root.repositoryName == "" && root.gitIndexPath == "" && len(trackers) > 0
}

// buildWorkspaceTrackerGraph preserves the existing bare multi-repository
// shape while adding initialized submodules below each real repository. A
// single sibling repository remains the unnamed root for compatibility with
// existing task workspaces.
func (m *Manager) buildWorkspaceTrackerGraph(
	ctx context.Context,
	workDir string,
	roots []string,
	includeAttachedRepos bool,
	forceBareRoot bool,
) (*WorkspaceTracker, []*WorkspaceTracker) {
	directRepos := scanRepositorySubdirs(workDir, roots)
	rootIsGit := resolveGitIndexPath(workDir) != ""

	if !rootIsGit && (len(directRepos) >= 2 || forceBareRoot) {
		root := NewWorkspaceTrackerForRepo(workDir, "", m.logger)
		m.configureTracker(root, "", roots)

		trackers := make([]*WorkspaceTracker, 0, len(directRepos))
		for _, repo := range directRepos {
			trackers = append(trackers, m.buildRepositoryScopeTrackers(ctx, repo, roots, workDir)...)
		}
		return root, trackers
	}

	root := NewWorkspaceTracker(workDir, m.logger)
	m.configureTracker(root, "", roots)
	trackers := m.discoverSubmoduleTrackers(ctx, root, roots, workDir)
	if rootIsGit && includeAttachedRepos {
		known := make(map[string]struct{}, len(trackers))
		for _, tracker := range trackers {
			known[tracker.RepositoryName()] = struct{}{}
		}
		for _, repo := range directRepos {
			if _, alreadyDiscovered := known[repo.name]; alreadyDiscovered {
				continue
			}
			trackers = append(trackers, m.buildRepositoryScopeTrackers(ctx, repo, roots, workDir)...)
		}
	}
	return root, trackers
}

func (m *Manager) configureTracker(tracker *WorkspaceTracker, repositoryName string, roots []string) {
	tracker.SetAllowedSourceRoots(roots)
	if !tracker.IsSubmodule() {
		tracker.SetBaseBranch(lookupBaseBranch(m.getBaseBranches(), repositoryName))
	}
}

func (m *Manager) buildRepositoryScopeTrackers(
	ctx context.Context,
	repo repositorySubdir,
	roots []string,
	workspaceRoot string,
) []*WorkspaceTracker {
	tracker := m.newTrackerForRepo(repo.path, repo.name)
	m.configureTracker(tracker, repo.name, roots)
	trackers := []*WorkspaceTracker{tracker}
	return append(trackers, m.discoverSubmoduleTrackers(ctx, tracker, roots, workspaceRoot)...)
}

// reconcileWorkspaceTrackerGraph keeps existing tracker instances when their
// scope and path are unchanged, while adding or removing submodule trackers
// discovered by a workspace rescan. New trackers inherit subscribers and the
// current polling mode just like the existing sibling-repository rescan path.
func (m *Manager) reconcileWorkspaceTrackerGraph(
	ctx context.Context,
	workDir string,
	roots []string,
	forceBareRoot bool,
) error {
	desiredRoot, desiredTrackers := m.buildWorkspaceTrackerGraph(ctx, workDir, roots, true, forceBareRoot)
	subs := m.snapshotSubscribers()
	oldRoot, oldTrackers := m.snapshotTrackers()

	oldByIdentity := make(map[repositoryTrackerKey]*WorkspaceTracker, len(oldTrackers))
	for _, tracker := range oldTrackers {
		oldByIdentity[repositoryTrackerIdentity(tracker.RepositoryName(), tracker.workDir)] = tracker
	}

	root := desiredRoot
	rootReused := false
	if oldRoot != nil && repositoryTrackerIdentity(oldRoot.RepositoryName(), oldRoot.workDir) ==
		repositoryTrackerIdentity(desiredRoot.RepositoryName(), desiredRoot.workDir) {
		root = oldRoot
		rootReused = true
		syncTrackerConfiguration(root, desiredRoot, roots)
		desiredRoot.Stop()
	}
	if !rootReused {
		startAndAttachTracker(ctx, root, subs)
	}

	retained := make([]*WorkspaceTracker, 0, len(desiredTrackers))
	used := make(map[*WorkspaceTracker]struct{}, len(desiredTrackers))
	newTrackers := make([]*WorkspaceTracker, 0)
	for _, desired := range desiredTrackers {
		identity := repositoryTrackerIdentity(desired.RepositoryName(), desired.workDir)
		if existing := oldByIdentity[identity]; existing != nil {
			syncTrackerConfiguration(existing, desired, roots)
			desired.Stop()
			retained = append(retained, existing)
			used[existing] = struct{}{}
			continue
		}
		startAndAttachTracker(ctx, desired, subs)
		retained = append(retained, desired)
		newTrackers = append(newTrackers, desired)
	}

	m.repoTrackersMu.Lock()
	m.workspaceTracker = root
	m.repoTrackers = retained
	m.cfg.WorkDir = workDir
	m.workspaceSourceRoots = append([]string(nil), roots...)
	m.cfg.WorkspaceSourceRoots = append([]string(nil), roots...)
	m.applyWorkspacePollModeLocked(newTrackers...)
	m.repoTrackersMu.Unlock()
	if m.processRunner != nil {
		m.processRunner.workspaceTracker = root
	}

	if oldRoot != nil && oldRoot != root {
		for _, sub := range subs {
			oldRoot.DetachWorkspaceStreamSubscriber(sub)
		}
		oldRoot.Stop()
	}
	for _, old := range oldTrackers {
		if _, keep := used[old]; keep {
			continue
		}
		for _, sub := range subs {
			old.DetachWorkspaceStreamSubscriber(sub)
		}
		old.Stop()
	}
	return nil
}

func (m *Manager) replaceWorkspaceTrackerGraph(
	ctx context.Context,
	workDir string,
	roots []string,
) error {
	desiredRoot, desiredTrackers := m.buildWorkspaceTrackerGraph(ctx, workDir, roots, true, true)
	subs := m.snapshotSubscribers()
	startAndAttachTracker(ctx, desiredRoot, subs)
	for _, tracker := range desiredTrackers {
		startAndAttachTracker(ctx, tracker, subs)
	}

	oldRoot, oldTrackers := m.snapshotTrackers()
	m.repoTrackersMu.Lock()
	m.workspaceTracker = desiredRoot
	m.repoTrackers = desiredTrackers
	m.cfg.WorkDir = workDir
	m.workspaceSourceRoots = append([]string(nil), roots...)
	m.cfg.WorkspaceSourceRoots = append([]string(nil), roots...)
	m.applyWorkspacePollModeLocked(append([]*WorkspaceTracker{desiredRoot}, desiredTrackers...)...)
	m.repoTrackersMu.Unlock()
	if m.processRunner != nil {
		m.processRunner.workspaceTracker = desiredRoot
	}

	if oldRoot != nil {
		for _, sub := range subs {
			oldRoot.DetachWorkspaceStreamSubscriber(sub)
		}
		oldRoot.Stop()
	}
	for _, tracker := range oldTrackers {
		for _, sub := range subs {
			tracker.DetachWorkspaceStreamSubscriber(sub)
		}
		tracker.Stop()
	}
	return nil
}

func syncTrackerConfiguration(target, source *WorkspaceTracker, roots []string) {
	target.SetAllowedSourceRoots(roots)
	if source.IsSubmodule() {
		target.SetComparisonAnchor(source.ComparisonAnchor())
		return
	}
	target.SetBaseBranch(source.BaseBranch())
}

func startAndAttachTracker(ctx context.Context, tracker *WorkspaceTracker, subs []types.WorkspaceStreamSubscriber) {
	tracker.Start(ctx)
	for _, sub := range subs {
		tracker.AttachWorkspaceStreamSubscriber(sub)
	}
}

// discoverSubmoduleTrackers returns initialized, declared gitlink children in
// stable path order. A child is anchored to the gitlink SHA in the parent's
// comparison tree, not to the child's current/default branch. The recursion
// only follows gitlink paths from Git, never arbitrary nested directories.
func (m *Manager) discoverSubmoduleTrackers(
	ctx context.Context,
	parent *WorkspaceTracker,
	roots []string,
	workspaceRoot string,
) []*WorkspaceTracker {
	paths, err := listGitlinkPaths(ctx, parent.workDir, "HEAD")
	if err != nil {
		m.logger.Warn("could not discover submodules",
			zap.String("repository_name", parent.RepositoryName()),
			zap.String("path", parent.workDir),
			zap.Error(err))
		return nil
	}
	parentAnchor, _ := parent.ResolveBaseAnchor(ctx)
	trackers := make([]*WorkspaceTracker, 0, len(paths))
	for _, relativePath := range paths {
		childPath, ok := m.initializedSubmodulePath(workspaceRoot, parent.workDir, relativePath)
		if !ok {
			continue
		}

		name := joinRepositoryScope(parent.RepositoryName(), relativePath)
		child := m.newTrackerForRepo(childPath, name)
		child.SetAllowedSourceRoots(roots)
		anchor := ""
		if parentAnchor != "" {
			anchor, err = gitlinkCommitAt(ctx, parent.workDir, parentAnchor, relativePath)
			if err != nil {
				m.logger.Warn("could not resolve submodule comparison anchor",
					zap.String("repository_name", name),
					zap.String("parent_anchor", parentAnchor),
					zap.Error(err))
			}
		}
		child.SetComparisonAnchor(anchor)

		trackers = append(trackers, child)
		trackers = append(trackers, m.discoverSubmoduleTrackers(ctx, child, roots, workspaceRoot)...)
	}
	return trackers
}

// initializedSubmodulePath resolves a Git-declared child and verifies that it
// is an initialized repository inside the task workspace. Symlinked children
// are rejected so a changed worktree cannot redirect discovery outside the
// authenticated workspace root.
func (m *Manager) initializedSubmodulePath(workspaceRoot, parentPath, relativePath string) (string, bool) {
	if !safeGitRelativePath(relativePath) {
		m.logger.Warn("submodule discovery rejected unsafe path", zap.String("path", relativePath))
		return "", false
	}
	childPath := filepath.Clean(filepath.Join(parentPath, filepath.FromSlash(relativePath)))
	childInfo, err := os.Lstat(childPath)
	if err != nil || !childInfo.IsDir() || childInfo.Mode()&os.ModeSymlink != 0 {
		m.logger.Warn("submodule discovery rejected child worktree",
			zap.String("path", childPath), zap.Error(err))
		return "", false
	}
	resolvedRoot, err := filepath.EvalSymlinks(workspaceRoot)
	if err != nil {
		m.logger.Warn("submodule discovery could not resolve workspace root",
			zap.String("path", workspaceRoot), zap.Error(err))
		return "", false
	}
	resolvedChild, err := filepath.EvalSymlinks(childPath)
	if err != nil {
		m.logger.Warn("submodule discovery could not resolve child worktree",
			zap.String("path", childPath), zap.Error(err))
		return "", false
	}
	relativeToRoot, err := filepath.Rel(resolvedRoot, resolvedChild)
	if err != nil || pathEscapesRoot(relativeToRoot) {
		m.logger.Warn("submodule discovery rejected child outside workspace root",
			zap.String("path", childPath), zap.String("workspace_root", workspaceRoot), zap.Error(err))
		return "", false
	}
	if resolveGitIndexPath(childPath) == "" {
		m.logger.Warn("submodule discovery rejected uninitialized child",
			zap.String("path", childPath))
		return "", false
	}
	return childPath, true
}

func listGitlinkPaths(ctx context.Context, workDir, ref string) ([]string, error) {
	output, err := runGitLifecycleOutput(ctx, workDir, "ls-tree", "-r", "-z", ref)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0)
	for _, entry := range strings.Split(string(output), "\x00") {
		if entry == "" {
			continue
		}
		header, relativePath, ok := strings.Cut(entry, "\t")
		if !ok || !strings.HasPrefix(header, "160000 ") || !safeGitRelativePath(relativePath) {
			continue
		}
		paths = append(paths, relativePath)
	}
	sort.Strings(paths)
	return paths, nil
}

func gitlinkCommitAt(ctx context.Context, workDir, ref, relativePath string) (string, error) {
	if !sha1HexPattern.MatchString(ref) || !safeGitRelativePath(relativePath) {
		return "", fmt.Errorf("invalid gitlink lookup")
	}
	output, err := runGitLifecycleOutput(ctx, workDir, "ls-tree", "-z", ref, "--", relativePath)
	if err != nil {
		return "", err
	}
	for _, entry := range strings.Split(string(output), "\x00") {
		header, entryPath, ok := strings.Cut(entry, "\t")
		if !ok || entryPath != relativePath || !strings.HasPrefix(header, "160000 ") {
			continue
		}
		fields := strings.Fields(header)
		if len(fields) == 3 && sha1HexPattern.MatchString(fields[2]) {
			return fields[2], nil
		}
	}
	return "", fmt.Errorf("gitlink %q not found at %s", relativePath, ref)
}

func runGitLifecycleOutput(ctx context.Context, workDir string, args ...string) ([]byte, error) {
	output, runErr, execErr := subproc.RunGitOutputAfterAcquire(
		ctx,
		subproc.GitLifecycle,
		submoduleDiscoveryTimeout,
		func(execCtx context.Context) *exec.Cmd {
			cmd := subproc.NewGitCommand(execCtx, args...)
			cmd.Dir = workDir
			return cmd
		},
	)
	if execErr != nil {
		return nil, execErr
	}
	if runErr != nil {
		return nil, runErr
	}
	return output, nil
}

func safeGitRelativePath(relativePath string) bool {
	if relativePath == "" || strings.ContainsRune(relativePath, '\x00') || strings.Contains(relativePath, "\\") {
		return false
	}
	clean := path.Clean(relativePath)
	return clean == relativePath && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../") && !path.IsAbs(clean)
}

func joinRepositoryScope(parent, child string) string {
	if parent == "" {
		return child
	}
	return path.Join(parent, child)
}
