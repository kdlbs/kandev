package sqlite

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func validateAgentDeliveryReconstructionRequest(request *models.AgentDeliveryReconstructionRequest) error {
	if request == nil || request.TaskID == "" || request.SessionID == "" || request.IncarnationID == "" ||
		request.HarnessGeneration <= 0 || request.ExpectedSessionState == "" || request.NativeSessionID == "" || request.MessageID == "" {
		return repoerrors.ErrAgentDeliveryReconstructionConflict
	}
	if !validReconstructionSubmission(request) || !validReconstructionBlock(request) ||
		!validReconstructionRecovery(request) || !validReconstructionProvenance(request) {
		return repoerrors.ErrAgentDeliveryReconstructionConflict
	}
	return validateReconstructionCursorEvidence(request)
}

func validReconstructionSubmission(request *models.AgentDeliveryReconstructionRequest) bool {
	submission := request.Submission
	return submission.ID == "prompt:"+request.MessageID && submission.SessionID == request.SessionID &&
		submission.IncarnationID == request.IncarnationID && submission.HarnessGeneration == request.HarnessGeneration &&
		submission.OwnerGeneration == request.HarnessGeneration && submission.DispatchAttemptID == request.MessageID &&
		submission.State == models.DeliverySubmissionInterruptedUnknown &&
		!submission.CreatedAt.IsZero() && !submission.UpdatedAt.IsZero() && validReconstructionSubmissionPayload(submission)
}

func validReconstructionSubmissionPayload(submission models.AgentDeliverySubmission) bool {
	if submission.PayloadHash == "" || len(submission.Payload) == 0 || len(submission.Payload) > agentDeliveryReconstructionPayloadLimit {
		return false
	}
	hash := sha256.Sum256(submission.Payload)
	return hex.EncodeToString(hash[:]) == submission.PayloadHash
}

func validReconstructionBlock(request *models.AgentDeliveryReconstructionRequest) bool {
	block := request.ExpectedBlock
	return block.ID != "" && block.SessionID == request.SessionID && block.IncarnationID == request.IncarnationID &&
		block.ExpectedGeneration == request.HarnessGeneration && block.State == models.RecoveryBlockOpen &&
		block.ConsumerReference == reconstructionDeliveryConsumer &&
		(block.Reason == "unknown_prompt_outcome" || block.Reason == "unresolved_durable_work") &&
		(block.DeliverySubmissionID == "" || block.DeliverySubmissionID == request.Submission.ID) &&
		(block.DeliveryStreamID == "" || block.DeliveryStreamID == request.Recovery.StreamID)
}

func validReconstructionRecovery(request *models.AgentDeliveryReconstructionRequest) bool {
	recovery := request.Recovery
	return recovery.Phase == models.AgentDeliveryRecoveryUncertain && recovery.SessionID == request.SessionID &&
		recovery.SubmissionID == request.Submission.ID && recovery.StreamID != "" && recovery.IncarnationID == request.IncarnationID &&
		recovery.HarnessGeneration == request.HarnessGeneration && recovery.Reconstruction != nil && validReconstructionControlIdentity(recovery)
}

func validReconstructionProvenance(request *models.AgentDeliveryReconstructionRequest) bool {
	provenance := request.Recovery.Reconstruction
	return provenance != nil && provenance.SourceSessionID == request.SessionID &&
		provenance.SourceIncarnationID == request.IncarnationID && provenance.SourceHarnessGeneration == request.HarnessGeneration &&
		provenance.SourceStreamID == request.Recovery.StreamID && provenance.PayloadHash == request.Submission.PayloadHash &&
		provenance.ObservedState != "" && provenance.SourceStreamHighWater == request.SourceStreamHighWater &&
		provenance.SourceStreamAcknowledged == request.SourceStreamAcknowledged && provenance.SourceStreamFirstRetained == request.SourceStreamFirstRetained
}

func validateReconstructionCursorEvidence(request *models.AgentDeliveryReconstructionRequest) error {
	if request.SourceStreamHighWater < 0 || request.SourceStreamAcknowledged < 0 ||
		request.SourceStreamAcknowledged > request.SourceStreamHighWater || request.SourceStreamFirstRetained < 1 {
		return repoerrors.ErrAgentDeliveryReconstructionConflict
	}
	cursor := request.ExpectedCursor
	if cursor == nil {
		if request.SourceStreamFirstRetained > 1 {
			return repoerrors.ErrAgentDeliveryReconstructionStale
		}
		return nil
	}
	if cursor.StreamID != request.Recovery.StreamID || cursor.SessionID != request.SessionID || cursor.IncarnationID != request.IncarnationID ||
		cursor.HarnessGeneration != request.HarnessGeneration || cursor.ReceivedSequence < request.SourceStreamAcknowledged ||
		cursor.ReceivedSequence > request.SourceStreamHighWater || cursor.ProjectedSequence > cursor.ReceivedSequence ||
		cursor.RemoteHighWater > request.SourceStreamHighWater {
		return repoerrors.ErrAgentDeliveryReconstructionStale
	}
	return nil
}
