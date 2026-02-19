package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// OrchestratorService defines the interface for orchestrator operations
type OrchestratorService interface {
	PromptTask(ctx context.Context, taskID, sessionID, prompt, model string, planMode bool, attachments []v1.MessageAttachment) (*orchestrator.PromptResult, error)
	ResumeTaskSession(ctx context.Context, taskID, taskSessionID string) error
	StartCreatedSession(ctx context.Context, taskID, sessionID, agentProfileID, prompt string) error
	ProcessOnTurnStart(ctx context.Context, taskID, sessionID string) error
}

// MessageHandlers handles WebSocket requests for messages
type MessageHandlers struct {
	service      *service.Service
	orchestrator OrchestratorService
	logger       *logger.Logger
}

// NewMessageHandlers creates a new MessageHandlers instance
func NewMessageHandlers(svc *service.Service, orchestrator OrchestratorService, log *logger.Logger) *MessageHandlers {
	return &MessageHandlers{
		service:      svc,
		orchestrator: orchestrator,
		logger:       log.WithFields(zap.String("component", "task-message-handlers")),
	}
}

// RegisterMessageRoutes registers message HTTP + WebSocket handlers
func RegisterMessageRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, svc *service.Service, orchestrator OrchestratorService, log *logger.Logger) {
	handlers := NewMessageHandlers(svc, orchestrator, log)
	handlers.registerHTTP(router)
	handlers.registerWS(dispatcher)
}

func (h *MessageHandlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/agent-sessions/:id/messages", h.httpListMessages)
	api.GET("/task-sessions/:id/messages", h.httpListMessages) // Alias for SSR compatibility
}

func (h *MessageHandlers) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionMessageAdd, h.wsAddMessage)
	dispatcher.RegisterFunc(ws.ActionMessageList, h.wsListMessages)
}

func (h *MessageHandlers) httpListMessages(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task session id is required"})
		return
	}
	before := c.Query("before")
	after := c.Query("after")
	sort := strings.ToLower(strings.TrimSpace(c.Query("sort")))
	limitProvided := strings.TrimSpace(c.Query("limit")) != ""
	paginated := limitProvided || before != "" || after != "" || sort != ""
	if before != "" && after != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only one of before or after can be set"})
		return
	}
	limit := 0
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil {
			limit = parsed
		}
	}
	if sort != "" && sort != "asc" && sort != "desc" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sort must be asc or desc"})
		return
	}

	var (
		resp dto.ListMessagesResponse
		err  error
	)
	if paginated {
		var messages []*models.Message
		var hasMore bool
		messages, hasMore, err = h.service.ListMessagesPaginated(c.Request.Context(), service.ListMessagesRequest{
			TaskSessionID: sessionID,
			Limit:         limit,
			Before:        before,
			After:         after,
			Sort:          sort,
		})
		if err == nil {
			result := make([]*v1.Message, 0, len(messages))
			for _, message := range messages {
				result = append(result, message.ToAPI())
			}
			cursor := ""
			if len(result) > 0 {
				cursor = result[len(result)-1].ID
			}
			resp = dto.ListMessagesResponse{
				Messages: result,
				Total:    len(result),
				HasMore:  hasMore,
				Cursor:   cursor,
			}
		}
	} else {
		var messages []*models.Message
		messages, err = h.service.ListMessages(c.Request.Context(), sessionID)
		if err == nil {
			result := make([]*v1.Message, 0, len(messages))
			for _, message := range messages {
				result = append(result, message.ToAPI())
			}
			resp = dto.ListMessagesResponse{
				Messages: result,
				Total:    len(result),
				HasMore:  false,
				Cursor:   "",
			}
		}
	}
	if err != nil {
		h.logger.Error("failed to list messages", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list messages"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// WS handlers

type wsAddMessageRequest struct {
	TaskID            string                 `json:"task_id"`
	TaskSessionID     string                 `json:"session_id"`
	Content           string                 `json:"content"`
	AuthorID          string                 `json:"author_id,omitempty"`
	Model             string                 `json:"model,omitempty"`
	PlanMode          bool                   `json:"plan_mode,omitempty"`
	HasReviewComments bool                   `json:"has_review_comments,omitempty"`
	Attachments       []v1.MessageAttachment `json:"attachments,omitempty"`
	ContextFiles      []v1.ContextFileMeta   `json:"context_files,omitempty"`
}

func (h *MessageHandlers) wsAddMessage(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsAddMessageRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	req.Content = strings.TrimSpace(req.Content)

	if errMsg := validateAddMessageRequest(req); errMsg != "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, errMsg, nil)
	}

	// Check session state — may block the message or flag it as a create-start
	sessionResp, wsErr := h.checkSessionStateForMessage(ctx, msg, req.TaskSessionID)
	if wsErr != nil {
		return wsErr, nil
	}
	isCreatedSession := sessionResp.Session.State == models.TaskSessionStateCreated

	// Transition task from REVIEW → IN_PROGRESS if needed
	if err := h.ensureTaskInProgress(ctx, req.TaskID); err != nil {
		h.logger.Error("failed to get task", zap.String("task_id", req.TaskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get task", nil)
	}

	// Build metadata with attachments, plan mode, review comments, and context files
	meta := orchestrator.NewUserMessageMeta().
		WithPlanMode(req.PlanMode).
		WithReviewComments(req.HasReviewComments).
		WithAttachments(req.Attachments).
		WithContextFiles(req.ContextFiles)

	message, err := h.service.CreateMessage(ctx, &service.CreateMessageRequest{
		TaskSessionID: req.TaskSessionID,
		TaskID:        req.TaskID,
		Content:       req.Content,
		AuthorType:    "user",
		AuthorID:      req.AuthorID,
		Metadata:      meta.ToMap(),
	})
	if err != nil {
		h.logger.Error("failed to create message", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create message", nil)
	}

	apiMsg := message.ToAPI()
	response, err := ws.NewResponse(msg.ID, msg.Action, apiMsg)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to encode response", nil)
	}

	// Auto-forward message as prompt to running agent if orchestrator is available.
	// This runs async so the WS request can respond immediately.
	// Use context.WithoutCancel so the prompt continues even if the WebSocket client disconnects.
	if h.orchestrator != nil {
		h.dispatchPromptAsync(ctx, req, sessionResp.Session.AgentProfileID, isCreatedSession)
	}

	return response, nil
}

// validateAddMessageRequest returns a non-empty error string if the request is invalid.
func validateAddMessageRequest(req wsAddMessageRequest) string {
	if req.TaskSessionID == "" {
		return "session_id is required"
	}
	if req.TaskID == "" {
		return "task_id is required"
	}
	// Content can be empty if there are attachments (image-only messages)
	if req.Content == "" && len(req.Attachments) == 0 {
		return "content or attachments are required"
	}
	return ""
}

// checkSessionStateForMessage loads the session and returns an error WS message if the
// session is in a state that blocks new messages (running, failed, cancelled).
func (h *MessageHandlers) checkSessionStateForMessage(ctx context.Context, msg *ws.Message, sessionID string) (*dto.GetTaskSessionResponse, *ws.Message) {
	session, err := h.service.GetTaskSession(ctx, sessionID)
	if err != nil {
		h.logger.Error("failed to get task session", zap.String("session_id", sessionID), zap.Error(err))
		wsErr, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get task session", nil)
		return nil, wsErr
	}
	sessionDTO := dto.FromTaskSession(session)
	resp := &dto.GetTaskSessionResponse{Session: sessionDTO}
	switch sessionDTO.State {
	case models.TaskSessionStateRunning:
		h.logger.Warn("rejected message submission while agent is busy",
			zap.String("session_id", sessionID),
			zap.String("session_state", string(sessionDTO.State)))
		wsErr, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Agent is currently processing. Please wait for the current operation to complete.", nil)
		return nil, wsErr
	case models.TaskSessionStateFailed, models.TaskSessionStateCancelled:
		wsErr, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Session has ended. Please create a new session to continue.", nil)
		return nil, wsErr
	}
	return resp, nil
}

// ensureTaskInProgress fetches the task and transitions it from REVIEW → IN_PROGRESS if needed.
// Returns an error only when the task cannot be fetched.
func (h *MessageHandlers) ensureTaskInProgress(ctx context.Context, taskID string) error {
	task, err := h.service.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task.State != v1.TaskStateReview {
		return nil
	}
	if _, err := h.service.UpdateTaskState(ctx, taskID, v1.TaskStateInProgress); err != nil {
		h.logger.Error("failed to transition task from REVIEW to IN_PROGRESS",
			zap.String("task_id", taskID),
			zap.Error(err))
	} else {
		h.logger.Info("task transitioned from REVIEW to IN_PROGRESS",
			zap.String("task_id", taskID))
	}
	return nil
}

// dispatchPromptAsync runs on_turn_start handling and then forwards the message to the
// agent as a prompt in a background goroutine.
func (h *MessageHandlers) dispatchPromptAsync(ctx context.Context, req wsAddMessageRequest, agentProfileID string, isCreatedSession bool) {
	// Process on_turn_start events synchronously BEFORE sending the prompt.
	// This transitions the task to the right step (e.g., Review → In Progress)
	// before the agent receives the message.
	if !isCreatedSession {
		if err := h.orchestrator.ProcessOnTurnStart(ctx, req.TaskID, req.TaskSessionID); err != nil {
			h.logger.Warn("failed to process on_turn_start",
				zap.String("task_id", req.TaskID),
				zap.String("session_id", req.TaskSessionID),
				zap.Error(err))
		}
	}
	taskID := req.TaskID
	sessionID := req.TaskSessionID
	content := req.Content
	model := req.Model
	planMode := req.PlanMode
	attachments := req.Attachments
	go func() {
		promptCtx := context.WithoutCancel(ctx)
		h.forwardMessageAsPrompt(promptCtx, taskID, sessionID, agentProfileID, content, model, planMode, attachments, isCreatedSession)
	}()
}

// forwardMessageAsPrompt sends a user message to the agent as a prompt.
// For CREATED sessions, it starts the agent; otherwise it prompts the running agent,
// with automatic resume handling if the agent is not found.
func (h *MessageHandlers) forwardMessageAsPrompt(
	ctx context.Context,
	taskID, sessionID, agentProfileID, content, model string,
	planMode bool,
	attachments []v1.MessageAttachment,
	startCreated bool,
) {
	// For CREATED sessions, start the agent with this message as the initial prompt
	if startCreated {
		if err := h.orchestrator.StartCreatedSession(ctx, taskID, sessionID, agentProfileID, content); err != nil {
			h.logger.Warn("failed to start created session from message",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))

			errorMsg := "Failed to start agent"
			if _, createErr := h.service.CreateMessage(ctx, &service.CreateMessageRequest{
				TaskSessionID: sessionID,
				TaskID:        taskID,
				Content:       errorMsg,
				AuthorType:    "agent",
				Type:          string(v1.MessageTypeError),
				Metadata: map[string]interface{}{
					"error": err.Error(),
				},
			}); createErr != nil {
				h.logger.Error("failed to create error message",
					zap.String("task_id", taskID),
					zap.String("session_id", sessionID),
					zap.Error(createErr))
			}
		}
		return
	}

	_, err := h.orchestrator.PromptTask(ctx, taskID, sessionID, content, model, planMode, attachments)
	if err != nil {
		err = h.handlePromptWithResume(ctx, taskID, sessionID, content, model, planMode, attachments, err)
	}
	if err != nil {
		h.createPromptErrorMessage(ctx, taskID, sessionID, err)
	}
}

// handlePromptWithResume attempts to resume a session and retry a prompt when the
// initial prompt fails with ErrExecutionNotFound.
func (h *MessageHandlers) handlePromptWithResume(
	ctx context.Context,
	taskID, sessionID, content, model string,
	planMode bool,
	attachments []v1.MessageAttachment,
	origErr error,
) error {
	if !errors.Is(origErr, executor.ErrExecutionNotFound) {
		return origErr
	}
	if resumeErr := h.orchestrator.ResumeTaskSession(ctx, taskID, sessionID); resumeErr != nil {
		h.logger.Warn("failed to resume task session for prompt",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(resumeErr))
		return origErr
	}
	// Wait for the agent to become ready after resume.
	// ResumeTaskSession starts the agent asynchronously, so we poll
	// the session state until it transitions to a promptable state.
	if waitErr := h.waitForSessionReady(ctx, sessionID); waitErr != nil {
		h.logger.Warn("session did not become ready after resume",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(waitErr))
		return waitErr
	}
	_, err := h.orchestrator.PromptTask(ctx, taskID, sessionID, content, model, planMode, attachments)
	return err
}

// createPromptErrorMessage creates an agent error message visible to the user when
// forwarding a prompt to the agent fails.
func (h *MessageHandlers) createPromptErrorMessage(ctx context.Context, taskID, sessionID string, promptErr error) {
	h.logger.Warn("failed to forward message as prompt to agent",
		zap.String("task_id", taskID),
		zap.Error(promptErr))

	errorMsg := "Failed to send message to agent"
	if strings.Contains(promptErr.Error(), "context deadline exceeded") || strings.Contains(promptErr.Error(), "timeout") {
		errorMsg = "Request timed out. The agent may be processing a complex task. Please try again."
	} else if errors.Is(promptErr, executor.ErrExecutionNotFound) {
		errorMsg = "Agent is not running. Please restart the session."
	}

	if _, createErr := h.service.CreateMessage(ctx, &service.CreateMessageRequest{
		TaskSessionID: sessionID,
		TaskID:        taskID,
		Content:       errorMsg,
		AuthorType:    "agent",
		Type:          string(v1.MessageTypeError),
		Metadata: map[string]interface{}{
			"error": promptErr.Error(),
		},
	}); createErr != nil {
		h.logger.Error("failed to create error message for prompt failure",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(createErr))
	}
}

// waitForSessionReady polls the session state after a resume operation until the agent
// is ready to accept prompts. It returns nil when the session reaches WAITING_FOR_INPUT
// state, or an error if the session transitions to FAILED or the timeout is exceeded.
func (h *MessageHandlers) waitForSessionReady(ctx context.Context, sessionID string) error {
	const (
		pollInterval = 1 * time.Second
		maxWait      = 90 * time.Second
	)
	deadline := time.Now().Add(maxWait)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for session to become ready after resume")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
		session, err := h.service.GetTaskSession(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("failed to check session state: %w", err)
		}
		switch session.State {
		case models.TaskSessionStateWaitingForInput:
			return nil
		case models.TaskSessionStateFailed:
			errMsg := session.ErrorMessage
			if errMsg == "" {
				errMsg = "session failed during resume"
			}
			return fmt.Errorf("session failed after resume: %s", errMsg)
		case models.TaskSessionStateCancelled, models.TaskSessionStateCompleted:
			return fmt.Errorf("session in unexpected state after resume: %s", session.State)
		default:
			// STARTING or RUNNING — keep polling
		}
	}
}

type wsListMessagesRequest struct {
	TaskSessionID string `json:"session_id"`
	Limit         int    `json:"limit"`
	Before        string `json:"before"`
	After         string `json:"after"`
	Sort          string `json:"sort"`
}

func (h *MessageHandlers) wsListMessages(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsListMessagesRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskSessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.Before != "" && req.After != "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "only one of before or after can be set", nil)
	}
	if req.Sort != "" && req.Sort != "asc" && req.Sort != "desc" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "sort must be asc or desc", nil)
	}

	messages, hasMore, err := h.service.ListMessagesPaginated(ctx, service.ListMessagesRequest{
		TaskSessionID: req.TaskSessionID,
		Limit:         req.Limit,
		Before:        req.Before,
		After:         req.After,
		Sort:          req.Sort,
	})
	if err != nil {
		h.logger.Error("failed to list messages", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list messages", nil)
	}
	result := make([]*v1.Message, 0, len(messages))
	for _, message := range messages {
		result = append(result, message.ToAPI())
	}
	cursor := ""
	if len(result) > 0 {
		cursor = result[len(result)-1].ID
	}
	resp := dto.ListMessagesResponse{
		Messages: result,
		Total:    len(result),
		HasMore:  hasMore,
		Cursor:   cursor,
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}
