package orchestrator

import (
	"context"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

// Reconciliation uses a persisted fence even when boot or first admission is
// the first observer of interrupted delivery. Native restoration alone never
// authorizes a prompt; the transactional proof and a fresh capability do.
func (s *Service) reconcileUnresolvedDeliveryProtocol(ctx context.Context, sessionID, executionID string) (string, error) {
	recoveryErr := s.deliveryRecoveryError(ctx, sessionID, durableDeliveryUnresolvedReason)
	block, err := s.GetOpenSessionRecoveryBlock(ctx, sessionID)
	if err != nil || block == nil {
		return "", recoveryErr
	}
	resolved, err := s.finishLoadedNativeDeliveryBlock(ctx, block)
	if err != nil || !resolved {
		return "", recoveryErr
	}
	decision, advertised := s.deliveryCapabilityForExecution(ctx, executionID)
	if advertised && decision.Mode == agentruntime.DurableDeliveryV1 && decision.Reason != durableDeliveryUnresolvedReason {
		return messagequeue.DeliveryProtocolV1, nil
	}
	return "", s.deliveryRecoveryError(ctx, sessionID, durableDeliveryUnresolvedReason)
}

func (s *Service) prepareBootDeliveryReadiness(ctx context.Context, sessionID, executionID string) bool {
	if executionID == "" {
		return true
	}
	if _, err := s.resolveDeliveryProtocol(ctx, sessionID, executionID, messagequeue.DeliveryProtocolPending); err != nil {
		return false
	}
	return s.checkSessionRecoveryBlock(ctx, sessionID) == nil
}
