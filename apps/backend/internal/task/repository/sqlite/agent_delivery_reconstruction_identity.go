package sqlite

import "github.com/kandev/kandev/internal/task/models"

func validReconstructionControlIdentity(recovery models.AgentDeliveryRecovery) bool {
	if recovery.Reconstruction.ProcessIdentityKnown {
		return recovery.AgentExecutionID != "" && recovery.PromptGeneration > 0 && recovery.OriginalRuntime.Validate() == nil
	}
	return recovery.AgentExecutionID == "" && recovery.PromptGeneration == 0
}

func canEnrichReconstructedIdentity(owner *agentDeliveryReconstructionOwner, submission models.AgentDeliverySubmission, block *models.SessionRecoveryBlock, request *models.AgentDeliveryReconstructionRequest) bool {
	if !owner.exists || owner.current.Reconstruction == nil || owner.current.Reconstruction.ProcessIdentityKnown ||
		owner.current.Revision != request.Recovery.Revision || !request.Recovery.Reconstruction.ProcessIdentityKnown {
		return false
	}
	// Compare the already committed immutable association independently of the
	// newly authenticated process proof. Only an incomplete identity can advance.
	candidate := *request
	candidate.Recovery = owner.current
	candidate.Recovery.Reconstruction = owner.current.Reconstruction
	return reconstructionAlreadyCommitted(owner, submission, block, &candidate)
}

func canRecordReconstructedHistoryGap(owner *agentDeliveryReconstructionOwner, submission models.AgentDeliverySubmission, request *models.AgentDeliveryReconstructionRequest) bool {
	current, next := owner.current, request.Recovery
	if !owner.exists || current.HistoryGap != nil || next.HistoryGap == nil {
		return false
	}
	return (current.Phase == models.AgentDeliveryRecoveryUncertain || current.Phase == models.AgentDeliveryRecoveryReconnecting) &&
		sameReconstructedRecoveryOwner(current, next) &&
		(submission.State == models.DeliverySubmissionInterruptedUnknown || submission.State == models.DeliverySubmissionDispatching)
}

func sameReconstructedRecoveryOwner(current, next models.AgentDeliveryRecovery) bool {
	return current.Revision == next.Revision && current.SessionID == next.SessionID &&
		current.IncarnationID == next.IncarnationID && current.HarnessGeneration == next.HarnessGeneration &&
		current.StreamID == next.StreamID && current.SubmissionID == next.SubmissionID &&
		current.AgentExecutionID == next.AgentExecutionID && current.PromptGeneration == next.PromptGeneration &&
		current.OriginalRuntime == next.OriginalRuntime
}
