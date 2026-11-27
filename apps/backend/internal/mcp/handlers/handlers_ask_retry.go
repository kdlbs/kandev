package handlers

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// clarificationDurableReader is an optional repository extension used to
// reconcile an exact retry of an interrupted ask_user_question call with the
// question messages already committed before its MCP wait was torn down.
type clarificationDurableReader interface {
	FindMessagesByPendingID(ctx context.Context, pendingID string) ([]*models.Message, error)
}

// clarificationReattacher is an optional repository extension that clears the
// detached marker from a still-pending bundle once a retry re-adopts it with a
// live waiter.
type clarificationReattacher interface {
	ReattachActiveClarificationBundle(ctx context.Context, sessionID, pendingID string) ([]*models.Message, bool, error)
}

type clarificationRetryRegistrar interface {
	CreateRetryRequestWithWaiter(req *clarification.Request) (string, bool, bool, func(context.Context, *clarification.Response) (*clarification.Response, error))
}

// clarificationBundlePublisher is an optional message-creator extension that
// exposes committed bundle rows to bus subscribers.
type clarificationBundlePublisher interface {
	PublishClarificationBundleUpdates(ctx context.Context, messages []*models.Message) error
}

// durableClarificationState is what an exact retry finds already recorded for
// its identity. exists means the visible question messages are present and
// must not be created again; response carries an answered/rejected outcome;
// closed names a cancelled/expired outcome that has no answer to return.
type durableClarificationState struct {
	exists          bool
	response        *clarification.Response
	closed          clarification.Status
	deliveryPending bool
}

// reconcileDurableClarification loads whatever the durable identity already
// recorded. A repository without the optional reader, or an empty identity,
// yields the zero state so the in-memory path behaves as before. A bundle
// owned by another session is never reused: the identity already binds the
// session, so a mismatch is treated as absent and logged.
func (h *Handlers) reconcileDurableClarification(ctx context.Context, sessionID, pendingID string) (durableClarificationState, error) {
	var state durableClarificationState
	if pendingID == "" {
		return state, nil
	}
	reader, ok := h.sessionRepo.(clarificationDurableReader)
	if !ok {
		return state, nil
	}
	messages, err := reader.FindMessagesByPendingID(ctx, pendingID)
	if err != nil {
		return state, err
	}
	if len(messages) == 0 {
		return state, nil
	}
	for _, message := range messages {
		if message.TaskSessionID != sessionID {
			h.logger.Warn("ignoring clarification bundle owned by another session on retry",
				zap.String("pending_id", pendingID),
				zap.String("session_id", sessionID),
				zap.String("owner_session_id", message.TaskSessionID))
			return state, nil
		}
	}
	state.exists = true
	for _, message := range messages {
		if metadataFlag(message.Metadata, "response_delivery_pending") {
			state.deliveryPending = true
			break
		}
	}
	status, response, recorded := clarification.RecordedOutcome(pendingID, messages, h.logger)
	if !recorded {
		return state, nil
	}
	if response != nil {
		state.response = response
	} else {
		state.closed = status
	}
	return state, nil
}

func metadataFlag(metadata map[string]interface{}, key string) bool {
	switch value := metadata[key].(type) {
	case bool:
		return value
	case string:
		return value == "true" || value == "1"
	case float64:
		return value == 1
	default:
		return false
	}
}

// reattachDurableClarification runs after a retry re-adopted a still-pending
// bundle and a live waiter exists for it again. It clears agent_disconnected
// from the bundle's current-turn rows and publishes the change so the
// projection and the orchestrator's live-clarification guard see a live
// waiter. The returned active flag is authoritative: superseded bundles and
// terminal sessions must never leave a retry parked on an unreachable waiter.
func (h *Handlers) reattachDurableClarification(ctx context.Context, sessionID, pendingID string) (bool, error) {
	reattacher, ok := h.sessionRepo.(clarificationReattacher)
	if !ok {
		return true, nil
	}
	messages, active, err := reattacher.ReattachActiveClarificationBundle(ctx, sessionID, pendingID)
	if err != nil {
		return false, err
	}
	if len(messages) == 0 {
		return active, nil
	}
	publisher, ok := h.messageCreator.(clarificationBundlePublisher)
	if !ok {
		return active, nil
	}
	if err := publisher.PublishClarificationBundleUpdates(ctx, messages); err != nil {
		h.logger.Warn("failed to publish re-adopted clarification bundle",
			zap.String("pending_id", pendingID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
	return active, nil
}

// reconcileClarificationAfterLostAdoption resolves the only race left after a
// retry registers its waiter: an answer claim can win the durable bundle lock
// before reattachment. A delivery-pending winner will find the registered
// waiter, while a finalized winner can be replayed. No winner means the bundle
// became inactive and must not be waited on.
func (h *Handlers) reconcileClarificationAfterLostAdoption(
	ctx context.Context,
	sessionID, pendingID string,
) (response *clarification.Response, wait bool, err error) {
	latest, err := h.reconcileDurableClarification(ctx, sessionID, pendingID)
	if err != nil {
		return nil, false, err
	}
	if latest.response == nil {
		return nil, false, nil
	}
	if latest.deliveryPending {
		return nil, true, nil
	}
	return latest.response, false, nil
}

// adoptDurableClarificationRetry checks whether a durable pending bundle is
// still authoritative and converts any race winner into an immediate result.
// A nil result with wait=true means the caller owns the registered waiter.
func (h *Handlers) adoptDurableClarificationRetry(
	ctx context.Context,
	msg *ws.Message,
	sessionID, taskID string,
	retry clarificationRetryRegistration,
) (result *ws.Message, resultErr error, wait bool) {
	pendingID, isNew := retry.pendingID, retry.isNew
	active, err := h.reattachDurableClarification(ctx, sessionID, pendingID)
	if err != nil {
		if isNew {
			h.clarificationSvc.CancelRequest(pendingID)
		}
		h.logger.Error("failed to adopt durable clarification retry",
			zap.String("pending_id", pendingID), zap.Error(err))
		result, resultErr = ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"failed to reconcile clarification retry", nil)
		return result, resultErr, false
	}
	if active {
		return nil, nil, true
	}

	recorded, deliveryPending, err := h.reconcileClarificationAfterLostAdoption(ctx, sessionID, pendingID)
	if err != nil {
		if isNew {
			h.clarificationSvc.CancelRequest(pendingID)
		}
		result, resultErr = ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"failed to reconcile clarification retry", nil)
		return result, resultErr, false
	}
	if recorded != nil {
		result, resultErr = h.replayClarificationRetry(ctx, msg, sessionID, taskID, retry, recorded)
		return result, resultErr, false
	}
	if deliveryPending {
		return nil, nil, true
	}

	h.clarificationSvc.CancelRequest(pendingID)
	result, resultErr = ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
		"Clarification request is no longer active", nil)
	return result, resultErr, false
}
