package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/controller"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// OrchestratorService defines the interface for orchestrator operations
type OrchestratorService interface {
	PromptTask(ctx context.Context, taskID, sessionID, prompt, model string, planMode bool, attachments []v1.MessageAttachment) (*orchestrator.PromptResult, error)
	ResumeTaskSession(ctx context.Context, taskID, taskSessionID string) error
}

// MessageHandlers handles WebSocket requests for messages
type MessageHandlers struct {
	messageController *controller.MessageController
	taskController    *controller.TaskController
	orchestrator      OrchestratorService
	logger            *logger.Logger
}

// NewMessageHandlers creates a new MessageHandlers instance
func NewMessageHandlers(messageCtrl *controller.MessageController, taskCtrl *controller.TaskController, orchestrator OrchestratorService, log *logger.Logger) *MessageHandlers {
	return &MessageHandlers{
		messageController: messageCtrl,
		taskController:    taskCtrl,
		orchestrator:      orchestrator,
		logger:            log.WithFields(zap.String("component", "task-message-handlers")),
	}
}

// RegisterMessageRoutes registers message HTTP + WebSocket handlers
func RegisterMessageRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, messageCtrl *controller.MessageController, taskCtrl *controller.TaskController, orchestrator OrchestratorService, log *logger.Logger) {
	handlers := NewMessageHandlers(messageCtrl, taskCtrl, orchestrator, log)
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
		resp, err = h.messageController.ListMessages(c.Request.Context(), dto.ListMessagesRequest{
			TaskSessionID: sessionID,
			Limit:         limit,
			Before:        before,
			After:         after,
			Sort:          sort,
		})
	} else {
		resp, err = h.messageController.ListAllMessages(c.Request.Context(), sessionID)
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
	TaskID        string                 `json:"task_id"`
	TaskSessionID string                 `json:"session_id"`
	Content       string                 `json:"content"`
	AuthorID      string                 `json:"author_id,omitempty"`
	Model         string                 `json:"model,omitempty"`
	PlanMode      bool                   `json:"plan_mode,omitempty"`
	Attachments   []v1.MessageAttachment `json:"attachments,omitempty"`
}

func (h *MessageHandlers) wsAddMessage(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsAddMessageRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskSessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	// Content can be empty if there are attachments (image-only messages)
	if req.Content == "" && len(req.Attachments) == 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "content or attachments are required", nil)
	}

	// Check if session is currently processing a prompt (RUNNING state)
	// This prevents duplicate/concurrent prompts that can cause race conditions
	sessionResp, err := h.taskController.GetTaskSession(ctx, dto.GetTaskSessionRequest{TaskSessionID: req.TaskSessionID})
	if err != nil {
		h.logger.Error("failed to get task session", zap.String("session_id", req.TaskSessionID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get task session", nil)
	}
	if sessionResp.Session.State == models.TaskSessionStateRunning {
		h.logger.Warn("rejected message submission while agent is busy",
			zap.String("session_id", req.TaskSessionID),
			zap.String("session_state", string(sessionResp.Session.State)))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Agent is currently processing. Please wait for the current operation to complete.", nil)
	}

	// Get the current task state to determine if we need to transition
	task, err := h.taskController.GetTask(ctx, dto.GetTaskRequest{ID: req.TaskID})
	if err != nil {
		h.logger.Error("failed to get task", zap.String("task_id", req.TaskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get task", nil)
	}

	// If task is in REVIEW state, transition back to IN_PROGRESS (also moves to matching column)
	if task.State == v1.TaskStateReview {
		if _, err := h.taskController.UpdateTaskState(ctx, dto.UpdateTaskStateRequest{ID: req.TaskID, State: v1.TaskStateInProgress}); err != nil {
			h.logger.Error("failed to transition task from REVIEW to IN_PROGRESS",
				zap.String("task_id", req.TaskID),
				zap.Error(err))
		} else {
			h.logger.Info("task transitioned from REVIEW to IN_PROGRESS",
				zap.String("task_id", req.TaskID))
		}
	}

	// Build metadata with attachments if present
	var metadata map[string]interface{}
	if len(req.Attachments) > 0 {
		metadata = map[string]interface{}{
			"attachments": req.Attachments,
		}
	}

	message, err := h.messageController.CreateMessage(ctx, dto.CreateMessageRequest{
		TaskSessionID: req.TaskSessionID,
		TaskID:        req.TaskID,
		Content:       req.Content,
		AuthorType:    "user",
		AuthorID:      req.AuthorID,
		Metadata:      metadata,
	})
	if err != nil {
		h.logger.Error("failed to create message", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create message", nil)
	}

	response, err := ws.NewResponse(msg.ID, msg.Action, message)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to encode response", nil)
	}

	// Auto-forward message as prompt to running agent if orchestrator is available.
	// This runs async so the WS request can respond immediately.
	// Use context.WithoutCancel so the prompt continues even if the WebSocket client disconnects.
	// The user's message is already saved, and agent responses are broadcast via notifications.
	if h.orchestrator != nil {
		taskID := req.TaskID
		sessionID := req.TaskSessionID
		content := req.Content
		model := req.Model
		planMode := req.PlanMode
		attachments := req.Attachments
		go func() {
			promptCtx := context.WithoutCancel(ctx)
			_, err := h.orchestrator.PromptTask(promptCtx, taskID, sessionID, content, model, planMode, attachments)
			if err != nil {
				if errors.Is(err, executor.ErrExecutionNotFound) {
					if resumeErr := h.orchestrator.ResumeTaskSession(promptCtx, taskID, sessionID); resumeErr != nil {
						h.logger.Warn("failed to resume task session for prompt",
							zap.String("task_id", taskID),
							zap.String("session_id", sessionID),
							zap.Error(resumeErr))
					} else {
						for attempt := 0; attempt < 3; attempt++ {
							time.Sleep(500 * time.Millisecond)
							_, err = h.orchestrator.PromptTask(promptCtx, taskID, sessionID, content, model, planMode, attachments)
							if err == nil {
								break
							}
						}
					}
				}
				// If prompt still failed after retries, create an error message for the user
				if err != nil {
					h.logger.Warn("failed to forward message as prompt to agent",
						zap.String("task_id", taskID),
						zap.Error(err))

					// Create an error message so the user sees feedback
					errorMsg := "Failed to send message to agent"
					if strings.Contains(err.Error(), "context deadline exceeded") || strings.Contains(err.Error(), "timeout") {
						errorMsg = "Request timed out. The agent may be processing a complex task. Please try again."
					} else if errors.Is(err, executor.ErrExecutionNotFound) {
						errorMsg = "Agent is not running. Please restart the session."
					}

					if _, createErr := h.messageController.CreateMessage(promptCtx, dto.CreateMessageRequest{
						TaskSessionID: sessionID,
						TaskID:        taskID,
						Content:       errorMsg,
						AuthorType:    "agent",
						Type:          string(v1.MessageTypeError),
						Metadata: map[string]interface{}{
							"error": err.Error(),
						},
					}); createErr != nil {
						h.logger.Error("failed to create error message for prompt failure",
							zap.String("task_id", taskID),
							zap.String("session_id", sessionID),
							zap.Error(createErr))
					}
				}
			}
		}()
	}

	return response, nil
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

	resp, err := h.messageController.ListMessages(ctx, dto.ListMessagesRequest{
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
	return ws.NewResponse(msg.ID, msg.Action, resp)
}
