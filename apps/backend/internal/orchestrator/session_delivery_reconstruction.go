package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

const (
	deliveryIdentityMismatchReason  = "delivery_identity_mismatch"
	retainedEvidenceAmbiguousReason = "retained_evidence_ambiguous"
	staleDeliveryRecoveryReason     = "stale_recovery"
)

type deliveryRecordEvidenceClient interface {
	InspectDeliveryRecordEvidence(context.Context, agentruntime.DeliveryRecordEvidenceRequest) (*agentruntime.DeliveryRecordEvidence, error)
	ReadDeliveryRecordSubmission(context.Context, agentruntime.DeliveryRecordSubmissionRequest) (*agentruntime.DeliveryRecordSubmission, error)
}

func selectDeliveryReconstructionBlock(
	blocks []*models.SessionRecoveryBlock,
) (*models.SessionRecoveryBlock, string) {
	var deliveryBlock *models.SessionRecoveryBlock
	var firstBlock *models.SessionRecoveryBlock
	for _, block := range blocks {
		if block == nil {
			continue
		}
		if firstBlock == nil {
			firstBlock = block
		}
		if block.ConsumerReference != agentDeliveryConsumer {
			continue
		}
		if deliveryBlock != nil {
			return nil, deliveryIdentityMismatchReason
		}
		deliveryBlock = block
	}
	if deliveryBlock != nil {
		return deliveryBlock, ""
	}
	if firstBlock != nil {
		return nil, "independent_recovery_block"
	}
	return nil, ""
}

func (s *Service) retryMissingCanonicalDeliverySubmission(ctx context.Context, session *models.TaskSession, incarnationID string, generation int64, block *models.SessionRecoveryBlock) (*SessionDeliveryRecoveryResponse, error) {
	blocked := func(reason string) (*SessionDeliveryRecoveryResponse, error) {
		return sessionDeliveryRecoveryResponse(session.TaskID, session.ID, SessionDeliveryRecoveryBlocked, reason, 0, nil), nil
	}
	if block == nil || block.ConsumerReference != agentDeliveryConsumer {
		return blocked("independent_recovery_block")
	}
	client, ok := s.agentManager.(deliveryRecordEvidenceClient)
	if !ok {
		return blocked("missing_canonical_submission")
	}
	store, ok := s.repo.(repository.AgentDeliveryReconstructionRepository)
	if !ok {
		return blocked("recovery_state_unavailable")
	}
	selected, reason, err := s.readReconstructionSelection(ctx, client, session, incarnationID, generation, block)
	if err != nil {
		return nil, err
	}
	if reason != "" {
		return blocked(reason)
	}
	request, reason, err := s.prepareReconstructionRequest(ctx, session, incarnationID, generation, block, selected)
	if err != nil {
		return nil, err
	}
	if reason != "" {
		return blocked(reason)
	}
	if reason := enrichReconstructionControlIdentity(ctx, client, selected.evidence, request); reason != "" {
		return blocked(reason)
	}
	return s.commitDeliveryReconstruction(ctx, session, store, request)
}

func (s *Service) commitDeliveryReconstruction(ctx context.Context, session *models.TaskSession, store repository.AgentDeliveryReconstructionRepository, request *models.AgentDeliveryReconstructionRequest) (*SessionDeliveryRecoveryResponse, error) {
	blocked := func(reason string) (*SessionDeliveryRecoveryResponse, error) {
		return sessionDeliveryRecoveryResponse(session.TaskID, session.ID, SessionDeliveryRecoveryBlocked, reason, 0, nil), nil
	}
	result, err := store.ReconstructAgentDeliverySubmission(ctx, request)
	if err != nil {
		if errors.Is(err, repository.ErrAgentDeliveryReconstructionConflict) {
			return blocked(deliveryIdentityMismatchReason)
		}
		if errors.Is(err, repository.ErrAgentDeliveryReconstructionStale) {
			return blocked(staleDeliveryRecoveryReason)
		}
		return nil, err
	}
	s.publishAgentDeliveryRecoveryState(ctx, session.ID)
	if !incompleteReconstructedRecovery(result.Recovery) {
		return s.RetrySessionDelivery(ctx, session.TaskID, session.ID)
	}
	identity := &SessionDeliveryRecoveryIdentity{
		SubmissionID: result.Recovery.SubmissionID, StreamID: result.Recovery.StreamID,
		IncarnationID: result.Recovery.IncarnationID, HarnessGeneration: result.Recovery.HarnessGeneration,
		PromptGeneration: result.Recovery.PromptGeneration,
	}
	return sessionDeliveryRecoveryResponse(session.TaskID, session.ID, SessionDeliveryRecoveryBlocked,
		"recovery_identity_incomplete", result.Recovery.Revision, identity), nil
}

func (s *Service) validateReconstructionMessage(ctx context.Context, session *models.TaskSession, messageID string) (string, error) {
	messages, ok := s.repo.(repository.MessageRepository)
	if !ok {
		return "recovery_state_unavailable", nil
	}
	message, err := messages.GetMessage(ctx, messageID)
	if err != nil || message == nil {
		if errors.Is(err, sql.ErrNoRows) || message == nil {
			return deliveryIdentityMismatchReason, nil
		}
		return "", err
	}
	if message.ID != messageID || message.TaskID != session.TaskID || message.TaskSessionID != session.ID ||
		message.AuthorType != models.MessageAuthorUser {
		return deliveryIdentityMismatchReason, nil
	}
	return "", nil
}

func selectReconstructionEvidence(
	evidence *agentruntime.DeliveryRecordEvidence,
	session *models.TaskSession,
	incarnationID string,
	generation int64,
	block *models.SessionRecoveryBlock,
) (journal.SubmissionSummary, *journal.Stream, string) {
	if evidence == nil || evidence.Descriptor.SubmissionsTruncated {
		return journal.SubmissionSummary{}, nil, retainedEvidenceAmbiguousReason
	}
	descriptor := evidence.Descriptor
	if descriptor.SubmissionCount != 1 || len(descriptor.Submissions) != 1 {
		return journal.SubmissionSummary{}, nil, retainedEvidenceAmbiguousReason
	}
	candidate := descriptor.Submissions[0]
	if candidate.SessionID != session.ID || candidate.IncarnationID != incarnationID ||
		candidate.HarnessGeneration != uint64(generation) || candidate.StreamID == "" ||
		(block.DeliverySubmissionID != "" && block.DeliverySubmissionID != candidate.ID) ||
		(block.DeliveryStreamID != "" && block.DeliveryStreamID != candidate.StreamID) {
		return journal.SubmissionSummary{}, nil, deliveryIdentityMismatchReason
	}
	stream := descriptor.Stream
	if !reconstructionStreamMatches(stream, session.ID, incarnationID, generation, candidate.StreamID) {
		return journal.SubmissionSummary{}, nil, deliveryIdentityMismatchReason
	}
	return candidate, stream, ""
}

func reconstructionStreamMatches(stream *journal.Stream, sessionID, incarnationID string, generation int64, streamID string) bool {
	return stream != nil && stream.SessionID == sessionID && stream.IncarnationID == incarnationID &&
		stream.HarnessGeneration == uint64(generation) && stream.StreamID == streamID &&
		stream.Acknowledged <= stream.HighWater && stream.FirstRetained > 0
}

func retainedPromptMessageID(submissionID string) (string, bool) {
	if !strings.HasPrefix(submissionID, "prompt:") {
		return "", false
	}
	messageID := strings.TrimPrefix(submissionID, "prompt:")
	return messageID, messageID != ""
}

func journalSequenceInt64(sequence uint64) (int64, bool) {
	const maxInt64 = 1<<63 - 1
	if sequence > maxInt64 {
		return 0, false
	}
	return int64(sequence), true
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func deliveryRecordEvidenceReason(err error) string {
	switch {
	case errors.Is(err, agentruntime.ErrDeliveryRecordEvidenceTruncated),
		errors.Is(err, agentruntime.ErrDeliveryRecordEvidenceAmbiguous):
		return retainedEvidenceAmbiguousReason
	case errors.Is(err, agentruntime.ErrDeliveryRecordEvidenceConflict),
		errors.Is(err, agentruntime.ErrDeliveryOwnerMismatch):
		return deliveryIdentityMismatchReason
	case errors.Is(err, agentruntime.ErrDeliveryRecordEvidenceStale):
		return staleDeliveryRecoveryReason
	default:
		return recoveryUnavailableReason
	}
}
