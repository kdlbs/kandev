package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/questions"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const (
	asyncScopeOwn      = "own"
	asyncScopeChildren = "children"
)

type listQuestionAnswersRequest struct {
	TaskID      string   `json:"task_id"`
	SessionID   string   `json:"session_id"`
	Scope       string   `json:"scope"`
	Status      string   `json:"status"`
	QuestionIDs []string `json:"question_ids"`
	Limit       int      `json:"limit"`
	Cursor      string   `json:"cursor"`
}

type withdrawQuestionRequest struct {
	TaskID     string `json:"task_id"`
	SessionID  string `json:"session_id"`
	QuestionID string `json:"question_id"`
	Reason     string `json:"reason"`
}

type forwardQuestionRequest struct {
	TaskID     string `json:"task_id"`
	SessionID  string `json:"session_id"`
	QuestionID string `json:"question_id"`
	Reason     string `json:"reason"`
}

type waitQuestionAnswersRequest struct {
	TaskID      string   `json:"task_id"`
	SessionID   string   `json:"session_id"`
	QuestionIDs []string `json:"question_ids"`
	Reason      string   `json:"reason"`
}

// questionReceipt is the async creation receipt returned to the agent.
func asyncQuestionReceipt(question *models.AsyncQuestion, recipient string) map[string]interface{} {
	receipt := map[string]interface{}{
		"question_id": question.ID,
		"mode":        "async",
		"status":      string(question.Status),
		"recipient":   recipient,
		"created_at":  question.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
	if question.ParentTaskID != "" {
		receipt["parent_task_id"] = question.ParentTaskID
	}
	return receipt
}

func asyncQuestionView(question *models.AsyncQuestion) map[string]interface{} {
	view := map[string]interface{}{
		"question_id": question.ID,
		"task_id":     question.TaskID,
		"session_id":  question.SessionID,
		"status":      string(question.Status),
		"recipient":   string(question.Recipient),
		"forwarded":   question.Forwarded,
		"context":     question.Context,
		"questions":   question.Questions,
		"created_at":  question.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		"updated_at":  question.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		"version":     question.Version,
		"waiting":     false,
	}
	if question.ParentTaskID != "" {
		view["parent_task_id"] = question.ParentTaskID
	}
	if question.Outcome != nil {
		view["outcome"] = question.Outcome
	}
	if question.WithdrawReason != "" {
		view["withdraw_reason"] = question.WithdrawReason
	}
	return view
}

// handleAsyncOperatorQuestion creates a durable operator request without
// registering a waiter, changing session state, or dispatching a turn.
func (h *Handlers) handleAsyncOperatorQuestion(ctx context.Context, msg *ws.Message, req askUserQuestionRequest) (*ws.Message, error) {
	if h.questionSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "async questions are not available", nil)
	}
	session, err := h.sessionRepo.GetTaskSession(ctx, req.SessionID)
	if err != nil || session == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "session not found", nil)
	}
	taskID := req.TaskID
	if taskID == "" {
		taskID = session.TaskID
	}
	if taskID != session.TaskID {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id does not belong to task_id", nil)
	}
	task, err := h.taskSvc.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "task not found", nil)
	}
	if task.Autopilot {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"autopilot tasks use ask_parent_question_kandev", nil)
	}
	if task.IsEphemeral {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"async questions are not available for ephemeral quick-chat tasks", nil)
	}
	result, err := h.questionSvc.Create(ctx, questions.CreateInput{
		TaskID:               task.ID,
		WorkspaceID:          task.WorkspaceID,
		SessionID:            session.ID,
		SessionIncarnationID: session.QueueIncarnationID,
		Recipient:            models.AsyncQuestionRecipientUser,
		Questions:            req.Questions,
		Context:              req.Context,
		RequestID:            req.RequestID,
		RetryKey:             req.RetryKey,
	})
	if err != nil {
		return h.asyncQuestionError(msg, err)
	}
	h.logger.Info("async operator question created",
		zap.String("question_id", result.Question.ID),
		zap.String("task_id", task.ID),
		zap.String("session_id", session.ID),
		zap.Bool("created", result.Created))
	return ws.NewResponse(msg.ID, msg.Action, asyncQuestionReceipt(result.Question, "user"))
}

func (h *Handlers) handleAsyncParentQuestion(ctx context.Context, msg *ws.Message, req parentQuestionRequest) (*ws.Message, error) {
	if h.questionSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "async questions are not available", nil)
	}
	target, targetErr := h.loadParentQuestionTarget(ctx, req)
	if targetErr != nil {
		return ws.NewError(msg.ID, msg.Action, targetErr.code, targetErr.message, nil)
	}
	if target.childTask.IsEphemeral {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"async questions are not available for ephemeral tasks", nil)
	}
	result, err := h.questionSvc.Create(ctx, questions.CreateInput{
		TaskID:                     target.childTask.ID,
		WorkspaceID:                target.childTask.WorkspaceID,
		SessionID:                  target.childSession.ID,
		SessionIncarnationID:       target.childSession.QueueIncarnationID,
		Recipient:                  models.AsyncQuestionRecipientParent,
		ParentTaskID:               target.parentTask.ID,
		ParentSession:              target.parentSession.ID,
		ParentSessionIncarnationID: target.parentSession.QueueIncarnationID,
		Questions:                  req.Questions,
		Context:                    req.Context,
		RequestID:                  req.RequestID,
		RetryKey:                   req.RetryKey,
	})
	if err != nil {
		return h.asyncQuestionError(msg, err)
	}
	h.logger.Info("async parent question created",
		zap.String("question_id", result.Question.ID),
		zap.String("child_task_id", target.childTask.ID),
		zap.String("parent_task_id", target.parentTask.ID),
		zap.Bool("created", result.Created))
	return ws.NewResponse(msg.ID, msg.Action, asyncQuestionReceipt(result.Question, "parent"))
}

func (h *Handlers) asyncQuestionError(msg *ws.Message, err error) (*ws.Message, error) {
	switch {
	case errors.Is(err, questions.ErrValidation):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	case errors.Is(err, questions.ErrConflict):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	case errors.Is(err, questions.ErrNotFound):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, err.Error(), nil)
	case errors.Is(err, questions.ErrUnauthorized):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	default:
		h.logger.Error("async question operation failed", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "async question operation failed", nil)
	}
}

func (h *Handlers) handleListQuestionAnswers(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.questionSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "async questions are not available", nil)
	}
	var req listQuestionAnswersRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	session, errResp, err := h.loadQuestionSession(ctx, msg, req.SessionID, req.TaskID)
	if errResp != nil || err != nil {
		return errResp, err
	}
	statuses, statusErr := asyncStatuses(req.Status)
	if statusErr != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, statusErr.Error(), nil)
	}
	page, err := h.readQuestionAnswerPage(ctx, session, req, statuses)
	if err != nil {
		return h.asyncQuestionError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, buildQuestionAnswerList(page))
}

func (h *Handlers) loadQuestionSession(
	ctx context.Context, msg *ws.Message, sessionID, taskID string,
) (*models.TaskSession, *ws.Message, error) {
	session, err := h.sessionRepo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "session not found", nil)
		return nil, response, responseErr
	}
	if taskID != "" && taskID != session.TaskID {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id does not belong to task_id", nil)
		return nil, response, responseErr
	}
	return session, nil, nil
}

func (h *Handlers) readQuestionAnswerPage(
	ctx context.Context,
	session *models.TaskSession,
	req listQuestionAnswersRequest,
	statuses []models.AsyncQuestionStatus,
) (*models.AsyncQuestionPage, error) {
	scope := strings.TrimSpace(req.Scope)
	if scope == "" || scope == asyncScopeOwn {
		cursorAt, cursorID, err := decodeAsyncCursor(req.Cursor)
		if err != nil {
			return nil, fmt.Errorf("%w: cursor is malformed", questions.ErrValidation)
		}
		return h.questionSvc.ListOwn(ctx, session.ID, statuses, req.QuestionIDs, req.Limit, cursorAt, cursorID)
	}
	if scope == asyncScopeChildren {
		cursorAt, cursorID, err := decodeAsyncCursor(req.Cursor)
		if err != nil {
			return nil, fmt.Errorf("%w: cursor is malformed", questions.ErrValidation)
		}
		return h.questionSvc.ListChildren(ctx, session.TaskID, statuses, req.QuestionIDs, req.Limit, cursorAt, cursorID)
	}
	return nil, fmt.Errorf("%w: scope must be own or children", questions.ErrValidation)
}

func buildQuestionAnswerList(page *models.AsyncQuestionPage) map[string]interface{} {
	views := make([]map[string]interface{}, 0, len(page.Questions))
	for _, question := range page.Questions {
		views = append(views, asyncQuestionView(question))
	}
	response := map[string]interface{}{
		"questions": views,
		"count":     len(views),
		"has_more":  page.HasMore,
	}
	if page.HasMore && len(page.Questions) > 0 {
		last := page.Questions[len(page.Questions)-1]
		response["next_cursor"] = encodeAsyncCursor(last.CreatedAt, last.ID)
	}
	return response
}

func (h *Handlers) handleWithdrawQuestion(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.questionSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "async questions are not available", nil)
	}
	var req withdrawQuestionRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	session, err := h.sessionRepo.GetTaskSession(ctx, req.SessionID)
	if err != nil || session == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "session not found", nil)
	}
	if req.TaskID != "" && req.TaskID != session.TaskID {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id does not belong to task_id", nil)
	}
	question, err := h.questionSvc.Withdraw(ctx, req.QuestionID, session.ID, req.Reason)
	if err != nil {
		return h.asyncQuestionError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, asyncQuestionView(question))
}

func (h *Handlers) handleForwardQuestion(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.questionSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "async questions are not available", nil)
	}
	var req forwardQuestionRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	session, err := h.sessionRepo.GetTaskSession(ctx, req.SessionID)
	if err != nil || session == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "session not found", nil)
	}
	task, err := h.taskSvc.GetTask(ctx, session.TaskID)
	if err != nil || task == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "task not found", nil)
	}
	if task.Autopilot {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"an autopilot task has no direct operator question access", nil)
	}
	question, err := h.questionSvc.Forward(ctx, req.QuestionID, task.ID, session.ID, req.Reason)
	if err != nil {
		return h.asyncQuestionError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, asyncQuestionView(question))
}

// handleWaitForQuestionAnswers establishes the explicit dependency gate and
// performs the guarded pause. A dependency that resolved before the pause is
// observed releases the gate immediately.
func (h *Handlers) handleWaitForQuestionAnswers(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.questionSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "async questions are not available", nil)
	}
	var req waitQuestionAnswersRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	session, errResp, err := h.loadQuestionSession(ctx, msg, req.SessionID, req.TaskID)
	if errResp != nil || err != nil {
		return errResp, err
	}
	turnID := h.captureQuestionWaitTurn(ctx, session.ID)
	task, err := h.taskSvc.GetTask(ctx, session.TaskID)
	if err != nil || task == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "task not found", nil)
	}
	result, err := h.questionSvc.Wait(ctx, questions.WaitInput{
		TaskID:       session.TaskID,
		WorkspaceID:  task.WorkspaceID,
		SessionID:    session.ID,
		TurnID:       turnID,
		Dependencies: req.QuestionIDs,
		Reason:       req.Reason,
	})
	if err != nil {
		return h.asyncQuestionError(msg, err)
	}
	if result.Status == questions.WaitStatusReady {
		return ws.NewResponse(msg.ID, msg.Action, readyWaitResponse(result.Questions))
	}
	if err := h.pauseForExplicitWait(ctx, session, result.Wait, turnID); err != nil {
		if errors.Is(err, orchestrator.ErrStaleQuestionWaitPause) {
			_ = h.questionSvc.MarkWaitCancelled(context.WithoutCancel(ctx), result.Wait.ID, "newer_turn")
			return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
				"status": "cancelled",
				"reason": "newer_turn",
			})
		}
		_ = h.questionSvc.MarkWaitCancelled(context.WithoutCancel(ctx), result.Wait.ID, "pause_failed")
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"failed to pause for the explicit question wait", nil)
		return response, responseErr
	}
	ids := append([]string(nil), result.Wait.DependencyIDs...)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"status":       "waiting",
		"wait_id":      result.Wait.ID,
		"question_ids": ids,
	})
}

func readyWaitResponse(questions []*models.AsyncQuestion) map[string]interface{} {
	views := make([]map[string]interface{}, 0, len(questions))
	for _, question := range questions {
		views = append(views, asyncQuestionView(question))
	}
	return map[string]interface{}{"status": "ready", "questions": views}
}

// pauseForExplicitWait performs the guarded pause bound to the captured turn
// and records the settled gate.
func (h *Handlers) pauseForExplicitWait(ctx context.Context, session *models.TaskSession, wait *models.AsyncQuestionWait, turnID string) error {
	pauseCtx := context.WithoutCancel(ctx)
	h.setSessionWaitingForInput(pauseCtx, session.TaskID, session.ID)
	if err := h.performGuardedPause(pauseCtx, session.ID, wait.ID, turnID); err != nil {
		return err
	}
	if _, err := h.questionSvc.AcknowledgeWaitPause(pauseCtx, wait.ID); err != nil {
		h.logger.Warn("failed to acknowledge explicit async question wait",
			zap.String("wait_id", wait.ID), zap.Error(err))
	}
	return nil
}

func (h *Handlers) performGuardedPause(ctx context.Context, sessionID, waitID, turnID string) error {
	if h.inputPauser == nil {
		// No pause implementation is wired (tests, headless modes). The gate
		// remains requested so completion still fails closed until release.
		h.logger.Warn("async question wait has no input pauser; gate remains requested",
			zap.String("wait_id", waitID))
		return nil
	}
	pauser, ok := h.inputPauser.(clarificationInputPauserWithOptions)
	if !ok {
		return nil
	}
	options := orchestrator.ClarificationPauseOptions{Force: true}
	if turnID != "" {
		options.ExpectedTurnID = &turnID
	}
	if _, err := pauser.PauseForClarificationInputWithOptions(ctx, sessionID, options); err != nil {
		h.logger.Warn("failed to pause for explicit async question wait",
			zap.String("wait_id", waitID), zap.Error(err))
		return err
	}
	return nil
}

func (h *Handlers) captureQuestionWaitTurn(ctx context.Context, sessionID string) string {
	capturer, ok := h.inputPauser.(clarificationTurnCapturer)
	if !ok {
		return ""
	}
	turnID, err := capturer.CaptureActiveTurnID(ctx, sessionID)
	if err != nil {
		h.logger.Warn("failed to capture the explicit question wait turn",
			zap.String("session_id", sessionID), zap.Error(err))
		return ""
	}
	return turnID
}

func asyncStatuses(value string) ([]models.AsyncQuestionStatus, error) {
	switch strings.TrimSpace(value) {
	case "", "all":
		return nil, nil
	case "pending":
		return []models.AsyncQuestionStatus{models.AsyncQuestionStatusPending}, nil
	case "answered":
		return []models.AsyncQuestionStatus{models.AsyncQuestionStatusAnswered}, nil
	case "rejected":
		return []models.AsyncQuestionStatus{models.AsyncQuestionStatusRejected}, nil
	case "withdrawn":
		return []models.AsyncQuestionStatus{models.AsyncQuestionStatusWithdrawn}, nil
	case "stale":
		return []models.AsyncQuestionStatus{models.AsyncQuestionStatusStale}, nil
	default:
		return nil, fmt.Errorf("status must be pending, answered, rejected, withdrawn, stale, or all")
	}
}

func encodeAsyncCursor(createdAt time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(createdAt.UTC().Format(time.RFC3339Nano) + "|" + id))
}

func decodeAsyncCursor(cursor string) (string, string, error) {
	if strings.TrimSpace(cursor) == "" {
		return "", "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("malformed cursor")
	}
	if _, err := time.Parse(time.RFC3339Nano, parts[0]); err != nil {
		return "", "", err
	}
	return parts[0], parts[1], nil
}
