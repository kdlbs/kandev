package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// RescanRepositories re-discovers the git-worktree children under newWorkDir
// (or cfg.WorkDir when newWorkDir is empty) and reconciles the running set of
// workspace trackers against the result. It exists for the multi-branch
// add-branch flow where a sibling worktree appears on disk AFTER the agent
// has launched — without a rescan, the original tracker set is frozen at
// construct time and the new worktree's file/git events never reach the UI.
//
// Behavior:
//   - When newWorkDir is non-empty and differs from cfg.WorkDir, cfg.WorkDir
//     is updated. The agent's CWD is NOT touched: this controls only what
//     the WORKSPACE trackers monitor, not where the child process runs.
//   - When the manager is in single-repo mode (no repoTrackers, single
//     workspaceTracker bound to a primary worktree) and the rescan finds
//     >= 2 sibling git children, it transitions to multi-repo mode:
//     the existing single-repo tracker is replaced by a bare task-root
//     tracker, and a per-repo tracker is created for each child including
//     the original primary (so its events get tagged with RepositoryName).
//   - When the manager is already in multi-repo mode, only NEW per-repo
//     trackers (children whose RepositoryName isn't currently tracked) are
//     added; stale trackers (children no longer present on disk) are left
//     in place — removal would race with in-flight notifications and the
//     stale tracker's git index path stops emitting anyway.
//   - All new trackers are Start()-ed and attached to every existing
//     workspace-stream subscriber so events flow without re-subscription.
//
// Idempotent: a rescan with no on-disk changes is a no-op.
func (m *Manager) RescanRepositories(ctx context.Context, newWorkDir string) error {
	return m.RescanRepositoriesWithSourceRoots(ctx, newWorkDir, nil)
}

// RescanRepositoriesWithSourceRoots discovers repository children and updates
// their tracker graph using proposed source roots as one serialized operation.
// A nil roots slice retains the existing policy for compatibility callers.
func (m *Manager) RescanRepositoriesWithSourceRoots(ctx context.Context, newWorkDir string, roots []string) error {
	release, err := m.admitStart()
	if err != nil {
		m.logger.Debug("workspace rescan rejected during teardown")
		return fmt.Errorf("workspace rescan rejected during teardown: %w", err)
	}
	defer release()
	// Serialize the whole rescan body. Two concurrent calls could otherwise
	// both observe existingTrackers == 0 between the write-lock snapshot
	// and the bootstrap branch, both calling transitionToMultiRepoMode and
	// leaking a duplicate bare-root tracker + duplicate per-repo trackers.
	m.rescanMu.Lock()
	defer m.rescanMu.Unlock()

	candidate, scopeChanged, err := m.rescanCandidateWorkDir(newWorkDir)
	if err != nil {
		return err
	}

	proposedRoots := roots != nil
	if roots == nil {
		roots = m.currentWorkspaceSourceRoots()
	} else {
		roots = canonicalWorkspaceSourceRoots(roots)
	}

	// Snapshot existing trackers while rescanMu serializes the graph mutation.
	// cfg.WorkDir and roots are committed only with the resulting graph below.
	m.repoTrackersMu.RLock()
	workDir := candidate
	existingTrackers := len(m.repoTrackers)
	trackers := append([]*WorkspaceTracker{m.workspaceTracker}, m.repoTrackers...)
	m.repoTrackersMu.RUnlock()
	for _, tracker := range trackers {
		if tracker != nil {
			tracker.SetAllowedSourceRoots(roots)
		}
	}

	children := scanRepositorySubdirs(workDir, roots)
	subs := m.snapshotSubscribers()

	m.logger.Info("workspace rescan started",
		zap.String("work_dir", workDir),
		zap.Int("children_found", len(children)),
		zap.Int("existing_repo_trackers", existingTrackers),
		zap.Int("subscribers", len(subs)))

	if candidate == "" {
		m.commitRescanWorkspaceState(workDir, roots)
		return nil
	}
	// A promoted root must always replace the old single-repo file tracker,
	// even when it contains just one git repository plus plain linked folders.
	if len(children) < 2 && !scopeChanged && newWorkDir != "" {
		// Nothing to do: a non-multi-repo workspace stays on its single
		// tracker. The legacy preferGitRepoChildIfRootIsBare fallback
		// covers single-repo construct-time setup.
		m.commitRescanWorkspaceState(workDir, roots)
		return nil
	}

	if existingTrackers == 0 && scopeChanged {
		m.transitionToMultiRepoMode(ctx, workDir, children, roots, subs)
		return nil
	}
	if proposedRoots {
		m.reconcileRepoTrackers(ctx, workDir, children, roots, subs)
		return nil
	}
	m.appendNewRepoTrackers(ctx, workDir, children, roots, subs)
	return nil
}

// rescanCandidateWorkDir resolves and validates a proposed tracking root
// without committing it, so a failed rescan retains the active graph.
func (m *Manager) rescanCandidateWorkDir(newWorkDir string) (string, bool, error) {
	m.repoTrackersMu.RLock()
	candidate := m.cfg.WorkDir
	m.repoTrackersMu.RUnlock()
	if newWorkDir == "" || newWorkDir == candidate {
		return candidate, false, nil
	}
	resolved, ok := resolveRescanPath(newWorkDir, candidate)
	if !ok {
		m.logger.Warn("workspace rescan: ignoring invalid work_dir",
			zap.String("work_dir", newWorkDir),
			zap.String("current_work_dir", candidate))
		return "", false, fmt.Errorf("invalid workspace work_dir: %s", newWorkDir)
	}
	// resolved is derived from currentWorkDir (trusted manager config),
	// not from newWorkDir, so os.Stat here doesn't see HTTP-supplied input.
	// CodeQL's path-injection trace ends at resolveRescanPath.
	if info, err := os.Stat(resolved); err == nil && info.IsDir() {
		return resolved, true, nil
	} else {
		m.logger.Warn("workspace rescan: ignoring invalid work_dir",
			zap.String("work_dir", newWorkDir), zap.Error(err))
		return "", false, fmt.Errorf("workspace work_dir is inaccessible: %s", newWorkDir)
	}
}

func (m *Manager) commitRescanWorkspaceState(workDir string, roots []string) {
	m.repoTrackersMu.Lock()
	m.cfg.WorkDir = workDir
	m.workspaceSourceRoots = append([]string(nil), roots...)
	m.cfg.WorkspaceSourceRoots = append([]string(nil), roots...)
	m.repoTrackersMu.Unlock()
}

// ReconcileRepositories makes the current-root repository tracker set exact.
// It is reserved for rollback after a materialized checkout has been removed:
// unlike RescanRepositories, it stops and removes trackers whose repository
// directories no longer exist. Existing trackers remain in place so their
// workspace-stream subscriptions and cached state are preserved.
func (m *Manager) ReconcileRepositories(ctx context.Context) error {
	release, err := m.admitStart()
	if err != nil {
		return fmt.Errorf("workspace reconcile rejected during teardown: %w", err)
	}
	defer release()
	m.rescanMu.Lock()
	defer m.rescanMu.Unlock()

	m.repoTrackersMu.RLock()
	workDir := m.cfg.WorkDir
	m.repoTrackersMu.RUnlock()
	if workDir == "" {
		return fmt.Errorf("workspace work_dir is required")
	}
	info, err := os.Stat(workDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("workspace work_dir is inaccessible: %s", workDir)
	}

	roots := m.currentWorkspaceSourceRoots()
	children := scanRepositorySubdirs(workDir, roots)
	subs := m.snapshotSubscribers()
	m.reconcileRepoTrackers(ctx, workDir, children, roots, subs)
	return nil
}

// RebindWorkspace replaces the complete workspace tracker graph after the
// owning host process has been stopped. Unlike RescanRepositories it never
// retains a tracker from the previous root: retaining one would keep file and
// git events scoped to a workspace the agent no longer executes in. Calling it
// again with the previous root is the rollback operation used by lifecycle.
func (m *Manager) RebindWorkspace(ctx context.Context, workDir string) error {
	return m.RebindWorkspaceWithSourceRoots(ctx, workDir, nil)
}

// RebindWorkspaceWithSourceRoots replaces the complete workspace tracker graph
// and installs proposed source roots before discovering linked repositories.
// A nil roots slice retains the current policy for compatibility callers.
func (m *Manager) RebindWorkspaceWithSourceRoots(ctx context.Context, workDir string, roots []string) error {
	if workDir == "" || !filepath.IsAbs(workDir) {
		return fmt.Errorf("workspace work_dir must be an absolute path")
	}
	resolved := filepath.Clean(workDir)
	// codeql[go/path-injection] Rebind accepts an authenticated absolute workspace root and rejects inaccessible directories.
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("workspace work_dir is inaccessible: %s", workDir)
	}
	release, err := m.admitStart()
	if err != nil {
		return fmt.Errorf("workspace rebind rejected during teardown: %w", err)
	}
	defer release()
	m.rescanMu.Lock()
	defer m.rescanMu.Unlock()

	if roots == nil {
		roots = m.currentWorkspaceSourceRoots()
	} else {
		roots = canonicalWorkspaceSourceRoots(roots)
	}
	subs := m.snapshotSubscribers()
	children := scanRepositorySubdirs(resolved, roots)
	bare := NewWorkspaceTrackerForRepo(resolved, "", m.logger)
	bare.SetBaseBranch(lookupBaseBranch(m.getBaseBranches(), ""))
	bare.SetAllowedSourceRoots(roots)
	bare.Start(ctx)
	for _, sub := range subs {
		bare.AttachWorkspaceStreamSubscriber(sub)
	}
	repos := make([]*WorkspaceTracker, 0, len(children))
	for _, child := range children {
		tracker := NewWorkspaceTrackerForRepo(child.path, child.name, m.logger)
		tracker.SetBaseBranch(lookupBaseBranch(m.getBaseBranches(), child.name))
		tracker.SetAllowedSourceRoots(roots)
		tracker.Start(ctx)
		for _, sub := range subs {
			tracker.AttachWorkspaceStreamSubscriber(sub)
		}
		repos = append(repos, tracker)
	}

	m.repoTrackersMu.Lock()
	oldBare, oldRepos := m.workspaceTracker, m.repoTrackers
	m.cfg.WorkDir, m.workspaceTracker, m.repoTrackers, m.workspaceSourceRoots = resolved, bare, repos, append([]string(nil), roots...)
	m.cfg.WorkspaceSourceRoots = append([]string(nil), roots...)
	m.repoTrackersMu.Unlock()
	if oldBare != nil {
		for _, sub := range subs {
			oldBare.DetachWorkspaceStreamSubscriber(sub)
		}
		oldBare.Stop()
	}
	for _, tracker := range oldRepos {
		for _, sub := range subs {
			tracker.DetachWorkspaceStreamSubscriber(sub)
		}
		tracker.Stop()
	}
	return nil
}

// transitionToMultiRepoMode replaces the single-repo workspaceTracker with a
// bare task-root tracker and stands up per-repo trackers for every detected
// child. Used when the agent launched as single-repo and a sibling worktree
// was added afterwards.
func (m *Manager) transitionToMultiRepoMode(ctx context.Context, workDir string, children []repositorySubdir, roots []string, subs []types.WorkspaceStreamSubscriber) {
	m.logger.Info("transitioning workspace to multi-repo mode",
		zap.String("work_dir", workDir),
		zap.Int("children", len(children)))

	bareRoot := NewWorkspaceTrackerForRepo(workDir, "", m.logger)
	bareRoot.SetBaseBranch(lookupBaseBranch(m.getBaseBranches(), ""))
	bareRoot.SetAllowedSourceRoots(roots)
	bareRoot.Start(ctx)
	for _, sub := range subs {
		bareRoot.AttachWorkspaceStreamSubscriber(sub)
	}

	newRepoTrackers := make([]*WorkspaceTracker, 0, len(children))
	for _, child := range children {
		tracker := NewWorkspaceTrackerForRepo(child.path, child.name, m.logger)
		tracker.SetBaseBranch(lookupBaseBranch(m.getBaseBranches(), child.name))
		tracker.SetAllowedSourceRoots(roots)
		tracker.Start(ctx)
		for _, sub := range subs {
			tracker.AttachWorkspaceStreamSubscriber(sub)
		}
		newRepoTrackers = append(newRepoTrackers, tracker)
	}

	m.repoTrackersMu.Lock()
	old := m.workspaceTracker
	m.workspaceTracker = bareRoot
	m.repoTrackers = append(m.repoTrackers, newRepoTrackers...)
	m.cfg.WorkDir = workDir
	m.workspaceSourceRoots = append([]string(nil), roots...)
	m.cfg.WorkspaceSourceRoots = append([]string(nil), roots...)
	m.repoTrackersMu.Unlock()

	if old != nil {
		for _, sub := range subs {
			old.DetachWorkspaceStreamSubscriber(sub)
		}
		old.Stop()
	}
}

// appendNewRepoTrackers adds trackers for child subdirs that don't already
// have one. Existing trackers (matched by RepositoryName) are left running
// so their cached git state and subscriber wiring stay intact.
func (m *Manager) appendNewRepoTrackers(ctx context.Context, workDir string, children []repositorySubdir, roots []string, subs []types.WorkspaceStreamSubscriber) {
	m.repoTrackersMu.RLock()
	existing := make(map[string]bool, len(m.repoTrackers))
	for _, t := range m.repoTrackers {
		existing[t.RepositoryName()] = true
	}
	m.repoTrackersMu.RUnlock()

	var newTrackers []*WorkspaceTracker
	for _, child := range children {
		if existing[child.name] {
			continue
		}
		m.logger.Info("adding per-repo tracker after rescan",
			zap.String("repository_name", child.name),
			zap.String("path", child.path))
		tracker := NewWorkspaceTrackerForRepo(child.path, child.name, m.logger)
		tracker.SetBaseBranch(lookupBaseBranch(m.getBaseBranches(), child.name))
		tracker.SetAllowedSourceRoots(roots)
		tracker.Start(ctx)
		for _, sub := range subs {
			tracker.AttachWorkspaceStreamSubscriber(sub)
		}
		newTrackers = append(newTrackers, tracker)
	}
	// Re-check membership inside the write-lock as a defense-in-depth
	// guard. rescanMu already serializes RescanRepositories callers, but
	// the explicit check here makes the invariant local: any tracker
	// already in the slice by name is dropped before append, so even if
	// the invariant moved, duplicates would still be rejected.
	m.repoTrackersMu.Lock()
	stillExisting := make(map[string]bool, len(m.repoTrackers))
	for _, t := range m.repoTrackers {
		stillExisting[t.RepositoryName()] = true
	}
	var dropped []*WorkspaceTracker
	for _, t := range newTrackers {
		if stillExisting[t.RepositoryName()] {
			dropped = append(dropped, t)
			continue
		}
		m.repoTrackers = append(m.repoTrackers, t)
	}
	m.cfg.WorkDir = workDir
	m.workspaceSourceRoots = append([]string(nil), roots...)
	m.cfg.WorkspaceSourceRoots = append([]string(nil), roots...)
	m.repoTrackersMu.Unlock()
	// Stop + detach any dropped trackers outside the lock so we don't block
	// readers on potentially-slow Stop() teardown.
	for _, t := range dropped {
		for _, sub := range subs {
			t.DetachWorkspaceStreamSubscriber(sub)
		}
		t.Stop()
	}
}

func (m *Manager) reconcileRepoTrackers(ctx context.Context, workDir string, children []repositorySubdir, roots []string, subs []types.WorkspaceStreamSubscriber) {
	wanted := make(map[repositoryTrackerKey]struct{}, len(children))
	for _, child := range children {
		wanted[repositoryTrackerIdentity(child.name, child.path)] = struct{}{}
	}
	m.repoTrackersMu.Lock()
	retained := make([]*WorkspaceTracker, 0, len(m.repoTrackers))
	removed := make([]*WorkspaceTracker, 0)
	for _, tracker := range m.repoTrackers {
		identity := repositoryTrackerIdentity(tracker.RepositoryName(), tracker.workDir)
		if _, ok := wanted[identity]; ok {
			retained = append(retained, tracker)
			delete(wanted, identity)
			continue
		}
		removed = append(removed, tracker)
	}
	m.repoTrackersMu.Unlock()
	newTrackers := make([]*WorkspaceTracker, 0, len(wanted))
	for _, child := range children {
		if _, needed := wanted[repositoryTrackerIdentity(child.name, child.path)]; !needed {
			continue
		}
		tracker := NewWorkspaceTrackerForRepo(child.path, child.name, m.logger)
		tracker.SetBaseBranch(lookupBaseBranch(m.getBaseBranches(), child.name))
		tracker.SetAllowedSourceRoots(roots)
		tracker.Start(ctx)
		for _, sub := range subs {
			tracker.AttachWorkspaceStreamSubscriber(sub)
		}
		newTrackers = append(newTrackers, tracker)
	}
	m.repoTrackersMu.Lock()
	retained = append(retained, newTrackers...)
	m.repoTrackers = retained
	m.cfg.WorkDir = workDir
	m.workspaceSourceRoots = append([]string(nil), roots...)
	m.cfg.WorkspaceSourceRoots = append([]string(nil), roots...)
	m.repoTrackersMu.Unlock()

	for _, tracker := range removed {
		for _, sub := range subs {
			tracker.DetachWorkspaceStreamSubscriber(sub)
		}
		tracker.Stop()
	}
}

type repositoryTrackerKey struct {
	name string
	path string
}

func repositoryTrackerIdentity(name, path string) repositoryTrackerKey {
	return repositoryTrackerKey{name: name, path: filepath.Clean(path)}
}

// resolveRescanPath maps an externally-supplied workspace path to a known-good
// path. The legitimate caller (kandev backend's branch materializer) promotes
// the workdir to the task root that contains the per-repo worktrees as
// siblings. Allowed transitions are:
//   - newPath equals currentWorkDir   → no-op (return current)
//   - newPath equals parent of current → return derived parent
//   - newPath is a different absolute directory that actually holds >=1 git
//     repo subdir → return cleaned newPath (recovery path: covers envs whose
//     workspace_path landed on the source repo's local_path instead of the
//     primary worktree, so the parent-only check would otherwise refuse to
//     ever switch the manager away from the wrong root)
//
// The third branch reintroduces the HTTP-supplied path as a Stat sink, but
// the endpoint is already authenticated via the bearer-token middleware and
// the manager verifies the path resolves to a real directory before
// committing — taint here is gated by auth, not path-shape.
//
// Returns ("", false) for any other input — first-launch case (currentWorkDir
// empty) is handled by the caller falling back to the existing workdir.
func resolveRescanPath(newPath, currentWorkDir string) (string, bool) {
	if newPath == "" {
		return "", false
	}
	clean := filepath.Clean(newPath)
	if !filepath.IsAbs(clean) {
		return "", false
	}
	if currentWorkDir != "" {
		currentClean := filepath.Clean(currentWorkDir)
		if clean == currentClean {
			return currentClean, true
		}
		parent := filepath.Dir(currentClean)
		if parent != currentClean && clean == parent {
			return parent, true
		}
	}
	// Recovery path: accept any absolute directory that actually contains git
	// repo subdirs. scanRepositorySubdirs reads the directory and validates
	// each child has a working .git entry, so a hostile or empty path returns
	// nil and the rescan stays a no-op below.
	if children := scanRepositorySubdirs(clean, nil); len(children) >= 1 {
		return clean, true
	}
	return "", false
}

// snapshotSubscribers returns a copy of the current workspace-stream
// subscribers so rescan callers can attach new trackers without holding the
// subscriber lock during git-status replays.
func (m *Manager) snapshotSubscribers() []types.WorkspaceStreamSubscriber {
	m.streamSubscribersMu.Lock()
	defer m.streamSubscribersMu.Unlock()
	out := make([]types.WorkspaceStreamSubscriber, 0, len(m.streamSubscribers))
	for s := range m.streamSubscribers {
		out = append(out, s)
	}
	return out
}
