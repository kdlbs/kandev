package handlers

import (
	"context"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/service"
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
	svc          *service.Service
	orchestrator OrchestratorService
	logger       *logger.Logger
}

// NewCommentHandlers creates a new CommentHandlers instance
func NewCommentHandlers(svc *service.Service, orchestrator OrchestratorService, log *logger.Logger) *CommentHandlers {
	return &CommentHandlers{
		svc:          svc,
		orchestrator: orchestrator,
		logger:       log.WithFields(zap.String("component", "task-comment-handlers")),
	}
}

// RegisterCommentRoutes registers comment WebSocket handlers
func RegisterCommentRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, svc *service.Service, orchestrator OrchestratorService, log *logger.Logger) {
	handlers := NewCommentHandlers(svc, orchestrator, log)
	handlers.registerWS(dispatcher)
}

func (h *CommentHandlers) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionCommentAdd, h.wsAddComment)
	dispatcher.RegisterFunc(ws.ActionCommentList, h.wsListComments)
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

	comment, err := h.svc.CreateComment(ctx, &service.CreateCommentRequest{
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
}

func (h *CommentHandlers) wsListComments(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsListCommentsRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	comments, err := h.svc.ListComments(ctx, req.TaskID)
	if err != nil {
		h.logger.Error("failed to list comments", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list comments", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, comments)
}

