package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

// resetNativeConversation preserves Kandev's visible session history while
// resetting provider context before a new coordinator turn. The core run queue
// serializes turns; active sessions are rejected here too.
func (s *Service) resetNativeConversation(ctx context.Context, task *models.Task) error {
	if native, _ := task.Metadata["native_conversation"].(bool); !native {
		return nil
	}
	session, err := s.repo.GetTaskSessionByTaskAndAgent(ctx, task.ID, task.AssigneeAgentProfileID)
	if err != nil || session == nil {
		return err
	}
	switch session.State {
	case models.TaskSessionStateCreated, models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed, models.TaskSessionStateCancelled:
		return s.repo.SetSessionMetadataKey(ctx, session.ID, "acp_session_id", "")
	case models.TaskSessionStateIdle, models.TaskSessionStateWaitingForInput:
		if !s.resetAgentContext(ctx, task.ID, session, "native conversation") {
			return fmt.Errorf("could not reset native conversation context")
		}
		// A cold executor has no live context to reset; clear its resume token
		// as well so the next launch cannot restore the old provider history.
		return s.repo.SetSessionMetadataKey(ctx, session.ID, "acp_session_id", "")
	default:
		return fmt.Errorf("conversation is already active: %s", session.State)
	}
}
