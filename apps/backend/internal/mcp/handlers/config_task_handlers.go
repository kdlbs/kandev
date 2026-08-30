package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/routing"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

func (h *Handlers) handleMoveTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID          string `json:"task_id"`
		WorkflowID      string `json:"workflow_id"`
		WorkflowStepID  string `json:"workflow_step_id"`
		Position        int    `json:"position"`
		Prompt          string `json:"prompt"`
		SenderSessionID string `json:"sender_session_id"`
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
	if replay, handled, err := h.replayMoveTaskOperation(
		ctx, msg, req.TaskID, req.WorkflowID, req.WorkflowStepID, req.Position,
	); handled {
		return replay, err
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

	// Active source session → deferred path. Running MoveTask immediately would
	// fail validateMoveSessions ("task has an active session (RUNNING)") and,
	// if it somehow succeeded, would race on_enter processing against the
	// agent's still-active turn. Defer until handleAgentReady fires
	// applyPendingMove on turn-end. Prompt is optional: omit it for simple
	// self-moves (e.g. Work → Done); include it for cross-agent hand-offs.
	if session != nil &&
		(session.State == models.TaskSessionStateRunning || session.State == models.TaskSessionStateStarting) {
		terminal, err := h.taskSvc.IsTerminalWorkflowStep(ctx, req.WorkflowStepID)
		if err != nil {
			// Preserve the deferred path's detailed target/workspace validation.
			// A missing or foreign step is a request error, not an internal one.
			return h.deferMoveTask(ctx, msg, req, session)
		}
		if terminal {
			return h.applyMoveTaskImmediate(ctx, msg, req, session, true)
		}
		return h.deferMoveTask(ctx, msg, req, session)
	}

	// Idle path — apply immediately. If a prompt was supplied, queue it on the
	// session so the receiving agent's next turn picks it up; if not, just move.
	return h.applyMoveTaskImmediate(ctx, msg, req, session, false)
}

func (h *Handlers) replayMoveTaskOperation(
	ctx context.Context,
	msg *ws.Message,
	taskID, workflowID, targetStepID string,
	position int,
) (*ws.Message, bool, error) {
	operationID := workflowRouteOperationID("mcp-move", msg.ID)
	operation, found, err := h.taskSvc.GetWorkflowRouteOperation(ctx, operationID)
	if err != nil {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"failed to read route operation", nil)
		return response, true, responseErr
	}
	if !found {
		return nil, false, nil
	}
	if operation.TaskID != taskID || operation.TargetStepID != targetStepID {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict,
			"route operation identity already belongs to a different request", nil)
		return response, true, responseErr
	}
	switch operation.Outcome {
	case routing.OutcomeCommitted, routing.OutcomeAlreadySatisfied:
		task, loadErr := h.taskSvc.GetTask(ctx, taskID)
		if loadErr != nil {
			response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
				"failed to read routed task", nil)
			return response, true, responseErr
		}
		response, responseErr := ws.NewResponse(msg.ID, msg.Action, dto.FromTask(task))
		return response, true, responseErr
	case routing.OutcomePending:
		workflowID, targetStepID, position, err = h.replayPendingMoveRequest(
			ctx, operationID, taskID, workflowID, targetStepID, position,
		)
		if err != nil {
			response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
				"failed to read pending route", nil)
			return response, true, responseErr
		}
		response, responseErr := ws.NewResponse(msg.ID, msg.Action,
			h.synthesizeMovedTaskDTO(ctx, taskID, workflowID, targetStepID, position))
		return response, true, responseErr
	case routing.OutcomeStaleSource:
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"workflow step changed before route commit", nil)
		return response, true, responseErr
	default:
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict,
			"route operation did not commit", nil)
		return response, true, responseErr
	}
}

type pendingMoveLister interface {
	ListPendingMoves(context.Context) ([]messagequeue.PendingMoveRecord, error)
}

func (h *Handlers) replayPendingMoveRequest(
	ctx context.Context,
	operationID, taskID, workflowID, targetStepID string,
	position int,
) (string, string, int, error) {
	lister, ok := h.messageQueue.(pendingMoveLister)
	if !ok {
		return workflowID, targetStepID, position, nil
	}
	records, err := lister.ListPendingMoves(ctx)
	if err != nil {
		return "", "", 0, err
	}
	for _, record := range records {
		move := record.Move
		if move.MoveID != operationID {
			continue
		}
		if move.TaskID != taskID || move.WorkflowStepID != targetStepID {
			return "", "", 0, routing.ErrOperationIdentityConflict
		}
		return move.WorkflowID, move.WorkflowStepID, move.Position, nil
	}
	return workflowID, targetStepID, position, nil
}

// deferMoveTask records a PendingMove for the agent's turn-end handler to
// apply. Optionally queues a hand-off prompt when the caller supplied one.
// Returns a synthetic moved-task DTO so the agent's tool call resolves
// successfully and ends the turn cleanly.
func (h *Handlers) deferMoveTask(
	ctx context.Context,
	msg *ws.Message,
	req struct {
		TaskID          string `json:"task_id"`
		WorkflowID      string `json:"workflow_id"`
		WorkflowStepID  string `json:"workflow_step_id"`
		Position        int    `json:"position"`
		Prompt          string `json:"prompt"`
		SenderSessionID string `json:"sender_session_id"`
	},
	session *models.TaskSession,
) (*ws.Message, error) {
	if h.messageQueue == nil {
		h.logger.Error("move_task: message queue not configured; cannot defer move from active session",
			zap.String("task_id", req.TaskID), zap.String("session_id", session.ID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"move_task requires message queue support while the source session is active", nil)
	}

	// Validate the target step exists and belongs to the requested workflow
	// before committing the deferred move. Without this check a stale or
	// foreign step_id would be stored and silently fail at turn-end, leaving
	// the task orphaned on the board.
	if h.workflowCtrl != nil {
		stepResp, err := h.workflowCtrl.GetStep(ctx, req.WorkflowStepID)
		if err != nil || stepResp == nil || stepResp.Step == nil {
			h.logger.Error("move_task: target step not found",
				zap.String("task_id", req.TaskID),
				zap.String("workflow_step_id", req.WorkflowStepID),
				zap.Error(err))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
				"target workflow_step_id does not exist", nil)
		}
		if stepResp.Step.WorkflowID != req.WorkflowID {
			h.logger.Error("move_task: target step belongs to a different workflow",
				zap.String("task_id", req.TaskID),
				zap.String("workflow_step_id", req.WorkflowStepID),
				zap.String("step_workflow_id", stepResp.Step.WorkflowID),
				zap.String("requested_workflow_id", req.WorkflowID))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
				"target workflow_step_id does not belong to the requested workflow_id", nil)
		}
	}

	// Validate the target workflow lives in the same workspace as the task.
	// The immediate-apply path (task.Service.MoveTask) already enforces this
	// via validateTaskMove; the deferred path bypasses that service entirely
	// (see applyPendingMove's doc comment), so it must check independently or
	// a cross-workspace move_task_kandev call would silently succeed.
	if h.taskSvc != nil {
		task, err := h.taskSvc.GetTask(ctx, req.TaskID)
		if err != nil {
			h.logger.Error("move_task: failed to load task for workspace validation",
				zap.String("task_id", req.TaskID), zap.Error(err))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
				"failed to load task for move validation", nil)
		}
		targetWorkflow, err := h.taskSvc.GetWorkflow(ctx, req.WorkflowID)
		if err != nil {
			h.logger.Error("move_task: target workflow not found",
				zap.String("task_id", req.TaskID),
				zap.String("workflow_id", req.WorkflowID),
				zap.Error(err))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
				"target workflow_id does not exist", nil)
		}
		if targetWorkflow.WorkspaceID != task.WorkspaceID {
			h.logger.Error("move_task: target workflow is in a different workspace",
				zap.String("task_id", req.TaskID),
				zap.String("workflow_id", req.WorkflowID),
				zap.String("task_workspace_id", task.WorkspaceID),
				zap.String("target_workspace_id", targetWorkflow.WorkspaceID))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
				"target workflow is in a different workspace", nil)
		}
	}

	moveID := workflowRouteOperationID("mcp-move", msg.ID)
	task, err := h.taskSvc.GetTask(ctx, req.TaskID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"failed to load task generation", nil)
	}
	turnID, producer, cause, causeID := h.workflowRouteCause(ctx, req.SenderSessionID, routing.ProducerDeferredMove)
	pending := &messagequeue.PendingMove{
		ID:                     uuid.NewString(),
		MoveID:                 moveID,
		TaskID:                 req.TaskID,
		WorkflowID:             req.WorkflowID,
		WorkflowStepID:         req.WorkflowStepID,
		Position:               req.Position,
		ExpectedWorkflowStepID: task.WorkflowStepID,
		Actor:                  string(wfmodels.StepTransitionActorAgent),
		SenderSessionID:        req.SenderSessionID,
		InitiatingTurnID:       turnID,
	}
	if err := h.messageQueue.SetPendingMove(ctx, session.ID, pending); err != nil {
		_ = h.taskSvc.RecordWorkflowRouteOperation(ctx, routing.Operation{
			ID: moveID, TaskID: req.TaskID, WorkspaceID: task.WorkspaceID,
			Producer: producer, ExpectedStepID: task.WorkflowStepID,
			ObservedStepID: task.WorkflowStepID, TargetStepID: req.WorkflowStepID,
			SessionID: req.SenderSessionID, ActorKind: string(steptelemetry.ActorAgent),
			ActorID: req.SenderSessionID, TurnID: turnID, ExternalCause: cause,
			ExternalCauseID: causeID, Outcome: routing.OutcomeConflict,
		})
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict,
			"another deferred move is already pending for this session", nil)
	}
	if err := h.taskSvc.RecordWorkflowRouteOperation(ctx, routing.Operation{
		ID: moveID, TaskID: req.TaskID, WorkspaceID: task.WorkspaceID,
		Producer: producer, ExpectedStepID: task.WorkflowStepID,
		ObservedStepID: task.WorkflowStepID, TargetStepID: req.WorkflowStepID,
		SessionID: req.SenderSessionID, ActorKind: string(steptelemetry.ActorAgent),
		ActorID: req.SenderSessionID, TurnID: turnID, ExternalCause: cause,
		ExternalCauseID: causeID, Outcome: routing.OutcomePending,
	}); err != nil {
		_, _ = h.messageQueue.DeletePendingMoveIfMatch(ctx,
			messagequeue.PendingMoveRecord{SessionID: session.ID, Move: *pending}, "")
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"failed to persist deferred route identity", nil)
	}
	if req.Prompt != "" {
		wrapped := "You were moved to this step with the following message: " + req.Prompt
		if err := h.queueMoveTaskPromptWithMoveID(ctx, req.TaskID, session.ID, wrapped, moveID); err != nil {
			_, cleanupErr := h.messageQueue.DeletePendingMoveIfMatch(ctx,
				messagequeue.PendingMoveRecord{SessionID: session.ID, Move: *pending}, "")
			h.logger.Error("move_task: failed to queue hand-off prompt",
				zap.String("task_id", req.TaskID), zap.String("session_id", session.ID),
				zap.Error(err), zap.Error(cleanupErr))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
				"failed to queue move_task hand-off prompt", nil)
		}
	}
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
		TaskID          string `json:"task_id"`
		WorkflowID      string `json:"workflow_id"`
		WorkflowStepID  string `json:"workflow_step_id"`
		Position        int    `json:"position"`
		Prompt          string `json:"prompt"`
		SenderSessionID string `json:"sender_session_id"`
	},
	session *models.TaskSession,
	allowActivePrimarySession bool,
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

	// Attribution uses the CALLING session (req.SenderSessionID, injected
	// server-side by the MCP server from its own bound session — see
	// moveTaskHandler), never the target task's session captured above:
	// move_task_kandev routinely moves a task the caller doesn't run a
	// session on, so the target's session is not who caused this move.
	// SenderSessionID is empty for a config-mode/admin MCP server with no
	// bound session, which correctly falls back to ActorSystem.
	attribution := steptelemetry.Attribution{Trigger: steptelemetry.TriggerMCPMove, ActorKind: steptelemetry.ActorSystem}
	if req.SenderSessionID != "" {
		attribution.ActorKind = steptelemetry.ActorAgent
		attribution.ActorID = req.SenderSessionID
		attribution.SessionID = req.SenderSessionID
	}
	moveCtx := steptelemetry.WithAttribution(ctx, attribution)
	current, currentErr := h.taskSvc.GetTask(ctx, req.TaskID)
	if currentErr != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to load route generation", nil)
	}
	turnID, producer, cause, causeID := h.workflowRouteCause(ctx, req.SenderSessionID, routing.ProducerManualMove)
	routeOperation := routing.Operation{
		ID: workflowRouteOperationID("mcp-move", msg.ID), TaskID: req.TaskID,
		WorkspaceID: current.WorkspaceID, Producer: producer,
		ExpectedStepID: current.WorkflowStepID, ObservedStepID: current.WorkflowStepID,
		TargetStepID: req.WorkflowStepID, SessionID: req.SenderSessionID, TurnID: turnID,
		ActorKind: string(attribution.ActorKind), ActorID: attribution.ActorID,
		ExternalCause: cause, ExternalCauseID: causeID,
	}
	moveCtx = routing.WithOperation(moveCtx, routeOperation)
	result, err := h.taskSvc.MoveTaskWithOptions(moveCtx, req.TaskID, req.WorkflowID, req.WorkflowStepID, req.Position,
		service.MoveTaskOptions{
			AllowActivePrimarySession: allowActivePrimarySession,
			ExpectedWorkflowStepID:    current.WorkflowStepID,
			StepHistoryActor:          wfmodels.StepTransitionActorAgent,
		})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "workflow step changed") {
			if observed, loadErr := h.taskSvc.GetTask(ctx, req.TaskID); loadErr == nil && observed != nil {
				routeOperation.ObservedStepID = observed.WorkflowStepID
			}
			routeOperation.Outcome = routing.OutcomeStaleSource
			_ = h.taskSvc.RecordWorkflowRouteOperation(ctx, routeOperation)
		}
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
		return ws.NewError(msg.ID, msg.Action, classifyMoveTaskError(err), moveTaskErrorMessage(err), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromTask(result.Task))
}

func workflowRouteOperationID(prefix, requestID string) string {
	if requestID == "" {
		return prefix + ":" + uuid.NewString()
	}
	return prefix + ":" + requestID
}

type workflowRouteCauseReader interface {
	ListTurnsBySession(context.Context, string) ([]*models.Turn, error)
	ListMessagesByTurnID(context.Context, string) ([]*models.Message, error)
}

func (h *Handlers) workflowRouteCause(
	ctx context.Context,
	sessionID string,
	fallback routing.Producer,
) (turnID string, producer routing.Producer, cause, causeID string) {
	producer = fallback
	reader, ok := h.sessionRepo.(workflowRouteCauseReader)
	if !ok || sessionID == "" {
		return "", producer, "", ""
	}
	turns, err := reader.ListTurnsBySession(ctx, sessionID)
	if err != nil || len(turns) == 0 || turns[len(turns)-1] == nil {
		return "", producer, "", ""
	}
	turnID = turns[len(turns)-1].ID
	messages, err := reader.ListMessagesByTurnID(ctx, turnID)
	if err != nil {
		return turnID, producer, "", ""
	}
	for index := len(messages) - 1; index >= 0; index-- {
		metadata := messages[index].Metadata
		if models.StringFromAny(metadata["origin"]) != "github_pr_automation" ||
			models.StringFromAny(metadata["automation_kind"]) != "merged" {
			continue
		}
		producer = routing.ProducerMergedPR
		cause = "github_pr_merged"
		causeID = models.StringFromAny(metadata["repository_id"]) + ":" + fmt.Sprint(metadata["pr_number"])
		return turnID, producer, cause, causeID
	}
	return turnID, producer, "", ""
}

func classifyMoveTaskError(err error) string {
	if err == nil {
		return ws.ErrorCodeInternalError
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "wip limit exceeded"),
		strings.Contains(msg, routing.ErrOperationIdentityConflict.Error()),
		strings.Contains(msg, "active session"),
		strings.Contains(msg, "archived tasks cannot be moved"),
		strings.Contains(msg, "different workspace"),
		strings.Contains(msg, "does not belong to target workflow"):
		return ws.ErrorCodeConflict
	case strings.Contains(msg, "invalid"),
		strings.Contains(msg, "required"):
		return ws.ErrorCodeValidation
	default:
		return ws.ErrorCodeInternalError
	}
}

func moveTaskErrorMessage(err error) string {
	switch classifyMoveTaskError(err) {
	case ws.ErrorCodeConflict:
		return "Move task conflicts with the current task or workflow state"
	case ws.ErrorCodeValidation:
		return "Invalid move_task request"
	default:
		return "Failed to move task"
	}
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
//     repository signals this with the taskrepo.ErrNoPrimarySession sentinel;
//     we treat it as a not-found rather than a failure so the caller can fall
//     through to the idle-move path instead of rejecting the request.
//   - (nil, err)     — real backend lookup failure (DB error, etc.). The
//     caller should map this to an internal error rather than collapsing it
//     into "no session" downstream.
func (h *Handlers) lookupSession(ctx context.Context, taskID string) (*models.TaskSession, error) {
	session, err := h.taskSvc.GetPrimarySession(ctx, taskID)
	if err != nil {
		// Classify the repo's not-found signal via the typed sentinel rather
		// than substring-matching the formatted message, which is brittle.
		if errors.Is(err, taskrepo.ErrNoPrimarySession) {
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
	return h.queueMoveTaskPromptWithMoveID(ctx, taskID, sessionID, prompt, "")
}

func (h *Handlers) queueMoveTaskPromptWithMoveID(ctx context.Context, taskID, sessionID, prompt, moveID string) error {
	if h.messageQueue == nil {
		return fmt.Errorf("message queue is unavailable")
	}
	if sessionID == "" {
		return fmt.Errorf("task has no primary session")
	}
	metadata := map[string]interface{}(nil)
	if moveID != "" {
		metadata = map[string]interface{}{messagequeue.MetadataDeferredMoveID: moveID}
	}
	if queueWithMetadata, ok := h.messageQueue.(messageMetadataQueuer); ok {
		if _, err := queueWithMetadata.QueueMessageWithMetadata(ctx, sessionID, taskID, prompt, "", messagequeue.QueuedByMoveTask, false, nil, metadata); err != nil {
			return fmt.Errorf("queue message: %w", err)
		}
		return nil
	}
	if _, err := h.messageQueue.QueueMessage(ctx, sessionID, taskID, prompt, "", messagequeue.QueuedByMoveTask, false, nil); err != nil {
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
	callerTaskID, err := unmarshalStringField(msg.Payload, "caller_task_id")
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if err := h.validateAutomationArchiveTarget(ctx, callerTaskID, taskID); err != nil {
		h.logger.Warn("rejected archive target", zap.String("task_id", taskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}

	if err := h.taskSvc.ArchiveTask(ctx, taskID); err != nil {
		// Archiving is a goal-state operation: a task that is already archived
		// is in the requested state, so report success instead of an opaque
		// internal error. The flag lets the caller tell a no-op from a real
		// state change.
		if errors.Is(err, service.ErrTaskAlreadyArchived) {
			return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
				"success":          true,
				"already_archived": true,
			})
		}
		h.logger.Error("failed to archive task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to archive task", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"success": true})
}

func (h *Handlers) validateAutomationArchiveTarget(ctx context.Context, callerTaskID, targetTaskID string) error {
	if callerTaskID == "" {
		return nil
	}
	if h.taskSvc == nil {
		return errors.New("archive caller task is unavailable")
	}
	caller, err := h.taskSvc.GetTask(ctx, callerTaskID)
	if err != nil || caller == nil {
		if err != nil {
			return fmt.Errorf("archive caller task cannot be resolved: %w", err)
		}
		return errors.New("archive caller task cannot be resolved")
	}
	if caller.Origin != models.TaskOriginAutomationRun ||
		models.StringFromAny(caller.Metadata["trigger_type"]) != string(automation.TriggerTypeGitHubPRMerged) {
		return nil
	}
	expectedTarget := models.StringFromAny(caller.Metadata[models.MetaKeyAutomationTargetTaskID])
	if expectedTarget == "" || expectedTarget != targetTaskID {
		return errors.New("archive target is not bound to this automation run")
	}
	return nil
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
	state := normalizeTaskState(req.State)
	if !isValidTaskState(state) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "invalid task state: "+req.State, nil)
	}

	task, err := h.taskSvc.UpdateTaskState(ctx, req.TaskID, state)
	if err != nil {
		h.logger.Error("failed to update task state", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update task state", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromTask(task))
}

func isValidTaskState(state v1.TaskState) bool {
	switch state {
	case v1.TaskStateTODO, v1.TaskStateCreated, v1.TaskStateScheduling,
		v1.TaskStateInProgress, v1.TaskStateReview, v1.TaskStateBlocked,
		v1.TaskStateWaitingForInput, v1.TaskStateCompleted,
		v1.TaskStateFailed, v1.TaskStateCancelled:
		return true
	default:
		return false
	}
}

// normalizeTaskState maps common agent-supplied aliases to canonical TaskState
// values. Agents often send lowercase or shorthand strings (e.g. "complete",
// "done") that are not valid v1.TaskState constants.
func normalizeTaskState(raw string) v1.TaskState {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return v1.TaskState("")
	}
	upper := strings.ToUpper(trimmed)
	switch upper {
	case "OPEN", "TODO":
		return v1.TaskStateTODO
	case "IN_PROGRESS", "INPROGRESS", "ACTIVE":
		return v1.TaskStateInProgress
	case "COMPLETE", "COMPLETED", "DONE":
		return v1.TaskStateCompleted
	case "BLOCKED":
		return v1.TaskStateBlocked
	case "CANCELLED", "CANCELED":
		return v1.TaskStateCancelled
	case "REVIEW":
		return v1.TaskStateReview
	case "FAILED":
		return v1.TaskStateFailed
	case "CREATED":
		return v1.TaskStateCreated
	case "SCHEDULING":
		return v1.TaskStateScheduling
	case "WAITING_FOR_INPUT", "WAITING":
		return v1.TaskStateWaitingForInput
	default:
		return v1.TaskState(trimmed)
	}
}
