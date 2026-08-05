package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

const (
	QueueSendNowScopeEntry = "entry"
	QueueSendNowScopeAll   = "all"
)

var (
	ErrSendNowConflict           = errors.New("send-now operation is already in progress")
	ErrSendNowQueueEmpty         = errors.New("send-now queue is empty")
	ErrSendNowEntryNotFound      = errors.New("send-now entry is no longer pending")
	ErrSendNowQueueChanged       = errors.New("send-now queue selection changed")
	ErrSendNowTurnChanged        = errors.New("send-now active turn changed")
	ErrSendNowAttachmentOverflow = messagequeue.ErrSendNowAttachmentOverflow
	ErrSendNowReferenceOverflow  = messagequeue.ErrSendNowReferenceOverflow
)

// SendQueuedNow claims either one exact entry or the click-time FIFO snapshot
// of all visible entries. A busy session is silently cancelled through the
// shared cancellation coordinator, then the exact claim is handed to one
// replacement prompt. The explicit Cancel path is deliberately not involved.
func (s *Service) SendQueuedNow(ctx context.Context, sessionID, scope, entryID string) (int, error) {
	if err := s.authorizeSession(ctx, sessionID); err != nil {
		return 0, err
	}
	if err := validateSendNowInput(sessionID, scope, entryID); err != nil {
		return 0, err
	}
	if s.messageQueue == nil {
		return 0, errors.New("message queue is not configured")
	}

	turnBefore, err := s.captureSendNowTurn(ctx, sessionID)
	if err != nil {
		return 0, err
	}

	lock, release := s.acquireCancelInFlightGuard(sessionID)
	lock.Lock()
	guardLocked := true
	unlockGuard := func() {
		if guardLocked {
			lock.Unlock()
			guardLocked = false
		}
	}
	relockGuard := func() {
		if !guardLocked {
			lock.Lock()
			guardLocked = true
		}
	}
	defer func() {
		unlockGuard()
		release()
	}()

	if s.currentCancellation(sessionID) != nil || s.isQueuedDispatchInFlight(sessionID) {
		return 0, ErrSendNowConflict
	}
	if err := s.verifySendNowTurn(ctx, sessionID, turnBefore); err != nil {
		return 0, err
	}

	taskID, sessionState, entryIDs, err := s.loadSendNowSelection(ctx, sessionID, scope, entryID)
	if err != nil {
		return 0, err
	}
	return s.dispatchSendNowSelection(ctx, sessionID, taskID, sessionState, scope, entryIDs, turnBefore, unlockGuard, relockGuard)
}

func validateSendNowInput(sessionID, scope, entryID string) error {
	switch {
	case sessionID == "":
		return errors.New("session_id is required")
	case scope != QueueSendNowScopeEntry && scope != QueueSendNowScopeAll:
		return fmt.Errorf("invalid send-now scope %q", scope)
	case scope == QueueSendNowScopeEntry && entryID == "":
		return errors.New("entry_id is required for entry scope")
	case scope == QueueSendNowScopeAll && entryID != "":
		return errors.New("entry_id is not allowed for all scope")
	default:
		return nil
	}
}

func (s *Service) captureSendNowTurn(ctx context.Context, sessionID string) (string, error) {
	turnID, err := s.peekActiveTurnID(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("%w: inspect active turn: %v", ErrSendNowTurnChanged, err)
	}
	return turnID, nil
}

func (s *Service) verifySendNowTurn(ctx context.Context, sessionID, expectedTurnID string) error {
	if s.turnService == nil {
		return nil
	}
	turnID, err := s.peekActiveTurnID(ctx, sessionID)
	if err != nil || turnID != expectedTurnID {
		return ErrSendNowTurnChanged
	}
	return nil
}

func (s *Service) loadSendNowSelection(
	ctx context.Context,
	sessionID, scope, entryID string,
) (string, models.TaskSessionState, []string, error) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return "", "", nil, fmt.Errorf("load session for send now: %w", err)
	}
	if session == nil {
		return "", "", nil, ErrSessionNotPromptable
	}
	entries, entryIDs, err := selectSendNowEntries(s.messageQueue.GetStatus(ctx, sessionID), scope, entryID)
	if err != nil {
		return "", "", nil, err
	}
	if _, err := messagequeue.BuildSendNowEnvelope(entries); err != nil {
		return "", "", nil, err
	}
	return session.TaskID, session.State, entryIDs, nil
}

func (s *Service) dispatchSendNowSelection(
	ctx context.Context,
	sessionID, taskID string,
	sessionState models.TaskSessionState,
	scope string,
	entryIDs []string,
	turnBefore string,
	unlockGuard, relockGuard func(),
) (int, error) {
	promptabilityErr := s.checkSessionPromptable(taskID, sessionID, sessionState)
	if promptabilityErr == nil {
		dispatched, err := s.claimAndDispatchSendNow(ctx, sessionID, scope, entryIDs)
		if err != nil {
			return 0, err
		}
		if !dispatched {
			return 0, ErrSendNowQueueChanged
		}
		return len(entryIDs), nil
	}
	if !errors.Is(promptabilityErr, ErrAgentPromptInProgress) {
		return 0, promptabilityErr
	}

	dispatched, err := s.cancelAgentSilentWithGuardActionKindExclusive(
		ctx,
		taskID,
		sessionID,
		unlockGuard,
		relockGuard,
		func(actionCtx context.Context) (bool, error) {
			return s.claimAndDispatchSendNow(actionCtx, sessionID, scope, entryIDs)
		},
		cancellationKindQueueSendNow,
		turnBefore,
	)
	if err != nil {
		return 0, err
	}
	if !dispatched {
		return 0, ErrSendNowQueueChanged
	}
	return len(entryIDs), nil
}

func selectSendNowEntries(status *messagequeue.QueueStatus, scope, entryID string) ([]messagequeue.QueuedMessage, []string, error) {
	if status == nil || len(status.Entries) == 0 {
		return nil, nil, ErrSendNowQueueEmpty
	}
	if scope == QueueSendNowScopeEntry {
		for _, entry := range status.Entries {
			if entry.ID == entryID {
				return []messagequeue.QueuedMessage{entry}, []string{entry.ID}, nil
			}
		}
		return nil, nil, ErrSendNowEntryNotFound
	}
	entries := append([]messagequeue.QueuedMessage(nil), status.Entries...)
	entryIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		entryIDs = append(entryIDs, entry.ID)
	}
	return entries, entryIDs, nil
}

func (s *Service) claimAndDispatchSendNow(ctx context.Context, sessionID, scope string, entryIDs []string) (bool, error) {
	claim, err := s.messageQueue.ClaimSendNow(ctx, sessionID, entryIDs)
	if err != nil {
		return false, mapSendNowClaimError(scope, err)
	}
	s.publishQueueStatusEvent(ctx, sessionID)
	s.markQueuedDispatchInFlight(sessionID, claim.Dispatch.ID)
	go s.executeSendNowClaim(claim)
	return true, nil
}

func mapSendNowClaimError(scope string, err error) error {
	switch {
	case errors.Is(err, messagequeue.ErrSendNowEmpty):
		return ErrSendNowQueueChanged
	case errors.Is(err, messagequeue.ErrSendNowReservationConflict):
		return ErrSendNowConflict
	case errors.Is(err, messagequeue.ErrSendNowClaimChanged):
		if scope == QueueSendNowScopeEntry {
			return ErrSendNowEntryNotFound
		}
		return ErrSendNowQueueChanged
	default:
		return err
	}
}

func (s *Service) executeSendNowClaim(claim *messagequeue.SendNowClaim) {
	if claim == nil {
		return
	}
	ctx := context.Background()
	sessionID := claim.Dispatch.SessionID
	defer s.clearQueuedDispatchInFlightIfCurrent(sessionID, claim.Dispatch.ID)

	restore := func() {
		if err := s.messageQueue.RestoreSendNowClaim(ctx, claim); err != nil {
			s.logger.Error("failed to restore send-now queue claim",
				zap.String("session_id", sessionID), zap.Error(err))
		}
		s.publishQueueStatusEvent(ctx, sessionID)
	}
	if s.isSessionResetInProgress(sessionID) {
		restore()
		return
	}

	attachments := make([]v1.MessageAttachment, len(claim.Dispatch.Attachments))
	for i, attachment := range claim.Dispatch.Attachments {
		attachments[i] = v1.MessageAttachment{
			Type:         attachment.Type,
			AttachmentID: attachment.AttachmentID,
			Data:         attachment.Data,
			MimeType:     attachment.MimeType,
			Name:         attachment.Name,
			SizeBytes:    attachment.SizeBytes,
			DeliveryMode: attachment.DeliveryMode,
		}
	}
	references := entityrefs.NormalizePersisted(claim.Dispatch.Metadata[messagequeue.MetadataEntityReferences])
	promptContent := AppendEntityReferenceContext(claim.Dispatch.Content, references)
	if err := s.recordQueuedUserMessage(ctx, &claim.Dispatch, attachments); err != nil {
		s.logger.Warn("failed to record send-now user message before prompt",
			zap.String("session_id", sessionID), zap.Error(err))
	}
	if session, err := s.repo.GetTaskSession(ctx, sessionID); err == nil && session != nil {
		s.processOnTurnStartViaEngine(ctx, claim.Dispatch.TaskID, session)
	}

	_, err := s.promptTask(ctx, claim.Dispatch.TaskID, sessionID, promptContent, claim.Dispatch.Model,
		claim.Dispatch.PlanMode, attachments, false, claim.Dispatch.ID, false, nil)
	if err != nil {
		s.logger.Warn("send-now replacement prompt failed; restoring queue claim",
			zap.String("session_id", sessionID), zap.Error(err))
		restore()
		return
	}
	if err := s.messageQueue.AcknowledgeSendNowClaim(ctx, claim); err != nil {
		s.logger.Error("failed to acknowledge accepted send-now queue claim",
			zap.String("session_id", sessionID), zap.Error(err))
		return
	}
	s.publishQueueStatusEvent(ctx, sessionID)
}
