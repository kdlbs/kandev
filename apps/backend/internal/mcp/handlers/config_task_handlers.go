package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

	// Prompt is OPTIONAL — config-mode/admin moves don't always have an agent
	// to hand off to. When supplied, it activates the deferred-move path that
	// hands the receiving agent a directive on its first turn at the new step.
	// When omitted, we just move the task and return.
	session, lookupErr := h.lookupSession(ctx, req.TaskID)
	if lookupErr != nil {
		// Backend lookup failure is an internal error, not validation — don't
		// collapse it into "you have no session" downstream.
		h.logger.Error("move_task: failed to look up primary session",
			zap.String("task_id", req.TaskID), zap.Error(lookupErr))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"failed to look up task's primary session", nil)
	}

	// Active source session + prompt → deferred path. Running MoveTask now
	// would race on_enter processing against the agent's still-active turn —
	// corrupting session state and orphaning the queued prompt. Defer until
	// handleAgentReady fires applyPendingMove on turn-end.
	if req.Prompt != "" && session != nil &&
		(session.State == models.TaskSessionStateRunning || session.State == models.TaskSessionStateStarting) {
		return h.deferMoveTaskWithPrompt(ctx, msg, req, session)
	}

	// Idle path — apply immediately. If a prompt was supplied, queue it on the
	// session so the receiving agent's next turn picks it up; if not, just move.
	return h.applyMoveTaskImmediate(ctx, msg, req, session)
}

// deferMoveTaskWithPrompt queues the hand-off prompt + records a PendingMove
// for the agent's turn-end handler to apply. Used when the source session is
// active and the caller supplied a prompt. Returns a synthetic moved-task DTO
// so the agent's tool call resolves successfully and ends the turn cleanly.
func (h *Handlers) deferMoveTaskWithPrompt(
	ctx context.Context,
	msg *ws.Message,
	req struct {
		TaskID         string `json:"task_id"`
		WorkflowID     string `json:"workflow_id"`
		WorkflowStepID string `json:"workflow_step_id"`
		Position       int    `json:"position"`
		Prompt         string `json:"prompt"`
	},
	session *models.TaskSession,
) (*ws.Message, error) {
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
	return ws.NewResponse(msg.ID, msg.Action,
		h.synthesizeMovedTaskDTO(ctx, req.TaskID, req.WorkflowID, req.WorkflowStepID, req.Position))
}

// applyMoveTaskImmediate runs the move now, optionally queueing a hand-off
// prompt on the (idle) primary session beforehand. Used when the source
// session is idle, when there's no source session at all, or when no prompt
// was supplied (config-mode/admin moves).
func (h *Handlers) applyMoveTaskImmediate(
	ctx context.Context,
	msg *ws.Message,
	req struct {
		TaskID         string `json:"task_id"`
		WorkflowID     string `json:"workflow_id"`
		WorkflowStepID string `json:"workflow_step_id"`
		Position       int    `json:"position"`
		Prompt         string `json:"prompt"`
	},
	session *models.TaskSession,
) (*ws.Message, error) {
	queuedSessionID := ""
	if req.Prompt != "" && session != nil {
		wrapped := "You were moved to this step with the following message: " + req.Prompt
		if err := h.queueMoveTaskPrompt(ctx, req.TaskID, session.ID, wrapped); err != nil {
			h.logger.Error("move_task: failed to queue hand-off prompt for idle session",
				zap.String("task_id", req.TaskID), zap.String("session_id", session.ID), zap.Error(err))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
				"failed to queue move_task hand-off prompt", nil)
		}
		queuedSessionID = session.ID
	}

	result, err := h.taskSvc.MoveTask(ctx, req.TaskID, req.WorkflowID, req.WorkflowStepID, req.Position)
	if err != nil {
		// Roll back the queued prompt — without this, the next turn would
		// deliver a "You were moved to this step…" message for a transition
		// that didn't actually happen.
		if queuedSessionID != "" && h.messageQueue != nil {
			if _, ok := h.messageQueue.TakeQueued(ctx, queuedSessionID); ok {
				h.logger.Warn("move_task: dropped queued hand-off prompt after MoveTask failure",
					zap.String("task_id", req.TaskID), zap.String("session_id", queuedSessionID))
			}
		}
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

// lookupSession returns the task's primary session.
//   - (session, nil) — task has a primary session.
//   - (nil, nil)     — task has no primary session yet (legitimate "empty"
//     state — task was created but no agent has been launched). The
//     repository signals this with a "no primary session found for task:"
//     error string; we treat it as a not-found rather than a failure so the
//     caller can fall through to the idle-move path instead of rejecting the
//     request.
//   - (nil, err)     — real backend lookup failure (DB error, etc.). The
//     caller should map this to an internal error rather than collapsing it
//     into "no session" downstream.
func (h *Handlers) lookupSession(ctx context.Context, taskID string) (*models.TaskSession, error) {
	session, err := h.taskSvc.GetPrimarySession(ctx, taskID)
	if err != nil {
		// "no primary session found for task: <id>" is the repo's not-found
		// signal — see scanTaskSession's noRowsErr formatting. Match by
		// substring; there's no exported sentinel to errors.Is against.
		if strings.Contains(err.Error(), "no primary session found for task") {
			return nil, nil
		}
		return nil, err
	}
	return session, nil
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
