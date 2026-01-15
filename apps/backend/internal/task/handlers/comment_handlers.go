package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/controller"
	"github.com/kandev/kandev/internal/task/dto"
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
}

// CommentHandlers handles WebSocket requests for comments
type CommentHandlers struct {
	commentController *controller.CommentController
	taskController    *controller.TaskController
	orchestrator OrchestratorService
	logger       *logger.Logger
}

// NewCommentHandlers creates a new CommentHandlers instance
func NewCommentHandlers(commentCtrl *controller.CommentController, taskCtrl *controller.TaskController, orchestrator OrchestratorService, log *logger.Logger) *CommentHandlers {
	return &CommentHandlers{
		commentController: commentCtrl,
		taskController:    taskCtrl,
		orchestrator: orchestrator,
		logger:       log.WithFields(zap.String("component", "task-comment-handlers")),
	}
}

// RegisterCommentRoutes registers comment WebSocket handlers
func RegisterCommentRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, commentCtrl *controller.CommentController, taskCtrl *controller.TaskController, orchestrator OrchestratorService, log *logger.Logger) {
	handlers := NewCommentHandlers(commentCtrl, taskCtrl, orchestrator, log)
	handlers.registerHTTP(router)
	handlers.registerWS(dispatcher)
}

func (h *CommentHandlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/tasks/:id/comments", h.httpListComments)
}

func (h *CommentHandlers) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionCommentAdd, h.wsAddComment)
	dispatcher.RegisterFunc(ws.ActionCommentList, h.wsListComments)
}

func (h *CommentHandlers) httpListComments(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task id is required"})
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
		resp dto.ListCommentsResponse
		err  error
	)
	if paginated {
		resp, err = h.commentController.ListComments(c.Request.Context(), dto.ListCommentsRequest{
			TaskID: taskID,
			Limit:  limit,
			Before: before,
			After:  after,
			Sort:   sort,
		})
	} else {
		resp, err = h.commentController.ListAllComments(c.Request.Context(), taskID)
	}
	if err != nil {
		h.logger.Error("failed to list comments", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list comments"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// WS handlers

type wsAddCommentRequest struct {
	TaskID   string `json:"task_id"`
	Content  string `json:"content"`
	AuthorID string `json:"author_id,omitempty"`
}

func (h *CommentHandlers) wsAddComment(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsAddCommentRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
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
	// This implements the backward movement in the review cycle:
	// REVIEW → (user sends follow-up) → IN_PROGRESS
	// The transition back to REVIEW happens in handleAgentReady when the agent finishes
	if task.State == v1.TaskStateReview {
		nextState := v1.TaskStateInProgress
		if _, err := h.taskController.UpdateTask(ctx, dto.UpdateTaskRequest{ID: req.TaskID, State: &nextState}); err != nil {
			h.logger.Error("failed to transition task from REVIEW to IN_PROGRESS",
				zap.String("task_id", req.TaskID),
				zap.Error(err))
			// Continue anyway - the comment should still be created
		} else {
			h.logger.Info("task transitioned from REVIEW to IN_PROGRESS",
				zap.String("task_id", req.TaskID))
		}
	}

	comment, err := h.commentController.CreateComment(ctx, dto.CreateCommentRequest{
		TaskID:     req.TaskID,
		Content:    req.Content,
		AuthorType: "user",
		AuthorID:   req.AuthorID,
	})
	if err != nil {
		h.logger.Error("failed to create comment", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create comment", nil)
	}

	// Auto-forward comment as prompt to running agent if orchestrator is available
	// The agent's response will be saved as a comment via the prompt_complete event
	// Note: State transition to REVIEW happens via handleAgentReady when the agent finishes
	if h.orchestrator != nil {
		_, err := h.orchestrator.PromptTask(ctx, req.TaskID, req.Content)
		if err != nil {
			h.logger.Warn("failed to forward comment as prompt to agent",
				zap.String("task_id", req.TaskID),
				zap.Error(err))
		}
	}

	return ws.NewResponse(msg.ID, msg.Action, comment)
}

type wsListCommentsRequest struct {
	TaskID string `json:"task_id"`
	Limit  int    `json:"limit"`
	Before string `json:"before"`
	After  string `json:"after"`
	Sort   string `json:"sort"`
}

func (h *CommentHandlers) wsListComments(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsListCommentsRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.Before != "" && req.After != "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "only one of before or after can be set", nil)
	}
	sort := strings.ToLower(strings.TrimSpace(req.Sort))
	if sort != "" && sort != "asc" && sort != "desc" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "sort must be asc or desc", nil)
	}

	var (
		resp dto.ListCommentsResponse
		err  error
	)
	if req.Limit != 0 || req.Before != "" || req.After != "" || sort != "" {
		resp, err = h.commentController.ListComments(ctx, dto.ListCommentsRequest{
			TaskID: req.TaskID,
			Limit:  req.Limit,
			Before: req.Before,
			After:  req.After,
			Sort:   sort,
		})
	} else {
		resp, err = h.commentController.ListAllComments(ctx, req.TaskID)
	}
	if err != nil {
		h.logger.Error("failed to list comments", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list comments", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, resp)
}
