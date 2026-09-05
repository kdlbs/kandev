package lifecycle

import (
	"context"
	"fmt"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// PublishWorkspacePreview forwards a current HTML buffer to the agentctl
// instance owned by one task session.
func (m *Manager) PublishWorkspacePreview(
	ctx context.Context,
	sessionID string,
	payload agentctl.WorkspacePreviewRequest,
) (agentctl.WorkspacePreviewResponse, error) {
	if sessionID == "" {
		return agentctl.WorkspacePreviewResponse{}, fmt.Errorf("session_id is required")
	}
	execution, ok := m.executionStore.GetBySessionID(sessionID)
	if !ok {
		return agentctl.WorkspacePreviewResponse{}, fmt.Errorf("%w: %s", ErrNoExecutionForSession, sessionID)
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return agentctl.WorkspacePreviewResponse{}, fmt.Errorf("agentctl client not available for session %s", sessionID)
	}
	return client.PublishWorkspacePreview(ctx, payload)
}
