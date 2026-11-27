package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) bindInitialAgentDeliverySubmission(
	ctx context.Context,
	session *models.TaskSession,
	executionID string,
	options promptTaskOptions,
) (promptTaskOptions, error) {
	generatedIdentity := options.deliverySubmissionID == ""
	if generatedIdentity {
		protocol, err := s.resolveDeliveryProtocol(ctx, session.ID, executionID, options.deliveryProtocol)
		if err != nil {
			return options, err
		}
		if protocol == messagequeue.DeliveryProtocolLegacy {
			return options, nil
		}
		incarnationID := session.QueueIncarnationID
		if incarnationID == "" {
			incarnationID = session.ID
		}
		generation, err := s.currentDeliveryGeneration(ctx, session, incarnationID)
		if err != nil {
			return options, s.deliveryRecoveryError(ctx, session.ID, "harness_generation_unavailable")
		}
		// The initial lifecycle dispatch derives this identity from the same owner and generation.
		options.deliverySubmissionID = fmt.Sprintf("prompt:initial:%s:%d:%s", session.ID, generation, executionID)
		options.deliveryProtocol = protocol
		options.expectedDeliveryGeneration = generation
	}
	// Bind identity before startup. The final prepared payload is admitted by
	// the lifecycle callback immediately before native dispatch.
	setter, ok := s.agentManager.(executor.InitialDeliverySubmissionIDSetter)
	if !ok {
		return options, s.deliveryRecoveryError(ctx, session.ID, "backend_delivery_admission_failed")
	}
	if err := setter.SetInitialDeliverySubmissionID(ctx, executionID, options.deliverySubmissionID); err != nil {
		return options, s.deliveryRecoveryError(ctx, session.ID, "backend_delivery_admission_failed")
	}
	return options, nil
}
