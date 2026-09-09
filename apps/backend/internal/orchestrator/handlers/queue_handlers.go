package handlers

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/entityrefs"
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
	fieldTaskID             = "task_id"
	fieldSessionID          = "session_id"
	fieldSessionIncarnation = "session_incarnation_id"
	fieldEntryID            = "entry_id"
	fieldQueueSize          = "queue_size"
	fieldMax                = "max"
	fieldAutoRun            = "auto_run"
	fieldAutoMergeEnabled   = "auto_merge_enabled"
)

// QueueService is the surface the handlers depend on. Real implementation lives
// in messagequeue.Service.
type QueueService interface {
	QueueMessageWithMetadata(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []messagequeue.MessageAttachment, metadata map[string]interface{}) (*messagequeue.QueuedMessage, error)
	QueueMessageWithMetadataAfterInsert(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []messagequeue.MessageAttachment, metadata map[string]interface{}, afterInsert func(context.Context, *messagequeue.QueuedMessage) error) (*messagequeue.QueuedMessage, error)
	AppendContent(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []messagequeue.MessageAttachment) (*messagequeue.QueuedMessage, bool, error)
	GetEntry(ctx context.Context, sessionID, entryID string) (*messagequeue.QueuedMessage, error)
	UpdateMessageWithMetadata(ctx context.Context, sessionID, entryID, content string, attachments []messagequeue.MessageAttachment, metadataUpdates map[string]interface{}, queuedBy string) error
	RemoveEntry(ctx context.Context, sessionID, entryID string) error
	MergeIntoAbove(ctx context.Context, sessionID, entryID, queuedBy string) (*messagequeue.QueuedMessage, error)
	ReorderEntries(ctx context.Context, sessionID string, orderedIDs []string) error
	CancelAll(ctx context.Context, sessionID string) (int, error)
	GetStatus(ctx context.Context, sessionID string) *messagequeue.QueueStatus
}
type QueueSnapshotService interface {
	Snapshot(context.Context, messagequeue.QueueSessionIdentity) (*messagequeue.QueueStatus, error)
}
type QueueIdentityAdmissionService interface {
	QueueMessageWithMetadataForSession(
		context.Context,
		messagequeue.QueueSessionIdentity,
		string,
		string,
		string,
		bool,
		[]messagequeue.MessageAttachment,
		map[string]interface{},
	) (*messagequeue.QueuedMessage, error)
	QueueMessageWithMetadataForSessionAfterInsert(
		context.Context,
		messagequeue.QueueSessionIdentity,
		string,
		string,
		string,
		bool,
		[]messagequeue.MessageAttachment,
		map[string]interface{},
		func(context.Context, *messagequeue.QueuedMessage) error,
	) (*messagequeue.QueuedMessage, error)
}
type QueueIdentityAttachmentAdmissionService interface {
	QueueMessageWithMetadataForSessionWithClaim(
		context.Context,
		messagequeue.QueueSessionIdentity,
		string,
		string,
		string,
		bool,
		[]messagequeue.MessageAttachment,
		map[string]interface{},
		messagequeue.QueueAttachmentClaim,
	) (*messagequeue.QueuedMessage, error)
}

type QueueIdentityMutationService interface {
	AppendContentForSession(context.Context, messagequeue.QueueSessionIdentity, string, string, string, bool, []messagequeue.MessageAttachment) (*messagequeue.QueuedMessage, bool, error)
	UpdateMessageWithMetadataForSession(context.Context, messagequeue.QueueSessionIdentity, string, string, []messagequeue.MessageAttachment, map[string]interface{}, string) error
	RemoveEntryForSession(context.Context, messagequeue.QueueSessionIdentity, string) (*messagequeue.QueueRemovalResult, error)
	MergeIntoAboveForSession(context.Context, messagequeue.QueueSessionIdentity, string, string) (*messagequeue.QueuedMessage, error)
	ReorderEntriesForSession(context.Context, messagequeue.QueueSessionIdentity, []string) error
	CancelAllForSession(context.Context, messagequeue.QueueSessionIdentity) (*messagequeue.QueueRemovalResult, error)
}

type QueueIdentityEntryService interface {
	GetEntryForSession(context.Context, messagequeue.QueueSessionIdentity, string) (*messagequeue.QueuedMessage, error)
}

type QueueIdentityAttachmentMutationService interface {
	UpdateMessageWithMetadataForSessionWithClaim(context.Context, messagequeue.QueueSessionIdentity, string, string, []messagequeue.MessageAttachment, map[string]interface{}, string, messagequeue.QueueAttachmentClaim) error
}

// QueueDrainer drains a single queued entry when the session is promptable.
type QueueDrainer interface {
	DrainQueuedMessage(ctx context.Context, sessionID string) (bool, error)
}
type QueueIdentityDrainer interface {
	DrainQueuedMessageForSession(context.Context, messagequeue.QueueSessionIdentity) (bool, error)
}

// QueueAdmissionReadinessChecker rechecks automatic dispatch after a queue
// entry is durably admitted. The check is best-effort: admission remains
// successful when the session is not ready yet or the dispatch check fails.
type QueueAdmissionReadinessChecker interface {
	CheckQueueAdmissionReadiness(context.Context, messagequeue.QueueSessionIdentity)
}

// QueueAutoRunController persists queue policy and may immediately dispatch
// one FIFO head when enabling an eligible session.
type QueueAutoRunController interface {
	SetQueueAutoRun(ctx context.Context, sessionID string, enabled bool) (autoRun bool, dispatched bool, err error)
}
type QueueIdentityAutoRunController interface {
	SetQueueAutoRunForSession(context.Context, messagequeue.QueueSessionIdentity, bool) (bool, bool, error)
}

// QueueAutoMergeController persists and resolves per-session automatic-merge policy.
type QueueAutoMergeController interface {
	SetSessionAutoMerge(context.Context, messagequeue.QueueSessionIdentity, bool) (messagequeue.AutoMergePolicy, error)
}

// QueueSendNowDispatcher is implemented by the orchestrator service. It is
// kept separate from QueueDrainer so queue-focused handlers can retain their
// small test doubles while the new action gets the replacement-turn contract.
type QueueSendNowDispatcher interface {
	SendQueuedNow(ctx context.Context, sessionID, scope, entryID string) (int, error)
}
type QueueIdentitySendNowDispatcher interface {
	SendQueuedNowForSession(context.Context, messagequeue.QueueSessionIdentity, string, string) (int, error)
}

// QueueAccessAuthorizer scopes queue reads and mutations to visible sessions.
type QueueAccessAuthorizer interface {
	AuthorizeSessionAccess(ctx context.Context, sessionID string) error
	AuthorizeTaskSessionAccess(ctx context.Context, taskID, sessionID string) error
}
type QueueSessionIdentityAuthorizer interface {
	AuthorizeTaskSessionIncarnationAccess(ctx context.Context, taskID, sessionID, incarnationID string) error
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

type QueueAttachmentClaimPreparer interface {
	PrepareQueueAttachmentClaim(context.Context, string, []v1.MessageAttachment) (messagequeue.QueueAttachmentClaim, error)
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
	queueReadiness      QueueAdmissionReadinessChecker
	queueAutoRun        QueueAutoRunController
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
	if readinessChecker, ok := queueDrainer.(QueueAdmissionReadinessChecker); ok {
		handlers.queueReadiness = readinessChecker
	}
	if controller, ok := queueDrainer.(QueueAutoRunController); ok {
		handlers.queueAutoRun = controller
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
	d.RegisterFunc(ws.ActionMessageQueueAutoRunSet, h.wsSetAutoRun)
	d.RegisterFunc(ws.ActionMessageQueueAutoMergeSet, h.wsSetAutoMerge)
	d.RegisterFunc(ws.ActionMessageQueueRemove, h.wsRemoveEntry)
	d.RegisterFunc(ws.ActionMessageQueueMerge, h.wsMergeIntoAbove)
	d.RegisterFunc(ws.ActionMessageQueueReorder, h.wsReorder)
}

type wsQueueMessageRequest struct {
	SessionID            string                           `json:"session_id"`
	TaskID               string                           `json:"task_id"`
	SessionIncarnationID string                           `json:"session_incarnation_id"`
	Content              string                           `json:"content"`
	Model                string                           `json:"model,omitempty"`
	PlanMode             bool                             `json:"plan_mode,omitempty"`
	Attachments          []messagequeue.MessageAttachment `json:"attachments,omitempty"`
	ContextFiles         []v1.ContextFileMeta             `json:"context_files,omitempty"`
	EntityReferences     []v1.EntityReference             `json:"entity_references,omitempty"`
	UserID               string                           `json:"user_id,omitempty"`
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
	if h.requiresQueueIdentity() && req.SessionIncarnationID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id, session_id, and session_incarnation_id are required", nil)
	}
	if denied := h.authorizeQueueIdentity(ctx, msg, req.TaskID, req.SessionID, req.SessionIncarnationID); denied != nil {
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
	queued, err := h.admitQueuedMessage(ctx, &req, queuedBy, metadata)
	if err != nil {
		if errors.Is(err, messagequeue.ErrQueueFull) {
			return h.queueFullResponse(ctx, msg, messagequeue.QueueSessionIdentity{
				TaskID: req.TaskID, SessionID: req.SessionID, SessionIncarnationID: req.SessionIncarnationID,
			})
		}
		if errors.Is(err, messagequeue.ErrTaskInactive) {
			// The task was archived or deleted between the caller's
			// authorization and the queue admission; do not queue a message
			// that would be orphaned behind the task's purge.
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Task is no longer active", nil)
		}
		if errors.Is(err, errQueuedAttachmentRollback) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to roll back queued attachment", nil)
		}
		if errors.Is(err, errQueuedAttachmentUnavailable) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Attachment is no longer available", nil)
		}
		h.logger.Error("failed to queue message", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to queue message", nil)
	}

	identity := messagequeue.QueueSessionIdentity{
		TaskID:               req.TaskID,
		SessionID:            req.SessionID,
		SessionIncarnationID: req.SessionIncarnationID,
	}
	h.publishStatusForIdentity(ctx, identity, queued)
	if h.queueReadiness != nil {
		h.queueReadiness.CheckQueueAdmissionReadiness(ctx, identity)
	}
	return ws.NewResponse(msg.ID, msg.Action, queued)
}

var (
	errQueuedAttachmentUnavailable = errors.New("queued attachment unavailable")
	errQueuedAttachmentRollback    = errors.New("queued attachment rollback failed")
)

func (h *QueueHandlers) admitQueuedMessage(ctx context.Context, req *wsQueueMessageRequest, queuedBy string, metadata map[string]interface{}) (*messagequeue.QueuedMessage, error) {
	if !h.requiresQueueIdentity() {
		if h.attachmentClaimer == nil || len(req.Attachments) == 0 {
			return h.queueService.QueueMessageWithMetadata(
				ctx, req.SessionID, req.TaskID, req.Content, req.Model, queuedBy, req.PlanMode, req.Attachments, metadata,
			)
		}
		return h.queueService.QueueMessageWithMetadataAfterInsert(
			ctx, req.SessionID, req.TaskID, req.Content, req.Model, queuedBy, req.PlanMode, req.Attachments, metadata,
			h.claimQueuedAttachmentsAfterInsert(req),
		)
	}
	admissions, ok := h.queueService.(QueueIdentityAdmissionService)
	if !ok {
		return nil, errors.New("identity-bound queue admission is unavailable")
	}
	identity := messagequeue.QueueSessionIdentity{
		TaskID: req.TaskID, SessionID: req.SessionID, SessionIncarnationID: req.SessionIncarnationID,
	}
	if h.attachmentClaimer == nil || len(req.Attachments) == 0 {
		return admissions.QueueMessageWithMetadataForSession(
			ctx, identity, req.Content, req.Model, queuedBy, req.PlanMode, req.Attachments, metadata,
		)
	}
	if preparer, ok := h.attachmentClaimer.(QueueAttachmentClaimPreparer); ok {
		claim, err := preparer.PrepareQueueAttachmentClaim(ctx, req.TaskID, queueAttachmentsToV1(req.Attachments))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errQueuedAttachmentUnavailable, err)
		}
		atomicAdmissions, ok := h.queueService.(QueueIdentityAttachmentAdmissionService)
		if !ok {
			return nil, errors.New("transactional attachment admission is unavailable")
		}
		return atomicAdmissions.QueueMessageWithMetadataForSessionWithClaim(
			ctx, identity, req.Content, req.Model, queuedBy, req.PlanMode, req.Attachments, metadata, claim,
		)
	}
	return admissions.QueueMessageWithMetadataForSessionAfterInsert(
		ctx, identity, req.Content, req.Model, queuedBy, req.PlanMode, req.Attachments, metadata,
		h.claimQueuedAttachmentsAfterInsert(req),
	)
}

func (h *QueueHandlers) claimQueuedAttachmentsAfterInsert(req *wsQueueMessageRequest) func(context.Context, *messagequeue.QueuedMessage) error {
	return func(admittedCtx context.Context, source *messagequeue.QueuedMessage) error {
		claimErr := h.attachmentClaimer.ClaimMessageAttachments(
			admittedCtx, req.TaskID, req.SessionID, queueAttachmentsToV1(req.Attachments),
		)
		if claimErr == nil {
			return nil
		}
		if rollbackErr := h.rollbackQueuedAttachmentClaim(admittedCtx, req.SessionID, source.ID); rollbackErr != nil {
			h.logger.Error("failed to roll back queued attachment", zap.Error(rollbackErr))
			return fmt.Errorf("%w: %v", errQueuedAttachmentRollback, rollbackErr)
		}
		return fmt.Errorf("%w: %v", errQueuedAttachmentUnavailable, claimErr)
	}
}

type wsUpdateMessageRequest struct {
	SessionID            string                           `json:"session_id"`
	TaskID               string                           `json:"task_id"`
	SessionIncarnationID string                           `json:"session_incarnation_id"`
	EntryID              string                           `json:"entry_id"`
	Content              string                           `json:"content"`
	Attachments          []messagequeue.MessageAttachment `json:"attachments,omitempty"`
	EntityReferences     []v1.EntityReference             `json:"entity_references,omitempty"`
	UserID               string                           `json:"user_id,omitempty"`
}

// updateQueuedMessage selects the identity-bound mutation and optional atomic attachment claim.
func (h *QueueHandlers) updateQueuedMessage(
	ctx context.Context,
	identity messagequeue.QueueSessionIdentity,
	req wsUpdateMessageRequest,
	metadataUpdates map[string]interface{},
	queuedBy string,
	atomicClaim *messagequeue.QueueAttachmentClaim,
) error {
	if !h.requiresQueueIdentity() {
		return h.queueService.UpdateMessageWithMetadata(
			ctx, req.SessionID, req.EntryID, req.Content, req.Attachments, metadataUpdates, queuedBy,
		)
	}
	if atomicClaim == nil {
		return h.queueService.(QueueIdentityMutationService).UpdateMessageWithMetadataForSession(
			ctx, identity, req.EntryID, req.Content, req.Attachments, metadataUpdates, queuedBy,
		)
	}
	atomicMutations, ok := h.queueService.(QueueIdentityAttachmentMutationService)
	if !ok {
		return errors.New("transactional attachment update is unavailable")
	}
	return atomicMutations.UpdateMessageWithMetadataForSessionWithClaim(
		ctx, identity, req.EntryID, req.Content, req.Attachments, metadataUpdates, queuedBy, *atomicClaim,
	)
}

func (h *QueueHandlers) releaseFailedQueueAttachmentClaims(
	ctx context.Context,
	releaser QueueAttachmentReleaser,
	previous *messagequeue.QueuedMessage,
	sessionID string,
	attachments []messagequeue.MessageAttachment,
) {
	if releaser == nil || previous == nil {
		return
	}
	if err := releaser.ReleaseMessageAttachments(
		ctx, previous.TaskID, sessionID, queueAttachmentsToV1(attachments),
	); err != nil {
		h.logger.Warn("failed to release attachments after queue update failure", zap.Error(err))
	}
}

// wsUpdateMessage handles ActionMessageQueueUpdate, replacing a queued entry's content.
func (h *QueueHandlers) wsUpdateMessage(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateMessageRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if h.requiresQueueIdentity() && (req.TaskID == "" || req.SessionIncarnationID == "") {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id, session_id, and session_incarnation_id are required", nil)
	}
	if denied := h.authorizeQueueIdentity(ctx, msg, req.TaskID, req.SessionID, req.SessionIncarnationID); denied != nil {
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
	identity := messagequeue.QueueSessionIdentity{
		TaskID: req.TaskID, SessionID: req.SessionID, SessionIncarnationID: req.SessionIncarnationID,
	}
	var previous *messagequeue.QueuedMessage
	var releaseClaims QueueAttachmentReleaser
	var newlyAdded []messagequeue.MessageAttachment
	var atomicClaim *messagequeue.QueueAttachmentClaim
	if h.attachmentClaimer != nil {
		var err error
		if h.requiresQueueIdentity() {
			entryService, ok := h.queueService.(QueueIdentityEntryService)
			if !ok {
				return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Identity-bound queue lookup is unavailable", nil)
			}
			previous, err = entryService.GetEntryForSession(ctx, identity, req.EntryID)
		} else {
			previous, err = h.queueService.GetEntry(ctx, req.SessionID, req.EntryID)
		}
		if err != nil {
			if errors.Is(err, messagequeue.ErrEntryNotFound) {
				return ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound, "Queue entry was already drained or not owned by caller", nil)
			}
			if isQueueIdentityError(err) {
				return queueAccessDeniedResponse(msg), nil
			}
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		newlyAdded = newlyAddedQueueAttachments(previous.Attachments, req.Attachments)
		if preparer, ok := h.attachmentClaimer.(QueueAttachmentClaimPreparer); ok && h.requiresQueueIdentity() {
			prepared, prepareErr := preparer.PrepareQueueAttachmentClaim(ctx, previous.TaskID, queueAttachmentsToV1(newlyAdded))
			if prepareErr != nil {
				return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Attachment is no longer available", nil)
			}
			atomicClaim = &prepared
		} else if err := h.attachmentClaimer.ClaimMessageAttachments(ctx, previous.TaskID, req.SessionID, queueAttachmentsToV1(newlyAdded)); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Attachment is no longer available", nil)
		}
		releaseClaims, _ = h.attachmentClaimer.(QueueAttachmentReleaser)
	}
	updateErr := h.updateQueuedMessage(ctx, identity, req, metadataUpdates, queuedBy, atomicClaim)
	if updateErr != nil {
		if atomicClaim == nil {
			h.releaseFailedQueueAttachmentClaims(ctx, releaseClaims, previous, req.SessionID, newlyAdded)
		}
		if errors.Is(updateErr, messagequeue.ErrEntryNotFound) {
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound, "Queue entry was already drained or not owned by caller", nil)
		}
		if isQueueIdentityError(updateErr) {
			return queueAccessDeniedResponse(msg), nil
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, updateErr.Error(), nil)
	}
	if releaseClaims != nil && previous != nil {
		if superseded := supersededQueueAttachments(previous.Attachments, req.Attachments); len(superseded) > 0 {
			if err := releaseClaims.ReleaseMessageAttachments(ctx, previous.TaskID, req.SessionID, queueAttachmentsToV1(superseded)); err != nil {
				h.logger.Warn("failed to release superseded queue attachments", zap.Error(err))
			}
		}
	}
	h.publishStatusForIdentity(ctx, identity)
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

type wsAppendToQueueRequest struct {
	SessionID            string `json:"session_id"`
	TaskID               string `json:"task_id"`
	SessionIncarnationID string `json:"session_incarnation_id"`
	Content              string `json:"content"`
	Model                string `json:"model,omitempty"`
	PlanMode             bool   `json:"plan_mode,omitempty"`
	UserID               string `json:"user_id,omitempty"`
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
	if h.requiresQueueIdentity() && req.SessionIncarnationID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id, session_id, and session_incarnation_id are required", nil)
	}
	if denied := h.authorizeQueueIdentity(ctx, msg, req.TaskID, req.SessionID, req.SessionIncarnationID); denied != nil {
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
	identity := messagequeue.QueueSessionIdentity{
		TaskID: req.TaskID, SessionID: req.SessionID, SessionIncarnationID: req.SessionIncarnationID,
	}
	var queued *messagequeue.QueuedMessage
	var appended bool
	var err error
	if h.requiresQueueIdentity() {
		queued, appended, err = h.queueService.(QueueIdentityMutationService).AppendContentForSession(ctx, identity, req.Content, req.Model, queuedBy, req.PlanMode, nil)
	} else {
		queued, appended, err = h.queueService.AppendContent(ctx, req.SessionID, req.TaskID, req.Content, req.Model, queuedBy, req.PlanMode, nil)
	}
	if err != nil {
		if errors.Is(err, messagequeue.ErrQueueFull) {
			return h.queueFullResponse(ctx, msg, identity)
		}
		if isQueueIdentityError(err) {
			return queueAccessDeniedResponse(msg), nil
		}
		h.logger.Error("failed to append to queue", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to queue message", nil)
	}

	h.publishStatusForIdentity(ctx, identity)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		fieldEntryID: queued.ID,
		"was_append": appended,
	})
}

func (h *QueueHandlers) queueFullResponse(
	ctx context.Context,
	msg *ws.Message,
	identity messagequeue.QueueSessionIdentity,
) (*ws.Message, error) {
	if !h.requiresQueueIdentity() {
		return queueFullErrorResponse(msg, h.queueService.GetStatus(ctx, identity.SessionID))
	}
	snapshots, ok := h.queueService.(QueueSnapshotService)
	if !ok {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Queue status is unavailable", nil)
	}
	status, err := snapshots.Snapshot(ctx, identity)
	if err == nil {
		return queueFullErrorResponse(msg, status)
	}
	if isQueueIdentityError(err) {
		return queueAccessDeniedResponse(msg), nil
	}
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to read queue status", nil)
}

func queueFullErrorResponse(msg *ws.Message, status *messagequeue.QueueStatus) (*ws.Message, error) {
	return ws.NewError(msg.ID, msg.Action, messagequeue.QueueFullErrorCode, "Queue is full",
		map[string]interface{}{
			fieldQueueSize: status.Count,
			fieldMax:       status.Max,
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
func (h *QueueHandlers) requiresQueueIdentity() bool {
	_, ok := h.accessAuthorizer.(QueueSessionIdentityAuthorizer)
	return ok
}

func (h *QueueHandlers) authorizeQueueIdentity(
	ctx context.Context,
	msg *ws.Message,
	taskID, sessionID, incarnationID string,
) *ws.Message {
	if taskID == "" {
		return h.authorizeSession(ctx, msg, sessionID)
	}
	if incarnationID == "" {
		return h.authorizeTaskSession(ctx, msg, taskID, sessionID)
	}
	authorizer, ok := h.accessAuthorizer.(QueueSessionIdentityAuthorizer)
	if !ok {
		return h.authorizeTaskSession(ctx, msg, taskID, sessionID)
	}
	if err := authorizer.AuthorizeTaskSessionIncarnationAccess(ctx, taskID, sessionID, incarnationID); err != nil {
		return queueAccessDeniedResponse(msg)
	}
	return nil
}

// queueAccessDeniedResponse builds the non-enumerating session-not-found error response.
func queueAccessDeniedResponse(msg *ws.Message) *ws.Message {
	response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, queueAccessDenied, nil)
	return response
}
