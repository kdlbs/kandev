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

	sendNowClaimRecoveryTimeout  = 10 * time.Second
	sendNowWorkerShutdownTimeout = 500 * time.Millisecond
)

var (
	ErrSendNowConflict           = errors.New("send-now operation is already in progress")
	ErrSendNowEditConflict       = errors.New("send-now entry is being edited")
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
		Dispatch:          messagequeue.QueuedMessage{SessionID: reservation.sessionID},
		SourceGenerations: make(map[string]int64),
	}
	sessionGeneration, lifecycleGeneration, captured := reservation.source.ReservationGenerations()
	if captured {
		restore.SessionGeneration = sessionGeneration
		if reservation.source.TaskID != "" {
			restore.SourceGenerations[reservation.source.TaskID] = lifecycleGeneration
		}
		return restore, nil
	}
	sessionGeneration, err := s.messageQueue.SessionGeneration(ctx, reservation.sessionID)
	if err != nil {
		return nil, ErrSendNowQueueChanged
	}
	restore.SessionGeneration = sessionGeneration
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
	case errors.Is(err, messagequeue.ErrEditConflict):
		return ErrSendNowEditConflict
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
	if s.sendNowWorkers == nil {
		s.sendNowWorkers = &sync.WaitGroup{}
	}
	workerCtx := s.sendNowCtx
	workers := s.sendNowWorkers
	workers.Add(1)
	s.sendNowMu.Unlock()
	go func() {
		defer workers.Done()
		s.executeSendNowClaimWithContext(workerCtx, claim, reservation)
	}()
	return true
}

func (s *Service) resetSendNowWorkers() error {
	s.sendNowMu.Lock()
	defer s.sendNowMu.Unlock()
	if s.sendNowDrain != nil {
		select {
		case <-s.sendNowDrain:
		default:
			return errors.New("previous Send Now workers are still recovering")
		}
	}
	s.sendNowStopped = false
	s.sendNowCtx, s.sendNowCancel = context.WithCancel(context.Background())
	s.sendNowWorkers = &sync.WaitGroup{}
	s.sendNowDrain = nil
	return nil
}

func (s *Service) stopSendNowWorkers() {
	s.sendNowMu.Lock()
	s.sendNowStopped = true
	cancel := s.sendNowCancel
	workers := s.sendNowWorkers
	done := s.sendNowDrain
	startWait := workers != nil && done == nil
	if startWait {
		newDone := make(chan struct{})
		done = newDone
		s.sendNowDrain = newDone
	}
	s.sendNowMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if workers == nil {
		return
	}
	if startWait {
		go func() {
			workers.Wait()
			close(done)
		}()
	}
	select {
	case <-done:
	case <-time.After(sendNowWorkerShutdownTimeout):
		s.logger.Warn("timed out waiting for Send Now workers; recovery remains owned by detached workers")
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
	deliveryAttempted, err := s.promptSendNowClaim(ctx, claim)
	if err != nil {
		var acceptedDispatch *acceptedPromptDispatchError
		if deliveryAttempted || errors.As(err, &acceptedDispatch) {
			s.logger.Warn("send-now replacement prompt was accepted but acceptance persistence failed; settling without restore",
				zap.String("session_id", sessionID), zap.Error(err))
			if markErr := s.markSendNowClaimAcceptedWithRetry(ctx, claim); markErr != nil {
				s.logger.Error("failed to persist accepted send-now queue claim",
					zap.String("session_id", sessionID), zap.Error(markErr))
			}
			if ackErr := s.acknowledgeSendNowClaimWithRetry(ctx, claim); ackErr != nil {
				s.logger.Error("failed to acknowledge accepted send-now queue claim",
					zap.String("session_id", sessionID), zap.Error(ackErr))
			}
			s.publishQueueStatusEvent(context.Background(), sessionID)
			return
		}
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

func (s *Service) promptSendNowClaim(ctx context.Context, claim *messagequeue.SendNowClaim) (bool, error) {
	sessionID := claim.Dispatch.SessionID
	deliveryAttempted := false
	attachments := queuedMessageAttachmentsToV1(claim.Dispatch.Attachments)
	references := entityrefs.NormalizePersisted(claim.Dispatch.Metadata[messagequeue.MetadataEntityReferences])
	promptContent := AppendEntityReferenceContext(claim.Dispatch.Content, references)
	promptContent = appendStepHandoffToPrompt(promptContent, stepHandoffFromQueuedMetadata(claim.Dispatch.Metadata))
	durablePlanComments := claim.HasDurablePlanComment()
	if err := s.prepareSendNowTranscript(ctx, claim, attachments, durablePlanComments); err != nil {
		return false, err
	}

	_, err := s.promptTask(ctx, claim.Dispatch.TaskID, sessionID, promptContent, claim.Dispatch.Model,
		claim.Dispatch.PlanMode, attachments, false, promptTaskOptions{
			claimEntryID:         claim.Dispatch.ID,
			afterClaim:           s.sendNowAfterClaim(ctx, claim, attachments, durablePlanComments),
			beforeDispatch:       s.sendNowDeliveryBoundary(ctx, claim, durablePlanComments, &deliveryAttempted),
			disableDispatchRetry: durablePlanComments,
			afterDispatch: func() error {
				return s.markSendNowClaimAcceptedWithRetry(ctx, claim)
			},
		})
	return deliveryAttempted, err
}

func (s *Service) prepareSendNowTranscript(
	ctx context.Context,
	claim *messagequeue.SendNowClaim,
	attachments []v1.MessageAttachment,
	durablePlanComments bool,
) error {
	if !durablePlanComments {
		return nil
	}
	if s.messageCreator == nil {
		return fmt.Errorf("%w: message creator is unavailable", errLifecyclePromptMessagePersistence)
	}
	sourceIDs := make([]string, 0, len(claim.Sources))
	for i := range claim.Sources {
		sourceIDs = append(sourceIDs, claim.Sources[i].ID)
	}
	if err := s.recordQueuedUserMessage(ctx, &claim.Dispatch, attachments, sourceIDs...); err != nil {
		return fmt.Errorf("%w: %v", errLifecyclePromptMessagePersistence, err)
	}
	markSendNowSourcesRecorded(claim)
	return nil
}

func markSendNowSourcesRecorded(claim *messagequeue.SendNowClaim) {
	for i := range claim.Sources {
		markQueuedUserMessageRecorded(&claim.Sources[i])
	}
}

func (s *Service) sendNowAfterClaim(
	ctx context.Context,
	claim *messagequeue.SendNowClaim,
	attachments []v1.MessageAttachment,
	durablePlanComments bool,
) func() error {
	return func() error {
		if !s.queuedDispatchIdentityIsCurrent(ctx, claim.Identity) {
			return errLifecyclePromptReservationSuperseded
		}
		if durablePlanComments {
			return nil
		}
		if err := s.recordQueuedUserMessage(ctx, &claim.Dispatch, attachments); err != nil {
			s.logger.Warn("failed to record send-now user message after prompt claim",
				zap.String("session_id", claim.Dispatch.SessionID), zap.Error(err))
		} else if s.messageCreator != nil {
			markSendNowSourcesRecorded(claim)
		}
		s.processSendNowTurnStart(ctx, claim)
		return nil
	}
}

func (s *Service) sendNowDeliveryBoundary(
	ctx context.Context,
	claim *messagequeue.SendNowClaim,
	durablePlanComments bool,
	deliveryAttempted *bool,
) func() error {
	return func() error {
		if !durablePlanComments {
			return nil
		}
		if err := s.messageQueue.MarkDeliveryAttemptedForSession(
			ctx, claim.Identity, planCommentSendNowReceipts(claim),
		); err != nil {
			return err
		}
		*deliveryAttempted = true
		for i := range claim.Sources {
			if claim.Sources[i].IsDurablePlanComment() {
				claim.Sources[i].Metadata[messagequeue.MetadataDeliveryAttempted] = true
			}
		}
		s.processSendNowTurnStart(ctx, claim)
		return nil
	}
}

func planCommentSendNowReceipts(claim *messagequeue.SendNowClaim) []messagequeue.QueuedMessage {
	receipts := make([]messagequeue.QueuedMessage, 0, len(claim.Sources))
	for i := range claim.Sources {
		if claim.Sources[i].IsDurablePlanComment() {
			receipts = append(receipts, claim.Sources[i])
		}
	}
	return receipts
}

func (s *Service) processSendNowTurnStart(ctx context.Context, claim *messagequeue.SendNowClaim) {
	session, err := s.repo.GetTaskSession(ctx, claim.Dispatch.SessionID)
	if err != nil || !s.queuedSessionMatchesIdentity(session, claim.Identity) ||
		turnStartAlreadyProcessed(claim.Dispatch.Metadata) {
		return
	}
	s.processOnTurnStartViaEngine(ctx, claim.Dispatch.TaskID, session)
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

func (s *Service) markSendNowClaimAcceptedWithRetry(
	ctx context.Context,
	claim *messagequeue.SendNowClaim,
) error {
	if claim == nil || s.messageQueue == nil || !s.messageQueue.PendingSendNowClaimPersistenceAvailable() {
		return nil
	}
	return s.retrySendNowClaimMutation(ctx, func(recoveryCtx context.Context) error {
		return s.messageQueue.MarkPendingSendNowClaimAccepted(recoveryCtx, claim)
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

func (s *Service) reconcilePendingSendNowClaimsOnStartup(ctx context.Context) error {
	if s.messageQueue == nil || !s.messageQueue.PendingSendNowClaimPersistenceAvailable() {
		return nil
	}
	claims, err := s.messageQueue.ListPendingSendNowClaims(ctx)
	if err != nil {
		return fmt.Errorf("list pending Send Now claims: %w", err)
	}
	for i := range claims {
		pending := &claims[i]
		claim := &pending.Claim
		var recoveryErr error
		if pending.Accepted {
			recoveryErr = s.acknowledgeSendNowClaimWithRetry(ctx, claim)
		} else {
			recoveryErr = s.restoreSendNowClaimWithRetry(ctx, claim)
		}
		if recoveryErr != nil {
			if !errors.Is(recoveryErr, messagequeue.ErrSendNowClaimChanged) {
				return fmt.Errorf("recover pending Send Now claim for session %s: %w", claim.Dispatch.SessionID, recoveryErr)
			}
			if deleteErr := s.messageQueue.DeletePendingSendNowClaim(
				context.WithoutCancel(ctx), claim,
			); deleteErr != nil && !errors.Is(deleteErr, messagequeue.ErrSendNowClaimChanged) {
				return fmt.Errorf("discard superseded Send Now claim for session %s: %w", claim.Dispatch.SessionID, deleteErr)
			}
		}
		s.publishQueueStatusEvent(ctx, claim.Dispatch.SessionID)
	}
	return nil
}
