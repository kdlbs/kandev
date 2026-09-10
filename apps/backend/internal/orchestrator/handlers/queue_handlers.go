package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
	"sync"
	"time"
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

type planCommentQueueService interface {
	QueueMessageWithPlanComments(
		context.Context,
		messagequeue.PlanCommentQueueRequest,
	) (*messagequeue.PlanCommentQueueResult, error)
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

// QueueEditLeaseController owns target-bound queued-message edit leases.
type QueueEditLeaseController interface {
	BeginEdit(context.Context, string, string, string) (*messagequeue.QueueEditLease, error)
	RenewEdit(context.Context, string, string, string, string) (*messagequeue.QueueEditLease, error)
	EndEdit(context.Context, string, string, string, string) error
	UpdateMessageWithLease(context.Context, string, string, string, string, string, int64, string, []messagequeue.MessageAttachment, map[string]interface{}) (int64, error)
}
type queueEditSavedLeaseController interface {
	EndEditAfterSave(context.Context, string, string, string, string) (bool, error)
}

// QueueAutoRunPreservingDrainer attempts one normal FIFO dispatch without
// changing the persisted Auto-run policy.
type QueueAutoRunPreservingDrainer interface {
	DrainQueuedMessageIfAutoRun(context.Context, string) (bool, error)
}

type queueEditAdmissionController interface {
	WithSessionAdmission(context.Context, string, func(context.Context) error) error
}
type queueEditLeaseStateReader interface {
	GetEditLease(context.Context, string, string) (*messagequeue.QueueEditLease, error)
}

type queueEntryLocator interface {
	FindEntryByID(context.Context, string) (*messagequeue.QueuedMessage, error)
}

type queueAttachmentReferenceChecker interface {
	ReferencedQueueAttachmentIDs(context.Context, string, string, []string) (map[string]struct{}, error)
}
type queueBatchCanceller interface {
	CancelAllWithEntries(context.Context, string) ([]messagequeue.QueuedMessage, error)
}
type queueAttachmentCleanupStore interface {
	AttachmentCleanupPersistenceAvailable() bool
	UpsertAttachmentCleanup(context.Context, messagequeue.AttachmentCleanup) error
	DeleteAttachmentCleanup(context.Context, string, string, string) error
	ListAttachmentCleanups(context.Context) ([]messagequeue.AttachmentCleanup, error)
}
type queueAttachmentCleanupLocator interface {
	GetAttachmentCleanup(context.Context, string, string, string) (*messagequeue.AttachmentCleanup, error)
}

type queueEntryRemover interface {
	RemoveEntryWithEntry(context.Context, string, string) (*messagequeue.QueuedMessage, error)
}

type queueEditAttachmentController interface {
	UpdateMessageWithLeaseAfterValidationAndFinalize(context.Context, string, string, string, string, string, int64, string, []messagequeue.MessageAttachment, map[string]interface{}, func(context.Context) error, func(context.Context) error, func(context.Context, *messagequeue.QueuedMessage) error) (int64, error)
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

type queueAttachmentCleanupOwnerResolver interface {
	ResolveMessageAttachmentOwner(
		context.Context, string, string, []v1.MessageAttachment,
	) (string, error)
}

type queueAttachmentCleanupReleaser interface {
	ReleaseMessageAttachmentsForCleanup(
		context.Context, string, string, []v1.MessageAttachment,
	) error
}

type internalQueueAttachmentReleaser struct {
	releaser queueAttachmentCleanupReleaser
}

func (r internalQueueAttachmentReleaser) ReleaseMessageAttachments(
	ctx context.Context, taskID, sessionID string, attachments []v1.MessageAttachment,
) error {
	return r.releaser.ReleaseMessageAttachmentsForCleanup(ctx, taskID, sessionID, attachments)
}

type queueEntryTaker interface {
	TakeQueuedEntry(context.Context, string, string) (*messagequeue.QueuedMessage, bool, error)
}
type pendingQueueAttachmentCleanupKey struct {
	sessionID   string
	entryID     string
	operationID string
}

type pendingQueueAttachmentCleanup struct {
	key              pendingQueueAttachmentCleanupKey
	req              wsUpdateMessageRequest
	previous         *messagequeue.QueuedMessage
	releaser         QueueAttachmentReleaser
	removeEntry      bool
	claimPending     bool
	entryFingerprint string
	currentSessionID string
	authCtx          context.Context
	wake             chan struct{}
}

// QueueHandlers handles WebSocket message-queue operations.
type QueueHandlers struct {
	queueService             QueueService
	queueDrainer             QueueDrainer
	queueReadiness           QueueAdmissionReadinessChecker
	queueAutoRun             QueueAutoRunController
	queueDispatcher          QueueSendNowDispatcher
	queueEdit                QueueEditLeaseController
	accessAuthorizer         QueueAccessAuthorizer
	sessionTaskResolver      SessionTaskResolver
	eventBus                 bus.EventBus
	logger                   *logger.Logger
	referenceValidator       entityrefs.SubmissionValidator
	attachmentClaimer        QueueAttachmentClaimer
	attachmentCleanupMu      sync.Mutex
	pendingAttachmentCleanup map[pendingQueueAttachmentCleanupKey]*pendingQueueAttachmentCleanup
	attachmentCleanupCtx     context.Context
	attachmentCleanupCancel  context.CancelFunc
	attachmentCleanupWG      sync.WaitGroup
	attachmentCleanupStarted bool
	attachmentCleanupStopped bool
}

// SetAttachmentClaimer wires task-owned attachment claiming into queue edits.
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
		queueService:             queueService,
		queueDrainer:             queueDrainer,
		accessAuthorizer:         accessAuthorizer,
		sessionTaskResolver:      sessionTaskResolver,
		eventBus:                 eventBus,
		logger:                   log.WithFields(zap.String("component", "queue-handlers")),
		referenceValidator:       referenceValidator,
		pendingAttachmentCleanup: make(map[pendingQueueAttachmentCleanupKey]*pendingQueueAttachmentCleanup),
	}
	if controller, ok := queueService.(QueueEditLeaseController); ok {
		handlers.queueEdit = controller
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

// Start owns the context used by pending attachment cleanup retries and reloads
// durable obligations left by an earlier process.
func (h *QueueHandlers) Start(ctx context.Context) {
	h.attachmentCleanupMu.Lock()
	if h.attachmentCleanupStarted || h.attachmentCleanupStopped {
		h.attachmentCleanupMu.Unlock()
		return
	}
	h.attachmentCleanupCtx, h.attachmentCleanupCancel = context.WithCancel(ctx)
	h.attachmentCleanupStarted = true
	h.loadPendingAttachmentCleanupsLocked(context.WithoutCancel(ctx))
	for _, pending := range h.pendingAttachmentCleanup {
		h.attachmentCleanupWG.Add(1)
		go h.retryPendingAttachmentCleanup(pending)
	}
	h.attachmentCleanupMu.Unlock()
}

func (h *QueueHandlers) loadPendingAttachmentCleanupsLocked(ctx context.Context) {
	store, ok := h.attachmentCleanupStore()
	releaser, canRelease := h.attachmentClaimer.(QueueAttachmentReleaser)
	if !ok || !canRelease {
		return
	}
	cleanups, err := store.ListAttachmentCleanups(ctx)
	if err != nil {
		h.logger.Error("failed to reload pending queue attachment cleanup", zap.Error(err))
		return
	}
	for _, cleanup := range cleanups {
		key := pendingQueueAttachmentCleanupKey{
			sessionID: cleanup.SessionID, entryID: cleanup.EntryID, operationID: cleanup.OperationID,
		}
		if _, exists := h.pendingAttachmentCleanup[key]; exists {
			continue
		}
		currentSessionID := cleanup.CurrentSessionID
		if currentSessionID == "" {
			currentSessionID = cleanup.SessionID
		}
		authCtx, cleanupReleaser := h.recoverAttachmentCleanupContext(
			ctx, cleanup, currentSessionID, releaser,
		)
		if cleanupReleaser == nil {
			continue
		}
		h.pendingAttachmentCleanup[key] = &pendingQueueAttachmentCleanup{
			key: key,
			req: wsUpdateMessageRequest{
				SessionID:   cleanup.SessionID,
				EntryID:     cleanup.EntryID,
				LeaseID:     cleanup.LeaseID,
				OperationID: cleanup.OperationID,
			},
			previous: &messagequeue.QueuedMessage{
				ID: cleanup.EntryID, SessionID: cleanup.SessionID, TaskID: cleanup.TaskID,
				Attachments: cleanup.Attachments,
			},
			releaser:         cleanupReleaser,
			removeEntry:      cleanup.RemoveEntry,
			claimPending:     cleanup.ClaimPending,
			entryFingerprint: cleanup.EntryFingerprint,
			currentSessionID: cleanup.CurrentSessionID,
			authCtx:          authCtx,
			wake:             make(chan struct{}, 1),
		}
	}
}

func (h *QueueHandlers) recoverAttachmentCleanupContext(
	ctx context.Context,
	cleanup messagequeue.AttachmentCleanup,
	currentSessionID string,
	releaser QueueAttachmentReleaser,
) (context.Context, QueueAttachmentReleaser) {
	authCtx := context.WithoutCancel(ctx)
	var ownerID string
	var resolveErr error
	if cleanup.OwnerID != "" {
		ownerID = cleanup.OwnerID
	} else if resolver, resolverOK := h.attachmentClaimer.(queueAttachmentCleanupOwnerResolver); resolverOK {
		ownerID, resolveErr = resolver.ResolveMessageAttachmentOwner(
			authCtx, cleanup.TaskID, currentSessionID, queueAttachmentsToV1(cleanup.Attachments),
		)
	}
	var cleanupReleaser QueueAttachmentReleaser
	if ownerID != "" {
		authCtx = authn.WithIdentity(authCtx, authn.Identity{UserID: ownerID})
		cleanupReleaser = releaser
	} else if cleanup.OwnerID == "" && !cleanup.ClaimPending {
		if internalReleaser, canRelease := h.attachmentClaimer.(queueAttachmentCleanupReleaser); canRelease {
			cleanupReleaser = internalQueueAttachmentReleaser{releaser: internalReleaser}
		}
	}
	if cleanupReleaser == nil && cleanup.OwnerID == "" {
		if resolveErr != nil {
			h.logger.Warn("failed to recover owner for queue attachment cleanup", zap.Error(resolveErr))
		} else {
			h.logger.Warn("queue attachment cleanup owner is unavailable")
		}
	}
	return authCtx, cleanupReleaser
}

func (h *QueueHandlers) attachmentCleanupStore() (queueAttachmentCleanupStore, bool) {
	store, ok := h.queueService.(queueAttachmentCleanupStore)
	return store, ok && store.AttachmentCleanupPersistenceAvailable()
}

// Stop cancels active retries after their obligations are durable. A later
// handler instance reloads unresolved work before accepting queue operations.
func (h *QueueHandlers) Stop() {
	h.attachmentCleanupMu.Lock()
	if !h.attachmentCleanupStopped {
		h.attachmentCleanupStopped = true
		if h.attachmentCleanupCancel != nil {
			h.attachmentCleanupCancel()
		}
	}
	h.attachmentCleanupMu.Unlock()
	h.attachmentCleanupWG.Wait()
}

// RegisterHandlers registers queue handlers with the dispatcher.
func (h *QueueHandlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionMessageQueueAdd, h.wsQueueMessage)
	d.RegisterFunc(ws.ActionMessageQueueCancel, h.wsCancelAll)
	d.RegisterFunc(ws.ActionMessageQueueGet, h.wsGetQueueStatus)
	d.RegisterFunc(ws.ActionMessageQueueUpdate, h.wsUpdateMessage)
	d.RegisterFunc(ws.ActionMessageQueueEditBegin, h.wsBeginEdit)
	d.RegisterFunc(ws.ActionMessageQueueEditRenew, h.wsRenewEdit)
	d.RegisterFunc(ws.ActionMessageQueueEditEnd, h.wsEndEdit)
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
	SessionID             string                           `json:"session_id"`
	TaskID                string                           `json:"task_id"`
	SessionIncarnationID  string                           `json:"session_incarnation_id"`
	ClientQueueID         string                           `json:"client_queue_id,omitempty"`
	Content               string                           `json:"content"`
	Model                 string                           `json:"model,omitempty"`
	PlanMode              bool                             `json:"plan_mode,omitempty"`
	Attachments           []messagequeue.MessageAttachment `json:"attachments,omitempty"`
	ContextFiles          []v1.ContextFileMeta             `json:"context_files,omitempty"`
	EntityReferences      []v1.EntityReference             `json:"entity_references,omitempty"`
	PlanCommentRefs       []models.TaskPlanCommentRef      `json:"plan_comment_refs,omitempty"`
	RequirePrimarySession bool                             `json:"require_primary_session,omitempty"`
	UserID                string                           `json:"user_id,omitempty"`
}

// wsQueueMessage handles ActionMessageQueueAdd, appending a new entry to the session queue.
func (h *QueueHandlers) wsQueueMessage(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsQueueMessageRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	req.ClientQueueID = strings.TrimSpace(req.ClientQueueID)

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
	if req.Content == "" && len(req.Attachments) == 0 && len(req.PlanCommentRefs) == 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "content or attachments are required", nil)
	}
	if validationError := validatePlanCommentQueueRequest(req); validationError != "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, validationError, nil)
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
	var snapshot *models.TaskPlanCommentSnapshot
	var replay bool
	var queued *messagequeue.QueuedMessage
	if len(req.PlanCommentRefs) > 0 {
		result, admissionErr := h.admitPlanCommentQueuedMessage(ctx, &req, queuedBy, metadata)
		err = admissionErr
		if result != nil {
			queued, snapshot, replay = result.Message, result.Snapshot, result.Replay
		}
	} else {
		queued, err = h.admitQueuedMessage(ctx, &req, queuedBy, metadata)
	}
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
		if conflict := planCommentQueueError(msg, err); conflict != nil {
			return conflict, nil
		}
		h.logger.Error("failed to queue message", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to queue message", nil)
	}

	identity := messagequeue.QueueSessionIdentity{
		TaskID:               req.TaskID,
		SessionID:            req.SessionID,
		SessionIncarnationID: req.SessionIncarnationID,
	}
	if !replay {
		h.publishPlanCommentSnapshot(ctx, snapshot)
	}
	h.publishStatusForIdentity(ctx, identity, queued)
	if h.queueReadiness != nil {
		h.queueReadiness.CheckQueueAdmissionReadiness(ctx, identity)
	}
	return ws.NewResponse(msg.ID, msg.Action, queued)
}

var (
	errQueuedAttachmentUnavailable   = errors.New("queued attachment unavailable")
	errQueuedAttachmentRollback      = errors.New("queued attachment rollback failed")
	errAttachmentCleanupLeaseActive  = errors.New("queue attachment cleanup blocked by edit lease")
	errAttachmentCleanupEntryChanged = errors.New("queue attachment cleanup entry changed")
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
			func(admittedCtx context.Context, source *messagequeue.QueuedMessage) error {
				releaser, canRelease := h.attachmentClaimer.(QueueAttachmentReleaser)
				if !canRelease {
					return errors.New("queue attachment claimer cannot release failed claims")
				}
				pending, err := h.preparePendingAttachmentCleanupWithState(
					admittedCtx,
					wsUpdateMessageRequest{
						SessionID: source.SessionID, EntryID: source.ID,
						OperationID: attachmentCleanupOperationID("", "admission"),
					},
					source.TaskID,
					source.Attachments,
					releaser,
					true,
					source,
				)
				if err != nil {
					_, rollbackErr := h.rollbackQueuedAttachmentClaim(admittedCtx, source.SessionID, source.ID)
					return fmt.Errorf("%w: %v", errQueuedAttachmentRollback, errors.Join(err, rollbackErr))
				}
				claimErr := h.attachmentClaimer.ClaimMessageAttachments(
					admittedCtx, req.TaskID, req.SessionID, queueAttachmentsToV1(req.Attachments),
				)
				if claimErr == nil {
					deleteErr := h.deletePendingAttachmentCleanup(pending)
					if deleteErr == nil {
						return nil
					}
					_, rollbackErr := h.rollbackQueuedAttachmentClaim(admittedCtx, req.SessionID, source.ID)
					cleanupErr := h.releaseSupersededQueueAttachmentsAdmitted(
						admittedCtx, pending.req, pending.previous, pending.releaser,
					)
					h.settlePendingAttachmentCleanup(pending, errors.Join(rollbackErr, cleanupErr))
					return fmt.Errorf(
						"%w: %v",
						errQueuedAttachmentRollback,
						errors.Join(deleteErr, rollbackErr, cleanupErr),
					)
				}
				pending.claimPending = false
				pending.removeEntry = true
				if err := h.persistPendingAttachmentCleanup(admittedCtx, pending); err != nil {
					_, rollbackErr := h.rollbackQueuedAttachmentClaim(admittedCtx, req.SessionID, source.ID)
					h.queuePendingAttachmentCleanup(pending)
					return fmt.Errorf("%w: %v", errQueuedAttachmentRollback, errors.Join(err, rollbackErr))
				}
				_, rollbackErr := h.rollbackQueuedAttachmentClaim(admittedCtx, req.SessionID, source.ID)
				cleanupErr := h.releaseSupersededQueueAttachmentsAdmitted(
					admittedCtx, pending.req, pending.previous, pending.releaser,
				)
				h.settlePendingAttachmentCleanup(pending, errors.Join(rollbackErr, cleanupErr))
				if rollbackErr != nil || cleanupErr != nil {
					return fmt.Errorf("%w: %v", errQueuedAttachmentRollback, errors.Join(rollbackErr, cleanupErr))
				}
				return fmt.Errorf("%w: %v", errQueuedAttachmentUnavailable, claimErr)
			},
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

func (h *QueueHandlers) admitPlanCommentQueuedMessage(
	ctx context.Context,
	req *wsQueueMessageRequest,
	queuedBy string,
	metadata map[string]interface{},
) (*messagequeue.PlanCommentQueueResult, error) {
	service, ok := h.queueService.(planCommentQueueService)
	if !ok {
		return nil, errors.New("plan comment queue admission is unavailable")
	}
	attachments := queueAttachmentsToV1(req.Attachments)
	var attachmentClaim *messagequeue.QueueAttachmentClaim
	if h.attachmentClaimer != nil && len(attachments) > 0 {
		preparer, ok := h.attachmentClaimer.(QueueAttachmentClaimPreparer)
		if !ok {
			return nil, errors.New("transactional attachment admission is unavailable")
		}
		claim, err := preparer.PrepareQueueAttachmentClaim(ctx, req.TaskID, attachments)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errQueuedAttachmentUnavailable, err)
		}
		attachmentClaim = &claim
	}
	return service.QueueMessageWithPlanComments(ctx, messagequeue.PlanCommentQueueRequest{
		ClientQueueID: req.ClientQueueID, SessionID: req.SessionID, TaskID: req.TaskID,
		SessionIncarnationID: req.SessionIncarnationID,
		Content:              req.Content, Model: req.Model, UserID: queuedBy, PlanMode: req.PlanMode,
		Attachments: req.Attachments, Metadata: metadata, PlanCommentRefs: req.PlanCommentRefs,
		RequirePrimarySession: req.RequirePrimarySession, AttachmentClaim: attachmentClaim,
	})
}

func validatePlanCommentQueueRequest(req wsQueueMessageRequest) string {
	if len(req.PlanCommentRefs) == 0 {
		return ""
	}
	if req.ClientQueueID == "" {
		return "client_queue_id is required with plan comments"
	}
	if len(req.ClientQueueID) > 128 {
		return "client_queue_id is too long"
	}
	if plancomments.ContainsReservedPlaceholder(req.Content) {
		return "content contains a reserved plan comment marker"
	}
	seen := make(map[string]struct{}, len(req.PlanCommentRefs))
	for _, ref := range req.PlanCommentRefs {
		if ref.ID == "" || ref.Version <= 0 {
			return "plan_comment_refs are invalid"
		}
		if _, duplicate := seen[ref.ID]; duplicate {
			return "plan_comment_refs contain duplicates"
		}
		seen[ref.ID] = struct{}{}
	}
	return ""
}

func planCommentQueueError(msg *ws.Message, err error) *ws.Message {
	var commentsChanged *plancommenttx.CommentsChangedError
	if errors.As(err, &commentsChanged) {
		details := map[string]interface{}{}
		if commentsChanged.Snapshot != nil {
			details["snapshot"] = dto.TaskPlanCommentSnapshotFromModel(commentsChanged.Snapshot)
		}
		response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodePlanCommentsChanged, "Task plan comments changed", details)
		return response
	}
	var primaryChanged *plancommenttx.PrimarySessionChangedError
	if errors.As(err, &primaryChanged) {
		details := map[string]interface{}{
			"primary_session_id": nil, "primary_session_state": nil,
		}
		if primaryChanged.SessionID != "" {
			details["primary_session_id"] = primaryChanged.SessionID
			details["primary_session_state"] = primaryChanged.State
		}
		response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodePrimarySessionChanged, "Primary session changed",
			details)
		return response
	}
	if errors.Is(err, repoerrors.ErrTaskSessionMismatch) {
		response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Session does not belong to task", nil)
		return response
	}
	var unavailable *plancommenttx.SessionUnavailableError
	if errors.As(err, &unavailable) {
		response, _ := ws.NewError(msg.ID, msg.Action, queueErrorCodeNotPromptable,
			"Session is not ready for input", map[string]interface{}{
				fieldSessionID: unavailable.SessionID, "session_state": unavailable.State,
			})
		return response
	}
	if errors.Is(err, messagequeue.ErrQueueIDConflict) {
		response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "client_queue_id is already used", nil)
		return response
	}
	return nil
}

func (h *QueueHandlers) publishPlanCommentSnapshot(
	ctx context.Context,
	snapshot *models.TaskPlanCommentSnapshot,
) {
	if snapshot == nil || h.eventBus == nil {
		return
	}
	if err := h.eventBus.Publish(ctx, events.TaskPlanCommentsChanged,
		bus.NewEvent(events.TaskPlanCommentsChanged, "queue-handlers", snapshot)); err != nil {
		h.logger.Error("publish consumed plan comments", zap.String("task_id", snapshot.TaskID), zap.Error(err))
	}
}

func (h *QueueHandlers) claimQueuedAttachmentsAfterInsert(req *wsQueueMessageRequest) func(context.Context, *messagequeue.QueuedMessage) error {
	return func(admittedCtx context.Context, source *messagequeue.QueuedMessage) error {
		claimErr := h.attachmentClaimer.ClaimMessageAttachments(
			admittedCtx, req.TaskID, req.SessionID, queueAttachmentsToV1(req.Attachments),
		)
		if claimErr == nil {
			return nil
		}
		if _, rollbackErr := h.rollbackQueuedAttachmentClaim(admittedCtx, req.SessionID, source.ID); rollbackErr != nil {
			h.logger.Error("failed to roll back queued attachment", zap.Error(rollbackErr))
			return fmt.Errorf("%w: %v", errQueuedAttachmentRollback, rollbackErr)
		}
		return fmt.Errorf("%w: %v", errQueuedAttachmentUnavailable, claimErr)
	}
}

type wsQueueEditRequest struct {
	SessionID         string `json:"session_id"`
	EntryID           string `json:"entry_id"`
	LeaseID           string `json:"lease_id"`
	DispatchIfAutoRun bool   `json:"dispatch_if_auto_run,omitempty"`
}

func (h *QueueHandlers) queueEditError(msg *ws.Message, err error) *ws.Message {
	switch {
	case errors.Is(err, messagequeue.ErrEntryNotFound):
		response, _ := ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound,
			"Queue entry was already drained or is no longer editable", nil)
		return response
	case errors.Is(err, messagequeue.ErrEditLeaseNotFound):
		response, _ := ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound,
			"Queue entry was already drained or is no longer editable", nil)
		return response
	case errors.Is(err, messagequeue.ErrEditConflict):
		response, _ := ws.NewError(msg.ID, msg.Action, "edit_conflict",
			"Queue entry is being edited by another view", nil)
		return response
	case errors.Is(err, messagequeue.ErrEditRevisionConflict):
		response, _ := ws.NewError(msg.ID, msg.Action, "queue_conflict",
			"Queue entry changed while it was being edited", nil)
		return response
	default:
		h.logger.Error("failed to process queue edit", zap.Error(err))
		response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"Failed to process queue edit", nil)
		return response
	}
}
func (h *QueueHandlers) queueEditLeaseError(msg *ws.Message, err error) *ws.Message {
	if errors.Is(err, messagequeue.ErrEditLeaseNotFound) {
		response, _ := ws.NewError(msg.ID, msg.Action, "edit_conflict", "Queue entry edit lease is no longer valid", nil)
		return response
	}
	return h.queueEditError(msg, err)
}

func (h *QueueHandlers) wsBeginEdit(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsQueueEditRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" || req.EntryID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id and entry_id are required", nil)
	}
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}
	if h.queueEdit == nil || ws.ConnectionID(ctx) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "Queue editing requires a WebSocket connection", nil)
	}
	lease, err := h.queueEdit.BeginEdit(ctx, req.SessionID, req.EntryID, ws.ConnectionID(ctx))
	if err != nil {
		return h.queueEditError(msg, err), nil
	}
	return ws.NewResponse(msg.ID, msg.Action, lease)
}

func (h *QueueHandlers) wsRenewEdit(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsQueueEditRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" || req.EntryID == "" || req.LeaseID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id, entry_id, and lease_id are required", nil)
	}
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}
	if h.queueEdit == nil || ws.ConnectionID(ctx) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "Queue editing requires a WebSocket connection", nil)
	}
	lease, err := h.queueEdit.RenewEdit(ctx, req.SessionID, req.EntryID, req.LeaseID, ws.ConnectionID(ctx))
	if err != nil {
		return h.queueEditLeaseError(msg, err), nil
	}
	return ws.NewResponse(msg.ID, msg.Action, lease)
}

func (h *QueueHandlers) wsEndEdit(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsQueueEditRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" || req.EntryID == "" || req.LeaseID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id, entry_id, and lease_id are required", nil)
	}
	if denied := h.authorizeSession(ctx, msg, req.SessionID); denied != nil {
		return denied, nil
	}
	if h.queueEdit == nil || ws.ConnectionID(ctx) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "Queue editing requires a WebSocket connection", nil)
	}
	var saved bool
	var err error
	if req.DispatchIfAutoRun {
		controller, ok := h.queueEdit.(queueEditSavedLeaseController)
		if !ok {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Queue edit save completion is unavailable", nil)
		}
		saved, err = controller.EndEditAfterSave(ctx, req.SessionID, req.EntryID, req.LeaseID, ws.ConnectionID(ctx))
	} else {
		err = h.queueEdit.EndEdit(ctx, req.SessionID, req.EntryID, req.LeaseID, ws.ConnectionID(ctx))
	}
	if err != nil {
		return h.queueEditLeaseError(msg, err), nil
	}
	h.signalPendingAttachmentCleanup(ctx, req.SessionID, req.EntryID)
	if saved {
		if drainer, ok := h.queueDrainer.(QueueAutoRunPreservingDrainer); ok {
			if _, drainErr := drainer.DrainQueuedMessageIfAutoRun(ctx, req.SessionID); drainErr != nil {
				h.logger.Warn("post-save queued message drain failed",
					zap.String(fieldSessionID, req.SessionID), zap.String(fieldEntryID, req.EntryID), zap.Error(drainErr))
			}
		} else {
			h.logger.Warn("post-save queued message drain unavailable",
				zap.String(fieldSessionID, req.SessionID), zap.String(fieldEntryID, req.EntryID))
		}
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]string{fieldSessionID: req.SessionID, fieldEntryID: req.EntryID})
}

type wsUpdateMessageRequest struct {
	SessionID            string                           `json:"session_id"`
	TaskID               string                           `json:"task_id"`
	SessionIncarnationID string                           `json:"session_incarnation_id"`
	EntryID              string                           `json:"entry_id"`
	LeaseID              string                           `json:"lease_id,omitempty"`
	OperationID          string                           `json:"operation_id,omitempty"`
	ExpectedRevision     *int64                           `json:"expected_target_revision,omitempty"`
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
	connectionID := ws.ConnectionID(ctx)
	if connectionID != "" && (h.queueEdit == nil || req.LeaseID == "" || req.OperationID == "" || req.ExpectedRevision == nil) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"lease_id, operation_id, and expected_target_revision are required", nil)
	}
	referencesProvided := req.EntityReferences != nil
	references, err := h.validateSubmittedReferences(ctx, req.SessionID, "", req.EntityReferences)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, queueInvalidReferences, nil)
	}
	// Default empty user_id to QueuedByUser so the UpdateContent guard always
	// runs against a non-empty owner. Agent entries (queued_by="agent") then
	// fail the filter, mirroring the canEdit UI gate at the WS layer.
	req.EntityReferences = references
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
		releaseClaims, _ = h.attachmentClaimer.(QueueAttachmentReleaser)
		if h.requiresQueueIdentity() {
			if preparer, ok := h.attachmentClaimer.(QueueAttachmentClaimPreparer); ok {
				prepared, prepareErr := preparer.PrepareQueueAttachmentClaim(ctx, previous.TaskID, queueAttachmentsToV1(newlyAdded))
				if prepareErr != nil {
					return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Attachment is no longer available", nil)
				}
				atomicClaim = &prepared
			}
		}
	}
	var supersededPending *pendingQueueAttachmentCleanup
	if releaseClaims != nil && previous != nil {
		cleanupReq := req
		cleanupReq.OperationID = attachmentCleanupOperationID(req.OperationID, "superseded")
		var prepareErr error
		supersededPending, prepareErr = h.preparePendingAttachmentCleanup(
			ctx,
			cleanupReq,
			previous.TaskID,
			supersededQueueAttachments(previous.Attachments, req.Attachments),
			releaseClaims,
		)
		if prepareErr != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, prepareErr.Error(), nil)
		}
	}
	var revision int64
	var updateErr error
	applyUpdate := func(updateCtx context.Context) (int64, error) {
		attachmentsToClaim := newlyAdded
		newlyAdded = nil
		var rollbackPending *pendingQueueAttachmentCleanup
		if atomicClaim == nil && h.attachmentClaimer != nil {
			rollbackReq := req
			rollbackReq.OperationID = attachmentCleanupOperationID(req.OperationID, "claim-rollback")
			var prepareErr error
			rollbackPending, prepareErr = h.preparePendingAttachmentCleanup(
				updateCtx, rollbackReq, previous.TaskID, attachmentsToClaim, releaseClaims,
			)
			if prepareErr != nil {
				return 0, prepareErr
			}
			if err := h.attachmentClaimer.ClaimMessageAttachments(updateCtx, previous.TaskID, req.SessionID, queueAttachmentsToV1(attachmentsToClaim)); err != nil {
				releaseErr := h.releaseQueuedAttachmentUpdateFailure(
					updateCtx, previous, req.SessionID, attachmentsToClaim, releaseClaims,
				)
				h.settlePendingAttachmentCleanup(rollbackPending, releaseErr)
				return 0, fmt.Errorf("%w: %v", errQueuedAttachmentUnavailable, err)
			}
		}
		switch {
		case connectionID != "":
			revision, updateErr = h.queueEdit.UpdateMessageWithLease(updateCtx, req.SessionID, req.EntryID,
				req.LeaseID, req.OperationID, connectionID, *req.ExpectedRevision, req.Content,
				req.Attachments, metadataUpdates)
		case h.requiresQueueIdentity():
			updateErr = h.updateQueuedMessage(updateCtx, identity, req, metadataUpdates, queuedBy, atomicClaim)
		default:
			// Non-WebSocket/MCP updates still use the service's durable lease fencing.
			updateErr = h.queueService.UpdateMessageWithMetadata(updateCtx, req.SessionID, req.EntryID,
				req.Content, req.Attachments, metadataUpdates, queuedBy)
		}
		if updateErr != nil && atomicClaim == nil && h.attachmentClaimer != nil {
			releaseErr := h.releaseQueuedAttachmentUpdateFailure(
				updateCtx, previous, req.SessionID, attachmentsToClaim, releaseClaims,
			)
			h.settlePendingAttachmentCleanup(rollbackPending, releaseErr)
		} else {
			h.settlePendingAttachmentCleanup(rollbackPending, nil)
		}
		return revision, updateErr
	}
	if connectionID != "" && h.attachmentClaimer != nil {
		revision, updateErr = h.updateMessageWithAttachmentLease(
			ctx, req, connectionID, previous, releaseClaims, supersededPending,
			&newlyAdded, metadataUpdates, applyUpdate,
		)
	} else {
		revision, updateErr = applyUpdate(ctx)
	}
	if updateErr != nil {
		h.settlePendingAttachmentCleanup(supersededPending, nil)
		return h.queueUpdateFailure(ctx, msg, req, updateErr)
	}
	if releaseClaims != nil && supersededPending != nil && connectionID == "" {
		releaseErr := h.releaseSupersededQueueAttachments(
			ctx, supersededPending.req, supersededPending.previous, releaseClaims,
		)
		h.settlePendingAttachmentCleanup(supersededPending, releaseErr)
	}
	response := map[string]interface{}{fieldEntryID: req.EntryID}
	if req.OperationID != "" {
		response["operation_id"] = req.OperationID
		response["target_revision"] = revision
	}
	h.publishStatus(ctx, req.SessionID)
	return ws.NewResponse(msg.ID, msg.Action, response)
}

func (h *QueueHandlers) releaseQueuedAttachmentUpdateFailure(
	ctx context.Context,
	previous *messagequeue.QueuedMessage,
	sessionID string,
	newlyAdded []messagequeue.MessageAttachment,
	releaseClaims QueueAttachmentReleaser,
) error {
	if releaseClaims == nil || previous == nil || len(newlyAdded) == 0 {
		return nil
	}
	cleanupCtx := context.WithoutCancel(ctx)
	candidates, err := h.unreferencedQueueAttachments(cleanupCtx, sessionID, previous.ID, newlyAdded)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return nil
	}
	if releaseErr := releaseClaims.ReleaseMessageAttachments(cleanupCtx, previous.TaskID, sessionID, queueAttachmentsToV1(candidates)); releaseErr != nil {
		h.logger.Warn("failed to release attachments after queue update failure", zap.Error(releaseErr))
		return releaseErr
	}
	return nil
}

// unreferencedQueueAttachments prevents cleanup from deleting a claim that a
// different pending entry in the same session still uses. A reference lookup
// failure fails closed because releasing in that case can destroy another
// queued prompt's attachment.
func (h *QueueHandlers) unreferencedQueueAttachments(
	ctx context.Context,
	sessionID, excludedEntryID string,
	candidates []messagequeue.MessageAttachment,
) ([]messagequeue.MessageAttachment, error) {
	ctx = context.WithoutCancel(ctx)
	if len(candidates) == 0 {
		return nil, nil
	}
	unique := make([]messagequeue.MessageAttachment, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, attachment := range candidates {
		if attachment.AttachmentID == "" {
			continue
		}
		if _, ok := seen[attachment.AttachmentID]; ok {
			continue
		}
		seen[attachment.AttachmentID] = struct{}{}
		unique = append(unique, attachment)
	}
	if len(unique) == 0 {
		return nil, nil
	}
	checker, ok := h.queueService.(queueAttachmentReferenceChecker)
	if !ok {
		return unique, nil
	}
	ids := make([]string, 0, len(unique))
	for _, attachment := range unique {
		ids = append(ids, attachment.AttachmentID)
	}
	referenced, err := checker.ReferencedQueueAttachmentIDs(ctx, sessionID, excludedEntryID, ids)
	if err != nil {
		h.logger.Warn("failed to inspect queue attachment references", zap.Error(err))
		return nil, err
	}
	unreferenced := make([]messagequeue.MessageAttachment, 0, len(unique))
	for _, attachment := range unique {
		if _, ok := referenced[attachment.AttachmentID]; !ok {
			unreferenced = append(unreferenced, attachment)
		}
	}
	return unreferenced, nil
}

// releaseSupersededQueueAttachments serializes attachment cleanup with later
// edits and recomputes the retained set from the current queue row. A second
// edit may reclaim an attachment after this update commits but before its
// cleanup runs; releasing from the stale request snapshot would then destroy
// a claim still referenced by the row.
func (h *QueueHandlers) releaseSupersededQueueAttachments(
	ctx context.Context,
	req wsUpdateMessageRequest,
	previous *messagequeue.QueuedMessage,
	releaser QueueAttachmentReleaser,
) error {
	release := func(admittedCtx context.Context) error {
		return h.releaseSupersededQueueAttachmentsAdmitted(admittedCtx, req, previous, releaser)
	}
	if admission, ok := h.queueService.(queueEditAdmissionController); ok {
		if err := admission.WithSessionAdmission(context.WithoutCancel(ctx), req.SessionID, release); err != nil {
			h.logger.Warn("failed to serialize superseded queue attachment cleanup", zap.Error(err))
			return err
		}
		return nil
	}
	if err := release(context.WithoutCancel(ctx)); err != nil {
		h.logger.Warn("failed to release superseded queue attachments", zap.Error(err))
		return err
	}
	return nil
}

func (h *QueueHandlers) releaseSupersededQueueAttachmentsAdmitted(
	ctx context.Context,
	req wsUpdateMessageRequest,
	previous *messagequeue.QueuedMessage,
	releaser QueueAttachmentReleaser,
) error {
	cleanupCtx := context.WithoutCancel(ctx)
	current, err := h.queueService.GetEntry(cleanupCtx, req.SessionID, req.EntryID)
	if errors.Is(err, messagequeue.ErrEntryNotFound) {
		if locator, ok := h.queueService.(queueEntryLocator); ok {
			current, err = locator.FindEntryByID(cleanupCtx, req.EntryID)
			if err == nil && current != nil {
				req.SessionID = current.SessionID
			}
		}
		if errors.Is(err, messagequeue.ErrEntryNotFound) {
			return h.releaseQueueAttachmentCandidates(
				cleanupCtx, previous.TaskID, req, previous.Attachments, releaser,
			)
		}
	}
	if err != nil {
		h.logger.Warn("failed to reload queue entry before attachment cleanup", zap.Error(err))
		return err
	}
	if current == nil {
		return nil
	}
	return h.releaseQueueAttachmentCandidates(
		cleanupCtx, previous.TaskID, req, supersededQueueAttachments(previous.Attachments, current.Attachments), releaser,
	)
}

func (h *QueueHandlers) releaseQueueAttachmentCandidates(
	ctx context.Context,
	taskID string,
	req wsUpdateMessageRequest,
	candidates []messagequeue.MessageAttachment,
	releaser QueueAttachmentReleaser,
) error {
	cleanupCtx := context.WithoutCancel(ctx)
	unreferenced, err := h.unreferencedQueueAttachments(cleanupCtx, req.SessionID, req.EntryID, candidates)
	if err != nil {
		return err
	}
	if len(unreferenced) == 0 {
		return nil
	}
	if err := releaser.ReleaseMessageAttachments(
		cleanupCtx, taskID, req.SessionID, queueAttachmentsToV1(unreferenced),
	); err != nil {
		h.logger.Warn("failed to release superseded queue attachments", zap.Error(err))
		return err
	}
	return nil
}

func (h *QueueHandlers) preparePendingAttachmentCleanup(
	ctx context.Context,
	req wsUpdateMessageRequest,
	taskID string,
	attachments []messagequeue.MessageAttachment,
	releaser QueueAttachmentReleaser,
) (*pendingQueueAttachmentCleanup, error) {
	return h.preparePendingAttachmentCleanupWithState(
		ctx, req, taskID, attachments, releaser, false, nil,
	)
}

func (h *QueueHandlers) preparePendingAttachmentCleanupWithState(
	ctx context.Context,
	req wsUpdateMessageRequest,
	taskID string,
	attachments []messagequeue.MessageAttachment,
	releaser QueueAttachmentReleaser,
	claimPending bool,
	expectedEntry *messagequeue.QueuedMessage,
) (*pendingQueueAttachmentCleanup, error) {
	if releaser == nil || len(attachments) == 0 {
		return nil, nil
	}
	entryFingerprint, err := queuedMessageFingerprint(expectedEntry)
	if err != nil {
		return nil, err
	}
	key := pendingQueueAttachmentCleanupKey{
		sessionID: req.SessionID, entryID: req.EntryID, operationID: req.OperationID,
	}
	cleanupCtx := context.Background()
	if identity, ok := authn.IdentityFromContext(ctx); ok {
		cleanupCtx = authn.WithIdentity(cleanupCtx, identity)
	}
	previous := &messagequeue.QueuedMessage{
		ID: req.EntryID, SessionID: req.SessionID, TaskID: taskID,
		Attachments: append([]messagequeue.MessageAttachment(nil), attachments...),
	}
	pending := &pendingQueueAttachmentCleanup{
		key:      key,
		req:      req,
		previous: previous,
		releaser: releaser, removeEntry: claimPending, claimPending: claimPending,
		entryFingerprint: entryFingerprint,
		authCtx:          cleanupCtx, wake: make(chan struct{}, 1),
	}
	persist := func(admittedCtx context.Context) error {
		if _, ok := h.attachmentCleanupStore(); !ok {
			return nil
		}
		current, err := h.queueService.GetEntry(admittedCtx, req.SessionID, req.EntryID)
		if err != nil {
			return fmt.Errorf("reload queue entry before cleanup persistence: %w", err)
		}
		h.setPendingAttachmentCleanupSessionID(pending, current.SessionID)
		return h.persistPendingAttachmentCleanup(admittedCtx, pending)
	}
	if admission, ok := h.queueService.(queueEditAdmissionController); ok {
		admissionCtx := ctx
		if admissionCtx.Err() != nil {
			admissionCtx = cleanupCtx
		}
		if err := admission.WithSessionAdmission(admissionCtx, req.SessionID, persist); err != nil {
			return nil, err
		}
		return pending, nil
	}
	if err := persist(cleanupCtx); err != nil {
		return nil, err
	}
	return pending, nil
}

func (h *QueueHandlers) persistPendingAttachmentCleanup(
	ctx context.Context,
	pending *pendingQueueAttachmentCleanup,
) error {
	store, ok := h.attachmentCleanupStore()
	if !ok {
		return nil
	}
	identity, hasIdentity := authn.IdentityFromContext(pending.authCtx)
	if !hasIdentity || identity.UserID == "" {
		return errors.New("queue attachment cleanup requires an owner identity")
	}
	if err := store.UpsertAttachmentCleanup(ctx, messagequeue.AttachmentCleanup{
		SessionID: pending.req.SessionID, CurrentSessionID: h.pendingAttachmentCleanupSessionID(pending),
		EntryID: pending.req.EntryID, OperationID: pending.req.OperationID,
		TaskID: pending.previous.TaskID, OwnerID: identity.UserID, LeaseID: pending.req.LeaseID,
		RemoveEntry:      pending.removeEntry,
		ClaimPending:     pending.claimPending,
		EntryFingerprint: pending.entryFingerprint,
		Attachments:      append([]messagequeue.MessageAttachment(nil), pending.previous.Attachments...),
		CreatedAt:        time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("persist queue attachment cleanup: %w", err)
	}
	return nil
}

func (h *QueueHandlers) queuePendingAttachmentCleanup(pending *pendingQueueAttachmentCleanup) {
	if pending == nil {
		return
	}
	h.attachmentCleanupMu.Lock()
	if h.attachmentCleanupStopped {
		h.attachmentCleanupMu.Unlock()
		return
	}
	if _, exists := h.pendingAttachmentCleanup[pending.key]; exists {
		h.attachmentCleanupMu.Unlock()
		return
	}
	h.pendingAttachmentCleanup[pending.key] = pending
	if !h.attachmentCleanupStarted {
		h.attachmentCleanupMu.Unlock()
		return
	}
	h.attachmentCleanupWG.Add(1)
	h.attachmentCleanupMu.Unlock()
	go h.retryPendingAttachmentCleanup(pending)
}

func (h *QueueHandlers) settlePendingAttachmentCleanup(
	pending *pendingQueueAttachmentCleanup,
	cleanupErr error,
) {
	if pending == nil {
		return
	}
	if cleanupErr != nil {
		h.queuePendingAttachmentCleanup(pending)
		return
	}
	if err := h.deletePendingAttachmentCleanup(pending); err != nil {
		h.logger.Warn("failed to acknowledge queue attachment cleanup", zap.Error(err))
		h.queuePendingAttachmentCleanup(pending)
	}
}

func (h *QueueHandlers) retryPendingAttachmentCleanup(pending *pendingQueueAttachmentCleanup) {
	defer h.attachmentCleanupWG.Done()
	delay := 10 * time.Millisecond
	for {
		if !h.editLeaseActive(pending) {
			if err := h.runPendingAttachmentCleanup(pending); err == nil {
				if err := h.deletePendingAttachmentCleanup(pending); err != nil {
					h.logger.Warn("failed to acknowledge queue attachment cleanup", zap.Error(err))
				} else {
					h.attachmentCleanupMu.Lock()
					delete(h.pendingAttachmentCleanup, pending.key)
					h.attachmentCleanupMu.Unlock()
					return
				}
			}
		}
		if !h.waitForAttachmentCleanupRetry(delay, pending.wake) {
			return
		}
		if delay < time.Second {
			delay *= 2
		}
	}
}

func (h *QueueHandlers) deletePendingAttachmentCleanup(pending *pendingQueueAttachmentCleanup) error {
	store, ok := h.attachmentCleanupStore()
	if !ok {
		return nil
	}
	return store.DeleteAttachmentCleanup(
		pending.authCtx, pending.key.sessionID, pending.key.entryID, pending.key.operationID,
	)
}

func (h *QueueHandlers) editLeaseActive(pending *pendingQueueAttachmentCleanup) bool {
	reader, ok := h.queueService.(queueEditLeaseStateReader)
	if !ok {
		return false
	}
	lease, err := reader.GetEditLease(
		pending.authCtx, h.pendingAttachmentCleanupSessionID(pending), pending.req.EntryID,
	)
	if err != nil {
		return !errors.Is(err, messagequeue.ErrEditLeaseNotFound)
	}
	return lease != nil
}

func (h *QueueHandlers) waitForAttachmentCleanupRetry(delay time.Duration, wake <-chan struct{}) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-h.attachmentCleanupCtx.Done():
		return false
	case <-wake:
		return true
	case <-timer.C:
		return true
	}
}

func (h *QueueHandlers) signalPendingAttachmentCleanup(ctx context.Context, sessionID, entryID string) {
	h.attachmentCleanupMu.Lock()
	pendingCleanups := make([]*pendingQueueAttachmentCleanup, 0)
	for key, pending := range h.pendingAttachmentCleanup {
		if key.entryID == entryID {
			pendingCleanups = append(pendingCleanups, pending)
		}
	}
	h.attachmentCleanupMu.Unlock()
	for _, pending := range pendingCleanups {
		if err := h.refreshPendingAttachmentCleanupSessionID(ctx, pending); err != nil {
			h.logger.Warn("failed to refresh queue attachment cleanup before wake", zap.Error(err))
		}
		currentSessionID := h.pendingAttachmentCleanupSessionID(pending)
		if pending.key.sessionID != sessionID && currentSessionID != sessionID {
			continue
		}
		select {
		case pending.wake <- struct{}{}:
		default:
		}
	}
}

func (h *QueueHandlers) pendingAttachmentCleanupSessionID(
	pending *pendingQueueAttachmentCleanup,
) string {
	h.attachmentCleanupMu.Lock()
	defer h.attachmentCleanupMu.Unlock()
	if pending.currentSessionID != "" {
		return pending.currentSessionID
	}
	return pending.req.SessionID
}

func (h *QueueHandlers) setPendingAttachmentCleanupSessionID(
	pending *pendingQueueAttachmentCleanup,
	sessionID string,
) {
	h.attachmentCleanupMu.Lock()
	pending.currentSessionID = sessionID
	h.attachmentCleanupMu.Unlock()
}

func (h *QueueHandlers) refreshPendingAttachmentCleanupSessionID(
	ctx context.Context,
	pending *pendingQueueAttachmentCleanup,
) error {
	if _, ok := h.attachmentCleanupStore(); !ok {
		return nil
	}
	locator, ok := h.queueService.(queueAttachmentCleanupLocator)
	if !ok {
		return nil
	}
	cleanup, err := locator.GetAttachmentCleanup(
		ctx, pending.key.sessionID, pending.key.entryID, pending.key.operationID,
	)
	if err != nil {
		return err
	}
	if cleanup == nil {
		return nil
	}
	sessionID := cleanup.CurrentSessionID
	if sessionID == "" {
		sessionID = cleanup.SessionID
	}
	h.setPendingAttachmentCleanupSessionID(pending, sessionID)
	return nil
}

func (h *QueueHandlers) runPendingAttachmentCleanup(pending *pendingQueueAttachmentCleanup) error {
	if err := h.refreshPendingAttachmentCleanupSessionID(pending.authCtx, pending); err != nil {
		return err
	}
	if _, err := h.retargetPendingAttachmentCleanup(pending.authCtx, pending); err != nil {
		return err
	}
	sessionID := h.pendingAttachmentCleanupSessionID(pending)
	cleanup := func(ctx context.Context) error {
		if h.editLeaseActive(pending) {
			return errAttachmentCleanupLeaseActive
		}
		return h.settlePendingAttachmentCleanupAdmitted(ctx, pending)
	}
	if admission, ok := h.queueService.(queueEditAdmissionController); ok {
		return admission.WithSessionAdmission(pending.authCtx, sessionID, cleanup)
	}
	return cleanup(pending.authCtx)
}

func (h *QueueHandlers) retargetPendingAttachmentCleanup(
	ctx context.Context,
	pending *pendingQueueAttachmentCleanup,
) (bool, error) {
	sessionID := h.pendingAttachmentCleanupSessionID(pending)
	current, err := h.queueService.GetEntry(ctx, sessionID, pending.req.EntryID)
	if err == nil {
		return current != nil, nil
	}
	if !errors.Is(err, messagequeue.ErrEntryNotFound) {
		return false, err
	}
	locator, ok := h.queueService.(queueEntryLocator)
	if !ok {
		return false, nil
	}
	current, err = locator.FindEntryByID(ctx, pending.req.EntryID)
	if errors.Is(err, messagequeue.ErrEntryNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if current == nil {
		return false, nil
	}
	h.setPendingAttachmentCleanupSessionID(pending, current.SessionID)
	return true, nil
}

func (h *QueueHandlers) settlePendingAttachmentCleanupAdmitted(
	ctx context.Context,
	pending *pendingQueueAttachmentCleanup,
) error {
	sessionID := h.pendingAttachmentCleanupSessionID(pending)
	req := pending.req
	req.SessionID = sessionID
	current, err := h.queueService.GetEntry(ctx, sessionID, req.EntryID)
	if errors.Is(err, messagequeue.ErrEntryNotFound) {
		found, retargetErr := h.retargetPendingAttachmentCleanup(ctx, pending)
		if retargetErr != nil {
			return retargetErr
		}
		if found {
			return errAttachmentCleanupEntryChanged
		}
		return h.releaseQueueAttachmentCandidates(
			ctx, pending.previous.TaskID, req, pending.previous.Attachments, pending.releaser,
		)
	}
	if err != nil {
		return err
	}
	if !pending.removeEntry && !pending.claimPending {
		return h.releaseQueueAttachmentCandidates(
			ctx,
			pending.previous.TaskID,
			req,
			supersededQueueAttachments(pending.previous.Attachments, current.Attachments),
			pending.releaser,
		)
	}
	currentFingerprint, err := queuedMessageFingerprint(current)
	if err != nil {
		return err
	}
	if pending.entryFingerprint == "" || currentFingerprint != pending.entryFingerprint {
		if sessionID != pending.key.sessionID {
			return errAttachmentCleanupEntryChanged
		}
		return h.releaseQueueAttachmentCandidates(
			ctx,
			pending.previous.TaskID,
			req,
			supersededQueueAttachments(pending.previous.Attachments, current.Attachments),
			pending.releaser,
		)
	}
	if pending.removeEntry {
		if _, err := h.rollbackQueuedAttachmentClaim(ctx, sessionID, req.EntryID); err != nil {
			return err
		}
		return h.releaseQueueAttachmentCandidates(
			ctx, pending.previous.TaskID, req, pending.previous.Attachments, pending.releaser,
		)
	}
	return h.attachmentClaimer.ClaimMessageAttachments(
		context.WithoutCancel(ctx),
		pending.previous.TaskID,
		sessionID,
		queueAttachmentsToV1(pending.previous.Attachments),
	)
}

func queuedMessageFingerprint(msg *messagequeue.QueuedMessage) (string, error) {
	if msg == nil {
		return "", nil
	}
	attachments := msg.Attachments
	if attachments == nil {
		attachments = []messagequeue.MessageAttachment{}
	}
	metadata := msg.Metadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	encoded, err := json.Marshal(struct {
		ID          string                           `json:"id"`
		TaskID      string                           `json:"task_id"`
		Content     string                           `json:"content"`
		Model       string                           `json:"model"`
		PlanMode    bool                             `json:"plan_mode"`
		Attachments []messagequeue.MessageAttachment `json:"attachments"`
		Metadata    map[string]interface{}           `json:"metadata"`
		QueuedAt    time.Time                        `json:"queued_at"`
		QueuedBy    string                           `json:"queued_by"`
	}{
		ID: msg.ID, TaskID: msg.TaskID, Content: msg.Content, Model: msg.Model, PlanMode: msg.PlanMode,
		Attachments: attachments, Metadata: metadata, QueuedAt: msg.QueuedAt.UTC(), QueuedBy: msg.QueuedBy,
	})
	if err != nil {
		return "", fmt.Errorf("fingerprint queued attachment source: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

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
func attachmentCleanupOperationID(operationID, kind string) string {
	if operationID == "" {
		return kind + "-" + uuid.NewString()
	}
	return operationID + ":" + kind
}

func (h *QueueHandlers) updateMessageWithAttachmentLease(
	ctx context.Context,
	req wsUpdateMessageRequest,
	connectionID string,
	previous *messagequeue.QueuedMessage,
	releaseClaims QueueAttachmentReleaser,
	supersededPending *pendingQueueAttachmentCleanup,
	newlyAdded *[]messagequeue.MessageAttachment,
	metadataUpdates map[string]interface{},
	applyUpdate func(context.Context) (int64, error),
) (int64, error) {
	controller, ok := h.queueEdit.(queueEditAttachmentController)
	if !ok {
		return h.updateMessageWithAttachmentAdmissionFallback(
			ctx, req, releaseClaims, supersededPending, applyUpdate,
		)
	}

	var claimedAttachments []messagequeue.MessageAttachment
	var claimAttempted bool
	var rollbackPending *pendingQueueAttachmentCleanup
	return controller.UpdateMessageWithLeaseAfterValidationAndFinalize(
		ctx, req.SessionID, req.EntryID, req.LeaseID, req.OperationID, connectionID,
		*req.ExpectedRevision, req.Content, req.Attachments, metadataUpdates,
		func(prepareCtx context.Context) error {
			claimedAttachments = append(claimedAttachments[:0], *newlyAdded...)
			*newlyAdded = nil
			rollbackReq := req
			rollbackReq.OperationID = attachmentCleanupOperationID(req.OperationID, "claim-rollback")
			var err error
			rollbackPending, err = h.preparePendingAttachmentCleanup(
				prepareCtx, rollbackReq, previous.TaskID, claimedAttachments, releaseClaims,
			)
			if err != nil {
				return err
			}
			claimAttempted = true
			if err := h.attachmentClaimer.ClaimMessageAttachments(prepareCtx, previous.TaskID, req.SessionID, queueAttachmentsToV1(claimedAttachments)); err != nil {
				return fmt.Errorf("%w: %v", errQueuedAttachmentUnavailable, err)
			}
			return nil
		},
		func(rollbackCtx context.Context) error {
			if claimAttempted && releaseClaims != nil && previous != nil {
				if len(claimedAttachments) == 0 {
					// Invoke the release callback after every attempted
					// claim, including a no-op claim, so rollback ordering
					// remains observable to attachment lifecycle owners.
					releaseErr := releaseClaims.ReleaseMessageAttachments(
						context.WithoutCancel(rollbackCtx), previous.TaskID, req.SessionID, nil,
					)
					h.settlePendingAttachmentCleanup(rollbackPending, releaseErr)
				} else {
					releaseErr := h.releaseQueuedAttachmentUpdateFailure(
						rollbackCtx, previous, req.SessionID, claimedAttachments, releaseClaims,
					)
					h.settlePendingAttachmentCleanup(rollbackPending, releaseErr)
				}
			}
			claimedAttachments = nil
			rollbackPending = nil
			return nil
		},
		func(finalizeCtx context.Context, _ *messagequeue.QueuedMessage) error {
			h.settlePendingAttachmentCleanup(rollbackPending, nil)
			rollbackPending = nil
			if releaseClaims == nil || supersededPending == nil {
				return nil
			}
			releaseErr := h.releaseSupersededQueueAttachmentsAdmitted(
				finalizeCtx, supersededPending.req, supersededPending.previous, releaseClaims,
			)
			h.settlePendingAttachmentCleanup(supersededPending, releaseErr)
			return nil
		},
	)
}

func (h *QueueHandlers) updateMessageWithAttachmentAdmissionFallback(
	ctx context.Context,
	req wsUpdateMessageRequest,
	releaseClaims QueueAttachmentReleaser,
	supersededPending *pendingQueueAttachmentCleanup,
	applyUpdate func(context.Context) (int64, error),
) (int64, error) {
	admission, ok := h.queueEdit.(queueEditAdmissionController)
	if !ok {
		return applyUpdate(ctx)
	}
	var revision int64
	err := admission.WithSessionAdmission(ctx, req.SessionID, func(admittedCtx context.Context) error {
		var err error
		revision, err = applyUpdate(admittedCtx)
		if err != nil {
			return err
		}
		if releaseClaims != nil && supersededPending != nil {
			cleanupErr := h.releaseSupersededQueueAttachmentsAdmitted(
				admittedCtx, supersededPending.req, supersededPending.previous, releaseClaims,
			)
			h.settlePendingAttachmentCleanup(supersededPending, cleanupErr)
		}
		return nil
	})
	return revision, err
}
func (h *QueueHandlers) queueUpdateFailure(
	_ context.Context,
	msg *ws.Message,
	req wsUpdateMessageRequest,
	updateErr error,
) (*ws.Message, error) {
	if errors.Is(updateErr, errQueuedAttachmentUnavailable) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Attachment is no longer available", nil)
	}
	if errors.Is(updateErr, messagequeue.ErrEditConflict) ||
		errors.Is(updateErr, messagequeue.ErrEditLeaseNotFound) ||
		errors.Is(updateErr, messagequeue.ErrEditRevisionConflict) {
		return h.queueEditLeaseError(msg, updateErr), nil
	}
	if errors.Is(updateErr, messagequeue.ErrEntryNotFound) {
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound, "Queue entry was already drained or not owned by caller", nil)
	}
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, updateErr.Error(), nil)
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

// rollbackQueuedAttachmentClaim removes the queue source whose attachment
// claim failed. removed is false only when another mutation already removed it.
func (h *QueueHandlers) rollbackQueuedAttachmentClaim(
	ctx context.Context,
	sessionID, entryID string,
) (bool, error) {
	rollbackCtx := context.WithoutCancel(ctx)
	if err := h.queueService.RemoveEntry(rollbackCtx, sessionID, entryID); err == nil {
		return true, nil
	} else if errors.Is(err, messagequeue.ErrEntryNotFound) {
		return false, nil
	} else {
		h.logger.Error("failed to remove queue entry after attachment claim failure",
			zap.String("entry_id", entryID), zap.Error(err))
	}
	taker, ok := h.queueService.(queueEntryTaker)
	if !ok {
		return false, errors.New("queue service cannot atomically remove a queued entry")
	}
	_, removed, err := taker.TakeQueuedEntry(rollbackCtx, sessionID, entryID)
	return removed, err
}

func (h *QueueHandlers) prepareEntryRemovalCleanup(
	ctx context.Context,
	entry *messagequeue.QueuedMessage,
	operationID string,
) (*pendingQueueAttachmentCleanup, error) {
	if entry == nil || len(entry.Attachments) == 0 || h.attachmentClaimer == nil {
		return nil, nil
	}
	releaser, ok := h.attachmentClaimer.(QueueAttachmentReleaser)
	if !ok {
		return nil, nil
	}
	return h.preparePendingAttachmentCleanup(ctx, wsUpdateMessageRequest{
		SessionID: entry.SessionID, EntryID: entry.ID, OperationID: operationID,
	}, entry.TaskID, entry.Attachments, releaser)
}

// releaseQueuedAttachments releases descriptors no longer referenced by the
// remaining queue entries. The queue entry has already been removed.
func (h *QueueHandlers) releaseQueuedAttachments(
	ctx context.Context,
	entry *messagequeue.QueuedMessage,
	pending *pendingQueueAttachmentCleanup,
) {
	if entry == nil || pending == nil {
		return
	}
	cleanupCtx := context.WithoutCancel(ctx)
	candidates, releaseErr := h.unreferencedQueueAttachments(
		cleanupCtx, entry.SessionID, entry.ID, entry.Attachments,
	)
	if len(candidates) > 0 {
		releaseErr = pending.releaser.ReleaseMessageAttachments(
			cleanupCtx, entry.TaskID, entry.SessionID, queueAttachmentsToV1(candidates),
		)
		if releaseErr != nil {
			h.logger.Warn("failed to release attachments after queue entry removal", zap.Error(releaseErr))
		}
	}
	h.settlePendingAttachmentCleanup(pending, releaseErr)
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
