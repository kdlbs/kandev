package process

import (
	"context"

	"go.uber.org/zap"
)

// renewWorkspaceTrackersForStart replaces every workspace tracker whose
// lifetime a previous Stop terminated before a replacement agent starts on
// this retained process manager. Trackers that are still live keep their
// identity, cache, subscriptions and polling mode, so an ordinary first start
// and a healthy graph cost nothing. Stopped lazy entries are dropped and
// recreated through the ordinary authorized lazy-read path.
//
// The caller holds start admission and startMu, which serializes renewal with
// process teardown. rescanMu and the graph locks keep it serialized with
// rescans, rebinds and subscriber churn.
func (m *Manager) renewWorkspaceTrackersForStart(ctx context.Context) {
	m.rescanMu.Lock()
	defer m.rescanMu.Unlock()

	m.pruneStoppedLazyWorkspaceTrackers()

	root, trackers := m.snapshotTrackers()
	if workspaceTrackerGraphLive(root, trackers) {
		return
	}

	m.repoTrackersMu.RLock()
	workDir := m.cfg.WorkDir
	m.repoTrackersMu.RUnlock()
	// Preserve the current root classification: a bare multi-repository task
	// root and a repository-less task root both own Files without becoming a
	// Git scope, even after every tracker was stopped.
	forceBareRoot := m.hasBareMultiRepoTrackerGraph() || m.hasBareTaskRootTracker(workDir)
	m.logger.Info("renewing stopped workspace trackers before agent start",
		zap.String("work_dir", workDir),
		zap.Bool("force_bare_root", forceBareRoot))
	if err := m.reconcileWorkspaceTrackerGraph(ctx, workDir, m.currentWorkspaceSourceRoots(), forceBareRoot); err != nil {
		m.logger.Warn("workspace tracker renewal failed", zap.Error(err))
	}
}

func workspaceTrackerGraphLive(root *WorkspaceTracker, trackers []*WorkspaceTracker) bool {
	if !root.live() {
		return false
	}
	for _, tracker := range trackers {
		if !tracker.live() {
			return false
		}
	}
	return true
}

// pruneStoppedLazyWorkspaceTrackers drops cached subpath trackers whose
// lifetime ended so the next authorized read rebuilds them. Their polling
// loops were already drained by stopWorkspaceTrackers.
func (m *Manager) pruneStoppedLazyWorkspaceTrackers() {
	m.workspaceTrackersMu.Lock()
	defer m.workspaceTrackersMu.Unlock()
	for subpath, tracker := range m.workspaceTrackersBySubpath {
		if !tracker.live() {
			delete(m.workspaceTrackersBySubpath, subpath)
		}
	}
}
