package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

type reconstructionSelection struct {
	evidence   *agentruntime.DeliveryRecordEvidence
	submission *journal.Submission
	stream     *journal.Stream
	messageID  string
}

func (s *Service) readReconstructionSelection(ctx context.Context, client deliveryRecordEvidenceClient, session *models.TaskSession, incarnationID string, generation int64, block *models.SessionRecoveryBlock) (*reconstructionSelection, string, error) {
	blocked := func(reason string) (*reconstructionSelection, string, error) { return nil, reason, nil }
	evidence, err := client.InspectDeliveryRecordEvidence(ctx, agentruntime.DeliveryRecordEvidenceRequest{
		TaskID: session.TaskID, SessionID: session.ID, IncarnationID: incarnationID,
		HarnessGeneration: uint64(generation),
	})
	if err != nil {
		return blocked(deliveryRecordEvidenceReason(err))
	}
	candidate, stream, reason := selectReconstructionEvidence(evidence, session, incarnationID, generation, block)
	if reason != "" {
		return blocked(reason)
	}
	messageID, ok := retainedPromptMessageID(candidate.ID)
	if !ok {
		return blocked(deliveryIdentityMismatchReason)
	}
	if reason, err := s.validateReconstructionMessage(ctx, session, messageID); err != nil {
		return nil, "", err
	} else if reason != "" {
		return blocked(reason)
	}
	selected, err := client.ReadDeliveryRecordSubmission(ctx, agentruntime.DeliveryRecordSubmissionRequest{
		TaskID: session.TaskID, SessionID: session.ID, IncarnationID: incarnationID,
		HarnessGeneration: uint64(generation), Candidate: candidate,
	})
	if err != nil {
		return blocked(deliveryRecordEvidenceReason(err))
	}
	if selected == nil || journal.VerifyReconstructionSubmission(*selected, candidate) != nil {
		return blocked(deliveryIdentityMismatchReason)
	}
	return &reconstructionSelection{evidence: evidence, submission: selected, stream: stream, messageID: messageID}, "", nil
}

func (s *Service) prepareReconstructionRequest(ctx context.Context, session *models.TaskSession, incarnationID string, generation int64, block *models.SessionRecoveryBlock, selection *reconstructionSelection) (*models.AgentDeliveryReconstructionRequest, string, error) {
	blocked := func(reason string) (*models.AgentDeliveryReconstructionRequest, string, error) {
		return nil, reason, nil
	}
	evidence, selected, stream, messageID := selection.evidence, selection.submission, selection.stream, selection.messageID
	continuity, ok := s.repo.(sessionContinuityStore)
	if !ok {
		return blocked("recovery_state_unavailable")
	}
	current, err := continuity.GetCurrentHarnessSessionGeneration(ctx, session.ID, incarnationID)
	if err != nil || current == nil || current.Generation != generation || current.NativeSessionID == "" {
		return blocked(staleDeliveryRecoveryReason)
	}
	submissions, ok := s.repo.(repository.AgentDeliveryRepository)
	if !ok {
		return blocked("recovery_state_unavailable")
	}
	cursor, err := submissions.GetAgentDeliveryCursor(ctx, stream.StreamID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, "", err
	}
	var expectedCursor *models.AgentDeliveryCursor
	if err == nil {
		expectedCursor = cursor
	}
	streamHighWater, okHigh := journalSequenceInt64(stream.HighWater)
	streamAcknowledged, okAcknowledged := journalSequenceInt64(stream.Acknowledged)
	streamFirstRetained, okFirst := journalSequenceInt64(stream.FirstRetained)
	if !okHigh || !okAcknowledged || !okFirst {
		return blocked(deliveryIdentityMismatchReason)
	}
	payloadHash := selected.Hash
	state := models.DeliverySubmissionInterruptedUnknown
	request := &models.AgentDeliveryReconstructionRequest{
		TaskID: session.TaskID, SessionID: session.ID, IncarnationID: incarnationID,
		HarnessGeneration: generation, ExpectedExecutionID: session.AgentExecutionID,
		ExpectedSessionState: session.State, ExpectedWorkspacePath: session.WorkspacePath,
		NativeSessionID: current.NativeSessionID, MessageID: messageID, ExpectedBlock: *block,
		SourceStreamHighWater: streamHighWater, SourceStreamAcknowledged: streamAcknowledged,
		SourceStreamFirstRetained: streamFirstRetained, ExpectedCursor: expectedCursor,
		Submission: models.AgentDeliverySubmission{
			ID: selected.ID, SessionID: selected.SessionID, IncarnationID: selected.IncarnationID,
			HarnessGeneration: int64(selected.HarnessGeneration), OwnerGeneration: int64(selected.HarnessGeneration),
			DispatchAttemptID: messageID, PayloadHash: payloadHash, Payload: append([]byte(nil), selected.Payload...),
			State: state, Outcome: "reconstructed_unknown", CreatedAt: selected.CreatedAt, UpdatedAt: selected.UpdatedAt,
		},
		Recovery: models.AgentDeliveryRecovery{
			Phase: models.AgentDeliveryRecoveryUncertain, SessionID: session.ID,
			SubmissionID: selected.ID, StreamID: selected.StreamID, IncarnationID: incarnationID,
			HarnessGeneration: generation,
			Reconstruction: &models.AgentDeliveryReconstructionProvenance{
				SourceSessionID: selected.SessionID, SourceIncarnationID: selected.IncarnationID,
				SourceHarnessGeneration: int64(selected.HarnessGeneration), SourceStreamID: selected.StreamID,
				PayloadHash: payloadHash, ObservedState: string(selected.State),
				SourceStreamHighWater: streamHighWater, SourceStreamAcknowledged: streamAcknowledged,
				SourceStreamFirstRetained: streamFirstRetained,
				ProcessTerminated:         cloneBool(evidence.ProcessTerminated),
			},
		},
	}
	if existing, present := models.LoadAgentDeliveryRecovery(session.Metadata); present {
		request.Recovery.Revision = existing.Revision
	}
	return request, "", nil
}
