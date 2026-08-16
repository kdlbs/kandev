package handlers

import (
	"context"
	"errors"

	"go.uber.org/zap"

	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// applyDeferredLaunchPromptUpdate rewrites the prompt a not-yet-started task
// will launch with. It returns a ready-to-send WS error on failure and nil on
// success, so update_task_kandev can abort before applying its other fields.
//
// The rejections are spelled out rather than collapsed into one message: "it
// already started" and "it never had a deferred launch" call for different next
// steps from the caller — message the running session versus check the task ID.
func (h *Handlers) applyDeferredLaunchPromptUpdate(
	ctx context.Context, msg *ws.Message, taskID, prompt string,
) *ws.Message {
	if _, err := h.taskSvc.UpdateDeferredLaunchPrompt(ctx, taskID, prompt); err != nil {
		switch {
		case errors.Is(err, service.ErrDeferredLaunchPromptEmpty):
			return wsError(msg.ID, msg.Action, ws.ErrorCodeValidation,
				"deferred_launch_prompt must not be empty")
		case errors.Is(err, service.ErrDeferredLaunchAlreadyStarted):
			return wsError(msg.ID, msg.Action, ws.ErrorCodeConflict,
				"task has already started, so its deferred launch prompt no longer applies — "+
					"send the new context with message_task_kandev instead")
		case errors.Is(err, service.ErrNoPendingDeferredLaunch):
			return wsError(msg.ID, msg.Action, ws.ErrorCodeConflict,
				"task has no pending deferred launch to update — it was never created with "+
					"blocked_by + start_agent, or its launch has already fired")
		case errors.Is(err, taskrepo.ErrTaskNotFound):
			return wsError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "task not found")
		}
		h.logger.Error("failed to update deferred launch prompt",
			zap.String("task_id", taskID), zap.Error(err))
		return wsError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"Failed to update deferred launch prompt")
	}
	return nil
}
