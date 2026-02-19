package handlers

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// QueueService defines the interface for message queue operations
type QueueService interface {
	QueueMessage(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []messagequeue.MessageAttachment) (*messagequeue.QueuedMessage, error)
	CancelQueued(ctx context.Context, sessionID string) (*messagequeue.QueuedMessage, error)
	GetStatus(ctx context.Context, sessionID string) *messagequeue.QueueStatus
	UpdateMessage(ctx context.Context, sessionID, content string) error
}

// QueueHandlers handles message queue operations
type QueueHandlers struct {
	queueService QueueService
	eventBus     bus.EventBus
	logger       *logger.Logger
}

// NewQueueHandlers creates a new QueueHandlers instance
func NewQueueHandlers(queueService QueueService, eventBus bus.EventBus, log *logger.Logger) *QueueHandlers {
	return &QueueHandlers{
		queueService: queueService,
		eventBus:     eventBus,
		logger:       log.WithFields(zap.String("component", "queue-handlers")),
	}
}

// RegisterHandlers registers queue handlers with the dispatcher
func (h *QueueHandlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionMessageQueueAdd, h.wsQueueMessage)
	d.RegisterFunc(ws.ActionMessageQueueCancel, h.wsCancelQueue)
	d.RegisterFunc(ws.ActionMessageQueueGet, h.wsGetQueueStatus)
	d.RegisterFunc(ws.ActionMessageQueueUpdate, h.wsUpdateMessage)
}

// WebSocket Handlers

type wsQueueMessageRequest struct {
	SessionID   string                           `json:"session_id"`
	TaskID      string                           `json:"task_id"`
	Content     string                           `json:"content"`
	Model       string                           `json:"model,omitempty"`
	PlanMode    bool                             `json:"plan_mode,omitempty"`
	Attachments []messagequeue.MessageAttachment `json:"attachments,omitempty"`
	UserID      string                           `json:"user_id,omitempty"`
}

func (h *QueueHandlers) wsQueueMessage(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsQueueMessageRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.Content == "" && len(req.Attachments) == 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "content or attachments are required", nil)
	}

	queued, err := h.queueService.QueueMessage(ctx, req.SessionID, req.TaskID, req.Content, req.Model, req.UserID, req.PlanMode, req.Attachments)
	if err != nil {
		h.logger.Error("failed to queue message", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to queue message", nil)
	}

	// Publish queue status changed event for real-time updates
	if h.eventBus != nil {
		status := h.queueService.GetStatus(ctx, req.SessionID)
		eventData := map[string]interface{}{
			"session_id": req.SessionID,
			"is_queued":  status.IsQueued,
			"message":    status.Message,
		}
		_ = h.eventBus.Publish(ctx, events.MessageQueueStatusChanged, bus.NewEvent(
			events.MessageQueueStatusChanged,
			"queue-handlers",
			eventData,
		))
	}

	return ws.NewResponse(msg.ID, msg.Action, queued)
}

type wsCancelQueueRequest struct {
	SessionID string `json:"session_id"`
}

func (h *QueueHandlers) wsCancelQueue(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCancelQueueRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	cancelled, err := h.queueService.CancelQueued(ctx, req.SessionID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}

	// Publish queue status changed event
	if h.eventBus != nil {
		status := h.queueService.GetStatus(ctx, req.SessionID)
		eventData := map[string]interface{}{
			"session_id": req.SessionID,
			"is_queued":  status.IsQueued,
			"message":    status.Message,
		}
		_ = h.eventBus.Publish(ctx, events.MessageQueueStatusChanged, bus.NewEvent(
			events.MessageQueueStatusChanged,
			"queue-handlers",
			eventData,
		))
	}

	return ws.NewResponse(msg.ID, msg.Action, cancelled)
}

type wsGetQueueStatusRequest struct {
	SessionID string `json:"session_id"`
}

func (h *QueueHandlers) wsGetQueueStatus(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetQueueStatusRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	status := h.queueService.GetStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, status)
}

type wsUpdateMessageRequest struct {
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
}

func (h *QueueHandlers) wsUpdateMessage(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateMessageRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	// Don't allow updating to empty content - user should cancel instead
	if req.Content == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "content cannot be empty", nil)
	}

	if err := h.queueService.UpdateMessage(ctx, req.SessionID, req.Content); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}

	status := h.queueService.GetStatus(ctx, req.SessionID)

	// Publish queue status changed event
	if h.eventBus != nil {
		eventData := map[string]interface{}{
			"session_id": req.SessionID,
			"is_queued":  status.IsQueued,
			"message":    status.Message,
		}
		_ = h.eventBus.Publish(ctx, events.MessageQueueStatusChanged, bus.NewEvent(
			events.MessageQueueStatusChanged,
			"queue-handlers",
			eventData,
		))
	}

	return ws.NewResponse(msg.ID, msg.Action, status)
}
