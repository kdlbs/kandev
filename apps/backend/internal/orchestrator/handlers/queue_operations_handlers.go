package handlers

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type wsCancelAllRequest struct {
	SessionID            string `json:"session_id"`
	TaskID               string `json:"task_id"`
	SessionIncarnationID string `json:"session_incarnation_id"`
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
	if h.requiresQueueIdentity() && (req.TaskID == "" || req.SessionIncarnationID == "") {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id, session_id, and session_incarnation_id are required", nil)
	}
	if denied := h.authorizeQueueIdentity(ctx, msg, req.TaskID, req.SessionID, req.SessionIncarnationID); denied != nil {
		return denied, nil
	}

	identity := messagequeue.QueueSessionIdentity{
		TaskID: req.TaskID, SessionID: req.SessionID, SessionIncarnationID: req.SessionIncarnationID,
	}
	if !h.requiresQueueIdentity() {
		removed, err := h.cancelAllLegacy(ctx, req.SessionID)
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		h.publishStatus(ctx, req.SessionID)
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			fieldSessionID: req.SessionID,
			"removed":      removed,
		})
	}
	var removal *messagequeue.QueueRemovalResult
	var removed int
	var err error
	if h.requiresQueueIdentity() {
		removal, err = h.queueService.(QueueIdentityMutationService).CancelAllForSession(ctx, identity)
		if removal != nil {
			removed = len(removal.Removed)
		}
	} else {
		before := h.queueService.GetStatus(ctx, req.SessionID)
		removed, err = h.queueService.CancelAll(ctx, req.SessionID)
		if err == nil {
			removal = &messagequeue.QueueRemovalResult{
				Removed:  append([]messagequeue.QueuedMessage(nil), before.Entries...),
				Retained: append([]messagequeue.QueuedMessage(nil), h.queueService.GetStatus(ctx, req.SessionID).Entries...),
			}
		}
	}
	if err != nil {
		if isQueueIdentityError(err) {
			return queueAccessDeniedResponse(msg), nil
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}

	h.releaseQueueRemovalAttachments(ctx, removal)
	h.publishStatusForIdentity(ctx, identity)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		fieldSessionID: req.SessionID,
		"removed":      removed,
	})
}

type wsDrainQueueRequest struct {
	SessionID            string `json:"session_id"`
	TaskID               string `json:"task_id"`
	SessionIncarnationID string `json:"session_incarnation_id"`
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
	if h.requiresQueueIdentity() && (req.TaskID == "" || req.SessionIncarnationID == "") {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id, session_id, and session_incarnation_id are required", nil)
	}
	if denied := h.authorizeQueueIdentity(ctx, msg, req.TaskID, req.SessionID, req.SessionIncarnationID); denied != nil {
		return denied, nil
	}
	if h.queueDrainer == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Queue drain is unavailable", nil)
	}

	identity := messagequeue.QueueSessionIdentity{
		TaskID: req.TaskID, SessionID: req.SessionID, SessionIncarnationID: req.SessionIncarnationID,
	}
	var drained bool
	var err error
	if h.requiresQueueIdentity() {
		drained, err = h.queueDrainer.(QueueIdentityDrainer).DrainQueuedMessageForSession(ctx, identity)
	} else {
		drained, err = h.queueDrainer.DrainQueuedMessage(ctx, req.SessionID)
	}
	if err != nil {
		switch {
		case errors.Is(err, orchestrator.ErrAgentPromptInProgress):
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeSessionBusy, "Session is busy", nil)
		case errors.Is(err, orchestrator.ErrSessionNotPromptable):
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeNotPromptable, "Session is not ready for input", nil)
		case isQueueIdentityError(err):
			return queueAccessDeniedResponse(msg), nil
		default:
			h.logger.Error("failed to drain queued message", zap.String(fieldSessionID, req.SessionID), zap.Error(err))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to drain queued message", nil)
		}
	}

	h.publishStatusForIdentity(ctx, identity)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		fieldSessionID: req.SessionID,
		"drained":      drained,
	})
}

type wsSetAutoRunRequest struct {
	SessionID            string `json:"session_id"`
	TaskID               string `json:"task_id"`
	SessionIncarnationID string `json:"session_incarnation_id"`
	Enabled              *bool  `json:"enabled"`
}

// wsSetAutoRun persists automatic queue processing and optionally starts the
// promptable FIFO head when enabling it.
func (h *QueueHandlers) wsSetAutoRun(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsSetAutoRunRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if h.requiresQueueIdentity() && (req.TaskID == "" || req.SessionIncarnationID == "") {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id, session_id, and session_incarnation_id are required", nil)
	}
	if req.Enabled == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "enabled is required", nil)
	}
	if denied := h.authorizeQueueIdentity(ctx, msg, req.TaskID, req.SessionID, req.SessionIncarnationID); denied != nil {
		return denied, nil
	}
	if h.queueAutoRun == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Queue Auto-run is unavailable", nil)
	}
	identity := messagequeue.QueueSessionIdentity{
		TaskID: req.TaskID, SessionID: req.SessionID, SessionIncarnationID: req.SessionIncarnationID,
	}
	var autoRun, dispatched bool
	var err error
	if h.requiresQueueIdentity() {
		autoRun, dispatched, err = h.queueAutoRun.(QueueIdentityAutoRunController).SetQueueAutoRunForSession(ctx, identity, *req.Enabled)
	} else {
		autoRun, dispatched, err = h.queueAutoRun.SetQueueAutoRun(ctx, req.SessionID, *req.Enabled)
	}
	if err != nil {
		if isQueueIdentityError(err) {
			return queueAccessDeniedResponse(msg), nil
		}
		h.logger.Error("failed to set queue Auto-run", zap.String(fieldSessionID, req.SessionID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to set queue Auto-run", nil)
	}

	h.publishStatusForIdentity(ctx, identity)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		fieldSessionID: req.SessionID,
		fieldAutoRun:   autoRun,
		"dispatched":   dispatched,
	})
}

type wsSetAutoMergeRequest struct {
	TaskID               string `json:"task_id"`
	SessionID            string `json:"session_id"`
	SessionIncarnationID string `json:"session_incarnation_id"`
	Enabled              *bool  `json:"enabled"`
}

func (h *QueueHandlers) wsSetAutoMerge(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsSetAutoMergeRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" || req.SessionID == "" || req.SessionIncarnationID == "" || req.Enabled == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id, session_id, session_incarnation_id, and enabled are required", nil)
	}
	if denied := h.authorizeQueueIdentity(ctx, msg, req.TaskID, req.SessionID, req.SessionIncarnationID); denied != nil {
		return denied, nil
	}
	controller, ok := h.queueService.(QueueAutoMergeController)
	if !ok {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Queue Auto-merge is unavailable", nil)
	}
	identity := messagequeue.QueueSessionIdentity{
		TaskID: req.TaskID, SessionID: req.SessionID, SessionIncarnationID: req.SessionIncarnationID,
	}
	policy, err := controller.SetSessionAutoMerge(ctx, identity, *req.Enabled)
	if err != nil {
		if errors.Is(err, messagequeue.ErrSessionIdentityMismatch) ||
			errors.Is(err, messagequeue.ErrTaskInactive) {
			return queueAccessDeniedResponse(msg), nil
		}
		h.logger.Error("failed to set queue Auto-merge", zap.String(fieldSessionID, req.SessionID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to set queue Auto-merge", nil)
	}
	h.publishStatusForIdentity(ctx, identity)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		fieldTaskID:             identity.TaskID,
		fieldSessionID:          identity.SessionID,
		fieldSessionIncarnation: identity.SessionIncarnationID,
		fieldAutoMergeEnabled:   policy.Enabled,
		"auto_merge_source":     policy.Source,
		"auto_merge_revision":   policy.Revision,
	})
}

type wsSendNowRequest struct {
	SessionID            string `json:"session_id"`
	TaskID               string `json:"task_id"`
	SessionIncarnationID string `json:"session_incarnation_id"`
	Scope                string `json:"scope"`
	EntryID              string `json:"entry_id,omitempty"`
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
	if h.requiresQueueIdentity() && (req.TaskID == "" || req.SessionIncarnationID == "") {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id, session_id, and session_incarnation_id are required", nil)
	}
	if denied := h.authorizeQueueIdentity(ctx, msg, req.TaskID, req.SessionID, req.SessionIncarnationID); denied != nil {
		return denied, nil
	}
	if validation := validateSendNowRequest(req); validation != "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, validation, nil)
	}
	if h.queueDispatcher == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Queue send-now is unavailable", nil)
	}

	identity := messagequeue.QueueSessionIdentity{
		TaskID: req.TaskID, SessionID: req.SessionID, SessionIncarnationID: req.SessionIncarnationID,
	}
	var sentCount int
	var err error
	if h.requiresQueueIdentity() {
		sentCount, err = h.queueDispatcher.(QueueIdentitySendNowDispatcher).SendQueuedNowForSession(ctx, identity, req.Scope, req.EntryID)
	} else {
		sentCount, err = h.queueDispatcher.SendQueuedNow(ctx, req.SessionID, req.Scope, req.EntryID)
	}
	if err != nil {
		return h.sendNowErrorResponse(msg, req.SessionID, err)
	}

	h.publishStatusForIdentity(ctx, identity)
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
	case errors.Is(err, orchestrator.ErrSendNowEditConflict):
		return ws.NewError(msg.ID, msg.Action, "edit_conflict", "Queue entry is being edited by another view", nil)
	case errors.Is(err, orchestrator.ErrSendNowConflict):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeSendNowConflict, "Another cancellation or Send Now operation is in progress", nil)
	case errors.Is(err, orchestrator.ErrSendNowTurnChanged):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeSendNowTurnChanged, "The active turn changed before Send Now could start", nil)
	case errors.Is(err, messagequeue.ErrSendNowAttachmentOverflow):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeSendNowAttachmentOverflow, "Combined attachments exceed the message limits", nil)
	case errors.Is(err, messagequeue.ErrSendNowReferenceOverflow):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeSendNowReferenceOverflow, "Combined entity references exceed the message limit", nil)
	case isQueueIdentityError(err):
		return queueAccessDeniedResponse(msg), nil
	case errors.Is(err, orchestrator.ErrSessionNotPromptable):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeNotPromptable, "Session is not ready for input", nil)
	default:
		h.logger.Error("failed to send queued message now", zap.String(fieldSessionID, sessionID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to send queued message now", nil)
	}
}

type wsGetQueueStatusRequest struct {
	TaskID               string `json:"task_id"`
	SessionID            string `json:"session_id"`
	SessionIncarnationID string `json:"session_incarnation_id"`
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
	if h.requiresQueueIdentity() && (req.TaskID == "" || req.SessionIncarnationID == "") {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id, session_id, and session_incarnation_id are required", nil)
	}
	if denied := h.authorizeQueueIdentity(ctx, msg, req.TaskID, req.SessionID, req.SessionIncarnationID); denied != nil {
		return denied, nil
	}
	if req.TaskID == "" || req.SessionIncarnationID == "" {
		return ws.NewResponse(msg.ID, msg.Action, h.queueService.GetStatus(ctx, req.SessionID))
	}
	snapshots, ok := h.queueService.(QueueSnapshotService)
	if !ok {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Queue status is unavailable", nil)
	}
	status, err := snapshots.Snapshot(ctx, messagequeue.QueueSessionIdentity{
		TaskID: req.TaskID, SessionID: req.SessionID, SessionIncarnationID: req.SessionIncarnationID,
	})
	if err != nil {
		if isQueueIdentityError(err) {
			return queueAccessDeniedResponse(msg), nil
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to read queue status", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, status)
}

type wsRemoveEntryRequest struct {
	SessionID            string `json:"session_id"`
	TaskID               string `json:"task_id"`
	SessionIncarnationID string `json:"session_incarnation_id"`
	EntryID              string `json:"entry_id"`
}

// wsRemoveEntry handles ActionMessageQueueRemove, deleting a single queued entry.
func (h *QueueHandlers) wsRemoveEntry(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsRemoveEntryRequest
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
	if !h.requiresQueueIdentity() {
		if err := h.removeEntryLegacy(ctx, req.SessionID, req.EntryID); err != nil {
			if errors.Is(err, messagequeue.ErrEntryNotFound) {
				return ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound, "Queue entry is no longer pending", nil)
			}
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		}
		h.publishStatus(ctx, req.SessionID)
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{fieldEntryID: req.EntryID})
	}

	identity := messagequeue.QueueSessionIdentity{
		TaskID: req.TaskID, SessionID: req.SessionID, SessionIncarnationID: req.SessionIncarnationID,
	}
	var removal *messagequeue.QueueRemovalResult
	var err error
	if h.requiresQueueIdentity() {
		removal, err = h.queueService.(QueueIdentityMutationService).RemoveEntryForSession(ctx, identity, req.EntryID)
	} else {
		previous, getErr := h.queueService.GetEntry(ctx, req.SessionID, req.EntryID)
		err = h.queueService.RemoveEntry(ctx, req.SessionID, req.EntryID)
		if err == nil && getErr == nil {
			removal = &messagequeue.QueueRemovalResult{
				Removed:  []messagequeue.QueuedMessage{*previous},
				Retained: append([]messagequeue.QueuedMessage(nil), h.queueService.GetStatus(ctx, req.SessionID).Entries...),
			}
		}
	}
	if err != nil {
		if errors.Is(err, messagequeue.ErrEntryNotFound) {
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound, "Queue entry is no longer pending", nil)
		}
		if isQueueIdentityError(err) {
			return queueAccessDeniedResponse(msg), nil
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}

	h.releaseQueueRemovalAttachments(ctx, removal)
	h.publishStatusForIdentity(ctx, identity)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{fieldEntryID: req.EntryID})
}

// cancelAllLegacy preserves the attachment cleanup contract for the
// pre-incarnation queue API. Identity-bound callers use the atomic removal
// result returned by CancelAllForSession above.
//
//nolint:cyclop,gocognit // Legacy cancellation preserves attachment cleanup across two queue service contracts.
func (h *QueueHandlers) cancelAllLegacy(ctx context.Context, sessionID string) (int, error) {
	var removed int
	var removedEntries []messagequeue.QueuedMessage
	var countRemovedEntries bool
	cancel := func(cancelCtx context.Context) error {
		prepared := make(map[string]*pendingQueueAttachmentCleanup)
		status := h.queueService.GetStatus(cancelCtx, sessionID)
		if status != nil {
			for i := range status.Entries {
				entry := &status.Entries[i]
				if entry.IsReservedInFlight() {
					continue
				}
				pending, prepareErr := h.prepareEntryRemovalCleanup(cancelCtx, entry, "cancel")
				if prepareErr != nil {
					for _, earlier := range prepared {
						h.settlePendingAttachmentCleanup(earlier, nil)
					}
					return prepareErr
				}
				if pending != nil {
					prepared[entry.ID] = pending
				}
			}
		}

		var err error
		if batchCanceller, ok := h.queueService.(queueBatchCanceller); ok {
			removedEntries, err = batchCanceller.CancelAllWithEntries(cancelCtx, sessionID)
			countRemovedEntries = true
		} else {
			removed, err = h.queueService.CancelAll(cancelCtx, sessionID)
			if err == nil && status != nil {
				removedEntries = status.Entries
			}
		}
		if err != nil {
			for _, pending := range prepared {
				h.settlePendingAttachmentCleanup(pending, nil)
			}
			return err
		}
		for i := range removedEntries {
			if removedEntries[i].IsReservedInFlight() {
				continue
			}
			if countRemovedEntries {
				removed++
			}
			h.releaseQueuedAttachments(cancelCtx, &removedEntries[i], prepared[removedEntries[i].ID])
		}
		return nil
	}
	if admission, ok := h.queueService.(queueEditAdmissionController); ok {
		if err := admission.WithSessionAdmission(ctx, sessionID, cancel); err != nil {
			return 0, err
		}
		return removed, nil
	}
	if err := cancel(ctx); err != nil {
		return 0, err
	}
	return removed, nil
}

// removeEntryLegacy preserves durable attachment cleanup for the
// pre-incarnation queue API. The cleanup obligation is prepared before the
// row is removed and settled only after the removal commits.
func (h *QueueHandlers) removeEntryLegacy(ctx context.Context, sessionID, entryID string) error {
	var err error
	remove := func(removeCtx context.Context) error {
		entry, getErr := h.queueService.GetEntry(removeCtx, sessionID, entryID)
		if getErr != nil {
			return getErr
		}
		pending, prepareErr := h.prepareEntryRemovalCleanup(removeCtx, entry, "remove")
		if prepareErr != nil {
			return prepareErr
		}
		if remover, ok := h.queueService.(queueEntryRemover); ok {
			entry, err = remover.RemoveEntryWithEntry(removeCtx, sessionID, entryID)
		} else {
			err = h.queueService.RemoveEntry(removeCtx, sessionID, entryID)
		}
		if err != nil {
			h.settlePendingAttachmentCleanup(pending, nil)
			return err
		}
		h.releaseQueuedAttachments(removeCtx, entry, pending)
		return nil
	}
	if admission, ok := h.queueService.(queueEditAdmissionController); ok {
		err = admission.WithSessionAdmission(ctx, sessionID, remove)
	} else {
		err = remove(ctx)
	}
	return err
}

func (h *QueueHandlers) releaseQueueRemovalAttachments(ctx context.Context, removal *messagequeue.QueueRemovalResult) {
	releaser, ok := h.attachmentClaimer.(QueueAttachmentReleaser)
	if !ok || removal == nil || len(removal.Removed) == 0 {
		return
	}
	retained := make(map[string]struct{})
	for _, entry := range removal.Retained {
		for _, attachment := range entry.Attachments {
			if attachment.AttachmentID != "" {
				retained[attachment.AttachmentID] = struct{}{}
			}
		}
	}
	taskID := removal.Removed[0].TaskID
	sessionID := removal.Removed[0].SessionID
	seen := make(map[string]struct{})
	released := make([]messagequeue.MessageAttachment, 0)
	for _, entry := range removal.Removed {
		for _, attachment := range entry.Attachments {
			if attachment.AttachmentID == "" {
				continue
			}
			if _, ok := retained[attachment.AttachmentID]; ok {
				continue
			}
			if _, ok := seen[attachment.AttachmentID]; ok {
				continue
			}
			seen[attachment.AttachmentID] = struct{}{}
			released = append(released, attachment)
		}
	}
	if len(released) == 0 {
		return
	}
	if err := releaser.ReleaseMessageAttachments(ctx, taskID, sessionID, queueAttachmentsToV1(released)); err != nil {
		h.logger.Warn("failed to release removed queue attachments", zap.Error(err))
	}
}

// wsMergeIntoAboveRequest is the payload for ActionMessageQueueMerge: the
// session whose queue is modified and the id of the entry to fold into the
// entry directly above it. user_id is forwarded for ownership checks and is
// optional (the server defaults to the reserved "user" identity).
type wsMergeIntoAboveRequest struct {
	SessionID            string `json:"session_id"`
	TaskID               string `json:"task_id"`
	SessionIncarnationID string `json:"session_incarnation_id"`
	EntryID              string `json:"entry_id"`
	UserID               string `json:"user_id,omitempty"`
}

func (h *QueueHandlers) mergeIntoAbove(
	ctx context.Context,
	identity messagequeue.QueueSessionIdentity,
	entryID, queuedBy string,
) (*messagequeue.QueuedMessage, error) {
	if h.requiresQueueIdentity() {
		return h.queueService.(QueueIdentityMutationService).
			MergeIntoAboveForSession(ctx, identity, entryID, queuedBy)
	}
	return h.queueService.MergeIntoAbove(ctx, identity.SessionID, entryID, queuedBy)
}

func (h *QueueHandlers) mergeIntoAboveError(msg *ws.Message, err error) (*ws.Message, error) {
	switch {
	case errors.Is(err, messagequeue.ErrEntryNotFound):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeEntryNotFound, "Queue entry was already drained or not owned by caller", nil)
	case errors.Is(err, messagequeue.ErrNoMergeTarget):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "No mergeable message above this entry", nil)
	case errors.Is(err, messagequeue.ErrMergeReferenceOverflow):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeMergeReferenceOverflow, err.Error(), nil)
	case errors.Is(err, messagequeue.ErrMergeDisabled):
		return ws.NewError(msg.ID, msg.Action, queueErrorCodeMergeDisabled, "Message merging is disabled", nil)
	case isQueueIdentityError(err):
		return queueAccessDeniedResponse(msg), nil
	default:
		h.logger.Error("failed to merge queued message", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to merge queued message", nil)
	}
}

// wsMergeIntoAbove handles ActionMessageQueueMerge, folding the referenced
// queued entry into the entry above it and broadcasting the updated queue.
func (h *QueueHandlers) wsMergeIntoAbove(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsMergeIntoAboveRequest
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
	if messagequeue.IsReservedQueuedBy(req.UserID) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, reservedIdentityError(req.UserID), nil)
	}
	// Default empty user_id to QueuedByUser so the merge ownership guard runs
	// against a non-empty owner, mirroring wsUpdateMessage.
	queuedBy := req.UserID
	if queuedBy == "" {
		queuedBy = messagequeue.QueuedByUser
	}

	identity := messagequeue.QueueSessionIdentity{
		TaskID: req.TaskID, SessionID: req.SessionID, SessionIncarnationID: req.SessionIncarnationID,
	}
	merged, err := h.mergeIntoAbove(ctx, identity, req.EntryID, queuedBy)
	if err != nil {
		return h.mergeIntoAboveError(msg, err)
	}

	h.publishStatusForIdentity(ctx, identity)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{fieldEntryID: merged.ID})
}

// wsReorderRequest is the payload for ActionMessageQueueReorder: the session
// whose queue is modified and the complete ordered list of visible pending
// entry ids the caller wants as the new FIFO order.
type wsReorderRequest struct {
	SessionID            string   `json:"session_id"`
	TaskID               string   `json:"task_id"`
	SessionIncarnationID string   `json:"session_incarnation_id"`
	OrderedIDs           []string `json:"ordered_ids"`
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
	if h.requiresQueueIdentity() && (req.TaskID == "" || req.SessionIncarnationID == "") {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id, session_id, and session_incarnation_id are required", nil)
	}
	if denied := h.authorizeQueueIdentity(ctx, msg, req.TaskID, req.SessionID, req.SessionIncarnationID); denied != nil {
		return denied, nil
	}
	if len(req.OrderedIDs) == 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "ordered_ids is required", nil)
	}
	if hasDuplicateIDs(req.OrderedIDs) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "ordered_ids must not contain duplicates", nil)
	}

	identity := messagequeue.QueueSessionIdentity{
		TaskID: req.TaskID, SessionID: req.SessionID, SessionIncarnationID: req.SessionIncarnationID,
	}
	var err error
	if h.requiresQueueIdentity() {
		err = h.queueService.(QueueIdentityMutationService).ReorderEntriesForSession(ctx, identity, req.OrderedIDs)
	} else {
		err = h.queueService.ReorderEntries(ctx, req.SessionID, req.OrderedIDs)
	}
	if err != nil {
		if errors.Is(err, messagequeue.ErrQueueChanged) {
			return ws.NewError(msg.ID, msg.Action, queueErrorCodeQueueChanged, "Queue changed before the reorder could be applied", nil)
		}
		if errors.Is(err, messagequeue.ErrEditConflict) {
			return h.queueEditLeaseError(msg, err), nil
		}
		if isQueueIdentityError(err) {
			return queueAccessDeniedResponse(msg), nil
		}
		h.logger.Error("failed to reorder queued messages", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to reorder queued messages", nil)
	}

	h.publishStatusForIdentity(ctx, identity)
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		fieldSessionID: req.SessionID,
		"reordered":    len(req.OrderedIDs),
	})
}

// publishStatus emits the latest QueueStatus on the event bus so the frontend
// updates its store after every mutation.
func (h *QueueHandlers) publishStatus(ctx context.Context, sessionID string, admitted ...*messagequeue.QueuedMessage) {
	if h.eventBus == nil {
		return
	}
	// A committed queue mutation still needs an authoritative snapshot when
	// the initiating request has already been cancelled.
	ctx = context.WithoutCancel(ctx)
	if admission, ok := h.queueService.(queueEditAdmissionController); ok {
		_ = admission.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
			h.publishStatusSnapshot(admittedCtx, sessionID, admitted...)
			return nil
		})
		return
	}
	h.publishStatusSnapshot(ctx, sessionID, admitted...)
}

func (h *QueueHandlers) publishStatusSnapshot(ctx context.Context, sessionID string, admitted ...*messagequeue.QueuedMessage) {
	status := h.queueService.GetStatus(ctx, sessionID)
	eventData := map[string]interface{}{
		fieldSessionID:  sessionID,
		"entries":       status.Entries,
		"count":         status.Count,
		fieldMax:        status.Max,
		fieldAutoRun:    status.AutoRun,
		"merge_enabled": status.MergeEnabled,
	}
	if len(admitted) > 0 && admitted[0] != nil && admitted[0].QueuedBy != "" && !messagequeue.IsReservedQueuedBy(admitted[0].QueuedBy) {
		eventData["queued_by"] = admitted[0].QueuedBy
		eventData["queued_at"] = admitted[0].QueuedAt
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
func (h *QueueHandlers) publishStatusForIdentity(
	ctx context.Context,
	identity messagequeue.QueueSessionIdentity,
	admitted ...*messagequeue.QueuedMessage,
) {
	if identity.TaskID == "" || identity.SessionIncarnationID == "" {
		h.publishStatus(ctx, identity.SessionID, admitted...)
		return
	}
	snapshots, ok := h.queueService.(QueueSnapshotService)
	if !ok {
		h.publishStatus(ctx, identity.SessionID, admitted...)
		return
	}
	status, err := snapshots.Snapshot(ctx, identity)
	if err != nil {
		h.logger.Warn("read identity-bound queue status",
			zap.String(fieldSessionID, identity.SessionID),
			zap.Error(err))
		return
	}
	h.publishIdentityStatus(ctx, status, admitted...)
}
func (h *QueueHandlers) publishIdentityStatus(
	ctx context.Context,
	status *messagequeue.QueueStatus,
	admitted ...*messagequeue.QueuedMessage,
) {
	if h.eventBus == nil || status == nil {
		return
	}
	eventData := map[string]interface{}{
		fieldTaskID:             status.TaskID,
		fieldSessionID:          status.SessionID,
		fieldSessionIncarnation: status.SessionIncarnationID,
		"status_epoch":          status.StatusEpoch,
		"status_generation":     status.StatusGeneration,
		"entries":               status.Entries,
		"count":                 status.Count,
		fieldMax:                status.Max,
		fieldAutoRun:            status.AutoRun,
		"merge_enabled":         status.MergeEnabled,
		"auto_merge_available":  status.AutoMergeAvailable,
	}
	if status.AutoMergeEnabled != nil {
		eventData[fieldAutoMergeEnabled] = *status.AutoMergeEnabled
	}
	if status.AutoMergeSource != "" {
		eventData["auto_merge_source"] = status.AutoMergeSource
	}
	if status.AutoMergeRevision != nil {
		eventData["auto_merge_revision"] = *status.AutoMergeRevision
	}
	if len(admitted) > 0 && admitted[0] != nil &&
		admitted[0].QueuedBy != "" &&
		!messagequeue.IsReservedQueuedBy(admitted[0].QueuedBy) {
		eventData["queued_by"] = admitted[0].QueuedBy
		eventData["queued_at"] = admitted[0].QueuedAt
	}
	_ = h.eventBus.Publish(ctx, events.MessageQueueStatusChanged, bus.NewEvent(
		events.MessageQueueStatusChanged,
		"queue-handlers",
		eventData,
	))
}

func isQueueIdentityError(err error) bool {
	return errors.Is(err, messagequeue.ErrSessionIdentityMismatch) ||
		errors.Is(err, messagequeue.ErrTaskInactive)
}
