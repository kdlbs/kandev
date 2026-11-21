package lifecycle

import (
	"context"
	"fmt"
)

// InspectSessionWorkspaceInfo reads and validates the same canonical workspace
// as a workspace-only restore, without repairing inventory or starting a runtime.
func (m *Manager) InspectSessionWorkspaceInfo(ctx context.Context, taskID, sessionID string) (*WorkspaceInfo, error) {
	if m == nil || m.workspaceInfoProvider == nil {
		return nil, fmt.Errorf("workspace information is unavailable")
	}
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if err := m.CheckSessionExecAccess(ctx, sessionID); err != nil {
		return nil, err
	}
	info, err := m.workspaceInfoProvider.GetWorkspaceInfoForSession(ctx, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, ErrSessionWorkspaceNotReady
	}
	if err := m.ensureWorkspaceSessionAdmitted(ctx, taskID, info); err != nil {
		return nil, err
	}
	if err := validateWorkspaceInfoForExecution(ctx, info); err != nil {
		return nil, err
	}
	view := *info
	view.WorkspaceRepositories = append([]WorkspaceRepositorySpec(nil), info.WorkspaceRepositories...)
	for i, repository := range view.WorkspaceRepositories {
		view.WorkspaceRepositories[i].WorktreePath = executionWorkspaceRepositoryCandidate(ctx, info, i, repository)
	}
	return &view, nil
}
