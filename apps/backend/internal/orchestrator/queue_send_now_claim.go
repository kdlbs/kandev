package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

type sendNowCreatedLaunchDetails struct {
	prompt                      string
	skipMessageRecord           bool
	planMode                    bool
	attachments                 []v1.MessageAttachment
	references                  []v1.EntityReference
	promptReferenceContext      string
	skipTaskDescriptionFallback bool
	promptAlreadyComposed       bool
	retryPrompt                 string
	preserveDirectPrompt        bool
	recordQueueMessage          bool
}

// sendNowCreatedLaunchInput folds the pending Continue into the exact
// workflow launch that was waiting for capacity. The workflow prompt may
// already be recorded and composed, so it must not be discarded or composed a
// second time. The queue message is recorded separately when the original
// launch already owns its transcript row.
func sendNowCreatedLaunchInput(
	task *models.Task,
	deferral *models.CeilingDeferral,
	continuation string,
	queueAttachments []v1.MessageAttachment,
	queuePlanMode bool,
) sendNowCreatedLaunchDetails {
	input := sendNowCreatedLaunchDetails{
		prompt:      continuation,
		planMode:    queuePlanMode,
		attachments: append([]v1.MessageAttachment(nil), queueAttachments...),
		references:  nil,
	}
	if deferral == nil {
		return input
	}

	payload := deferral.Payload
	input.skipMessageRecord = boolField(payload, "skip_message_record")
	input.skipTaskDescriptionFallback = boolField(payload, "skip_task_description_fallback")
	input.promptAlreadyComposed = boolField(payload, "prompt_already_composed")
	input.preserveDirectPrompt = boolField(payload, "preserve_direct_prompt")
	input.planMode = input.planMode || boolField(payload, metaKeyPlanMode)
	input.promptReferenceContext = stringField(payload, "prompt_reference_context")

	originalPrompt := stringField(payload, metaKeyPrompt)
	input.retryPrompt = stringField(payload, "retry_prompt")
	if input.promptAlreadyComposed && originalPrompt == "" {
		originalPrompt = input.retryPrompt
	}
	if originalPrompt == "" && !input.skipTaskDescriptionFallback && task != nil {
		originalPrompt = task.Description
	}
	input.prompt = appendSendNowPrompt(originalPrompt, continuation)
	if input.retryPrompt == "" {
		input.retryPrompt = originalPrompt
	}
	input.retryPrompt = appendSendNowPrompt(input.retryPrompt, continuation)

	var originalAttachments []v1.MessageAttachment
	decodeCeilingPayloadField(payload[metaKeyAttachments], &originalAttachments)
	input.attachments = append(originalAttachments, input.attachments...)
	decodeCeilingPayloadField(payload["references"], &input.references)

	// A workflow auto-start records its own prompt before admission and sets
	// skip_message_record. Record the queued Continue before launching so a
	// failed launch can restore the queue without losing its transcript receipt.
	input.recordQueueMessage = input.skipMessageRecord &&
		(strings.TrimSpace(continuation) != "" || len(queueAttachments) > 0)
	return input
}

func appendSendNowPrompt(original, continuation string) string {
	switch {
	case strings.TrimSpace(original) == "":
		return continuation
	case strings.TrimSpace(continuation) == "":
		return original
	default:
		return original + "\n\n" + continuation
	}
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

// releaseUnrecoverableSendNowClaim closes a claim that can be neither delivered
// nor restored. It verifies the exact claim still owns its sources, marks the
// user message not delivered while the claim remains available for retry, then
// releases the claim and the source rows it still owns so FIFO cannot run work
// already reported as undelivered. The step is idempotent and retries transient
// failures, because a leftover claim blocks its entry through the ownership
// rule until the next reconciliation.
func (s *Service) releaseUnrecoverableSendNowClaim(
	ctx context.Context,
	claim *messagequeue.SendNowClaim,
) error {
	if claim == nil || s.messageQueue == nil {
		return nil
	}
	return s.retrySendNowClaimMutation(ctx, func(recoveryCtx context.Context) error {
		return s.releaseUnrecoverableSendNowClaimOnce(recoveryCtx, claim)
	})
}

func (s *Service) releaseUnrecoverableSendNowClaimOnce(
	ctx context.Context,
	claim *messagequeue.SendNowClaim,
) error {
	replaced, err := s.sendNowClaimIdentityReplaced(ctx, claim.Identity)
	if err != nil {
		return err
	}
	if replaced {
		// A replaced or deleted session keeps its existing handling: discard
		// the exact claim without marking a message that no longer belongs to
		// the current session identity.
		return s.discardSendNowClaim(ctx, claim)
	}
	canRelease, err := s.messageQueue.CanReleaseSendNowClaim(ctx, claim)
	if err != nil {
		return err
	}
	if !canRelease {
		// A successor owns a surviving source or the claim was already
		// acknowledged. Never mark the message not delivered in that case.
		s.publishQueueStatusEventForIdentity(ctx, claim.Identity)
		return nil
	}
	// Mark while the claim is still present, so a failed status write can be
	// retried. The claim continues to block its sources until release succeeds.
	if err := s.markSendNowClaimNotDelivered(ctx, claim); err != nil {
		return err
	}
	if _, err := s.messageQueue.ReleaseSendNowClaim(ctx, claim); err != nil {
		return err
	}
	s.publishQueueStatusEventForIdentity(ctx, claim.Identity)
	return nil
}

// sendNowClaimIdentityReplaced distinguishes a confirmed replaced session from
// a transient identity lookup error, so release retries the latter.
func (s *Service) sendNowClaimIdentityReplaced(
	ctx context.Context,
	identity messagequeue.QueueSessionIdentity,
) (bool, error) {
	if identity.SessionIncarnationID == "" {
		return false, nil
	}
	current, err := s.messageQueue.ResolveSessionIdentity(ctx, identity.TaskID, identity.SessionID)
	if errors.Is(err, messagequeue.ErrSessionIdentityMismatch) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return current != identity, nil
}

func (s *Service) discardSendNowClaim(ctx context.Context, claim *messagequeue.SendNowClaim) error {
	err := s.messageQueue.DeletePendingSendNowClaim(ctx, claim)
	if err != nil && !errors.Is(err, messagequeue.ErrSendNowClaimChanged) {
		return err
	}
	return nil
}

func (s *Service) markSendNowClaimNotDelivered(
	ctx context.Context,
	claim *messagequeue.SendNowClaim,
) error {
	if s.messageCreator == nil {
		return errors.New("message creator is unavailable for an undeliverable Send Now claim")
	}
	messageID := sendNowClaimTranscriptMessageID(claim)
	if messageID == "" {
		return errors.New("send-now claim has no transcript message identity")
	}
	marked, err := s.messageCreator.SetUserMessageDeliveryStatus(
		ctx, messageID, claim.Dispatch.SessionID, models.MessageDeliveryStatusNotDelivered,
	)
	if err != nil {
		return err
	}
	if marked {
		return nil
	}
	// The prompt failed before any path recorded its user message. Record it
	// from the dispatch envelope in a completed lifecycle-only turn so the
	// release never starts or adopts a conversation turn.
	dispatch := claim.Dispatch
	metadata := make(map[string]interface{}, len(dispatch.Metadata)+1)
	for key, value := range dispatch.Metadata {
		metadata[key] = value
	}
	metadata[models.MessageMetaKeyDeliveryStatus] = models.MessageDeliveryStatusNotDelivered
	delete(metadata, metaKeyUserMessageRecorded)
	dispatch.Metadata = metadata
	attachments := queuedMessageAttachmentsToV1(dispatch.Attachments)
	record, err := queuedUserMessageRecordFor(
		&dispatch, attachments, nil, sendNowClaimTranscriptSourceIDs(claim),
	)
	if err != nil {
		return err
	}
	if record.messageID != messageID {
		return fmt.Errorf("send-now message identity changed: %s != %s", record.messageID, messageID)
	}
	return s.messageCreator.CreateCompletedUserMessageIdempotent(
		ctx, record.messageID, dispatch.TaskID, record.content, dispatch.SessionID, record.metadata,
	)
}

// sendNowClaimTranscriptMessageID reproduces the identity used when the claim's
// replacement prompt records its user message. A durable plan comment records
// one message for the whole source set; every other dispatch records one for
// the dispatch envelope itself.
func sendNowClaimTranscriptMessageID(claim *messagequeue.SendNowClaim) string {
	if claim == nil {
		return ""
	}
	return queuedTranscriptMessageID(&claim.Dispatch, sendNowClaimTranscriptSourceIDs(claim))
}

func sendNowClaimTranscriptSourceIDs(claim *messagequeue.SendNowClaim) []string {
	if claim == nil {
		return nil
	}
	if !claim.HasDurablePlanComment() {
		return []string{claim.Dispatch.ID}
	}
	sourceIDs := make([]string, 0, len(claim.Sources))
	for i := range claim.Sources {
		sourceIDs = append(sourceIDs, claim.Sources[i].ID)
	}
	return sourceIDs
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
		if err := s.reconcilePendingSendNowClaimOnStartup(ctx, pending); err != nil {
			return err
		}
		s.publishQueueStatusEvent(ctx, pending.Claim.Dispatch.SessionID)
	}
	return nil
}

func (s *Service) reconcilePendingSendNowClaimOnStartup(
	ctx context.Context,
	pending *messagequeue.PendingSendNowClaim,
) error {
	claim := &pending.Claim
	sessionID := claim.Dispatch.SessionID
	if pending.Accepted {
		if err := s.acknowledgeSendNowClaimWithRetry(ctx, claim); err != nil {
			if !errors.Is(err, messagequeue.ErrSendNowClaimChanged) {
				return fmt.Errorf("recover pending Send Now claim for session %s: %w", sessionID, err)
			}
			// An accepted claim was delivered; a superseded record must not
			// mark its message not delivered.
			return s.discardSendNowClaim(ctx, claim)
		}
		return nil
	}
	if _, supported := s.repo.(sessionRecoveryBlockStore); supported {
		recoverySignal := &sessionRecoveryRequiredError{
			Block: &models.SessionRecoveryBlock{Reason: "unknown_send_now_outcome"},
		}
		recoveryErr := s.recordSessionRecoveryBlock(ctx, sessionID, "send_now", recoverySignal)
		if recoveryErr != nil {
			if !errors.Is(recoveryErr, messagequeue.ErrSendNowClaimChanged) {
				return fmt.Errorf("recover pending Send Now claim for session %s: %w", sessionID, recoveryErr)
			}
			return s.discardSendNowClaim(ctx, claim)
		}
		s.logger.Warn("parked Send Now claim with unknown pre-crash delivery outcome",
			zap.String("claim_id", claim.ClaimID),
			zap.String("session_id", sessionID))
		return nil
	}
	if err := s.restoreSendNowClaimWithRetry(ctx, claim); err != nil {
		if !errors.Is(err, messagequeue.ErrSendNowClaimChanged) {
			return fmt.Errorf("recover pending Send Now claim for session %s: %w", sessionID, err)
		}
		if releaseErr := s.releaseUnrecoverableSendNowClaim(ctx, claim); releaseErr != nil {
			return fmt.Errorf("release unrecoverable Send Now claim for session %s: %w", sessionID, releaseErr)
		}
	}
	return nil
}

func (s *Service) restorePendingSendNowClaimsForRecovery(ctx context.Context, sessionID string) error {
	if s.messageQueue == nil || sessionID == "" || !s.messageQueue.PendingSendNowClaimPersistenceAvailable() {
		return nil
	}
	claims, err := s.messageQueue.ListPendingSendNowClaims(ctx)
	if err != nil {
		return fmt.Errorf("list pending Send Now claims for recovery: %w", err)
	}
	for i := range claims {
		pending := &claims[i]
		claim := &pending.Claim
		if claim.Dispatch.SessionID != sessionID {
			continue
		}
		if pending.Accepted {
			if err := s.acknowledgeSendNowClaimWithRetry(ctx, claim); err != nil {
				if !errors.Is(err, messagequeue.ErrSendNowClaimChanged) {
					return fmt.Errorf("release pending Send Now claim for session %s: %w", sessionID, err)
				}
				// An accepted claim was delivered; a superseded record must not
				// mark its message not delivered.
				if discardErr := s.discardSendNowClaim(ctx, claim); discardErr != nil {
					return fmt.Errorf("discard superseded Send Now claim for session %s: %w", sessionID, discardErr)
				}
			}
			continue
		}
		if err := s.restoreSendNowClaimWithRetry(context.WithoutCancel(ctx), claim); err != nil {
			if !errors.Is(err, messagequeue.ErrSendNowClaimChanged) {
				return fmt.Errorf("release pending Send Now claim for session %s: %w", sessionID, err)
			}
			if releaseErr := s.releaseUnrecoverableSendNowClaim(ctx, claim); releaseErr != nil {
				return fmt.Errorf("release unrecoverable Send Now claim for session %s: %w", sessionID, releaseErr)
			}
		}
	}
	return nil
}
