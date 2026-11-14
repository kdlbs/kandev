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
		!validReconstructionRecovery(request) || !validReconstructionProvenance(request) ||
		!validReconstructionPromptTurnBinding(request) {
		return repoerrors.ErrAgentDeliveryReconstructionConflict
	}
	return validateReconstructionCursorEvidence(request)
}

func validReconstructionPromptTurnBinding(request *models.AgentDeliveryReconstructionRequest) bool {
	binding := request.PromptTurnBinding
	if binding == nil {
		return true
	}
	return reconstructionPromptTurnBindingMatchesRequest(binding, request) &&
		reconstructionPromptTurnBindingMatchesRecovery(binding, request) &&
		reconstructionPromptTurnBindingHasDeadSource(request)
}

func reconstructionPromptTurnBindingMatchesRequest(binding *models.AgentDeliveryPromptTurnBinding, request *models.AgentDeliveryReconstructionRequest) bool {
	return binding.TaskID == request.TaskID && binding.SessionID == request.SessionID &&
		binding.IncarnationID == request.IncarnationID && binding.HarnessGeneration == uint64(request.HarnessGeneration) &&
		binding.SubmissionID == request.Submission.ID && binding.StreamID == request.Recovery.StreamID &&
		binding.ExecutionID != "" && binding.TurnID != "" && binding.PromptGeneration > 0 &&
		binding.OriginalRuntime.Validate() == nil
}

func reconstructionPromptTurnBindingMatchesRecovery(binding *models.AgentDeliveryPromptTurnBinding, request *models.AgentDeliveryReconstructionRequest) bool {
	return request.Recovery.AgentExecutionID == binding.ExecutionID &&
		request.Recovery.PromptGeneration == binding.PromptGeneration && request.Recovery.OriginalRuntime == binding.OriginalRuntime &&
		request.Recovery.Reconstruction.ProcessIdentityKnown
}

func reconstructionPromptTurnBindingHasDeadSource(request *models.AgentDeliveryReconstructionRequest) bool {
	processTerminated := request.Recovery.Reconstruction.ProcessTerminated
	return processTerminated != nil && *processTerminated
}

func validReconstructionSubmission(request *models.AgentDeliveryReconstructionRequest) bool {
	submission := request.Submission
	return submission.ID == "prompt:"+request.MessageID && submission.SessionID == request.SessionID &&
		submission.IncarnationID == request.IncarnationID && submission.HarnessGeneration == request.HarnessGeneration &&
		submission.OwnerGeneration == request.HarnessGeneration && validReconstructionDispatchAttempt(request) &&
		submission.State == models.DeliverySubmissionInterruptedUnknown &&
		!submission.CreatedAt.IsZero() && !submission.UpdatedAt.IsZero() && validReconstructionSubmissionPayload(submission)
}

func validReconstructionDispatchAttempt(request *models.AgentDeliveryReconstructionRequest) bool {
	if request.ExistingSubmissionOnly {
		return request.Submission.DispatchAttemptID != ""
	}
	return request.Submission.DispatchAttemptID == request.MessageID
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
	if gap := request.Recovery.HistoryGap; gap != nil {
		var received, projected int64
		if cursor != nil {
			received, projected = cursor.ReceivedSequence, cursor.ProjectedSequence
		}
		proof := request.Recovery.Reconstruction.ProcessTerminated
		if proof == nil || !*proof || !request.Recovery.Reconstruction.ProcessIdentityKnown ||
			!gap.MatchesPrunedStream(received, projected, request.SourceStreamHighWater, request.SourceStreamAcknowledged, request.SourceStreamFirstRetained) {
			return repoerrors.ErrAgentDeliveryReconstructionConflict
		}
	}
	if cursor == nil {
		if request.SourceStreamFirstRetained > 1 && request.Recovery.HistoryGap == nil {
			return repoerrors.ErrAgentDeliveryReconstructionStale
		}
		return nil
	}
	if cursor.StreamID != request.Recovery.StreamID || cursor.SessionID != request.SessionID || cursor.IncarnationID != request.IncarnationID ||
		cursor.HarnessGeneration != request.HarnessGeneration || (cursor.ReceivedSequence < request.SourceStreamAcknowledged && request.Recovery.HistoryGap == nil) ||
		cursor.ReceivedSequence > request.SourceStreamHighWater || cursor.ProjectedSequence > cursor.ReceivedSequence ||
		cursor.RemoteHighWater > request.SourceStreamHighWater {
		return repoerrors.ErrAgentDeliveryReconstructionStale
	}
	return nil
}
