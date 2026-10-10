package orchestrator

import (
	"context"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

func incompleteReconstructedRecovery(recovery models.AgentDeliveryRecovery) bool {
	return recovery.Reconstruction != nil && (recovery.AgentExecutionID == "" || recovery.PromptGeneration == 0 ||
		!recovery.Reconstruction.ProcessIdentityKnown || recovery.OriginalRuntime.Validate() != nil)
}

func enrichReconstructionControlIdentity(ctx context.Context, client deliveryRecordEvidenceClient, evidence *agentruntime.DeliveryRecordEvidence, request *models.AgentDeliveryReconstructionRequest) string {
	identity := evidence.ControlIdentity
	if identity == nil {
		return ""
	}
	if identity.TaskID != request.TaskID || identity.SessionID != request.SessionID ||
		identity.IncarnationID != request.IncarnationID || identity.HarnessGeneration != uint64(request.HarnessGeneration) ||
		identity.SubmissionID != request.Submission.ID || identity.StreamID != request.Recovery.StreamID ||
		identity.ExecutionID == "" || identity.PromptGeneration == 0 || identity.OriginalRuntime.Validate() != nil {
		return deliveryIdentityMismatchReason
	}
	// Payload selection and process ownership are separate reads. Recheck the
	// exact process-bound prompt before committing a control identity.
	latest, err := client.InspectDeliveryRecordEvidence(ctx, agentruntime.DeliveryRecordEvidenceRequest{
		TaskID: request.TaskID, SessionID: request.SessionID, IncarnationID: request.IncarnationID,
		HarnessGeneration: uint64(request.HarnessGeneration),
	})
	if err != nil {
		return deliveryRecordEvidenceReason(err)
	}
	if latest == nil || latest.ControlIdentity == nil || *latest.ControlIdentity != *identity {
		return staleDeliveryRecoveryReason
	}
	request.Recovery.AgentExecutionID = identity.ExecutionID
	request.Recovery.PromptGeneration = identity.PromptGeneration
	request.Recovery.OriginalRuntime = identity.OriginalRuntime
	request.Recovery.Reconstruction.ProcessIdentityKnown = true
	return ""
}
