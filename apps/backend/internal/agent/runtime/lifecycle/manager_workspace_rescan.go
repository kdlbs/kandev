package lifecycle

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// MaterializedWorktree describes a worktree just created on disk by the
// branch materializer. Used by NotifyWorktreeMaterialized to push a
// frontend-visible event so the session's worktree tabs and Changes panel
// pick up the new entry without a full page reload.
type MaterializedWorktree struct {
	TaskID         string
	SessionID      string
	WorktreeID     string
	WorktreePath   string
	WorktreeBranch string
	RepositoryID   string
	BranchSlug     string
	// TaskWorkspacePath is the task root after sibling promotion (env's
	// promoted workspace_path). The frontend reads this to repoint the file
	// browser to the task root when the task becomes multi-branch.
	TaskWorkspacePath string
}

// NotifyWorktreeMaterialized publishes an AgentctlReady-shaped event so
// the gateway forwards it to the frontend. The web handler treats
// AgentctlReady as the canonical "session has a new worktree to display"
// signal — setWorktree + setSessionWorktrees fire from it — so reusing
// the same event keeps the frontend store update logic in one place.
//
// Best-effort: a missing event bus or execution-store entry means the
// frontend won't auto-refresh, but the worktree row still exists in the
// DB and the next page reload will pick it up.
func (m *Manager) NotifyWorktreeMaterialized(ctx context.Context, wt MaterializedWorktree) {
	if m.eventPublisher == nil || m.eventPublisher.eventBus == nil {
		return
	}
	execution, _ := m.GetExecutionBySessionID(wt.SessionID)
	execID := ""
	taskEnvID := ""
	if execution != nil {
		execID = execution.ID
		taskEnvID = execution.TaskEnvironmentID
	}
	payload := AgentctlEventPayload{
		TaskID:            wt.TaskID,
		SessionID:         wt.SessionID,
		TaskEnvironmentID: taskEnvID,
		AgentExecutionID:  execID,
		WorktreeID:        wt.WorktreeID,
		WorktreePath:      wt.WorktreePath,
		WorktreeBranch:    wt.WorktreeBranch,
		TaskWorkspacePath: wt.TaskWorkspacePath,
	}
	event := bus.NewEvent(events.AgentctlReady, "branch-materializer", payload)
	if err := m.eventPublisher.eventBus.Publish(ctx, events.AgentctlReady, event); err != nil {
		m.logger.Warn("failed to publish worktree materialized event",
			zap.String("session_id", wt.SessionID),
			zap.String("worktree_id", wt.WorktreeID),
			zap.Error(err))
	}
}

// RescanWorkspaceForSession asks the agentctl instance attached to sessionID
// to re-discover repo subdirs and reconcile its tracker set. Used by the
// kandev backend's branch materializer after creating a sibling worktree
// on disk (multi-branch add_branch flow) so the new worktree's git/file
// events flow to the UI without a session restart.
//
// workDir is optional. When non-empty the agentctl manager updates its
// scope to that path before rescanning — required for the single-to-multi
// transition where workspace_path moved from primary worktree to task
// root. Empty means "rescan whatever you're already tracking".
//
// Looks up the execution from the in-memory store. The executors_running
// table does not carry the live agentctl URL (it's runtime state held by
// the lifecycle manager), so the materializer routes through here instead
// of trying to read the URL from the row.
//
// Best-effort: returns an error but the caller treats it as a warning.
// The worktree still exists on disk; the next session relaunch will pick
// it up via prepareMultiRepo regardless of whether this rescan succeeded.
func (m *Manager) RescanWorkspaceForSession(ctx context.Context, sessionID, workDir string, sourceRoots ...[]string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	execution, ok := m.GetExecutionBySessionID(sessionID)
	if !ok || execution == nil {
		// No active execution for this session — agentctl isn't running, so
		// there's nothing to rescan. The next launch will build trackers
		// against the up-to-date task layout via prepareMultiRepo.
		m.logger.Debug("rescan skipped: no execution for session",
			zap.String("session_id", sessionID))
		return nil
	}
	client := execution.GetAgentCtlClient()
	if client == nil {
		m.logger.Debug("rescan skipped: execution has no agentctl client",
			zap.String("session_id", sessionID),
			zap.String("execution_id", execution.ID))
		return nil
	}
	oldRoots := append([]string(nil), execution.WorkspaceSourceRoots...)
	newRoots := optionalWorkspaceSourceRoots(oldRoots, sourceRoots)
	execution.WorkspaceSourceRoots = newRoots
	if err := client.RescanWorkspace(ctx, workDir, newRoots); err != nil {
		execution.WorkspaceSourceRoots = oldRoots
		return fmt.Errorf("rescan workspace via agentctl: %w", err)
	}
	m.logger.Info("agentctl workspace rescan ok",
		zap.String("session_id", sessionID),
		zap.String("execution_id", execution.ID),
		zap.String("work_dir", workDir))
	return nil
}

func optionalWorkspaceSourceRoots(current []string, roots [][]string) []string {
	if len(roots) == 0 {
		return append([]string(nil), current...)
	}
	return append([]string(nil), roots[0]...)
}
