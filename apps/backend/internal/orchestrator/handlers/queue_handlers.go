package handlers

import (
	"context"
	"encoding/base64"
	"errors"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

const (
	// queueErrorCodeEntryNotFound is surfaced when an edit/remove targets an entry
	// that has already been drained (atomic-take won the race).
	queueErrorCodeEntryNotFound       = "entry_not_found"
	queueErrorCodeSessionBusy         = "session_busy"
	queueErrorCodeNotPromptable       = "session_not_promptable"
	queueErrorCodeSendNowQueueEmpty   = "queue_empty"
	queueErrorCodeSendNowQueueChanged = "queue_changed"
	// queueErrorCodeQueueChanged is the shared "your snapshot is stale" signal
	// for reorder drift; the wire value matches the send-now code so clients
	// reconcile with one handler.
	queueErrorCodeQueueChanged              = "queue_changed"
	queueErrorCodeSendNowConflict           = "send_now_conflict"
	queueErrorCodeSendNowTurnChanged        = "turn_changed"
	queueErrorCodeSendNowAttachmentOverflow = "send_now_attachment_overflow"
	queueErrorCodeSendNowReferenceOverflow  = "send_now_reference_overflow"
	// queueErrorCodeMergeReferenceOverflow is surfaced when a merge would push
	// the combined entity references past the per-message cap; the merge is
	// rejected atomically instead of dropping persisted references.
	queueErrorCodeMergeReferenceOverflow = "merge_reference_overflow"
	// queueErrorCodeMergeDisabled is surfaced when queued-message merging is
	// disabled via the message queue system setting.
	queueErrorCodeMergeDisabled = "merge_disabled"
	queueInvalidReferences      = "Invalid entity references"
	queueAccessDenied           = "Session not found"

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
	GetEntry(ctx context.Context, sessionID, entryID string) (*messagequeue.QueuedMessage, error)
	UpdateMessageWithMetadata(ctx context.Context, sessionID, entryID, content string, attachments []messagequeue.MessageAttachment, metadataUpdates map[string]interface{}, queuedBy string) error
	RemoveEntry(ctx context.Context, sessionID, entryID string) error
	MergeIntoAbove(ctx context.Context, sessionID, entryID, queuedBy string) (*messagequeue.QueuedMessage, error)
	ReorderEntries(ctx context.Context, sessionID string, orderedIDs []string) error
	CancelAll(ctx context.Context, sessionID string) (int, error)
	GetStatus(ctx context.Context, sessionID string) *messagequeue.QueueStatus
}

// QueueDrainer drains a single queued entry when the session is promptable.
type QueueDrainer interface {
	DrainQueuedMessage(ctx context.Context, sessionID string) (bool, error)
}

// QueueSendNowDispatcher is implemented by the orchestrator service. It is
// kept separate from QueueDrainer so queue-focused handlers can retain their
// small test doubles while the new action gets the replacement-turn contract.
type QueueSendNowDispatcher interface {
	SendQueuedNow(ctx context.Context, sessionID, scope, entryID string) (int, error)
}

// QueueAccessAuthorizer scopes queue reads and mutations to visible sessions.
type QueueAccessAuthorizer interface {
	AuthorizeSessionAccess(ctx context.Context, sessionID string) error
	AuthorizeTaskSessionAccess(ctx context.Context, taskID, sessionID string) error
}

// SessionTaskResolver returns the task that owns a session. It enriches the
// message.queue.status_changed event with task_id so task-scoped consumers
// (e.g. the status summary projector) can refresh per-task queued counts.
// An empty result omits the field; an error is logged and also omits it.
type SessionTaskResolver func(ctx context.Context, sessionID string) (string, error)

// QueueAttachmentClaimer binds staged file descriptors to the task/session
// after a queue entry has been durably accepted.
type QueueAttachmentClaimer interface {
	ClaimMessageAttachments(ctx context.Context, taskID, sessionID string, attachments []v1.MessageAttachment) error
}

type QueueAttachmentReleaser interface {
	ReleaseMessageAttachments(ctx context.Context, taskID, sessionID string, attachments []v1.MessageAttachment) error
}

type queueEntryTaker interface {
	TakeQueuedEntry(context.Context, string, string) (*messagequeue.QueuedMessage, bool, error)
}

// QueueHandlers handles WebSocket message-queue operations.
type QueueHandlers struct {
	queueService        QueueService
	queueDrainer        QueueDrainer
	queueDispatcher     QueueSendNowDispatcher
	accessAuthorizer    QueueAccessAuthorizer
	sessionTaskResolver SessionTaskResolver
	eventBus            bus.EventBus
	logger              *logger.Logger
	referenceValidator  entityrefs.SubmissionValidator
	attachmentClaimer   QueueAttachmentClaimer
}

// SetAttachmentClaimer wires the task attachment registry into queue adds.
// It is optional so queue-focused tests and non-task consumers remain small.
func (h *QueueHandlers) SetAttachmentClaimer(claimer QueueAttachmentClaimer) {
	h.attachmentClaimer = claimer
}

// NewQueueHandlers creates a new QueueHandlers instance. sessionTaskResolver
// enriches published queue status events with the owning task_id; nil keeps
// the payload unchanged.
func NewQueueHandlers(
	queueService QueueService,
	eventBus bus.EventBus,
	log *logger.Logger,
	queueDrainer QueueDrainer,
	accessAuthorizer QueueAccessAuthorizer,
	sessionTaskResolver SessionTaskResolver,
	validators ...entityrefs.SubmissionValidator,
) *QueueHandlers {
	var referenceValidator entityrefs.SubmissionValidator
	if len(validators) > 0 {
		referenceValidator = validators[0]
	}
	handlers := &QueueHandlers{
		queueService:        queueService,
		queueDrainer:        queueDrainer,
		accessAuthorizer:    accessAuthorizer,
		sessionTaskResolver: sessionTaskResolver,
		eventBus:            eventBus,
		logger:              log.WithFields(zap.String("component", "queue-handlers")),
		referenceValidator:  referenceValidator,
	}
	if dispatcher, ok := queueDrainer.(QueueSendNowDispatcher); ok {
		handlers.queueDispatcher = dispatcher
	}
	return handlers
}

// RegisterHandlers registers queue handlers with the dispatcher.
func (h *QueueHandlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionMessageQueueAdd, h.wsQueueMessage)
	d.RegisterFunc(ws.ActionMessageQueueCancel, h.wsCancelAll)
	d.RegisterFunc(ws.ActionMessageQueueGet, h.wsGetQueueStatus)
	d.RegisterFunc(ws.ActionMessageQueueUpdate, h.wsUpdateMessage)
	d.RegisterFunc(ws.ActionMessageQueueAppend, h.wsAppendToQueue)
	d.RegisterFunc(ws.ActionMessageQueueDrain, h.wsDrainQueue)
	d.RegisterFunc(ws.ActionMessageQueueSendNow, h.wsSendNow)
	d.RegisterFunc(ws.ActionMessageQueueRemove, h.wsRemoveEntry)
	d.RegisterFunc(ws.ActionMessageQueueMerge, h.wsMergeIntoAbove)
	d.RegisterFunc(ws.ActionMessageQueueReorder, h.wsReorder)
}

type wsQueueMessageRequest struct {
	SessionID        string                           `json:"session_id"`
	TaskID           string                           `json:"task_id"`
	Content          string                           `json:"content"`
	Model            string                           `json:"model,omitempty"`
	PlanMode         bool                             `json:"plan_mode,omitempty"`
	Attachments      []messagequeue.MessageAttachment `json:"attachments,omitempty"`
	ContextFiles     []v1.ContextFileMeta             `json:"context_files,omitempty"`
	EntityReferences []v1.EntityReference             `json:"entity_references,omitempty"`
	UserID           string                           `json:"user_id,omitempty"`
}

// wsQueueMessage handles ActionMessageQueueAdd, appending a new entry to the session queue.
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
	if invalid := firstInvalidAttachment(req.Attachments); invalid >= 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "attachment metadata is invalid",
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
	metadata := orchestrator.NewUserMessageMeta().
		WithContextFiles(req.ContextFiles).
		WithEntityReferences(req.EntityReferences).
		ToMap()
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
	if h.attachmentClaimer != nil && len(req.Attachments) > 0 {
		if err := h.attachmentClaimer.ClaimMessageAttachments(ctx, req.TaskID, req.SessionID, queueAttachmentsToV1(req.Attachments)); err != nil {
			if rollbackErr := h.rollbackQueuedAttachmentClaim(ctx, req.SessionID, queued.ID); rollbackErr != nil {
				return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to roll back queued attachment", nil)
			}
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Attachment is no longer available", nil)
		}
	}

	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, queued)
}

type wsCancelAllRequest struct {
	SessionID string `json:"session_id"`
}

// wsCancelAll handles ActionMessageQueueCancel, clearing every pending entry for a session.
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

// wsDrainQueue handles ActionMessageQueueDrain, dispatching one queued entry when the session is promptable.
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

type wsSendNowRequest struct {
	SessionID string `json:"session_id"`
	Scope     string `json:"scope"`
	EntryID   string `json:"entry_id,omitempty"`
}

// wsSendNow handles ActionMessageQueueSendNow, interrupting the active turn with an exact queue selection.
func (h *QueueHandlers) wsSendNow(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsSendNowRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}
	if validation := validateSendNowRequest(req); validation != "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, validation, nil)
	}
	if h.queueDispatcher == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Queue send-now is unavailable", nil)
	}

	sentCount, err := h.queueDispatcher.SendQueuedNow(ctx, req.SessionID, req.Scope, req.EntryID)
	if err != nil {
		return h.sendNowErrorResponse(msg, req.SessionID, err)
	}

	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		fieldSessionID: req.SessionID,
		"dispatched":   true,
		"sent_count":   sentCount,
	})
}

// validateSendNowRequest validates the send-now scope and entry id combination.
func validateSendNowRequest(req wsSendNowRequest) string {
	switch {
	case req.Scope != orchestrator.QueueSendNowScopeEntry && req.Scope != orchestrator.QueueSendNowScopeAll:
		return "scope must be entry or all"
	case req.Scope == orchestrator.QueueSendNowScopeEntry && req.EntryID == "":
		return "entry_id is required for entry scope"
	case req.Scope == orchestrator.QueueSendNowScopeAll && req.EntryID != "":
		return "entry_id is not allowed for all scope"
	default:
		return ""
	}
}

// sendNowErrorResponse maps send-now failures to their stable websocket error codes.
func (h *QueueHandlers) sendNowErrorResponse(msg *ws.Message, sessionID string, err error) (*ws.Message, error) {
	switch {
	case errors.Is(err, orchestrator.ErrSendNowEntryNotFound):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound, "Queue entry is no longer pending", nil)
	case errors.Is(err, orchestrator.ErrSendNowQueueEmpty):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeSendNowQueueEmpty, "Queue is empty", nil)
	case errors.Is(err, orchestrator.ErrSendNowQueueChanged):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeSendNowQueueChanged, "Queue changed before Send Now could start", nil)
	case errors.Is(err, orchestrator.ErrSendNowConflict):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeSendNowConflict, "Another cancellation or Send Now operation is in progress", nil)
	case errors.Is(err, orchestrator.ErrSendNowTurnChanged):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeSendNowTurnChanged, "The active turn changed before Send Now could start", nil)
	case errors.Is(err, messagequeue.ErrSendNowAttachmentOverflow):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeSendNowAttachmentOverflow, "Combined attachments exceed the message limits", nil)
	case errors.Is(err, messagequeue.ErrSendNowReferenceOverflow):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeSendNowReferenceOverflow, "Combined entity references exceed the message limit", nil)
	case errors.Is(err, orchestrator.ErrSessionNotPromptable):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeNotPromptable, "Session is not ready for input", nil)
	default:
		h.logger.Error("failed to send queued message now", zap.String(fieldSessionID, sessionID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to send queued message now", nil)
	}
}

type wsGetQueueStatusRequest struct {
	SessionID string `json:"session_id"`
}

// wsGetQueueStatus handles ActionMessageQueueGet, returning the pending list and capacity.
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

// wsUpdateMessage handles ActionMessageQueueUpdate, replacing a queued entry's content.
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
	if invalid := firstInvalidAttachment(req.Attachments); invalid >= 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "attachment metadata is invalid",
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
	var previous *messagequeue.QueuedMessage
	var releaseClaims QueueAttachmentReleaser
	var newlyAdded []messagequeue.MessageAttachment
	if h.attachmentClaimer != nil {
		var err error
		previous, err = h.queueService.GetEntry(ctx, req.SessionID, req.EntryID)
		if err != nil {
			if errors.Is(err, messagequeue.ErrEntryNotFound) {
				return ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound, "Queue entry was already drained or not owned by caller", nil)
			}
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		newlyAdded = newlyAddedQueueAttachments(previous.Attachments, req.Attachments)
		if err := h.attachmentClaimer.ClaimMessageAttachments(ctx, previous.TaskID, req.SessionID, queueAttachmentsToV1(newlyAdded)); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Attachment is no longer available", nil)
		}
		releaseClaims, _ = h.attachmentClaimer.(QueueAttachmentReleaser)
	}
	if err := h.queueService.UpdateMessageWithMetadata(ctx, req.SessionID, req.EntryID, req.Content, req.Attachments, metadataUpdates, queuedBy); err != nil {
		if releaseClaims != nil && previous != nil {
			if releaseErr := releaseClaims.ReleaseMessageAttachments(ctx, previous.TaskID, req.SessionID, queueAttachmentsToV1(newlyAdded)); releaseErr != nil {
				h.logger.Warn("failed to release attachments after queue update failure", zap.Error(releaseErr))
			}
		}
		if errors.Is(err, messagequeue.ErrEntryNotFound) {
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound, "Queue entry was already drained or not owned by caller", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	if releaseClaims != nil && previous != nil {
		if superseded := supersededQueueAttachments(previous.Attachments, req.Attachments); len(superseded) > 0 {
			if err := releaseClaims.ReleaseMessageAttachments(ctx, previous.TaskID, req.SessionID, queueAttachmentsToV1(superseded)); err != nil {
				h.logger.Warn("failed to release superseded queue attachments", zap.Error(err))
			}
		}
	}
	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{fieldEntryID: req.EntryID})
}

// newlyAddedQueueAttachments returns attachment descriptors that were not present before.
func newlyAddedQueueAttachments(previous, replacement []messagequeue.MessageAttachment) []messagequeue.MessageAttachment {
	retained := make(map[string]struct{}, len(previous))
	for _, attachment := range previous {
		if attachment.AttachmentID != "" {
			retained[attachment.AttachmentID] = struct{}{}
		}
	}
	var newlyAdded []messagequeue.MessageAttachment
	for _, attachment := range replacement {
		if attachment.AttachmentID == "" {
			continue
		}
		if _, ok := retained[attachment.AttachmentID]; !ok {
			newlyAdded = append(newlyAdded, attachment)
		}
	}
	return newlyAdded
}

// supersededQueueAttachments returns attachment descriptors dropped by the replacement.
func supersededQueueAttachments(previous, replacement []messagequeue.MessageAttachment) []messagequeue.MessageAttachment {
	retained := make(map[string]struct{}, len(replacement))
	for _, attachment := range replacement {
		if attachment.AttachmentID != "" {
			retained[attachment.AttachmentID] = struct{}{}
		}
	}
	var superseded []messagequeue.MessageAttachment
	for _, attachment := range previous {
		if attachment.AttachmentID == "" {
			continue
		}
		if _, ok := retained[attachment.AttachmentID]; !ok {
			superseded = append(superseded, attachment)
		}
	}
	return superseded
}

// validateSubmittedReferences runs the entity-reference submission validator when configured.
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

// rollbackQueuedAttachmentClaim releases attachments claimed for a queue entry that failed to persist.
func (h *QueueHandlers) rollbackQueuedAttachmentClaim(ctx context.Context, sessionID, entryID string) error {
	if err := h.queueService.RemoveEntry(ctx, sessionID, entryID); err == nil {
		return nil
	} else {
		h.logger.Error("failed to remove queue entry after attachment claim failure",
			zap.String("entry_id", entryID), zap.Error(err))
	}
	taker, ok := h.queueService.(queueEntryTaker)
	if !ok {
		return errors.New("queue service cannot atomically remove a queued entry")
	}
	_, _, err := taker.TakeQueuedEntry(ctx, sessionID, entryID)
	return err
}

// firstInvalidDeliveryMode returns the index of the first attachment with an unknown delivery mode.
func firstInvalidDeliveryMode(attachments []messagequeue.MessageAttachment) int {
	for i, att := range attachments {
		if att.DeliveryMode != "" && att.DeliveryMode != "prompt" && att.DeliveryMode != "path" {
			return i
		}
	}
	return -1
}

// firstInvalidAttachment returns the index of the first structurally invalid attachment.
func firstInvalidAttachment(attachments []messagequeue.MessageAttachment) int {
	if len(attachments) > models.MaxMessageAttachmentCount {
		return models.MaxMessageAttachmentCount
	}
	var total int64
	for i, attachment := range attachments {
		if attachment.Type != "image" && attachment.Type != "audio" && attachment.Type != "resource" {
			return i
		}
		bytes, valid := attachmentPayloadBytes(attachment)
		if !valid {
			return i
		}
		total += bytes
		if total > models.MaxMessageAttachmentBytes {
			return i
		}
	}
	return -1
}

// attachmentPayloadBytes returns the decoded payload size and whether the base64 is valid.
func attachmentPayloadBytes(attachment messagequeue.MessageAttachment) (int64, bool) {
	if attachment.AttachmentID != "" {
		if attachment.Data != "" || attachment.Name == "" || attachment.MimeType == "" {
			return 0, false
		}
		if attachment.SizeBytes < 0 || attachment.SizeBytes > models.MaxMessageAttachmentBytes {
			return 0, false
		}
		return attachment.SizeBytes, true
	}
	if attachment.Data == "" || len(attachment.Data) > 10*1024*1024 {
		return 0, false
	}
	decoded, err := base64.StdEncoding.DecodeString(attachment.Data)
	if err != nil {
		return 0, false
	}
	return int64(len(decoded)), true
}

// queueAttachmentsToV1 converts queue attachments to the API v1 representation.
func queueAttachmentsToV1(attachments []messagequeue.MessageAttachment) []v1.MessageAttachment {
	if len(attachments) == 0 {
		return nil
	}
	converted := make([]v1.MessageAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		converted = append(converted, v1.MessageAttachment{
			AttachmentID: attachment.AttachmentID,
			Type:         attachment.Type,
			Data:         attachment.Data,
			MimeType:     attachment.MimeType,
			Name:         attachment.Name,
			SizeBytes:    attachment.SizeBytes,
			DeliveryMode: attachment.DeliveryMode,
		})
	}
	return converted
}

type wsRemoveEntryRequest struct {
	SessionID string `json:"session_id"`
	EntryID   string `json:"entry_id"`
}

// wsRemoveEntry handles ActionMessageQueueRemove, deleting a single queued entry.
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
		if errors.Is(err, messagequeue.ErrMergeDisabled) {
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeMergeDisabled, "Message merging is disabled", nil)
		}
		h.logger.Error("failed to merge queued message", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to merge queued message", nil)
	}

	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{fieldEntryID: merged.ID})
}

// wsReorderRequest is the payload for ActionMessageQueueReorder: the session
// whose queue is modified and the complete ordered list of visible pending
// entry ids the caller wants as the new FIFO order.
type wsReorderRequest struct {
	SessionID  string   `json:"session_id"`
	OrderedIDs []string `json:"ordered_ids"`
}

// wsReorder handles ActionMessageQueueReorder, rewriting the session's visible
// pending order to match ordered_ids and broadcasting the updated queue. Any
// drift from the persisted visible set (a drain/remove/merge raced the drag)
// is rejected atomically with queue_changed so the client refetches.
func (h *QueueHandlers) wsReorder(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsReorderRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}
	if len(req.OrderedIDs) == 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "ordered_ids is required", nil)
	}
	if hasDuplicateIDs(req.OrderedIDs) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "ordered_ids must not contain duplicates", nil)
	}

	if err := h.queueService.ReorderEntries(ctx, req.SessionID, req.OrderedIDs); err != nil {
		if errors.Is(err, messagequeue.ErrQueueChanged) {
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeQueueChanged, "Queue changed before the reorder could be applied", nil)
		}
		h.logger.Error("failed to reorder queued messages", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to reorder queued messages", nil)
	}

	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		fieldSessionID: req.SessionID,
		"reordered":    len(req.OrderedIDs),
	})
}

type wsAppendToQueueRequest struct {
	SessionID string `json:"session_id"`
	TaskID    string `json:"task_id"`
	Content   string `json:"content"`
	Model     string `json:"model,omitempty"`
	PlanMode  bool   `json:"plan_mode,omitempty"`
	UserID    string `json:"user_id,omitempty"`
}

// wsAppendToQueue handles ActionMessageQueueAppend, appending or inserting a user message.
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

// hasDuplicateIDs reports whether ids contains any id more than once.
func hasDuplicateIDs(ids []string) bool {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, duplicate := seen[id]; duplicate {
			return true
		}
		seen[id] = struct{}{}
	}
	return false
}

// reservedIdentityError builds the validation message for reserved caller identities.
func reservedIdentityError(queuedBy string) string {
	if queuedBy == messagequeue.QueuedByAgent {
		return "user_id may not impersonate the agent identity"
	}
	return "user_id may not impersonate a reserved identity"
}

// authorizeSession denies the request when the caller cannot access the session.
func (h *QueueHandlers) authorizeSession(ctx context.Context, msg *ws.Message, sessionID string) *ws.Message {
	if h.accessAuthorizer == nil {
		return queueAccessDeniedResponse(msg)
	}
	if err := h.accessAuthorizer.AuthorizeSessionAccess(ctx, sessionID); err != nil {
		return queueAccessDeniedResponse(msg)
	}
	return nil
}

// authorizeTaskSession denies the request when the caller cannot access the task/session pair.
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

// queueAccessDeniedResponse builds the non-enumerating session-not-found error response.
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
	eventData := map[string]interface{}{
		fieldSessionID:  sessionID,
		"entries":       status.Entries,
		"count":         status.Count,
		fieldMax:        status.Max,
		"merge_enabled": status.MergeEnabled,
	}
	if h.sessionTaskResolver != nil {
		if taskID, err := h.sessionTaskResolver(ctx, sessionID); err != nil {
			h.logger.Warn("resolve session task for queue status event",
				zap.String("session_id", sessionID),
				zap.Error(err))
		} else if taskID != "" {
			eventData["task_id"] = taskID
		}
	}
	_ = h.eventBus.Publish(ctx, events.MessageQueueStatusChanged, bus.NewEvent(
		events.MessageQueueStatusChanged,
		"queue-handlers",
		eventData,
	))
}
