package process

import (
	"context"
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
func (m *Manager) RescanRepositories(ctx context.Context, newWorkDir string) {
	// Serialize the whole rescan body. Two concurrent calls could otherwise
	// both observe existingTrackers == 0 between the write-lock snapshot
	// and the bootstrap branch, both calling transitionToMultiRepoMode and
	// leaking a duplicate bare-root tracker + duplicate per-repo trackers.
	m.rescanMu.Lock()
	defer m.rescanMu.Unlock()

	// Resolve the candidate workDir and prove it's a readable directory
	// BEFORE committing cfg.WorkDir. If newWorkDir is bogus, leaving the
	// manager pointing at the existing root keeps path-based handlers
	// (vscode, git, files) consistent with the trackers that never moved.
	m.repoTrackersMu.RLock()
	candidate := m.cfg.WorkDir
	m.repoTrackersMu.RUnlock()
	if newWorkDir != "" && newWorkDir != candidate {
		resolved, ok := resolveRescanPath(newWorkDir, candidate)
		if !ok {
			m.logger.Warn("workspace rescan: ignoring invalid work_dir",
				zap.String("work_dir", newWorkDir),
				zap.String("current_work_dir", candidate))
			return
		}
		// resolved is derived from currentWorkDir (trusted manager config),
		// not from newWorkDir, so os.Stat here doesn't see HTTP-supplied
		// input. CodeQL's path-injection trace ends at resolveRescanPath.
		if info, err := os.Stat(resolved); err == nil && info.IsDir() {
			candidate = resolved
		} else {
			m.logger.Warn("workspace rescan: ignoring invalid work_dir",
				zap.String("work_dir", newWorkDir), zap.Error(err))
			return
		}
	}

	// Read existingTrackers under the same write-lock that commits the new
	// cfg.WorkDir so two concurrent rescans don't both observe 0 trackers
	// and double-bootstrap the multi-repo set.
	m.repoTrackersMu.Lock()
	m.cfg.WorkDir = candidate
	workDir := m.cfg.WorkDir
	existingTrackers := len(m.repoTrackers)
	m.repoTrackersMu.Unlock()

	children := scanRepositorySubdirs(workDir)
	subs := m.snapshotSubscribers()

	m.logger.Info("workspace rescan started",
		zap.String("work_dir", workDir),
		zap.Int("children_found", len(children)),
		zap.Int("existing_repo_trackers", existingTrackers),
		zap.Int("subscribers", len(subs)))

	if len(children) < 2 {
		// Nothing to do: a non-multi-repo workspace stays on its single
		// tracker. The legacy preferGitRepoChildIfRootIsBare fallback
		// covers single-repo construct-time setup.
		return
	}

	if existingTrackers == 0 {
		m.transitionToMultiRepoMode(ctx, workDir, children, subs)
		return
	}
	m.appendNewRepoTrackers(ctx, children, subs)
}

// transitionToMultiRepoMode replaces the single-repo workspaceTracker with a
// bare task-root tracker and stands up per-repo trackers for every detected
// child. Used when the agent launched as single-repo and a sibling worktree
// was added afterwards.
func (m *Manager) transitionToMultiRepoMode(ctx context.Context, workDir string, children []repositorySubdir, subs []types.WorkspaceStreamSubscriber) {
	m.logger.Info("transitioning workspace to multi-repo mode",
		zap.String("work_dir", workDir),
		zap.Int("children", len(children)))

	bareRoot := NewWorkspaceTrackerForRepo(workDir, "", m.logger)
	bareRoot.SetBaseBranch(lookupBaseBranch(m.getBaseBranches(), ""))
	bareRoot.Start(ctx)
	for _, sub := range subs {
		bareRoot.AttachWorkspaceStreamSubscriber(sub)
	}

	newRepoTrackers := make([]*WorkspaceTracker, 0, len(children))
	for _, child := range children {
		tracker := NewWorkspaceTrackerForRepo(child.path, child.name, m.logger)
		tracker.SetBaseBranch(lookupBaseBranch(m.getBaseBranches(), child.name))
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
func (m *Manager) appendNewRepoTrackers(ctx context.Context, children []repositorySubdir, subs []types.WorkspaceStreamSubscriber) {
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
		tracker.Start(ctx)
		for _, sub := range subs {
			tracker.AttachWorkspaceStreamSubscriber(sub)
		}
		newTrackers = append(newTrackers, tracker)
	}
	if len(newTrackers) == 0 {
		return
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
	if children := scanRepositorySubdirs(clean); len(children) >= 1 {
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
