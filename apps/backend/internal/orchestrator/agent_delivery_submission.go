package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const durableDeliveryUnresolvedReason = "unresolved_durable_work"

type durableDeliveryCapabilityReader interface {
	DurableDeliveryCapabilityForExecution(
		context.Context,
		string,
	) (agentruntime.DurableDeliveryCapability, bool)
}

type deliveryClaimUpdater func(context.Context, string, string, string) error

type agentDeliverySubmissionRuntime struct {
	store        repository.AgentDeliveryRepository
	id           string
	sessionID    string
	completed    bool
	dispatchOnly bool
}

func (r *agentDeliverySubmissionRuntime) markInterrupted(
	ctx context.Context,
	outcome string,
) error {
	if r == nil || r.store == nil || r.id == "" || r.completed {
		return nil
	}
	_, err := r.store.TransitionAgentDeliverySubmission(
		ctx,
		r.id,
		models.DeliverySubmissionDispatching,
		models.DeliverySubmissionInterruptedUnknown,
		outcome,
		time.Now().UTC(),
	)
	return err
}

func (r *agentDeliverySubmissionRuntime) markCompleted(ctx context.Context) error {
	if r == nil || r.store == nil || r.id == "" || r.completed || r.dispatchOnly {
		return nil
	}
	changed, err := r.store.TransitionAgentDeliverySubmission(
		ctx,
		r.id,
		models.DeliverySubmissionDispatching,
		models.DeliverySubmissionCompleted,
		"completed",
		time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if !changed {
		return fmt.Errorf("delivery submission %q was not dispatching", r.id)
	}
	r.completed = true
	return nil
}

type agentDeliveryPromptPayload struct {
	Text        string                 `json:"text"`
	Attachments []v1.MessageAttachment `json:"attachments,omitempty"`
	Steer       bool                   `json:"steer,omitempty"`
}

func marshalAgentDeliveryPromptPayload(
	prompt string,
	attachments []v1.MessageAttachment,
) ([]byte, error) {
	return json.Marshal(agentDeliveryPromptPayload{
		Text: prompt, Attachments: attachments,
	})
}

func normalizeDeliverySubmissionID(id string) string {
	if strings.HasPrefix(id, "prompt:") {
		return id
	}
	return "prompt:" + id
}

func (s *Service) deliveryRecoveryError(
	ctx context.Context,
	sessionID string,
	reason string,
) error {
	recoveryErr := &sessionRecoveryRequiredError{
		Block: &models.SessionRecoveryBlock{Reason: reason},
	}
	if err := s.recordSessionRecoveryBlock(ctx, sessionID, "agent_delivery", recoveryErr); err != nil {
		return errors.Join(recoveryErr, fmt.Errorf("persist durable delivery recovery block: %w", err))
	}
	return recoveryErr
}

func (s *Service) deliveryCapabilityForExecution(
	ctx context.Context,
	executionID string,
) (agentruntime.DurableDeliveryDecision, bool) {
	reader, ok := s.agentManager.(durableDeliveryCapabilityReader)
	if !ok {
		return agentruntime.DurableDeliveryDecision{
			Mode:   agentruntime.DurableDeliveryLegacy,
			Reason: "legacy_peer",
		}, false
	}
	capability, advertised := reader.DurableDeliveryCapabilityForExecution(ctx, executionID)
	if !advertised {
		return agentruntime.DurableDeliveryDecision{
			Mode:   agentruntime.DurableDeliveryLegacy,
			Reason: "legacy_peer",
		}, false
	}
	return agentruntime.NegotiateDurableDelivery(
		journal.StorageCapability{
			Version: journal.CurrentVersion,
			Durable: true,
		},
		capability.Version,
		capability.Durable,
		capability.Unresolved,
	), true
}

func (s *Service) currentDeliveryGeneration(
	ctx context.Context,
	session *models.TaskSession,
	incarnationID string,
) (int64, error) {
	if continuity, ok := s.repo.(sessionContinuityStore); ok {
		current, err := continuity.GetCurrentHarnessSessionGeneration(
			ctx, session.ID, incarnationID,
		)
		switch {
		case err == nil && current != nil && current.Generation > 0:
			return current.Generation, nil
		case err == nil:
		case errors.Is(err, models.ErrTaskSessionNotFound), errors.Is(err, sql.ErrNoRows):
		default:
			return 0, fmt.Errorf("load current harness generation: %w", err)
		}
	}
	return 1, nil
}

func (s *Service) prepareAgentDeliverySubmission(
	ctx context.Context,
	session *models.TaskSession,
	executionID string,
	prompt string,
	attachments []v1.MessageAttachment,
	options promptTaskOptions,
) (*agentDeliverySubmissionRuntime, error) {
	if session == nil || options.deliverySubmissionID == "" {
		return nil, nil
	}

	protocol, err := s.resolveDeliveryProtocol(
		ctx, session.ID, executionID, options.deliveryProtocol,
	)
	if err != nil {
		return nil, err
	}

	if protocol == messagequeue.DeliveryProtocolLegacy {
		if err := persistDeliveryClaim(
			ctx, options.deliveryClaimUpdater, protocol, options.deliverySubmissionID, "",
		); err != nil {
			return nil, err
		}
		return nil, nil
	}

	payload, err := marshalAgentDeliveryPromptPayload(prompt, attachments)
	if err != nil {
		return nil, fmt.Errorf("marshal durable delivery payload: %w", err)
	}
	payloadHash := journal.SubmissionHash(payload)
	if options.deliveryPayloadHash != "" && options.deliveryPayloadHash != payloadHash {
		return nil, s.deliveryRecoveryError(ctx, session.ID, "delivery_payload_changed")
	}
	if err := persistDeliveryClaim(
		ctx, options.deliveryClaimUpdater, messagequeue.DeliveryProtocolV1,
		options.deliverySubmissionID, payloadHash,
	); err != nil {
		return nil, err
	}
	return s.prepareBackendDeliverySubmission(ctx, session, options, payload, payloadHash)
}

func (s *Service) resolveDeliveryProtocol(
	ctx context.Context,
	sessionID, executionID, requested string,
) (string, error) {
	protocol := requested
	if protocol == "" {
		protocol = messagequeue.DeliveryProtocolPending
	}
	switch protocol {
	case messagequeue.DeliveryProtocolLegacy:
		return protocol, nil
	case messagequeue.DeliveryProtocolV1:
		return s.validateV1DeliveryProtocol(ctx, sessionID, executionID)
	case messagequeue.DeliveryProtocolPending:
		return s.resolvePendingDeliveryProtocol(ctx, sessionID, executionID)
	default:
		return "", s.deliveryRecoveryError(ctx, sessionID, "unknown_delivery_protocol")
	}
}

func (s *Service) resolvePendingDeliveryProtocol(
	ctx context.Context,
	sessionID, executionID string,
) (string, error) {
	decision, _ := s.deliveryCapabilityForExecution(ctx, executionID)
	switch decision.Mode {
	case agentruntime.DurableDeliveryLegacy:
		return messagequeue.DeliveryProtocolLegacy, nil
	case agentruntime.DurableDeliveryV1:
		if decision.Reason == durableDeliveryUnresolvedReason {
			return "", s.deliveryRecoveryError(ctx, sessionID, decision.Reason)
		}
		return messagequeue.DeliveryProtocolV1, nil
	default:
		return "", s.deliveryRecoveryError(ctx, sessionID, decision.Reason)
	}
}

func (s *Service) validateV1DeliveryProtocol(
	ctx context.Context,
	sessionID, executionID string,
) (string, error) {
	decision, advertised := s.deliveryCapabilityForExecution(ctx, executionID)
	if !advertised || decision.Mode != agentruntime.DurableDeliveryV1 {
		reason := decision.Reason
		if reason == "" {
			reason = "incompatible_delivery_version"
		}
		return "", s.deliveryRecoveryError(ctx, sessionID, reason)
	}
	if decision.Reason == durableDeliveryUnresolvedReason {
		return "", s.deliveryRecoveryError(ctx, sessionID, decision.Reason)
	}
	return messagequeue.DeliveryProtocolV1, nil
}

func persistDeliveryClaim(
	ctx context.Context,
	updater deliveryClaimUpdater,
	protocol, submissionID, payloadHash string,
) error {
	if updater == nil {
		return nil
	}
	err := updater(ctx, protocol, submissionID, payloadHash)
	if err == nil {
		return nil
	}
	if errors.Is(err, messagequeue.ErrQueueDispatchClaimChanged) ||
		errors.Is(err, messagequeue.ErrSendNowClaimChanged) {
		return errQueuedDispatchSuperseded
	}
	return fmt.Errorf("persist %s delivery protocol: %w", protocol, err)
}

func (s *Service) prepareBackendDeliverySubmission(
	ctx context.Context,
	session *models.TaskSession,
	options promptTaskOptions,
	payload []byte,
	payloadHash string,
) (*agentDeliverySubmissionRuntime, error) {
	store, ok := s.repo.(repository.AgentDeliveryRepository)
	if !ok {
		return nil, s.deliveryRecoveryError(ctx, session.ID, "backend_delivery_storage_unavailable")
	}
	incarnationID := session.QueueIncarnationID
	if incarnationID == "" {
		incarnationID = session.ID
	}
	generation, err := s.currentDeliveryGeneration(ctx, session, incarnationID)
	if err != nil {
		return nil, s.deliveryRecoveryError(ctx, session.ID, "harness_generation_unavailable")
	}
	if options.expectedDeliveryGeneration > 0 && generation != options.expectedDeliveryGeneration {
		return nil, s.deliveryRecoveryError(ctx, session.ID, "harness_generation_changed")
	}
	submissionID := normalizeDeliverySubmissionID(options.deliverySubmissionID)
	now := time.Now().UTC()
	submission := &models.AgentDeliverySubmission{
		ID:                submissionID,
		SessionID:         session.ID,
		IncarnationID:     incarnationID,
		HarnessGeneration: generation,
		OwnerGeneration:   generation,
		DispatchAttemptID: options.deliverySubmissionID,
		PayloadHash:       payloadHash,
		Payload:           payload,
		State:             models.DeliverySubmissionPrepared,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if reason := admitBackendDeliverySubmission(ctx, store, submission, now); reason != "" {
		return nil, s.deliveryRecoveryError(ctx, session.ID, reason)
	}
	return &agentDeliverySubmissionRuntime{
		store: store, id: submissionID, sessionID: session.ID,
		dispatchOnly: false,
	}, nil
}

func admitBackendDeliverySubmission(
	ctx context.Context,
	store repository.AgentDeliveryRepository,
	submission *models.AgentDeliverySubmission,
	now time.Time,
) string {
	if _, err := store.PrepareAgentDeliverySubmission(ctx, submission); err != nil {
		if errors.Is(err, repository.ErrAgentDeliverySubmissionConflict) {
			return "delivery_submission_conflict"
		}
		return "backend_delivery_admission_failed"
	}
	stored, err := store.GetAgentDeliverySubmission(ctx, submission.ID)
	if err != nil {
		return "backend_delivery_admission_unreadable"
	}
	switch stored.State {
	case models.DeliverySubmissionPrepared:
		changed, transitionErr := store.TransitionAgentDeliverySubmission(
			ctx, submission.ID, models.DeliverySubmissionPrepared,
			models.DeliverySubmissionAccepted, "accepted", now,
		)
		if transitionErr != nil || !changed {
			return "backend_delivery_acceptance_failed"
		}
		stored.State = models.DeliverySubmissionAccepted
	case models.DeliverySubmissionAccepted:
	case models.DeliverySubmissionDispatching,
		models.DeliverySubmissionInterruptedUnknown,
		models.DeliverySubmissionCompleted,
		models.DeliverySubmissionFailed,
		models.DeliverySubmissionCancelled:
		return "delivery_submission_requires_reconciliation"
	default:
		return "unknown_delivery_submission_state"
	}
	if stored.State != models.DeliverySubmissionAccepted {
		return ""
	}
	changed, transitionErr := store.TransitionAgentDeliverySubmission(
		ctx, submission.ID, models.DeliverySubmissionAccepted,
		models.DeliverySubmissionDispatching, "dispatching", now,
	)
	if transitionErr != nil || !changed {
		return "backend_delivery_dispatch_admission_failed"
	}
	return ""
}
