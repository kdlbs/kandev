package orchestrator

import (
	"context"
	"errors"

	"go.uber.org/zap"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

// requeueUndispatchedDelivery returns a comment-bearing receipt whose dispatch
// failed before prompt transport to its FIFO position, instead of
// acknowledging a message the agent never received. It reports whether the
// receipt was handled; false leaves the caller's at-most-once handling.
func (s *Service) requeueUndispatchedDelivery(
	ctx context.Context,
	identity messagequeue.QueueSessionIdentity,
	queuedMsg *messagequeue.QueuedMessage,
	dispatchErr error,
) bool {
	if !queuedMsg.IsDurablePlanComment() || !errors.Is(dispatchErr, agentruntime.ErrPromptNotDispatched) ||
		s.messageQueue == nil {
		return false
	}
	if identity.SessionIncarnationID == "" {
		resolved, err := s.messageQueue.ResolveSessionIdentity(ctx, queuedMsg.TaskID, queuedMsg.SessionID)
		if err != nil {
			s.logUndispatchedRevokeFailure(queuedMsg, dispatchErr, err)
			return false
		}
		identity = resolved
	}
	if err := s.messageQueue.ClearDeliveryAttemptForSession(ctx, identity, queuedMsg); err != nil {
		s.logUndispatchedRevokeFailure(queuedMsg, dispatchErr, err)
		return false
	}
	delete(queuedMsg.Metadata, messagequeue.MetadataDeliveryAttempted)
	s.logger.Warn("comment-bearing queued message failed before dispatch; keeping it queued",
		zap.String("task_id", queuedMsg.TaskID),
		zap.String("session_id", queuedMsg.SessionID),
		zap.String("queue_id", queuedMsg.ID),
		zap.Error(dispatchErr))
	if err := s.messageQueue.ReleaseQueuedDeliveryForSession(ctx, identity, queuedMsg); err != nil {
		// The receipt is now an unattempted reservation; lease expiry and
		// startup reconciliation resume it.
		s.logPostClaimQueueMutationFailure("release undispatched comment receipt", identity, queuedMsg, err)
	}
	s.publishQueueStatusEventForIdentity(ctx, identity)
	return true
}

func (s *Service) logUndispatchedRevokeFailure(
	queuedMsg *messagequeue.QueuedMessage, dispatchErr, revokeErr error,
) {
	s.logger.Error("could not revoke delivery attempt for undispatched comment message; completing it at most once",
		zap.String("task_id", queuedMsg.TaskID),
		zap.String("session_id", queuedMsg.SessionID),
		zap.String("queue_id", queuedMsg.ID),
		zap.NamedError("dispatch_error", dispatchErr),
		zap.Error(revokeErr))
}

// logPossiblyDeliveredAcknowledgement records a comment-bearing entry that is
// completed without retry because its prompt may already have reached the agent.
func (s *Service) logPossiblyDeliveredAcknowledgement(queuedMsg *messagequeue.QueuedMessage, err error) {
	s.logger.Warn("comment-bearing queued message completed after a possibly delivered dispatch failure",
		zap.String("task_id", queuedMsg.TaskID),
		zap.String("session_id", queuedMsg.SessionID),
		zap.String("queue_id", queuedMsg.ID),
		zap.Error(err))
}
