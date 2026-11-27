package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/coordinator"
)

// DeliverQueued delivers a prompt to one named session of a task through the
// path message_task_kandev uses, queued behind a running turn and never
// interrupting it. It backs an approved coordinator message and returns the
// session the prompt reached, or coordinator.ErrMessageQueueFull.
func (h *Handlers) DeliverQueued(ctx context.Context, taskID, sessionID, prompt string) (string, error) {
	if h.taskSvc == nil {
		return "", errors.New("task service not available")
	}
	session, err := h.taskSvc.GetTaskSession(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("get session: %w", err)
	}
	if session == nil || session.TaskID != taskID {
		return "", errors.New("session does not belong to the task")
	}
	result, err := h.dispatchTaskMessage(ctx, taskID, session, prompt, nil, false, true)
	if err != nil {
		var qf *queueFullDispatchError
		if errors.As(err, &qf) {
			return "", coordinator.ErrMessageQueueFull
		}
		return "", err
	}
	return result.sessionID, nil
}
