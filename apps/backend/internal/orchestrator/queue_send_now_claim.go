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
		} else if _, supported := s.repo.(sessionRecoveryBlockStore); supported {
			recoverySignal := &sessionRecoveryRequiredError{
				Block: &models.SessionRecoveryBlock{Reason: "unknown_send_now_outcome"},
			}
			recoveryErr = s.recordSessionRecoveryBlock(
				ctx, claim.Dispatch.SessionID, "send_now", recoverySignal,
			)
			if recoveryErr == nil {
				s.logger.Warn("parked Send Now claim with unknown pre-crash delivery outcome",
					zap.String("claim_id", claim.ClaimID),
					zap.String("session_id", claim.Dispatch.SessionID))
			}
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
			err = s.acknowledgeSendNowClaimWithRetry(ctx, claim)
		} else {
			err = s.restoreSendNowClaimWithRetry(context.WithoutCancel(ctx), claim)
		}
		if err != nil {
			if !errors.Is(err, messagequeue.ErrSendNowClaimChanged) {
				return fmt.Errorf("release pending Send Now claim for session %s: %w", sessionID, err)
			}
			if deleteErr := s.messageQueue.DeletePendingSendNowClaim(context.WithoutCancel(ctx), claim); deleteErr != nil &&
				!errors.Is(deleteErr, messagequeue.ErrSendNowClaimChanged) {
				return fmt.Errorf("discard superseded Send Now claim for session %s: %w", sessionID, deleteErr)
			}
		}
	}
	return nil
}
