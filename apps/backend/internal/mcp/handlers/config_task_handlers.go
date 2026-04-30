package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

func (h *Handlers) handleMoveTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID         string `json:"task_id"`
		WorkflowID     string `json:"workflow_id"`
		WorkflowStepID string `json:"workflow_step_id"`
		Position       int    `json:"position"`
		Prompt         string `json:"prompt"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.WorkflowID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_id is required", nil)
	}
	if req.WorkflowStepID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_step_id is required", nil)
	}
	if req.Prompt == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "prompt is required: provide a hand-off message for the receiving agent", nil)
	}

	// When the call originates from an agent mid-turn (the common case for
	// move_task_kandev), running MoveTask now races on_enter processing against
	// the agent's still-active turn — corrupting session state, evaluating
	// on_turn_complete on the wrong step, and orphaning the queued prompt.
	// Defer instead: queue the prompt and record a pending move so
	// handleAgentReady applies it deterministically when the turn ends.
	session := h.lookupSession(ctx, req.TaskID)
	if session != nil && (session.State == models.TaskSessionStateRunning || session.State == models.TaskSessionStateStarting) {
		// The deferred path requires the message queue: it's where the hand-off
		// prompt sits and where SetPendingMove lives. If neither is available
		// we cannot honor the contract — surface a real error rather than panic
		// or silently drop the prompt.
		if h.messageQueue == nil {
			h.logger.Error("move_task: message queue not configured; cannot defer move from active session",
				zap.String("task_id", req.TaskID), zap.String("session_id", session.ID))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
				"move_task requires message queue support while the source session is active", nil)
		}
		wrapped := "You were moved to this step with the following message: " + req.Prompt
		if err := h.queueMoveTaskPrompt(ctx, req.TaskID, session.ID, wrapped); err != nil {
			h.logger.Error("move_task: failed to queue hand-off prompt",
				zap.String("task_id", req.TaskID), zap.String("session_id", session.ID), zap.Error(err))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
				"failed to queue move_task hand-off prompt", nil)
		}
		h.messageQueue.SetPendingMove(ctx, session.ID, &messagequeue.PendingMove{
			TaskID:         req.TaskID,
			WorkflowID:     req.WorkflowID,
			WorkflowStepID: req.WorkflowStepID,
			Position:       req.Position,
		})
		// Return a task DTO that reflects the post-move state. The actual DB
		// transition happens later in handleAgentReady → applyPendingMove, but
		// the agent's tool contract is "move_task → moved task back". Echoing a
		// pending/deferred shape confuses the agent into retrying or looping;
		// a normal task DTO lets it close out the turn so agent.ready can fire.
		return ws.NewResponse(msg.ID, msg.Action, h.synthesizeMovedTaskDTO(ctx, req.TaskID, req.WorkflowID, req.WorkflowStepID, req.Position))
	}

	// Idle session (e.g. UI-driven move via MCP) — apply immediately.
	// The hand-off prompt is required (validated above), so when there's no
	// session to deliver it on, we'd have to drop it on the floor — which would
	// silently violate the tool's contract. Reject explicitly instead.
	if session == nil {
		h.logger.Warn("move_task: no primary session for task; cannot deliver required hand-off prompt",
			zap.String("task_id", req.TaskID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"move_task requires the task to have an active session so the hand-off prompt can be delivered", nil)
	}
	wrapped := "You were moved to this step with the following message: " + req.Prompt
	if err := h.queueMoveTaskPrompt(ctx, req.TaskID, session.ID, wrapped); err != nil {
		h.logger.Error("move_task: failed to queue hand-off prompt for idle session",
			zap.String("task_id", req.TaskID), zap.String("session_id", session.ID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"failed to queue move_task hand-off prompt", nil)
	}
	result, err := h.taskSvc.MoveTask(ctx, req.TaskID, req.WorkflowID, req.WorkflowStepID, req.Position)
	if err != nil {
		h.logger.Error("failed to move task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to move task", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromTask(result.Task))
}

// synthesizeMovedTaskDTO returns a task DTO with the post-move step/workflow
// values filled in. Used by the deferred-move path so the agent's tool call
// sees a "successful move" response shape, freeing it to end the turn (which
// is what triggers applyPendingMove). If we can't load the task, fall back to
// a minimal map so the call still resolves.
func (h *Handlers) synthesizeMovedTaskDTO(ctx context.Context, taskID, workflowID, workflowStepID string, position int) any {
	task, err := h.taskSvc.GetTask(ctx, taskID)
	if err != nil || task == nil {
		h.logger.Warn("failed to load task for synthetic move response",
			zap.String("task_id", taskID),
			zap.Error(err))
		return map[string]any{
			"id":               taskID,
			"workflow_id":      workflowID,
			"workflow_step_id": workflowStepID,
			"position":         position,
		}
	}
	clone := *task
	clone.WorkflowID = workflowID
	clone.WorkflowStepID = workflowStepID
	clone.Position = position
	return dto.FromTask(&clone)
}

// lookupSession returns the task's primary session, or nil if none exists.
// Errors are logged at warn level — the move can still proceed via the
// immediate path even when session lookup fails.
func (h *Handlers) lookupSession(ctx context.Context, taskID string) *models.TaskSession {
	session, err := h.taskSvc.GetPrimarySession(ctx, taskID)
	if err != nil || session == nil {
		h.logger.Warn("failed to resolve primary session for task",
			zap.String("task_id", taskID),
			zap.Error(err))
		return nil
	}
	return session
}

// queueMoveTaskPrompt enqueues a user-supplied prompt on the task's primary session.
// Returns an error when the queue itself is missing or QueueMessage fails — the
// caller decides whether to fail the whole move (running-session deferred path)
// or proceed (idle path), since a queue failure makes the deferred contract
// impossible to honor.
func (h *Handlers) queueMoveTaskPrompt(ctx context.Context, taskID, sessionID, prompt string) error {
	if h.messageQueue == nil {
		return fmt.Errorf("message queue is unavailable")
	}
	if sessionID == "" {
		return fmt.Errorf("task has no primary session")
	}
	if _, err := h.messageQueue.QueueMessage(ctx, sessionID, taskID, prompt, "", "mcp-move-task", false, nil); err != nil {
		return fmt.Errorf("queue message: %w", err)
	}
	return nil
}

func (h *Handlers) handleDeleteTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	taskID, err := unmarshalStringField(msg.Payload, "task_id")
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if taskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	if err := h.taskSvc.DeleteTask(ctx, taskID); err != nil {
		h.logger.Error("failed to delete task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete task", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"success": true})
}

func (h *Handlers) handleArchiveTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	taskID, err := unmarshalStringField(msg.Payload, "task_id")
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if taskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	if err := h.taskSvc.ArchiveTask(ctx, taskID); err != nil {
		h.logger.Error("failed to archive task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to archive task", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"success": true})
}

func (h *Handlers) handleUpdateTaskState(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.State == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "state is required", nil)
	}
	state := v1.TaskState(req.State)
	switch state {
	case v1.TaskStateTODO, v1.TaskStateCreated, v1.TaskStateScheduling,
		v1.TaskStateInProgress, v1.TaskStateReview, v1.TaskStateBlocked,
		v1.TaskStateWaitingForInput, v1.TaskStateCompleted,
		v1.TaskStateFailed, v1.TaskStateCancelled:
		// valid
	default:
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "invalid task state: "+req.State, nil)
	}

	task, err := h.taskSvc.UpdateTaskState(ctx, req.TaskID, state)
	if err != nil {
		h.logger.Error("failed to update task state", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update task state", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromTask(task))
}
