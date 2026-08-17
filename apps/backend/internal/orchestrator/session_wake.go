package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

const sessionWakeCoalescePrefix = "session-wake:"

// DeliverSessionWake persists one server-owned wake prompt. A busy session is
// left untouched with its wake queued; an idle session uses the existing
// guarded FIFO drain path to begin the next turn without an interrupt.
func (s *Service) DeliverSessionWake(ctx context.Context, taskID, sessionID, wakeID, prompt string) (string, error) {
	if s.messageQueue == nil {
		return "", errors.New("message queue is not configured")
	}
	if taskID == "" || sessionID == "" || wakeID == "" {
		return "", fmt.Errorf("task_id, session_id, and wake_id are required")
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID {
		return "", fmt.Errorf("session does not belong to task")
	}
	_, _, err = s.messageQueue.QueueMessageWithCoalesceKey(ctx, sessionID, taskID, prompt, "", messagequeue.QueuedByServer, false, nil, nil, sessionWakeCoalescePrefix+wakeID, true)
	if err != nil {
		return "", fmt.Errorf("queue session wake: %w", err)
	}
	if dispatched, err := s.DrainQueuedMessage(ctx, sessionID); err == nil && dispatched {
		return "sent", nil
	}
	return "queued", nil
}
