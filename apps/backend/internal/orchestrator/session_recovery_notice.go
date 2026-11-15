package orchestrator

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

const sessionRecoveryNoticeContent = "The session was interrupted. Your conversation is restored. You can continue."

type recoveryNoticePendingMarkerStore interface {
	ClearSessionMetadataKeyIfStringValue(context.Context, string, string, string) (bool, error)
}

type recoveryNoticeSubmissionReader interface {
	GetAgentDeliverySubmission(context.Context, string) (*models.AgentDeliverySubmission, error)
}

type recoveryNoticeDispositionVerifier interface {
	VerifyCommittedRecoveryNoticeSource(context.Context, models.AgentDeliveryRecovery) (bool, error)
}

type lifecycleRecoveryNoticeMessageCreator interface {
	CreateSessionLifecycleMessageIdempotent(
		context.Context, string, string, string, string, string, map[string]interface{},
	) error
}

func sessionRecoveryNoticeMessageIDFromIdentity(identity string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("session-restored-after-interruption:"+identity)).String()
}

func (s *Service) publishPendingRecoveryNotice(ctx context.Context, sessionID string) error {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return nil
	}
	pending, ok := session.Metadata[models.SessionMetaKeyRecoveryNoticePending].(string)
	if !ok || pending == "" {
		return nil
	}
	if s.messageCreator == nil {
		return nil
	}
	return s.publishRecoveryNoticeForPending(ctx, session, pending)
}

func (s *Service) publishRecoveryNoticeForPending(
	ctx context.Context,
	session *models.TaskSession,
	pending string,
) error {
	source, valid := models.ParseRecoveryNoticeSourceIdentity(pending)
	if !valid || !recoveryNoticeSourceMatchesSession(source, session) ||
		source.HarnessGeneration <= 0 || source.SubmissionID == "" {
		return errors.New("recovery notice source identity is incomplete")
	}
	if err := s.validateRecoveryNoticeSubmission(ctx, session, source); err != nil {
		return err
	}
	if err := s.createRecoveryNoticeMessage(ctx, session, source, pending); err != nil {
		return err
	}
	return s.clearRecoveryNoticePendingMarker(ctx, session.ID, pending)
}

func recoveryNoticeSourceMatchesSession(source models.AgentDeliveryRecovery, session *models.TaskSession) bool {
	if session == nil {
		return false
	}
	incarnationID := session.QueueIncarnationID
	if incarnationID == "" {
		incarnationID = session.ID
	}
	return source.SessionID == session.ID && source.IncarnationID == incarnationID
}

func (s *Service) warnPendingRecoveryNoticeFailure(sessionID string, err error) {
	if err != nil {
		s.logger.Warn("could not publish pending session recovery notice", zap.String("session_id", sessionID), zap.Error(err))
	}
}

func (s *Service) validateRecoveryNoticeSubmission(
	ctx context.Context,
	session *models.TaskSession,
	source models.AgentDeliveryRecovery,
) error {
	submissions, ok := s.repo.(recoveryNoticeSubmissionReader)
	if !ok {
		return errors.New("recovery notice submission reader is unavailable")
	}
	submission, err := submissions.GetAgentDeliverySubmission(ctx, source.SubmissionID)
	if err != nil {
		return err
	}
	if !canonicalRecoverySubmissionMatches(session, source, submission, source.HarnessGeneration) {
		return errors.New("recovery notice source submission changed")
	}
	disposition, ok := s.repo.(recoveryNoticeDispositionVerifier)
	if !ok {
		return errors.New("recovery notice disposition verifier is unavailable")
	}
	committed, err := disposition.VerifyCommittedRecoveryNoticeSource(ctx, source)
	if err != nil {
		return err
	}
	if !committed {
		return errors.New("recovery notice source disposition changed")
	}
	return nil
}

func (s *Service) createRecoveryNoticeMessage(
	ctx context.Context,
	session *models.TaskSession,
	source models.AgentDeliveryRecovery,
	pending string,
) error {
	messageID := sessionRecoveryNoticeMessageIDFromIdentity(pending)
	metadata := map[string]interface{}{
		"kind":                      "session_restored_after_interruption",
		"source_session_id":         session.ID,
		"source_incarnation_id":     source.IncarnationID,
		"source_harness_generation": source.HarnessGeneration,
		"source_submission_id":      source.SubmissionID,
	}
	var err error
	if creator, ok := s.messageCreator.(lifecycleRecoveryNoticeMessageCreator); ok {
		err = creator.CreateSessionLifecycleMessageIdempotent(
			ctx, messageID, session.TaskID, sessionRecoveryNoticeContent, session.ID,
			string(v1.MessageTypeStatus), metadata,
		)
	} else {
		err = s.messageCreator.CreateSessionMessageIdempotent(
			ctx, messageID, session.TaskID, sessionRecoveryNoticeContent, session.ID,
			string(v1.MessageTypeStatus), "", metadata, false,
		)
	}
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) clearRecoveryNoticePendingMarker(ctx context.Context, sessionID, pending string) error {
	markerStore, ok := s.repo.(recoveryNoticePendingMarkerStore)
	if !ok {
		return errors.New("recovery notice pending marker store is unavailable")
	}
	cleared, err := markerStore.ClearSessionMetadataKeyIfStringValue(
		ctx, sessionID, models.SessionMetaKeyRecoveryNoticePending, pending,
	)
	if err != nil {
		return err
	}
	if !cleared {
		return errors.New("recovery notice pending marker changed before publication completed")
	}
	return nil
}

func (s *Service) publishPendingRecoveryNoticeForCandidate(
	ctx context.Context,
	candidate models.SilentRestoreCandidate,
) error {
	owner, eligible, err := s.loadSilentRestoreOwner(ctx, candidate)
	if err != nil || !eligible {
		return err
	}
	return s.publishPendingRecoveryNotice(owner.ctx, candidate.SessionID)
}
