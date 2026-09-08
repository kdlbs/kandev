package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

const (
	QueueSendNowScopeEntry = "entry"
	QueueSendNowScopeAll   = "all"

	sendNowClaimRecoveryTimeout = 10 * time.Second
)

var (
	ErrSendNowConflict           = errors.New("send-now operation is already in progress")
	ErrSendNowQueueEmpty         = errors.New("send-now queue is empty")
	ErrSendNowEntryNotFound      = errors.New("send-now entry is no longer pending")
	ErrSendNowQueueChanged       = errors.New("send-now queue selection changed")
	ErrSendNowTurnChanged        = errors.New("send-now active turn changed")
	ErrSendNowAttachmentOverflow = messagequeue.ErrSendNowAttachmentOverflow
	ErrSendNowReferenceOverflow  = messagequeue.ErrSendNowReferenceOverflow
	errSendNowDispatchNotTracked = errors.New("send-now dispatch is no longer tracked")
)

// SendQueuedNow claims either one exact entry or the click-time FIFO snapshot
// of all visible entries. A busy session is silently cancelled through the
// shared cancellation coordinator, then the exact claim is handed to one
// replacement prompt. The explicit Cancel path is deliberately not involved.
func (s *Service) SendQueuedNow(ctx context.Context, sessionID, scope, entryID string) (int, error) {
	return s.sendQueuedNow(ctx, nil, sessionID, scope, entryID)
}

// SendQueuedNowForSession dispatches the selection only for the exact session incarnation.
func (s *Service) SendQueuedNowForSession(ctx context.Context, identity messagequeue.QueueSessionIdentity, scope, entryID string) (int, error) {
	return s.sendQueuedNow(ctx, &identity, identity.SessionID, scope, entryID)
}

func (s *Service) sendQueuedNow(ctx context.Context, identity *messagequeue.QueueSessionIdentity, sessionID, scope, entryID string) (int, error) {
	// Dispatching a queued prompt puts an agent to work: session.prompt.
	if err := s.authorizeSessionPrompt(ctx, sessionID); err != nil {
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
	return s.sendQueuedNowAfterCapture(ctx, identity, sessionID, scope, entryID, turnBefore)
}

type sendNowGuard struct {
	lock    *sync.Mutex
	release func()
	locked  bool
}

func newSendNowGuard(s *Service, sessionID string) *sendNowGuard {
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	lock.Lock()
	return &sendNowGuard{lock: lock, release: release, locked: true}
}

func (guard *sendNowGuard) unlock() {
	if !guard.locked {
		return
	}
	guard.lock.Unlock()
	guard.locked = false
}

func (guard *sendNowGuard) relock() {
	if guard.locked {
		return
	}
	guard.lock.Lock()
	guard.locked = true
}

func (guard *sendNowGuard) close() {
	guard.unlock()
	guard.release()
}

func (s *Service) sendQueuedNowAfterCapture(
	ctx context.Context,
	identity *messagequeue.QueueSessionIdentity,
	sessionID, scope, entryID, turnBefore string,
) (int, error) {
	guard := newSendNowGuard(s, sessionID)
	defer guard.close()

	if s.currentCancellation(sessionID) != nil || s.isQueuedDispatchAccepted(sessionID) {
		return 0, ErrSendNowConflict
	}
	if err := s.restorePendingQueuedDispatchForSendNow(ctx, sessionID); err != nil {
		return 0, err
	}
	if err := s.verifySendNowTurn(ctx, sessionID, turnBefore); err != nil {
		return 0, err
	}

	taskID, sessionState, entries, err := s.loadSendNowSelection(ctx, identity, sessionID, scope, entryID)
	if err != nil {
		return 0, err
	}
	return s.dispatchSendNowSelection(
		ctx,
		identity,
		sessionID,
		taskID,
		sessionState,
		scope,
		entries,
		turnBefore,
		guard.unlock,
		guard.relock,
	)
}

func (s *Service) restorePendingQueuedDispatchForSendNow(ctx context.Context, sessionID string) error {
	// A live Send Now successor is intentionally replaceable after the
	// cancellation coordinator has settled it. Only a pending FIFO handoff
	// needs to be restored here; an accepted FIFO handoff is rejected by the
	// caller and a live successor must pass through cancellation below.
	if s.pendingQueuedDispatch(sessionID) == nil {
		return nil
	}
	reservation, err := s.pendingQueuedDispatchForSendNow(sessionID)
	if err != nil {
		return err
	}
	if reservation == nil || reservation.source == nil {
		return ErrSendNowConflict
	}
	restore, err := s.sendNowRestoreClaimForReservation(ctx, reservation)
	if err != nil {
		return err
	}
	if err := s.messageQueue.RestoreSendNowClaim(ctx, restore); err != nil {
		s.logger.Warn("failed to restore FIFO reservation for send now",
			zap.String("session_id", sessionID),
			zap.String("queue_id", reservation.source.ID),
			zap.Error(err),
		)
		s.publishQueueStatusEventForIdentity(ctx, reservation.identity)
		return ErrSendNowQueueChanged
	}
	return s.supersedeQueuedDispatchForSendNow(sessionID, reservation)
}

func (s *Service) sendNowRestoreClaimForReservation(
	ctx context.Context,
	reservation *queuedDispatchReservation,
) (*messagequeue.SendNowClaim, error) {
	restore := &messagequeue.SendNowClaim{
		Identity:          reservation.identity,
		Sources:           []messagequeue.QueuedMessage{*reservation.source},
		SourceGenerations: make(map[string]int64),
	}
	if reservation.source.TaskID == "" {
		return restore, nil
	}
	generation, err := s.messageQueue.LifecycleGeneration(ctx, reservation.source.TaskID)
	if err != nil {
		return nil, ErrSendNowQueueChanged
	}
	restore.SourceGenerations[reservation.source.TaskID] = generation
	return restore, nil
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
	identity *messagequeue.QueueSessionIdentity,
	sessionID, scope, entryID string,
) (string, models.TaskSessionState, []messagequeue.QueuedMessage, error) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return "", "", nil, fmt.Errorf("load session for send now: %w", err)
	}
	if session == nil {
		return "", "", nil, ErrSessionNotPromptable
	}
	var status *messagequeue.QueueStatus
	if identity != nil {
		status, err = s.messageQueue.Snapshot(ctx, *identity)
		if err != nil {
			return "", "", nil, err
		}
	} else {
		status = s.messageQueue.GetStatus(ctx, sessionID)
	}
	entries, _, err := selectSendNowEntries(status, scope, entryID)
	if err != nil {
		return "", "", nil, err
	}
	if err := messagequeue.ValidateSendNowEntries(entries); err != nil {
		return "", "", nil, err
	}
	return session.TaskID, session.State, entries, nil
}

func (s *Service) dispatchSendNowSelection(
	ctx context.Context,
	identity *messagequeue.QueueSessionIdentity,
	sessionID, taskID string,
	sessionState models.TaskSessionState,
	scope string,
	entries []messagequeue.QueuedMessage,
	turnBefore string,
	unlockGuard, relockGuard func(),
) (int, error) {
	promptabilityErr := s.checkSessionPromptable(taskID, sessionID, sessionState)
	if promptabilityErr == nil {
		dispatched, err := s.claimAndDispatchSendNow(ctx, identity, sessionID, scope, entries)
		if err != nil {
			return 0, err
		}
		if !dispatched {
			return 0, ErrSendNowQueueChanged
		}
		return len(entries), nil
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
			return s.claimAndDispatchSendNow(actionCtx, identity, sessionID, scope, entries)
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
	return len(entries), nil
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

func (s *Service) claimAndDispatchSendNow(ctx context.Context, identity *messagequeue.QueueSessionIdentity, sessionID, scope string, entries []messagequeue.QueuedMessage) (bool, error) {
	var claim *messagequeue.SendNowClaim
	var err error
	if identity != nil {
		claim, err = s.messageQueue.ClaimSendNowForSession(ctx, *identity, entries)
	} else {
		claim, err = s.messageQueue.ClaimSendNow(ctx, sessionID, entries)
	}
	if err != nil {
		return false, mapSendNowClaimError(scope, err)
	}
	s.publishQueueStatusEventForIdentity(ctx, claim.Identity)
	var reservation *queuedDispatchReservation
	if claim.Identity.SessionIncarnationID != "" {
		reservation = s.markQueuedDispatchInFlightWithIdentityLocked(claim.Identity, claim.Dispatch.ID, nil)
	} else {
		reservation = s.markQueuedDispatchInFlightWithSourceLocked(sessionID, claim.Dispatch.ID, nil)
	}
	if reservation != nil {
		reservation.liveEligible.Store(true)
	}
	if !s.launchSendNowClaim(claim, reservation) {
		if restoreErr := s.restoreSendNowClaimWithRetry(context.Background(), claim); restoreErr != nil {
			s.logger.Error("failed to restore send-now claim after service shutdown",
				zap.String("session_id", sessionID), zap.Error(restoreErr))
		}
		s.clearQueuedDispatchInFlightIfCurrent(sessionID, reservation)
		s.publishQueueStatusEventForIdentity(context.Background(), claim.Identity)
		return false, nil
	}
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

func (s *Service) launchSendNowClaim(
	claim *messagequeue.SendNowClaim,
	reservation *queuedDispatchReservation,
) bool {
	if claim == nil {
		return false
	}
	s.sendNowMu.Lock()
	if s.sendNowStopped {
		s.sendNowMu.Unlock()
		return false
	}
	if s.sendNowCtx == nil {
		s.sendNowCtx, s.sendNowCancel = context.WithCancel(context.Background())
	}
	workerCtx := s.sendNowCtx
	s.sendNowWorkers.Add(1)
	s.sendNowMu.Unlock()
	go func() {
		defer s.sendNowWorkers.Done()
		s.executeSendNowClaimWithContext(workerCtx, claim, reservation)
	}()
	return true
}

func (s *Service) resetSendNowWorkers() {
	s.sendNowMu.Lock()
	defer s.sendNowMu.Unlock()
	s.sendNowStopped = false
	s.sendNowCtx, s.sendNowCancel = context.WithCancel(context.Background())
}

func (s *Service) stopSendNowWorkers() {
	s.sendNowMu.Lock()
	s.sendNowStopped = true
	cancel := s.sendNowCancel
	s.sendNowMu.Unlock()
	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		s.sendNowWorkers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(sendNowClaimRecoveryTimeout):
		s.logger.Warn("timed out waiting for send-now workers during shutdown")
	}
}

func (s *Service) executeSendNowClaim(claim *messagequeue.SendNowClaim) {
	s.executeSendNowClaimWithContext(context.Background(), claim, nil)
}

func (s *Service) executeSendNowClaimWithContext(
	ctx context.Context,
	claim *messagequeue.SendNowClaim,
	reservation *queuedDispatchReservation,
) {
	if claim == nil {
		return
	}
	sessionID := claim.Dispatch.SessionID
	if reservation == nil {
		reservation = s.queuedDispatchReservationForEntry(sessionID, claim.Dispatch.ID)
	}
	defer func() {
		s.clearQueuedDispatchInFlightIfCurrent(sessionID, reservation)
		s.drainQueuedDispatchIfPending(sessionID)
	}()

	restore := func() {
		if err := s.restoreSendNowClaimWithRetry(ctx, claim); err != nil {
			s.logger.Error("failed to restore send-now queue claim",
				zap.String("session_id", sessionID), zap.Error(err))
		}
		s.publishQueueStatusEventForIdentity(ctx, claim.Identity)
	}
	if s.isSessionResetInProgress(sessionID) {
		restore()
		return
	}
	if claimErr := s.claimSendNowExecution(sessionID, claim.Dispatch.ID); claimErr != nil {
		s.logger.Warn("send-now dispatch lost prompt ownership; restoring queue claim",
			zap.String("session_id", sessionID), zap.Error(claimErr))
		restore()
		return
	}
	if claim.Identity.SessionIncarnationID != "" {
		current, err := s.messageQueue.ResolveSessionIdentity(
			ctx,
			claim.Identity.TaskID,
			claim.Identity.SessionID,
		)
		if err != nil || current != claim.Identity {
			s.logger.Info("discarding send-now dispatch for replaced session",
				zap.String("session_id", sessionID),
				zap.String("task_id", claim.Identity.TaskID))
			restore()
			return
		}
	}
	if err := s.promptSendNowClaim(ctx, claim); err != nil {
		s.logger.Warn("send-now replacement prompt failed; restoring queue claim",
			zap.String("session_id", sessionID), zap.Error(err))
		restore()
		return
	}
	if err := s.acknowledgeSendNowClaimWithRetry(ctx, claim); err != nil {
		s.logger.Error("failed to acknowledge accepted send-now queue claim",
			zap.String("session_id", sessionID), zap.Error(err))
		s.publishQueueStatusEventForIdentity(context.Background(), claim.Identity)
		return
	}
	s.publishQueueStatusEventForIdentity(ctx, claim.Identity)
}

func (s *Service) claimSendNowExecution(sessionID, dispatchID string) error {
	tracked, err := s.claimQueuedDispatchForExecution(sessionID, dispatchID, nil)
	if err != nil {
		return err
	}
	if !tracked {
		return errSendNowDispatchNotTracked
	}
	return nil
}

func (s *Service) promptSendNowClaim(ctx context.Context, claim *messagequeue.SendNowClaim) error {
	sessionID := claim.Dispatch.SessionID
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
	promptContent = appendStepHandoffToPrompt(promptContent, stepHandoffFromQueuedMetadata(claim.Dispatch.Metadata))

	_, err := s.promptTask(ctx, claim.Dispatch.TaskID, sessionID, promptContent, claim.Dispatch.Model,
		claim.Dispatch.PlanMode, attachments, false, promptTaskOptions{
			claimEntryID: claim.Dispatch.ID,
			afterClaim: func() error {
				if !s.queuedDispatchIdentityIsCurrent(ctx, claim.Identity) {
					return errLifecyclePromptReservationSuperseded
				}
				if err := s.recordQueuedUserMessage(ctx, &claim.Dispatch, attachments); err != nil {
					s.logger.Warn("failed to record send-now user message after prompt claim",
						zap.String("session_id", sessionID), zap.Error(err))
				} else if s.messageCreator != nil {
					for i := range claim.Sources {
						markQueuedUserMessageRecorded(&claim.Sources[i])
					}
				}
				if session, loadErr := s.repo.GetTaskSession(ctx, sessionID); loadErr == nil &&
					s.queuedSessionMatchesIdentity(session, claim.Identity) &&
					!turnStartAlreadyProcessed(claim.Dispatch.Metadata) {
					s.processOnTurnStartViaEngine(ctx, claim.Dispatch.TaskID, session)
				}
				return nil
			},
		})
	return err
}

func (s *Service) restoreSendNowClaimWithRetry(
	ctx context.Context,
	claim *messagequeue.SendNowClaim,
) error {
	return s.retrySendNowClaimMutation(ctx, func(recoveryCtx context.Context) error {
		return s.messageQueue.RestoreSendNowClaim(recoveryCtx, claim)
	})
}

func (s *Service) acknowledgeSendNowClaimWithRetry(
	ctx context.Context,
	claim *messagequeue.SendNowClaim,
) error {
	return s.retrySendNowClaimMutation(ctx, func(recoveryCtx context.Context) error {
		return s.messageQueue.AcknowledgeSendNowClaim(recoveryCtx, claim)
	})
}

func (s *Service) retrySendNowClaimMutation(
	ctx context.Context,
	mutate func(context.Context) error,
) error {
	recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sendNowClaimRecoveryTimeout)
	defer cancel()
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if err = mutate(recoveryCtx); err == nil {
			return nil
		}
		if attempt == 2 {
			break
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-recoveryCtx.Done():
			timer.Stop()
			return recoveryCtx.Err()
		case <-timer.C:
		}
	}
	return err
}
