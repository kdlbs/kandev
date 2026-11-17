package orchestrator

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

const fixedProfileWaitMetadataKey = "fixed_profile_resource_wait"

func (s *Service) markFixedProfileWait(ctx context.Context, session *models.TaskSession) {
	if s.messageQueue == nil {
		return
	}
	identity, err := s.messageQueue.ResolveSessionIdentity(ctx, session.TaskID, session.ID)
	if err != nil {
		s.logger.Warn("could not retain fixed-profile admission wait", zap.String("session_id", session.ID), zap.Error(err))
		return
	}
	wait := map[string]interface{}{
		"incarnation_id": identity.SessionIncarnationID,
		"since":          time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := s.repo.SetSessionMetadataKey(ctx, session.ID, fixedProfileWaitMetadataKey, wait); err != nil {
		s.logger.Warn("could not persist fixed-profile admission wait", zap.String("session_id", session.ID), zap.Error(err))
	}
}

func (s *Service) queueDrainBlockedByFixedProfile(ctx context.Context, sessionID string) bool {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || s.isDynamicPromptSession(session) {
		return false
	}
	suspension, known := s.fixedProfileSuspensionForSession(ctx, session)
	if !known || !suspension.Blocked() {
		return false
	}
	s.surfaceFixedProfileSuspension(ctx, sessionID, suspension)
	return true
}

func (s *Service) retainQueuedMessageForFixedProfile(
	ctx context.Context,
	callerSessionID string,
	queuedMsg *messagequeue.QueuedMessage,
	reservation *queuedDispatchReservation,
	userMessageRecorded bool,
	err error,
) bool {
	if queuedMsg == nil || !isFixedProfileSuspendedError(err) {
		return false
	}
	ctx = context.WithoutCancel(ctx)
	if userMessageRecorded {
		markQueuedUserMessageRecorded(queuedMsg)
	}
	switch {
	case queuedMsg.IsReservedDelivery():
		s.releaseFixedProfileQueuedDelivery(ctx, callerSessionID, queuedMsg, reservation)
	case reservation != nil && reservation.identity.SessionIncarnationID != "":
		s.restoreQueuedMessageForSession(ctx, reservation.identity, queuedMsg)
	default:
		s.restoreQueuedMessage(ctx, queuedMsg)
	}
	return true
}

func (s *Service) releaseFixedProfileQueuedDelivery(ctx context.Context, callerSessionID string, entry *messagequeue.QueuedMessage, reservation *queuedDispatchReservation) {
	if entry.IsDeliveryAttempted() || reservation == nil || reservation.identity.SessionIncarnationID == "" {
		return
	}
	if err := s.messageQueue.ReleaseQueuedDeliveryForSession(ctx, reservation.identity, entry); err != nil {
		if errors.Is(err, messagequeue.ErrSessionIdentityMismatch) {
			s.discardLifecycleReservationForReplacedSession(ctx, entry, reservation.identity)
			return
		}
		s.logger.Warn("could not release unattempted delivery after circuit refusal",
			zap.String("session_id", callerSessionID), zap.String("queue_id", entry.ID), zap.Error(err))
		return
	}
	s.publishQueueStatusEventForIdentity(ctx, reservation.identity)
}

// The existing admission sweep rechecks only recorded waits. No per-refusal
// timer or retry worker is created, and queue auto-run policy is unchanged.
func (s *Service) resumeFixedProfileWaits(ctx context.Context) {
	if s.messageQueue == nil || s.profileExecutionResolver == nil {
		return
	}
	reader, ok := s.repo.(interface {
		ListFixedProfileWaitSessions(context.Context) ([]*models.TaskSession, error)
	})
	if !ok {
		return
	}
	sessions, err := reader.ListFixedProfileWaitSessions(ctx)
	if err != nil {
		return
	}
	for _, session := range sessions {
		wait, ok := session.Metadata[fixedProfileWaitMetadataKey].(map[string]interface{})
		if !ok || wait["incarnation_id"] != session.QueueIncarnationID {
			wait, ok = session.Metadata[fixedProfileAutomationWaitKey].(map[string]interface{})
		}
		if !ok || wait["incarnation_id"] != session.QueueIncarnationID {
			continue
		}
		suspension, known := s.fixedProfileSuspensionForSession(ctx, session)
		if known && suspension.Blocked() {
			continue
		}
		if owned, done := s.resumeFixedProfileAutomation(ctx, session); owned {
			if done {
				s.clearFixedProfileWait(ctx, session.ID, wait)
			}
			continue
		}
		if !s.clearFixedProfileQueueWait(ctx, session) {
			continue
		}
		s.CheckQueueAdmissionReadiness(ctx, messagequeue.QueueSessionIdentity{
			TaskID: session.TaskID, SessionID: session.ID, SessionIncarnationID: session.QueueIncarnationID,
		})
	}
}

// clearFixedProfileQueueWait clears only the observed wait and its error. A
// newer session revision retains its state, metadata, and diagnostic.
func (s *Service) clearFixedProfileQueueWait(ctx context.Context, session *models.TaskSession) bool {
	updater, ok := s.repo.(interface {
		UpdateTaskSessionIfCurrentSnapshot(context.Context, *models.TaskSession, models.TaskSessionState, time.Time, map[string]interface{}) (bool, error)
	})
	if !ok {
		return false
	}
	updated := *session
	updated.Metadata = cloneSessionMetadata(session.Metadata)
	delete(updated.Metadata, fixedProfileWaitMetadataKey)
	if strings.HasPrefix(updated.ErrorMessage, "resource suspended: scope=") {
		updated.ErrorMessage = ""
	}
	changed, err := updater.UpdateTaskSessionIfCurrentSnapshot(ctx, &updated, session.State, session.UpdatedAt, updated.Metadata)
	if err != nil {
		s.logger.Warn("could not clear fixed-profile admission wait", zap.String("session_id", session.ID), zap.Error(err))
		return false
	}
	if changed {
		s.publishTaskSessionStateChanged(ctx, session.TaskID, session.ID, session.State, updated.State, updated.ErrorMessage, &updated.UpdatedAt, &updated)
	}
	return changed
}

func (s *Service) clearFixedProfileWait(ctx context.Context, sessionID string, wait interface{}) {
	s.clearFixedProfileMetadataWait(ctx, sessionID, fixedProfileWaitMetadataKey, wait)
}

func (s *Service) clearFixedProfileMetadataWait(ctx context.Context, sessionID, key string, wait interface{}) {
	remover, ok := s.repo.(interface {
		RemoveSessionMetadataKeyIfJSONValue(context.Context, string, string, interface{}) (bool, error)
	})
	if ok {
		_, _ = remover.RemoveSessionMetadataKeyIfJSONValue(ctx, sessionID, key, wait)
	}
}
