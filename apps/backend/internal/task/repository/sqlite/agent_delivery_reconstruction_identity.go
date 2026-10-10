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
