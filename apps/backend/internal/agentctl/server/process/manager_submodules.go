package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/fsdiagnostics"
	"github.com/kandev/kandev/internal/common/subproc"
	"go.uber.org/zap"
)

const submoduleDiscoveryTimeout = 10 * time.Second
const maxSubmoduleDiscoveryDepth = 32

// submoduleDiagnosticFields supplies the closed operation and error class for
// submodule discovery failures. An exit code is added only when a typed
// process result supplies one.
func submoduleDiagnosticFields(err error) []zap.Field {
	fields := []zap.Field{zap.String("operation", "submodule_discovery")}
	if err == nil {
		return fields
	}
	fields = append(fields, zap.String("error_class", fsdiagnostics.ErrorClass(err)))
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		fields = append(fields, zap.Int("exit_code", exitErr.ExitCode()))
	}
	return fields
}

func (m *Manager) hasBareMultiRepoTrackerGraph() bool {
	root, trackers := m.snapshotTrackers()
	return root != nil && root.repositoryName == "" && root.gitIndexPath == "" && len(trackers) > 0
}

// hasBareTaskRootTracker reports whether the active root tracker is already a
// plain task-root scope with no per-repository children. A rescan must keep
// this scope when a repository appears beneath it so the file tree continues
// to expose both task-root files and the attached repository directory.
func (m *Manager) hasBareTaskRootTracker(workDir string) bool {
	root, trackers := m.snapshotTrackers()
	return root != nil && len(trackers) == 0 && root.repositoryName == "" &&
		root.gitIndexPath == "" && filepath.Clean(root.workDir) == filepath.Clean(workDir)
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
		root.suppressGitPublication = true
		m.configureTracker(root, "", roots)

		trackers := make([]*WorkspaceTracker, 0, len(directRepos))
		for _, repo := range directRepos {
			trackers = append(trackers, m.buildRepositoryScopeTrackers(ctx, repo, roots, workDir)...)
		}
		return root, trackers
	}

	root := NewWorkspaceTracker(workDir, m.logger)
	m.configureTracker(root, "", roots)
	trackers := m.discoverSubmoduleTrackers(ctx, root, roots, workDir, 0)
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
	if m.cfg != nil {
		tracker.SetDiagnosticIdentity(m.cfg.TaskID, m.cfg.SessionID)
	}
	tracker.SetGitEnvironment(m.trackerGitEnvironment())
	tracker.SetAllowedSourceRoots(roots)
	tracker.SetRecoveryArtifactExclusions(m.currentWorkspaceFileExclusions())
	if !tracker.IsSubmodule() {
		tracker.SetBaseBranch(lookupBaseBranch(m.getBaseBranches(), repositoryName))
	}
	tracker.SetComparisonTarget(m.comparisonTargetFor(repositoryName))
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
	return append(trackers, m.discoverSubmoduleTrackers(ctx, tracker, roots, workspaceRoot, 0)...)
}

// reconcileWorkspaceTrackerGraph keeps live tracker instances when their scope
// and path are unchanged, while adding or removing submodule trackers
// discovered by a workspace rescan. Stopped trackers are never reused: Stop is
// terminal, so they are replaced and retired like any other removed scope. New
// trackers inherit subscribers and the current polling mode.
func (m *Manager) reconcileWorkspaceTrackerGraph(
	ctx context.Context,
	workDir string,
	roots []string,
	forceBareRoot bool,
) error {
	desiredRoot, desiredTrackers := m.buildWorkspaceTrackerGraph(ctx, workDir, roots, true, forceBareRoot)
	oldRoot, oldTrackers := m.snapshotTrackers()
	oldByIdentity := liveRepositoryTrackersByIdentity(oldTrackers)

	root := desiredRoot
	rootReused := canReuseRootTracker(oldRoot, desiredRoot)
	if rootReused {
		syncTrackerConfiguration(oldRoot, desiredRoot, roots)
		m.prepareTrackerComparisonTarget(oldRoot, false)
		desiredRoot.Stop()
		root = oldRoot
	}

	retained := make([]*WorkspaceTracker, 0, len(desiredTrackers))
	added := make([]*WorkspaceTracker, 0, len(desiredTrackers)+1)
	used := make(map[*WorkspaceTracker]struct{}, len(desiredTrackers))
	for _, desired := range desiredTrackers {
		identity := repositoryTrackerIdentity(desired.RepositoryName(), desired.workDir)
		if existing := oldByIdentity[identity]; existing != nil {
			syncTrackerConfiguration(existing, desired, roots)
			m.prepareTrackerComparisonTarget(existing, false)
			desired.Stop()
			retained = append(retained, existing)
			used[existing] = struct{}{}
			continue
		}
		desired.Start(ctx)
		retained = append(retained, desired)
		added = append(added, desired)
	}
	if !rootReused {
		root.Start(ctx)
		added = append(added, root)
	}

	retiredRoot, retired := splitRetiredWorkspaceTrackers(root, oldRoot, oldTrackers, used)
	m.installWorkspaceTrackerGraph(root, retained, workDir, roots, added, retiredRoot, retired)
	return nil
}

func (m *Manager) replaceWorkspaceTrackerGraph(
	ctx context.Context,
	workDir string,
	roots []string,
) error {
	desiredRoot, desiredTrackers := m.buildWorkspaceTrackerGraph(ctx, workDir, roots, true, true)
	oldRoot, oldTrackers := m.snapshotTrackers()
	desiredRoot.Start(ctx)
	for _, tracker := range desiredTrackers {
		tracker.Start(ctx)
	}
	all := append([]*WorkspaceTracker{desiredRoot}, desiredTrackers...)
	retiredRoot, retired := splitRetiredWorkspaceTrackers(desiredRoot, oldRoot, oldTrackers, nil)
	m.installWorkspaceTrackerGraph(desiredRoot, desiredTrackers, workDir, roots, all, retiredRoot, retired)
	return nil
}

// splitRetiredWorkspaceTrackers separates the previous graph into trackers
// that are leaving it. Reused trackers stay in the published graph and must not
// be detached or stopped.
func splitRetiredWorkspaceTrackers(
	root, oldRoot *WorkspaceTracker,
	oldTrackers []*WorkspaceTracker,
	used map[*WorkspaceTracker]struct{},
) (*WorkspaceTracker, []*WorkspaceTracker) {
	var retiredRoot *WorkspaceTracker
	if oldRoot != nil && oldRoot != root {
		retiredRoot = oldRoot
	}
	retired := make([]*WorkspaceTracker, 0, len(oldTrackers))
	for _, old := range oldTrackers {
		if _, keep := used[old]; keep {
			continue
		}
		retired = append(retired, old)
	}
	return retiredRoot, retired
}

// liveRepositoryTrackersByIdentity indexes only trackers whose lifetime is
// still open. A stopped tracker keeps its old identity fields but cannot serve
// reads or polling, so it must not authorize reuse.
func liveRepositoryTrackersByIdentity(trackers []*WorkspaceTracker) map[repositoryTrackerKey]*WorkspaceTracker {
	byIdentity := make(map[repositoryTrackerKey]*WorkspaceTracker, len(trackers))
	for _, tracker := range trackers {
		if !tracker.live() {
			continue
		}
		byIdentity[repositoryTrackerIdentity(tracker.RepositoryName(), tracker.workDir)] = tracker
	}
	return byIdentity
}

func canReuseRootTracker(oldRoot, desiredRoot *WorkspaceTracker) bool {
	return oldRoot.live() &&
		repositoryTrackerIdentity(oldRoot.RepositoryName(), oldRoot.workDir) ==
			repositoryTrackerIdentity(desiredRoot.RepositoryName(), desiredRoot.workDir) &&
		oldRoot.suppressGitPublication == desiredRoot.suppressGitPublication
}

// installWorkspaceTrackerGraph publishes a replacement graph and moves
// workspace-stream subscribers from the retired trackers to the new ones as a
// single ownership handoff under streamSubscribersMu. Subscribe, unsubscribe
// and graph publication therefore cannot interleave: a retired tracker is
// detached from every channel it actually holds before any concurrent
// unsubscribe may close that channel, including a subscriber already removed
// from the manager registry during the transition.
//
// Tracker Stop and waits happen after the handoff lock is released. The caller
// starts new trackers before entry; no Git work runs here.
func (m *Manager) installWorkspaceTrackerGraph(
	root *WorkspaceTracker,
	trackers []*WorkspaceTracker,
	workDir string,
	roots []string,
	started []*WorkspaceTracker,
	retiredRoot *WorkspaceTracker,
	retired []*WorkspaceTracker,
) {
	m.streamSubscribersMu.Lock()
	m.publishWorkspaceTrackerGraph(root, trackers, workDir, roots, started...)
	if m.workspaceGraphAfterPublish != nil {
		m.workspaceGraphAfterPublish()
	}
	for _, tracker := range started {
		if tracker == nil {
			continue
		}
		for sub := range m.streamSubscribers {
			tracker.AttachWorkspaceStreamSubscriber(sub)
		}
	}
	if retiredRoot != nil {
		retiredRoot.detachAllWorkspaceStreamSubscribers()
	}
	for _, old := range retired {
		old.detachAllWorkspaceStreamSubscribers()
	}
	m.streamSubscribersMu.Unlock()

	if m.processRunner != nil {
		m.processRunner.setWorkspaceTracker(root)
	}
	if retiredRoot != nil {
		retiredRoot.Stop()
	}
	for _, old := range retired {
		old.Stop()
	}
}

// publishWorkspaceTrackerGraph swaps the root and repository tracker set under
// the graph lock and stamps the new trackers with the effective polling mode.
// Tracker start, Stop, Git work and subscriber callbacks stay outside it.
func (m *Manager) publishWorkspaceTrackerGraph(
	root *WorkspaceTracker,
	trackers []*WorkspaceTracker,
	workDir string,
	roots []string,
	started ...*WorkspaceTracker,
) {
	m.repoTrackersMu.Lock()
	m.workspaceTracker = root
	m.repoTrackers = trackers
	m.setWorkspaceWorkDirLocked(workDir)
	m.workspaceSourceRoots = append([]string(nil), roots...)
	m.cfg.WorkspaceSourceRoots = append([]string(nil), roots...)
	m.applyWorkspacePollModeLocked(started...)
	m.repoTrackersMu.Unlock()
}

func syncTrackerConfiguration(target, source *WorkspaceTracker, roots []string) {
	target.SetAllowedSourceRoots(roots)
	targetComparison := source.ComparisonTargetSnapshot()
	sourceComparison := source.ComparisonResolution()
	if targetComparison != nil && sourceComparison.Status == comparisonTargetStatusReady && sourceComparison.Ref != "" {
		target.SetComparisonTargetReady(targetComparison, sourceComparison.Ref)
	} else {
		target.SetComparisonTarget(targetComparison)
	}
	if source.IsSubmodule() {
		target.SetComparisonAnchor(source.ComparisonAnchor())
		return
	}
	target.SetBaseBranch(source.BaseBranch())
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
	depth int,
) []*WorkspaceTracker {
	if depth >= maxSubmoduleDiscoveryDepth {
		m.logger.Warn("submodule discovery depth limit reached",
			append([]zap.Field{
				zap.String("repository_name", parent.RepositoryName()),
				zap.Int("depth", depth),
				zap.Int("max_depth", maxSubmoduleDiscoveryDepth),
			}, submoduleDiagnosticFields(nil)...)...)
		return nil
	}
	paths, err := listGitlinkPaths(ctx, parent.workDir, "HEAD")
	if err != nil {
		m.logger.Warn("could not discover submodules",
			append([]zap.Field{
				zap.String("repository_name", parent.RepositoryName()),
				zap.String("path", parent.workDir),
				zap.Error(err),
			}, submoduleDiagnosticFields(err)...)...)
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
					append([]zap.Field{
						zap.String("repository_name", name),
						zap.String("parent_anchor", parentAnchor),
						zap.Error(err),
					}, submoduleDiagnosticFields(err)...)...)
			}
		}
		child.SetComparisonAnchor(anchor)

		trackers = append(trackers, child)
		trackers = append(trackers, m.discoverSubmoduleTrackers(ctx, child, roots, workspaceRoot, depth+1)...)
	}
	return trackers
}

// initializedSubmodulePath resolves a Git-declared child and verifies that it
// is an initialized repository inside the task workspace. Symlinked children
// are rejected so a changed worktree cannot redirect discovery outside the
// authenticated workspace root.
func (m *Manager) initializedSubmodulePath(workspaceRoot, parentPath, relativePath string) (string, bool) {
	if !safeGitRelativePath(relativePath) {
		m.logger.Warn("submodule discovery rejected unsafe path",
			append([]zap.Field{zap.String("path", relativePath)}, submoduleDiagnosticFields(nil)...)...)
		return "", false
	}
	parentRoot, err := os.OpenRoot(parentPath)
	if err != nil {
		m.logger.Warn("submodule discovery could not open parent worktree",
			append([]zap.Field{zap.String("path", parentPath), zap.Error(err)}, submoduleDiagnosticFields(err)...)...)
		return "", false
	}
	defer func() { _ = parentRoot.Close() }()
	// Root.Lstat bounds the Git-declared relative path to the authenticated
	// parent worktree before the absolute path is used by the tracker.
	childInfo, err := parentRoot.Lstat(filepath.FromSlash(relativePath))
	childPath := filepath.Clean(filepath.Join(parentPath, filepath.FromSlash(relativePath)))
	if err != nil || !childInfo.IsDir() || childInfo.Mode()&os.ModeSymlink != 0 {
		m.logger.Warn("submodule discovery rejected child worktree",
			append([]zap.Field{zap.String("path", childPath), zap.Error(err)}, submoduleDiagnosticFields(err)...)...)
		return "", false
	}
	childRoot, ok := m.openVerifiedSubmoduleRoot(parentRoot, childInfo, childPath, relativePath)
	if !ok {
		return "", false
	}
	defer func() { _ = childRoot.Close() }()
	resolvedRoot, err := filepath.EvalSymlinks(workspaceRoot)
	if err != nil {
		m.logger.Warn("submodule discovery could not resolve workspace root",
			append([]zap.Field{zap.String("path", workspaceRoot), zap.Error(err)}, submoduleDiagnosticFields(err)...)...)
		return "", false
	}
	resolvedChild, err := filepath.EvalSymlinks(childPath)
	if err != nil {
		m.logger.Warn("submodule discovery could not resolve child worktree",
			append([]zap.Field{zap.String("path", childPath), zap.Error(err)}, submoduleDiagnosticFields(err)...)...)
		return "", false
	}
	relativeToRoot, err := filepath.Rel(resolvedRoot, resolvedChild)
	if err != nil || pathEscapesRoot(relativeToRoot) {
		m.logger.Warn("submodule discovery rejected child outside workspace root",
			append([]zap.Field{
				zap.String("path", childPath),
				zap.String("workspace_root", workspaceRoot),
				zap.Error(err),
			}, submoduleDiagnosticFields(err)...)...)
		return "", false
	}
	if resolveGitIndexPath(childPath) == "" {
		m.logger.Warn("submodule discovery rejected uninitialized child",
			append([]zap.Field{zap.String("path", childPath)}, submoduleDiagnosticFields(nil)...)...)
		return "", false
	}
	currentChildInfo, err := parentRoot.Lstat(filepath.FromSlash(relativePath))
	if err != nil || currentChildInfo.Mode()&os.ModeSymlink != 0 ||
		!os.SameFile(childInfo, currentChildInfo) {
		m.logger.Warn("submodule discovery rejected changed child worktree",
			append([]zap.Field{zap.String("path", childPath), zap.Error(err)}, submoduleDiagnosticFields(err)...)...)
		return "", false
	}
	return childPath, true
}

func (m *Manager) openVerifiedSubmoduleRoot(
	parentRoot *os.Root,
	childInfo os.FileInfo,
	childPath string,
	relativePath string,
) (*os.Root, bool) {
	childRoot, err := parentRoot.OpenRoot(filepath.FromSlash(relativePath))
	if err != nil {
		m.logger.Warn("submodule discovery could not open child worktree",
			append([]zap.Field{zap.String("path", childPath), zap.Error(err)}, submoduleDiagnosticFields(err)...)...)
		return nil, false
	}
	openedChildInfo, err := childRoot.Stat(".")
	if err != nil || !os.SameFile(childInfo, openedChildInfo) {
		_ = childRoot.Close()
		m.logger.Warn("submodule discovery rejected replaced child worktree",
			append([]zap.Field{zap.String("path", childPath), zap.Error(err)}, submoduleDiagnosticFields(err)...)...)
		return nil, false
	}
	if _, err := childRoot.Lstat(".git"); err != nil {
		_ = childRoot.Close()
		m.logger.Warn("submodule discovery rejected child without git entry",
			append([]zap.Field{zap.String("path", childPath), zap.Error(err)}, submoduleDiagnosticFields(err)...)...)
		return nil, false
	}
	return childRoot, true
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
	if !gitObjectIDPattern.MatchString(ref) || !safeGitRelativePath(relativePath) {
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
		if len(fields) == 3 && gitObjectIDPattern.MatchString(fields[2]) {
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
