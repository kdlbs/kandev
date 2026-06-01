package process

import (
	"context"
	"os"
	"path/filepath"
	"strings"

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
	// Resolve the candidate workDir and prove it's a readable directory
	// BEFORE committing cfg.WorkDir. If newWorkDir is bogus, leaving the
	// manager pointing at the existing root keeps path-based handlers
	// (vscode, git, files) consistent with the trackers that never moved.
	m.repoTrackersMu.RLock()
	candidate := m.cfg.WorkDir
	if newWorkDir != "" && newWorkDir != candidate {
		validated, ok := validateRescanPath(newWorkDir, candidate)
		if !ok {
			m.repoTrackersMu.RUnlock()
			m.logger.Warn("workspace rescan: ignoring invalid work_dir",
				zap.String("work_dir", newWorkDir),
				zap.String("current_work_dir", candidate))
			return
		}
		if info, err := os.Stat(validated); err == nil && info.IsDir() {
			candidate = validated
		} else {
			m.repoTrackersMu.RUnlock()
			m.logger.Warn("workspace rescan: ignoring invalid work_dir",
				zap.String("work_dir", newWorkDir), zap.Error(err))
			return
		}
	}
	existingTrackers := len(m.repoTrackers)
	m.repoTrackersMu.RUnlock()

	m.repoTrackersMu.Lock()
	m.cfg.WorkDir = candidate
	workDir := m.cfg.WorkDir
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
	bareRoot.Start(ctx)
	for _, sub := range subs {
		bareRoot.AttachWorkspaceStreamSubscriber(sub)
	}

	newRepoTrackers := make([]*WorkspaceTracker, 0, len(children))
	for _, child := range children {
		tracker := NewWorkspaceTrackerForRepo(child.path, child.name, m.logger)
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
		tracker.Start(ctx)
		for _, sub := range subs {
			tracker.AttachWorkspaceStreamSubscriber(sub)
		}
		newTrackers = append(newTrackers, tracker)
	}
	if len(newTrackers) == 0 {
		return
	}
	m.repoTrackersMu.Lock()
	m.repoTrackers = append(m.repoTrackers, newTrackers...)
	m.repoTrackersMu.Unlock()
}

// validateRescanPath sanitizes an externally-supplied workspace path so the
// agentctl rescan endpoint can't be coerced into watching an arbitrary host
// directory. The legitimate caller (kandev backend's branch materializer)
// only promotes the workdir from a child worktree to the task root, so the
// new path must be the same as, or an ancestor of, the current workdir.
//
// Path-injection defense applies even though agentctl binds to localhost:
// any process colocated with agentctl could otherwise direct it at /etc or
// the user's home directory by POSTing /api/v1/workspace/rescan.
//
// Returns the cleaned absolute path on success, or ("", false) on rejection.
func validateRescanPath(newPath, currentWorkDir string) (string, bool) {
	if newPath == "" {
		return "", false
	}
	clean := filepath.Clean(newPath)
	if !filepath.IsAbs(clean) {
		return "", false
	}
	if strings.Contains(clean, "..") {
		return "", false
	}
	if currentWorkDir == "" {
		// No anchor to compare against — accept the cleaned absolute path.
		// This only fires at first launch before cfg.WorkDir is set.
		return clean, true
	}
	currentClean := filepath.Clean(currentWorkDir)
	if clean == currentClean {
		return clean, true
	}
	// Allow promotion to an ancestor (sibling-layout multi-branch case:
	// <task>/<repo>/ ⇒ <task>/). Reject anything else.
	rel, err := filepath.Rel(clean, currentClean)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return clean, true
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
