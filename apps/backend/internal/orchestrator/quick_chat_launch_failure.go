package orchestrator

import (
	"context"
	"errors"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Newly allocated Quick Chats retain admission failures through the same typed
// failure transition as executor startup. Existing sessions and capacity
// deferrals keep their current owner and lifecycle.
func (s *Service) handleQuickChatStartFailure(ctx context.Context, task *v1.Task, sessionID string, created bool, launchErr error) error {
	if !task.IsEphemeral || !created || sessionID == "" || errors.Is(launchErr, ErrCeilingLaunchDeferred) {
		return launchErr
	}
	s.clearTransientRetryState(sessionID)
	return s.executor.RecordEarlyLaunchFailure(ctx, task.ID, sessionID, launchErr)
}
