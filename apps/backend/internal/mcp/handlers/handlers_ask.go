package handlers

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/clarification"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type askUserQuestionRequest struct {
	SessionID         string                   `json:"session_id"`
	TaskID            string                   `json:"task_id"`
	Questions         []clarification.Question `json:"questions"`
	Context           string                   `json:"context"`
	AllowFreeTextOnly bool                     `json:"allow_free_text_only,omitempty"`
	// RetryKey is the connection-scoped transport identity the MCP server
	// attaches so an exact retry maps to the same durable bundle.
	RetryKey string `json:"retry_key"`
}

func decodeAskUserQuestion(msg *ws.Message) (askUserQuestionRequest, *ws.Message, error) {
	var req askUserQuestionRequest

	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		response, err := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return req, response, err
	}
	if req.SessionID == "" {
		response, err := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
		return req, response, err
	}
	// Single source of truth — same validator the HTTP handler uses, so
	// duplicate IDs / bad option counts / empty prompts can't slip through
	// either path.
	validateQuestions := clarification.NormalizeAndValidateQuestions
	if req.AllowFreeTextOnly {
		validateQuestions = clarification.NormalizeAndValidateQuestionsAllowFreeTextOnly
	}
	if errMsg := validateQuestions(req.Questions); errMsg != "" {
		response, err := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, errMsg, nil)
		return req, response, err
	}

	return req, nil, nil
}

func (h *Handlers) clarificationTaskID(ctx context.Context, taskIDArg, sessionID string) string {
	// Look up task ID from session if not provided
	taskID := taskIDArg
	if taskID == "" {
		session, err := h.sessionRepo.GetTaskSession(ctx, sessionID)
		if err != nil {
			h.logger.Warn("failed to look up task for session",
				zap.String("session_id", sessionID),
				zap.Error(err))
		} else if session != nil {
			taskID = session.TaskID
		}
	}

	return taskID
}

type clarificationRetryRegistration struct {
	pendingID      string
	isNew          bool
	deliveryMissed bool
	wait           func(context.Context, *clarification.Response) (*clarification.Response, error)
}

func (h *Handlers) registerClarificationRetry(req askUserQuestionRequest, taskID string) clarificationRetryRegistration {
	// Register the retry before reading its durable bundle. This ordering makes
	// the handoff to Resolver linearizable: a concurrent durable answer either
	// finds this waiter, or records that it already chose detached delivery.
	retryPendingID := clarification.PendingIDForRequest(req.SessionID, req.RetryKey, req.Questions, req.Context)
	clarificationReq := &clarification.Request{
		PendingID: retryPendingID,
		SessionID: req.SessionID,
		TaskID:    taskID,
		Questions: req.Questions,
		Context:   req.Context,
	}
	var retry clarificationRetryRegistration
	if registrar, ok := h.clarificationSvc.(clarificationRetryRegistrar); ok && retryPendingID != "" {
		retry.pendingID, retry.isNew, retry.deliveryMissed, retry.wait = registrar.CreateRetryRequestWithWaiter(clarificationReq)
	} else {
		retry.pendingID, retry.isNew = h.clarificationSvc.CreateRequest(clarificationReq)
	}

	return retry
}

func (h *Handlers) reconcileClarificationRetry(ctx context.Context, msg *ws.Message, req askUserQuestionRequest, taskID string, retry clarificationRetryRegistration) (durableClarificationState, *ws.Message, error) {

	durable, err := h.reconcileDurableClarification(ctx, req.SessionID, retry.pendingID)
	if err != nil {
		if retry.isNew {
			h.clarificationSvc.CancelRequest(retry.pendingID)
		}
		h.logger.Error("failed to reconcile clarification retry",
			zap.String("pending_id", retry.pendingID), zap.Error(err))
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"failed to reconcile clarification retry", nil)
		return durable, response, responseErr
	}
	if durable.response != nil && !durable.deliveryPending && (!retry.deliveryMissed || durable.response.Rejected) {
		response, responseErr := h.replayClarificationRetry(ctx, msg, req.SessionID, taskID, retry, durable.response)
		return durable, response, responseErr
	}
	if retry.deliveryMissed {
		h.logger.Info("clarification retry joined an answer already committed to detached delivery",
			zap.String("pending_id", retry.pendingID),
			zap.String("session_id", req.SessionID))
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"Clarification response is already being delivered after the interrupted wait", nil)
		return durable, response, responseErr
	}
	if durable.closed != "" {
		h.clarificationSvc.CancelRequest(retry.pendingID)
		h.logger.Warn("clarification retry found closed bundle",
			zap.String("pending_id", retry.pendingID),
			zap.String("session_id", req.SessionID),
			zap.String("status", string(durable.closed)))
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"Clarification request timed out or was cancelled", nil)
		return durable, response, responseErr
	}

	return durable, nil, nil
}

func (h *Handlers) replayClarificationRetry(ctx context.Context, msg *ws.Message, sessionID, taskID string, retry clarificationRetryRegistration, recorded *clarification.Response) (*ws.Message, error) {

	if retry.wait != nil {
		confirmed, confirmErr := retry.wait(ctx, recorded)
		if confirmErr != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
				"failed to confirm clarification retry", nil)
		}
		recorded = confirmed
	}
	if retry.isNew {
		h.clarificationSvc.CancelRequest(retry.pendingID)
	}
	h.setSessionRunning(ctx, taskID, sessionID)
	h.logger.Info("clarification retry returned recorded outcome",
		zap.String("pending_id", retry.pendingID),
		zap.String("session_id", sessionID),
		zap.Bool("rejected", recorded.Rejected))
	return ws.NewResponse(msg.ID, msg.Action, recorded)
}

// handleAskUserQuestion retains the same-turn tool waiter through durable delivery confirmation.
func (h *Handlers) handleAskUserQuestion(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	req, response, err := decodeAskUserQuestion(msg)
	if response != nil || err != nil {
		return response, err
	}
	taskID := h.clarificationTaskID(ctx, req.TaskID, req.SessionID)
	retry := h.registerClarificationRetry(req, taskID)
	durable, response, err := h.reconcileClarificationRetry(ctx, msg, req, taskID, retry)
	if response != nil || err != nil {
		return response, err
	}

	// Create one chat message per question (triggers WS events to frontend).
	// If the create fails, the in-store pending entry must be cancelled too —
	// otherwise the agent's WaitForResponse would block for the full 2-hour
	// timeout while the user never sees clarification cards.
	// When dedup fires (retry.isNew=false) the messages already exist, so skip
	// creation; likewise when the retry identity already has durable messages.
	if retry.isNew && !durable.exists && h.messageCreator != nil {
		if _, err := h.messageCreator.CreateClarificationRequestMessages(
			ctx, taskID, req.SessionID, retry.pendingID, req.Questions, req.Context,
		); err != nil {
			h.logger.Error("failed to create clarification request messages",
				zap.String("pending_id", retry.pendingID),
				zap.String("session_id", req.SessionID),
				zap.Error(err))
			h.clarificationSvc.CancelRequest(retry.pendingID)
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
				"failed to create clarification messages: "+err.Error(), nil)
		}
	}
	if durable.exists && durable.response == nil {
		// Reattachment is also the authoritative activeness check. It is
		// serialized with answer claims and successor-turn creation.
		if result, resultErr, wait := h.adoptDurableClarificationRetry(
			ctx, msg, req.SessionID, taskID, retry,
		); !wait {
			return result, resultErr
		}
	}

	return h.waitForAskUserQuestion(ctx, msg, req.SessionID, taskID, retry)
}

func (h *Handlers) waitForAskUserQuestion(ctx context.Context, msg *ws.Message, sessionID, taskID string, retry clarificationRetryRegistration) (*ws.Message, error) {

	// Update session and task states to waiting for input
	h.setSessionWaitingForInput(ctx, taskID, sessionID)

	h.logger.Info("clarification request created, waiting for user response",
		zap.String("pending_id", retry.pendingID),
		zap.String("session_id", sessionID),
		zap.String("task_id", taskID))

	// WaitForResponse can outlast the agent client's idle watchdog because the
	// MCP server emits progress while this call is blocked. If the agent
	// cancels, cleanup and the event fallback resume the interaction on a new
	// turn.
	var resp *clarification.Response
	var err error
	if retry.wait != nil {
		resp, err = retry.wait(ctx, nil)
	} else {
		resp, err = h.clarificationSvc.WaitForResponse(ctx, retry.pendingID)
	}
	if err != nil {
		if h.inputPauser != nil {
			if _, pauseErr := h.inputPauser.PauseForClarificationInput(context.WithoutCancel(ctx), sessionID); pauseErr != nil {
				h.logger.Warn("failed to pause session after clarification ended without answer",
					zap.String("pending_id", retry.pendingID),
					zap.String("session_id", sessionID),
					zap.Error(pauseErr))
			}
		}
		h.logger.Warn("clarification wait ended without response",
			zap.String("pending_id", retry.pendingID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"Clarification request timed out or was cancelled", nil)
	}

	// User responded — set session back to running
	h.setSessionRunning(ctx, taskID, sessionID)

	h.logger.Info("clarification answered, returning to agent",
		zap.String("pending_id", retry.pendingID),
		zap.String("session_id", sessionID),
		zap.Bool("rejected", resp.Rejected))

	// Return response in format expected by agentctl's extractQuestionAnswer
	return ws.NewResponse(msg.ID, msg.Action, resp)
}
