package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
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
	if err != nil || session == nil {
		return models.SessionWakeDeliveryTerminal, models.NewSessionWakeTerminalDeliveryError("session was deleted or cannot be found")
	}
	if session.TaskID != taskID {
		return models.SessionWakeDeliveryTerminal, models.NewSessionWakeTerminalDeliveryError("session does not belong to task")
	}
	if isTerminalSessionState(session.State) {
		return models.SessionWakeDeliveryTerminal, models.NewSessionWakeTerminalDeliveryError(fmt.Sprintf("session %s is %s", session.ID, session.State))
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
