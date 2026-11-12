package orchestrator

import (
	"context"
	"errors"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

func incompleteReconstructedRecovery(recovery models.AgentDeliveryRecovery) bool {
	return recovery.Reconstruction != nil && (recovery.AgentExecutionID == "" || recovery.PromptGeneration == 0 ||
		!recovery.Reconstruction.ProcessIdentityKnown || recovery.OriginalRuntime.Validate() != nil)
}

func (s *Service) enrichReconstructionControlIdentity(ctx context.Context, client deliveryRecordEvidenceClient, evidence *agentruntime.DeliveryRecordEvidence, request *models.AgentDeliveryReconstructionRequest) (string, error) {
	identity, persistedBinding, reason, err := s.reconstructionControlIdentity(ctx, evidence, request)
	if err != nil || reason != "" || identity == nil {
		return reason, err
	}
	if !reconstructionControlIdentityMatchesRequest(*identity, request) {
		return deliveryIdentityMismatchReason, nil
	}
	reason, err = s.verifyReconstructionControlIdentity(ctx, client, evidence, *identity, persistedBinding, request)
	if err != nil || reason != "" {
		return reason, err
	}
	request.Recovery.AgentExecutionID = identity.ExecutionID
	request.Recovery.PromptGeneration = identity.PromptGeneration
	request.Recovery.OriginalRuntime = identity.OriginalRuntime
	request.Recovery.Reconstruction.ProcessIdentityKnown = true
	return "", nil
}

func (s *Service) reconstructionControlIdentity(
	ctx context.Context,
	evidence *agentruntime.DeliveryRecordEvidence,
	request *models.AgentDeliveryReconstructionRequest,
) (*agentruntime.AgentDeliveryRecoveryIdentity, bool, string, error) {
	if evidence == nil {
		return nil, false, staleDeliveryRecoveryReason, nil
	}
	if evidence.ControlIdentity != nil {
		return evidence.ControlIdentity, false, "", nil
	}
	binding, err := s.persistedPromptTurnBinding(ctx, request)
	if err != nil {
		return nil, false, "", err
	}
	if binding == nil {
		return nil, false, "", nil
	}
	if evidence.ProcessTerminated == nil || !*evidence.ProcessTerminated {
		return nil, false, staleDeliveryRecoveryReason, nil
	}
	if !promptTurnBindingMatchesRequest(*binding, request) {
		return nil, false, deliveryIdentityMismatchReason, nil
	}
	request.PromptTurnBinding = binding
	return &agentruntime.AgentDeliveryRecoveryIdentity{
		OriginalRuntime: binding.OriginalRuntime, TaskID: binding.TaskID,
		SessionID: binding.SessionID, ExecutionID: binding.ExecutionID,
		SubmissionID: binding.SubmissionID, StreamID: binding.StreamID,
		IncarnationID: binding.IncarnationID, HarnessGeneration: binding.HarnessGeneration,
		PromptGeneration: binding.PromptGeneration,
	}, true, "", nil
}

func reconstructionControlIdentityMatchesRequest(identity agentruntime.AgentDeliveryRecoveryIdentity, request *models.AgentDeliveryReconstructionRequest) bool {
	return identity.TaskID == request.TaskID && identity.SessionID == request.SessionID &&
		identity.IncarnationID == request.IncarnationID && identity.HarnessGeneration == uint64(request.HarnessGeneration) &&
		identity.SubmissionID == request.Submission.ID && identity.StreamID == request.Recovery.StreamID &&
		identity.ExecutionID != "" && identity.PromptGeneration > 0 && identity.OriginalRuntime.Validate() == nil
}

func (s *Service) verifyReconstructionControlIdentity(
	ctx context.Context,
	client deliveryRecordEvidenceClient,
	evidence *agentruntime.DeliveryRecordEvidence,
	identity agentruntime.AgentDeliveryRecoveryIdentity,
	persistedBinding bool,
	request *models.AgentDeliveryReconstructionRequest,
) (string, error) {
	// Payload selection and process ownership are separate reads. Recheck the
	// exact process-bound prompt before committing a control identity.
	storedSubmissionID := ""
	if evidence.StoredSubmission != nil {
		storedSubmissionID = evidence.StoredSubmission.ID
	}
	latest, err := client.InspectDeliveryRecordEvidence(ctx, agentruntime.DeliveryRecordEvidenceRequest{
		TaskID: request.TaskID, SessionID: request.SessionID, IncarnationID: request.IncarnationID,
		HarnessGeneration: uint64(request.HarnessGeneration), StoredSubmissionID: storedSubmissionID,
	})
	if err != nil {
		return deliveryRecordEvidenceReason(err), nil
	}
	if latest == nil {
		return staleDeliveryRecoveryReason, nil
	}
	if !reconstructionControlIdentityMatchesLatest(evidence, latest, identity, persistedBinding, request) {
		return staleDeliveryRecoveryReason, nil
	}
	return "", nil
}

func reconstructionControlIdentityMatchesLatest(
	evidence, latest *agentruntime.DeliveryRecordEvidence,
	identity agentruntime.AgentDeliveryRecoveryIdentity,
	persistedBinding bool,
	request *models.AgentDeliveryReconstructionRequest,
) bool {
	if persistedBinding {
		if !persistedReconstructionIdentityMatches(evidence, latest, identity) {
			return false
		}
	} else if !liveReconstructionIdentityMatches(latest, identity) {
		return false
	}
	return reconstructionHistoryGapMatches(evidence, latest, request)
}

func persistedReconstructionIdentityMatches(
	evidence, latest *agentruntime.DeliveryRecordEvidence,
	identity agentruntime.AgentDeliveryRecoveryIdentity,
) bool {
	return latest.ProcessTerminated != nil && *latest.ProcessTerminated &&
		sameRestartDescriptor(evidence.Descriptor, latest.Descriptor) &&
		(latest.ControlIdentity == nil || *latest.ControlIdentity == identity)
}

func liveReconstructionIdentityMatches(
	latest *agentruntime.DeliveryRecordEvidence,
	identity agentruntime.AgentDeliveryRecoveryIdentity,
) bool {
	return latest.ControlIdentity != nil && *latest.ControlIdentity == identity
}

func reconstructionHistoryGapMatches(
	evidence, latest *agentruntime.DeliveryRecordEvidence,
	request *models.AgentDeliveryReconstructionRequest,
) bool {
	if request.Recovery.HistoryGap == nil {
		return true
	}
	return sameRestartDescriptor(evidence.Descriptor, latest.Descriptor) &&
		latest.ProcessTerminated != nil && *latest.ProcessTerminated
}

func (s *Service) persistedPromptTurnBinding(ctx context.Context, request *models.AgentDeliveryReconstructionRequest) (*models.AgentDeliveryPromptTurnBinding, error) {
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, request.SessionID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil, nil
	}
	if err != nil || running == nil {
		return nil, err
	}
	binding, ok := models.LoadAgentDeliveryPromptTurnBinding(running.Metadata)
	if !ok {
		return nil, nil
	}
	return &binding, nil
}

func promptTurnBindingMatchesRequest(binding models.AgentDeliveryPromptTurnBinding, request *models.AgentDeliveryReconstructionRequest) bool {
	return binding.TaskID == request.TaskID && binding.SessionID == request.SessionID &&
		binding.IncarnationID == request.IncarnationID && binding.HarnessGeneration == uint64(request.HarnessGeneration) &&
		binding.SubmissionID == request.Submission.ID && binding.StreamID == request.Recovery.StreamID &&
		binding.ExecutionID != "" && binding.TurnID != "" && binding.PromptGeneration > 0 &&
		binding.OriginalRuntime.Validate() == nil
}
