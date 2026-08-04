package handlers

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

const (
	// queueErrorCodeEntryNotFound is surfaced when an edit/remove targets an entry
	// that has already been drained (atomic-take won the race).
	queueErrorCodeEntryNotFound = "entry_not_found"
	queueErrorCodeSessionBusy   = "session_busy"
	queueErrorCodeNotPromptable = "session_not_promptable"
	// queueErrorCodeMergeReferenceOverflow is surfaced when a merge would push
	// the combined entity references past the per-message cap; the merge is
	// rejected atomically instead of dropping persisted references.
	queueErrorCodeMergeReferenceOverflow = "merge_reference_overflow"
	queueInvalidReferences               = "Invalid entity references"
	queueAccessDenied                    = "Session not found"

	// Payload field names — extracted to satisfy goconst (≥3 occurrences).
	fieldSessionID = "session_id"
	fieldEntryID   = "entry_id"
	fieldQueueSize = "queue_size"
	fieldMax       = "max"
)

// QueueService is the surface the handlers depend on. Real implementation lives
// in messagequeue.Service.
type QueueService interface {
	QueueMessageWithMetadata(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []messagequeue.MessageAttachment, metadata map[string]interface{}) (*messagequeue.QueuedMessage, error)
	AppendContent(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []messagequeue.MessageAttachment) (*messagequeue.QueuedMessage, bool, error)
	UpdateMessageWithMetadata(ctx context.Context, sessionID, entryID, content string, attachments []messagequeue.MessageAttachment, metadataUpdates map[string]interface{}, queuedBy string) error
	RemoveEntry(ctx context.Context, sessionID, entryID string) error
	MergeIntoAbove(ctx context.Context, sessionID, entryID, queuedBy string) (*messagequeue.QueuedMessage, error)
	CancelAll(ctx context.Context, sessionID string) (int, error)
	GetStatus(ctx context.Context, sessionID string) *messagequeue.QueueStatus
}

// QueueDrainer drains a single queued entry when the session is promptable.
type QueueDrainer interface {
	DrainQueuedMessage(ctx context.Context, sessionID string) (bool, error)
}

// QueueAccessAuthorizer scopes queue reads and mutations to visible sessions.
type QueueAccessAuthorizer interface {
	AuthorizeSessionAccess(ctx context.Context, sessionID string) error
	AuthorizeTaskSessionAccess(ctx context.Context, taskID, sessionID string) error
}

// QueueHandlers handles WebSocket message-queue operations.
type QueueHandlers struct {
	queueService       QueueService
	queueDrainer       QueueDrainer
	accessAuthorizer   QueueAccessAuthorizer
	eventBus           bus.EventBus
	logger             *logger.Logger
	referenceValidator entityrefs.SubmissionValidator
}

// NewQueueHandlers creates a new QueueHandlers instance.
func NewQueueHandlers(
	queueService QueueService,
	eventBus bus.EventBus,
	log *logger.Logger,
	queueDrainer QueueDrainer,
	accessAuthorizer QueueAccessAuthorizer,
	validators ...entityrefs.SubmissionValidator,
) *QueueHandlers {
	var referenceValidator entityrefs.SubmissionValidator
	if len(validators) > 0 {
		referenceValidator = validators[0]
	}
	return &QueueHandlers{
		queueService:       queueService,
		queueDrainer:       queueDrainer,
		accessAuthorizer:   accessAuthorizer,
		eventBus:           eventBus,
		logger:             log.WithFields(zap.String("component", "queue-handlers")),
		referenceValidator: referenceValidator,
	}
}

// RegisterHandlers registers queue handlers with the dispatcher.
func (h *QueueHandlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionMessageQueueAdd, h.wsQueueMessage)
	d.RegisterFunc(ws.ActionMessageQueueCancel, h.wsCancelAll)
	d.RegisterFunc(ws.ActionMessageQueueGet, h.wsGetQueueStatus)
	d.RegisterFunc(ws.ActionMessageQueueUpdate, h.wsUpdateMessage)
	d.RegisterFunc(ws.ActionMessageQueueAppend, h.wsAppendToQueue)
	d.RegisterFunc(ws.ActionMessageQueueDrain, h.wsDrainQueue)
	d.RegisterFunc(ws.ActionMessageQueueRemove, h.wsRemoveEntry)
	d.RegisterFunc(ws.ActionMessageQueueMerge, h.wsMergeIntoAbove)
}

type wsQueueMessageRequest struct {
	SessionID        string                           `json:"session_id"`
	TaskID           string                           `json:"task_id"`
	Content          string                           `json:"content"`
	Model            string                           `json:"model,omitempty"`
	PlanMode         bool                             `json:"plan_mode,omitempty"`
	Attachments      []messagequeue.MessageAttachment `json:"attachments,omitempty"`
	EntityReferences []v1.EntityReference             `json:"entity_references,omitempty"`
	UserID           string                           `json:"user_id,omitempty"`
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
	if denied := h.authorizeTaskSession(ctx, msg, req.TaskID, req.SessionID); denied != nil {
		return denied, nil
	}
	if req.Content == "" && len(req.Attachments) == 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "content or attachments are required", nil)
	}
	if invalid := firstInvalidDeliveryMode(req.Attachments); invalid >= 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "attachment delivery_mode must be prompt or path",
			map[string]interface{}{"attachment_index": invalid})
	}
	if messagequeue.IsReservedQueuedBy(req.UserID) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, reservedIdentityError(req.UserID), nil)
	}
	references, err := h.validateSubmittedReferences(ctx, req.SessionID, req.TaskID, req.EntityReferences)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, queueInvalidReferences, nil)
	}
	req.EntityReferences = references

	// Default empty user_id to QueuedByUser so the entry has a non-empty owner;
	// the UpdateMessage handler relies on this so its filter against agent
	// entries (queued_by="agent") is always meaningful.
	queuedBy := req.UserID
	if queuedBy == "" {
		queuedBy = messagequeue.QueuedByUser
	}
	metadata := orchestrator.NewUserMessageMeta().WithEntityReferences(req.EntityReferences).ToMap()
	queued, err := h.queueService.QueueMessageWithMetadata(ctx, req.SessionID, req.TaskID, req.Content, req.Model, queuedBy, req.PlanMode, req.Attachments, metadata)
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
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
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

type wsDrainQueueRequest struct {
	SessionID string `json:"session_id"`
}

func (h *QueueHandlers) wsDrainQueue(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsDrainQueueRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}
	if h.queueDrainer == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Queue drain is unavailable", nil)
	}

	drained, err := h.queueDrainer.DrainQueuedMessage(ctx, req.SessionID)
	if err != nil {
		switch {
		case errors.Is(err, orchestrator.ErrAgentPromptInProgress):
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeSessionBusy, "Session is busy", nil)
		case errors.Is(err, orchestrator.ErrSessionNotPromptable):
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeNotPromptable, "Session is not ready for input", nil)
		default:
			h.logger.Error("failed to drain queued message", zap.String(fieldSessionID, req.SessionID), zap.Error(err))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to drain queued message", nil)
		}
	}

	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		fieldSessionID: req.SessionID,
		"drained":      drained,
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
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}

	status := h.queueService.GetStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, status)
}

type wsUpdateMessageRequest struct {
	SessionID        string                           `json:"session_id"`
	EntryID          string                           `json:"entry_id"`
	Content          string                           `json:"content"`
	Attachments      []messagequeue.MessageAttachment `json:"attachments,omitempty"`
	EntityReferences []v1.EntityReference             `json:"entity_references,omitempty"`
	UserID           string                           `json:"user_id,omitempty"`
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
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}
	if req.EntryID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "entry_id is required", nil)
	}
	if req.Content == "" && len(req.Attachments) == 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "content or attachments are required", nil)
	}
	if invalid := firstInvalidDeliveryMode(req.Attachments); invalid >= 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "attachment delivery_mode must be prompt or path",
			map[string]interface{}{"attachment_index": invalid})
	}

	// Reject any client-supplied identity that would impersonate the agent.
	// Without this guard a hostile WS client could send user_id="agent" to
	// satisfy the `WHERE queued_by = ?` filter on inter-task entries and
	// overwrite their content. The reserved sentinel must be settable only
	// from the inter-task dispatch path inside the backend.
	if messagequeue.IsReservedQueuedBy(req.UserID) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, reservedIdentityError(req.UserID), nil)
	}
	referencesProvided := req.EntityReferences != nil
	references, err := h.validateSubmittedReferences(ctx, req.SessionID, "", req.EntityReferences)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, queueInvalidReferences, nil)
	}
	req.EntityReferences = references
	// Default empty user_id to QueuedByUser so the UpdateContent guard always
	// runs against a non-empty owner. Agent entries (queued_by="agent") then
	// fail the filter, mirroring the canEdit UI gate at the WS layer.
	queuedBy := req.UserID
	if queuedBy == "" {
		queuedBy = messagequeue.QueuedByUser
	}
	var metadataUpdates map[string]interface{}
	if referencesProvided {
		var referenceMetadata interface{}
		if len(req.EntityReferences) > 0 {
			referenceMetadata = req.EntityReferences
		}
		metadataUpdates = map[string]interface{}{messagequeue.MetadataEntityReferences: referenceMetadata}
	}
	if err := h.queueService.UpdateMessageWithMetadata(ctx, req.SessionID, req.EntryID, req.Content, req.Attachments, metadataUpdates, queuedBy); err != nil {
		if errors.Is(err, messagequeue.ErrEntryNotFound) {
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound, "Queue entry was already drained or not owned by caller", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}

	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{fieldEntryID: req.EntryID})
}

func (h *QueueHandlers) validateSubmittedReferences(
	ctx context.Context,
	sessionID, taskID string,
	references []v1.EntityReference,
) ([]v1.EntityReference, error) {
	if len(references) == 0 {
		return nil, nil
	}
	if h.referenceValidator == nil {
		return nil, entityrefs.ErrUnauthorizedReference
	}
	return h.referenceValidator.ValidateForSubmission(ctx, sessionID, taskID, references)
}

func firstInvalidDeliveryMode(attachments []messagequeue.MessageAttachment) int {
	for i, att := range attachments {
		if att.DeliveryMode != "" && att.DeliveryMode != "prompt" && att.DeliveryMode != "path" {
			return i
		}
	}
	return -1
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
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}
	if req.EntryID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "entry_id is required", nil)
	}

	if err := h.queueService.RemoveEntry(ctx, req.SessionID, req.EntryID); err != nil {
		if errors.Is(err, messagequeue.ErrEntryNotFound) {
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound, "Queue entry is no longer pending", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}

	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{fieldEntryID: req.EntryID})
}

// wsMergeIntoAboveRequest is the payload for ActionMessageQueueMerge: the
// session whose queue is modified and the id of the entry to fold into the
// entry directly above it. user_id is forwarded for ownership checks and is
// optional (the server defaults to the reserved "user" identity).
type wsMergeIntoAboveRequest struct {
	SessionID string `json:"session_id"`
	EntryID   string `json:"entry_id"`
	UserID    string `json:"user_id,omitempty"`
}

// wsMergeIntoAbove handles ActionMessageQueueMerge, folding the referenced
// queued entry into the entry above it and broadcasting the updated queue.
func (h *QueueHandlers) wsMergeIntoAbove(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsMergeIntoAboveRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		// Required so publishStatus can broadcast the post-merge list.
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}
	if req.EntryID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "entry_id is required", nil)
	}
	if messagequeue.IsReservedQueuedBy(req.UserID) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, reservedIdentityError(req.UserID), nil)
	}
	// Default empty user_id to QueuedByUser so the merge ownership guard runs
	// against a non-empty owner, mirroring wsUpdateMessage.
	queuedBy := req.UserID
	if queuedBy == "" {
		queuedBy = messagequeue.QueuedByUser
	}

	merged, err := h.queueService.MergeIntoAbove(ctx, req.SessionID, req.EntryID, queuedBy)
	if err != nil {
		if errors.Is(err, messagequeue.ErrEntryNotFound) {
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound, "Queue entry was already drained or not owned by caller", nil)
		}
		if errors.Is(err, messagequeue.ErrNoMergeTarget) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "No mergeable message above this entry", nil)
		}
		if errors.Is(err, messagequeue.ErrMergeReferenceOverflow) {
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeMergeReferenceOverflow, err.Error(), nil)
		}
		h.logger.Error("failed to merge queued message", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to merge queued message", nil)
	}

	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{fieldEntryID: merged.ID})
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
	if denied := h.authorizeTaskSession(ctx, msg, req.TaskID, req.SessionID); denied != nil {
		return denied, nil
	}
	if req.Content == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "content is required", nil)
	}
	if messagequeue.IsReservedQueuedBy(req.UserID) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, reservedIdentityError(req.UserID), nil)
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

func reservedIdentityError(queuedBy string) string {
	if queuedBy == messagequeue.QueuedByAgent {
		return "user_id may not impersonate the agent identity"
	}
	return "user_id may not impersonate a reserved identity"
}

func (h *QueueHandlers) authorizeSession(ctx context.Context, msg *ws.Message, sessionID string) *ws.Message {
	if h.accessAuthorizer == nil {
		return queueAccessDeniedResponse(msg)
	}
	if err := h.accessAuthorizer.AuthorizeSessionAccess(ctx, sessionID); err != nil {
		return queueAccessDeniedResponse(msg)
	}
	return nil
}

func (h *QueueHandlers) authorizeTaskSession(
	ctx context.Context,
	msg *ws.Message,
	taskID, sessionID string,
) *ws.Message {
	if h.accessAuthorizer == nil {
		return queueAccessDeniedResponse(msg)
	}
	if err := h.accessAuthorizer.AuthorizeTaskSessionAccess(ctx, taskID, sessionID); err != nil {
		return queueAccessDeniedResponse(msg)
	}
	return nil
}

func queueAccessDeniedResponse(msg *ws.Message) *ws.Message {
	response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, queueAccessDenied, nil)
	return response
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
