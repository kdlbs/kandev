package handlers

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

const (
	// queueErrorCodeEntryNotFound is surfaced when an edit/remove targets an entry
	// that has already been drained (atomic-take won the race).
	queueErrorCodeEntryNotFound = "entry_not_found"

	// Payload field names — extracted to satisfy goconst (≥3 occurrences).
	fieldSessionID = "session_id"
	fieldEntryID   = "entry_id"
	fieldQueueSize = "queue_size"
	fieldMax       = "max"
)

// QueueService is the surface the handlers depend on. Real implementation lives
// in messagequeue.Service.
type QueueService interface {
	QueueMessage(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []messagequeue.MessageAttachment) (*messagequeue.QueuedMessage, error)
	AppendContent(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []messagequeue.MessageAttachment) (*messagequeue.QueuedMessage, bool, error)
	UpdateMessage(ctx context.Context, sessionID, entryID, content string, attachments []messagequeue.MessageAttachment, queuedBy string) error
	RemoveEntry(ctx context.Context, sessionID, entryID string) error
	CancelAll(ctx context.Context, sessionID string) (int, error)
	GetStatus(ctx context.Context, sessionID string) *messagequeue.QueueStatus
}

// QueueHandlers handles WebSocket message-queue operations.
type QueueHandlers struct {
	queueService QueueService
	eventBus     bus.EventBus
	logger       *logger.Logger
}

// NewQueueHandlers creates a new QueueHandlers instance.
func NewQueueHandlers(queueService QueueService, eventBus bus.EventBus, log *logger.Logger) *QueueHandlers {
	return &QueueHandlers{
		queueService: queueService,
		eventBus:     eventBus,
		logger:       log.WithFields(zap.String("component", "queue-handlers")),
	}
}

// RegisterHandlers registers queue handlers with the dispatcher.
func (h *QueueHandlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionMessageQueueAdd, h.wsQueueMessage)
	d.RegisterFunc(ws.ActionMessageQueueCancel, h.wsCancelAll)
	d.RegisterFunc(ws.ActionMessageQueueGet, h.wsGetQueueStatus)
	d.RegisterFunc(ws.ActionMessageQueueUpdate, h.wsUpdateMessage)
	d.RegisterFunc(ws.ActionMessageQueueAppend, h.wsAppendToQueue)
	d.RegisterFunc(ws.ActionMessageQueueRemove, h.wsRemoveEntry)
}

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
	if req.UserID == messagequeue.QueuedByAgent {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "user_id may not impersonate the agent identity", nil)
	}

	// Default empty user_id to QueuedByUser so the entry has a non-empty owner;
	// the UpdateMessage handler relies on this so its filter against agent
	// entries (queued_by="agent") is always meaningful.
	queuedBy := req.UserID
	if queuedBy == "" {
		queuedBy = messagequeue.QueuedByUser
	}
	queued, err := h.queueService.QueueMessage(ctx, req.SessionID, req.TaskID, req.Content, req.Model, queuedBy, req.PlanMode, req.Attachments)
	if err != nil {
		if errors.Is(err, messagequeue.ErrQueueFull) {
			status := h.queueService.GetStatus(ctx, req.SessionID)
			return ws.NewError(msg.ID, msg.Action, messagequeue.QueueFullErrorCode, "Queue is full",
				map[string]interface{}{
					fieldQueueSize: status.Count,
					fieldMax:       status.Max,
				})
		}
		h.logger.Error("failed to queue message", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to queue message", nil)
	}

	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, queued)
}

type wsCancelAllRequest struct {
	SessionID string `json:"session_id"`
}

func (h *QueueHandlers) wsCancelAll(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCancelAllRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	removed, err := h.queueService.CancelAll(ctx, req.SessionID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}

	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		fieldSessionID: req.SessionID,
		"removed":      removed,
	})
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
	SessionID   string                           `json:"session_id"`
	EntryID     string                           `json:"entry_id"`
	Content     string                           `json:"content"`
	Attachments []messagequeue.MessageAttachment `json:"attachments,omitempty"`
	UserID      string                           `json:"user_id,omitempty"`
}

func (h *QueueHandlers) wsUpdateMessage(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateMessageRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		// Required so publishStatus can broadcast the post-update list to other
		// connected clients; without it they'd be left with a stale view.
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.EntryID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "entry_id is required", nil)
	}
	if req.Content == "" && len(req.Attachments) == 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "content or attachments are required", nil)
	}

	// Reject any client-supplied identity that would impersonate the agent.
	// Without this guard a hostile WS client could send user_id="agent" to
	// satisfy the `WHERE queued_by = ?` filter on inter-task entries and
	// overwrite their content. The reserved sentinel must be settable only
	// from the inter-task dispatch path inside the backend.
	if req.UserID == messagequeue.QueuedByAgent {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "user_id may not impersonate the agent identity", nil)
	}
	// Default empty user_id to QueuedByUser so the UpdateContent guard always
	// runs against a non-empty owner. Agent entries (queued_by="agent") then
	// fail the filter, mirroring the canEdit UI gate at the WS layer.
	queuedBy := req.UserID
	if queuedBy == "" {
		queuedBy = messagequeue.QueuedByUser
	}
	if err := h.queueService.UpdateMessage(ctx, req.SessionID, req.EntryID, req.Content, req.Attachments, queuedBy); err != nil {
		if errors.Is(err, messagequeue.ErrEntryNotFound) {
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound, "Queue entry was already drained or not owned by caller", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}

	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{fieldEntryID: req.EntryID})
}

type wsRemoveEntryRequest struct {
	SessionID string `json:"session_id"`
	EntryID   string `json:"entry_id"`
}

func (h *QueueHandlers) wsRemoveEntry(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsRemoveEntryRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		// Required so publishStatus can broadcast the post-removal list.
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.EntryID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "entry_id is required", nil)
	}

	if err := h.queueService.RemoveEntry(ctx, req.SessionID, req.EntryID); err != nil {
		if errors.Is(err, messagequeue.ErrEntryNotFound) {
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound, "Queue entry was already drained or not owned by caller", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}

	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{fieldEntryID: req.EntryID})
}

type wsAppendToQueueRequest struct {
	SessionID string `json:"session_id"`
	TaskID    string `json:"task_id"`
	Content   string `json:"content"`
	Model     string `json:"model,omitempty"`
	PlanMode  bool   `json:"plan_mode,omitempty"`
	UserID    string `json:"user_id,omitempty"`
}

func (h *QueueHandlers) wsAppendToQueue(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsAppendToQueueRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.Content == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "content is required", nil)
	}
	if req.UserID == messagequeue.QueuedByAgent {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "user_id may not impersonate the agent identity", nil)
	}

	queuedBy := req.UserID
	if queuedBy == "" {
		queuedBy = messagequeue.QueuedByUser
	}
	queued, appended, err := h.queueService.AppendContent(ctx, req.SessionID, req.TaskID, req.Content, req.Model, queuedBy, req.PlanMode, nil)
	if err != nil {
		if errors.Is(err, messagequeue.ErrQueueFull) {
			status := h.queueService.GetStatus(ctx, req.SessionID)
			return ws.NewError(msg.ID, msg.Action, messagequeue.QueueFullErrorCode, "Queue is full",
				map[string]interface{}{
					fieldQueueSize: status.Count,
					fieldMax:       status.Max,
				})
		}
		h.logger.Error("failed to append to queue", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to queue message", nil)
	}

	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		fieldEntryID: queued.ID,
		"was_append": appended,
	})
}

// publishStatus emits the latest QueueStatus on the event bus so the frontend
// updates its store after every mutation.
func (h *QueueHandlers) publishStatus(ctx context.Context, sessionID string) {
	if h.eventBus == nil {
		return
	}
	status := h.queueService.GetStatus(ctx, sessionID)
	_ = h.eventBus.Publish(ctx, events.MessageQueueStatusChanged, bus.NewEvent(
		events.MessageQueueStatusChanged,
		"queue-handlers",
		map[string]interface{}{
			fieldSessionID: sessionID,
			"entries":      status.Entries,
			"count":        status.Count,
			fieldMax:       status.Max,
		},
	))
}
