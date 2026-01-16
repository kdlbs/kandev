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
	"github.com/kandev/kandev/internal/task/controller"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// PromptResult represents the result of prompting an agent
type PromptResult struct {
	StopReason   string `json:"stop_reason"`
	AgentMessage string `json:"agent_message"`
}

// OrchestratorService defines the interface for orchestrator operations
type OrchestratorService interface {
	PromptTask(ctx context.Context, taskID string, prompt string) (*PromptResult, error)
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
			Limit:          limit,
			Before:         before,
			After:          after,
			Sort:           sort,
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
	TaskID         string `json:"task_id"`
	TaskSessionID  string `json:"task_session_id"`
	Content        string `json:"content"`
	AuthorID       string `json:"author_id,omitempty"`
}

func (h *MessageHandlers) wsAddMessage(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsAddMessageRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskSessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_session_id is required", nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.Content == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "content is required", nil)
	}

	// Get the current task state to determine if we need to transition
	task, err := h.taskController.GetTask(ctx, dto.GetTaskRequest{ID: req.TaskID})
	if err != nil {
		h.logger.Error("failed to get task", zap.String("task_id", req.TaskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get task", nil)
	}

	// If task is in REVIEW state, transition back to IN_PROGRESS
	if task.State == v1.TaskStateReview {
		nextState := v1.TaskStateInProgress
		if _, err := h.taskController.UpdateTask(ctx, dto.UpdateTaskRequest{ID: req.TaskID, State: &nextState}); err != nil {
			h.logger.Error("failed to transition task from REVIEW to IN_PROGRESS",
				zap.String("task_id", req.TaskID),
				zap.Error(err))
		} else {
			h.logger.Info("task transitioned from REVIEW to IN_PROGRESS",
				zap.String("task_id", req.TaskID))
		}
	}

	message, err := h.messageController.CreateMessage(ctx, dto.CreateMessageRequest{
		TaskSessionID: req.TaskSessionID,
		TaskID:         req.TaskID,
		Content:        req.Content,
		AuthorType:     "user",
		AuthorID:       req.AuthorID,
	})
	if err != nil {
		h.logger.Error("failed to create message", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create message", nil)
	}

	// Auto-forward message as prompt to running agent if orchestrator is available
	if h.orchestrator != nil {
		_, err := h.orchestrator.PromptTask(ctx, req.TaskID, req.Content)
		if err != nil {
			if errors.Is(err, executor.ErrExecutionNotFound) {
				if resumeErr := h.orchestrator.ResumeTaskSession(ctx, req.TaskID, req.TaskSessionID); resumeErr != nil {
					h.logger.Warn("failed to resume task session for prompt",
						zap.String("task_id", req.TaskID),
						zap.String("task_session_id", req.TaskSessionID),
						zap.Error(resumeErr))
				} else {
					for attempt := 0; attempt < 3; attempt++ {
						time.Sleep(500 * time.Millisecond)
						_, err = h.orchestrator.PromptTask(ctx, req.TaskID, req.Content)
						if err == nil {
							break
						}
					}
				}
			}
			h.logger.Warn("failed to forward message as prompt to agent",
				zap.String("task_id", req.TaskID),
				zap.Error(err))
		}
	}

	return ws.NewResponse(msg.ID, msg.Action, message)
}

type wsListMessagesRequest struct {
	TaskSessionID  string `json:"task_session_id"`
	Limit          int    `json:"limit"`
	Before         string `json:"before"`
	After          string `json:"after"`
	Sort           string `json:"sort"`
}

func (h *MessageHandlers) wsListMessages(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsListMessagesRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskSessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_session_id is required", nil)
	}
	if req.Before != "" && req.After != "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "only one of before or after can be set", nil)
	}
	if req.Sort != "" && req.Sort != "asc" && req.Sort != "desc" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "sort must be asc or desc", nil)
	}

	resp, err := h.messageController.ListMessages(ctx, dto.ListMessagesRequest{
		TaskSessionID: req.TaskSessionID,
		Limit:          req.Limit,
		Before:         req.Before,
		After:          req.After,
		Sort:           req.Sort,
	})
	if err != nil {
		h.logger.Error("failed to list messages", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list messages", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}
