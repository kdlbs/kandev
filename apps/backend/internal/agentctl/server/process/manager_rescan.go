package process

import (
	"context"

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
	if newWorkDir != "" && newWorkDir != m.cfg.WorkDir {
		m.cfg.WorkDir = newWorkDir
	}
	workDir := m.cfg.WorkDir

	children := scanRepositorySubdirs(workDir)
	subs := m.snapshotSubscribers()

	m.logger.Info("workspace rescan started",
		zap.String("work_dir", workDir),
		zap.Int("children_found", len(children)),
		zap.Int("existing_repo_trackers", len(m.repoTrackers)),
		zap.Int("subscribers", len(subs)))

	if len(children) < 2 {
		// Nothing to do: a non-multi-repo workspace stays on its single
		// tracker. The legacy preferGitRepoChildIfRootIsBare fallback
		// covers single-repo construct-time setup.
		return
	}

	if len(m.repoTrackers) == 0 {
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

	old := m.workspaceTracker
	if old != nil {
		for _, sub := range subs {
			old.DetachWorkspaceStreamSubscriber(sub)
		}
		old.Stop()
	}

	bareRoot := NewWorkspaceTrackerForRepo(workDir, "", m.logger)
	bareRoot.Start(ctx)
	for _, sub := range subs {
		bareRoot.AttachWorkspaceStreamSubscriber(sub)
	}
	m.workspaceTracker = bareRoot

	for _, child := range children {
		tracker := NewWorkspaceTrackerForRepo(child.path, child.name, m.logger)
		tracker.Start(ctx)
		for _, sub := range subs {
			tracker.AttachWorkspaceStreamSubscriber(sub)
		}
		m.repoTrackers = append(m.repoTrackers, tracker)
	}
}

// appendNewRepoTrackers adds trackers for child subdirs that don't already
// have one. Existing trackers (matched by RepositoryName) are left running
// so their cached git state and subscriber wiring stay intact.
func (m *Manager) appendNewRepoTrackers(ctx context.Context, children []repositorySubdir, subs []types.WorkspaceStreamSubscriber) {
	existing := make(map[string]bool, len(m.repoTrackers))
	for _, t := range m.repoTrackers {
		existing[t.RepositoryName()] = true
	}
	for _, child := range children {
		if existing[child.name] {
			continue
		}
		m.logger.Info("adding per-repo tracker after rescan",
			zap.String("repository_name", child.name),
			zap.String("path", child.path))
		tracker := NewWorkspaceTrackerForRepo(child.path, child.name, m.logger)
		tracker.Start(ctx)
		for _, sub := range subs {
			tracker.AttachWorkspaceStreamSubscriber(sub)
		}
		m.repoTrackers = append(m.repoTrackers, tracker)
	}
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
