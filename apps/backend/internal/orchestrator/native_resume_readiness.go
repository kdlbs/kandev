package orchestrator

import (
	"context"
	"errors"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

type bootDeliveryReadinessContextKey struct{}

func withBootDeliveryReadinessContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, bootDeliveryReadinessContextKey{}, true)
}

func isBootDeliveryReadinessContext(ctx context.Context) bool {
	active, _ := ctx.Value(bootDeliveryReadinessContextKey{}).(bool)
	return active
}

// Reconciliation uses a persisted fence even when boot or first admission is
// the first observer of interrupted delivery. Native restoration alone never
// authorizes a prompt; the transactional proof and a fresh capability do.
func (s *Service) reconcileUnresolvedDeliveryProtocol(ctx context.Context, sessionID, executionID string) (string, error) {
	recoveryErr := s.deliveryRecoveryError(ctx, sessionID, durableDeliveryUnresolvedReason)
	block, err := s.GetOpenSessionRecoveryBlock(ctx, sessionID)
	if err != nil {
		return "", requiredDeliveryRecoveryError(recoveryErr, block, err)
	}
	if block == nil {
		return "", recoveryErr
	}
	resolved, err := s.finishLoadedNativeDeliveryBlock(ctx, block)
	if err != nil {
		return "", requiredDeliveryRecoveryError(recoveryErr, block, err)
	}
	if !resolved {
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
		s.logBootDeliveryRecoveryRefusal(ctx, sessionID, err, nil)
		return false
	}
	if err := s.checkSessionRecoveryBlock(withBootDeliveryReadinessContext(ctx), sessionID); err != nil {
		return false
	}
	return true
}

func requiredDeliveryRecoveryError(existing error, block *models.SessionRecoveryBlock, cause error) *sessionRecoveryRequiredError {
	if block == nil {
		var prior *sessionRecoveryRequiredError
		if errors.As(existing, &prior) {
			block = prior.Block
		}
	}
	return &sessionRecoveryRequiredError{
		Block: block, DeliveryRefusalReason: deliveryRecoveryRefusalReason(cause), internalCause: cause,
	}
}

func (s *Service) logBootDeliveryRecoveryRefusal(ctx context.Context, sessionID string, refusal error, block *models.SessionRecoveryBlock) {
	_, active := s.resumeAttemptStore().current(sessionID)
	s.logDeliveryRecoveryRefusalAtLevel(ctx, sessionID, refusal, block, active)
}

func (s *Service) logDeliveryRecoveryRefusal(ctx context.Context, sessionID string, refusal error, block *models.SessionRecoveryBlock) {
	s.logDeliveryRecoveryRefusalAtLevel(ctx, sessionID, refusal, block, false)
}

func (s *Service) logDeliveryRecoveryRefusalAtLevel(ctx context.Context, sessionID string, refusal error, block *models.SessionRecoveryBlock, debug bool) {
	if block == nil {
		var required *sessionRecoveryRequiredError
		if errors.As(refusal, &required) && required.Block != nil {
			block = required.Block
		}
	}
	if block == nil {
		block, _ = s.GetOpenSessionRecoveryBlock(ctx, sessionID)
	}
	if block == nil || block.State != models.RecoveryBlockOpen || block.ConsumerReference != agentDeliveryConsumer {
		return
	}
	if sessionID == "" {
		sessionID = block.SessionID
	}
	reason := deliveryRecoveryRefusalReason(refusal)
	fields := []zap.Field{
		zap.String("session_id", sessionID),
		zap.String("block_id", block.ID),
		zap.String("reason", reason),
	}
	if cause, ok := deliveryRecoveryRefusalCauseField(refusal, reason); ok {
		fields = append(fields, cause)
	}
	if debug {
		s.logger.Debug("agent_delivery recovery remains required", fields...)
		return
	}
	s.logger.Warn("agent_delivery recovery remains required", fields...)
}

func deliveryRecoveryRefusalCauseField(refusal error, reason string) (zap.Field, bool) {
	var required *sessionRecoveryRequiredError
	switch {
	case errors.As(refusal, &required) && required.internalCause != nil:
		return zap.NamedError("cause", required.internalCause), true
	case refusal == nil:
		return zap.Field{}, false
	case reason == agentruntime.DeliveryRecoveryRefusalUnclassified:
		return zap.Error(refusal), true
	default:
		var reasoner agentruntime.DeliveryRecoveryRefusalReasoner
		if errors.As(refusal, &reasoner) {
			return zap.NamedError("cause", refusal), true
		}
		return zap.Field{}, false
	}
}

func deliveryRecoveryRefusalReason(err error) string {
	if err == nil {
		return agentruntime.DeliveryRecoveryRefusalUnclassified
	}
	var reasoner agentruntime.DeliveryRecoveryRefusalReasoner
	if errors.As(err, &reasoner) {
		if reason := reasoner.DeliveryRecoveryRefusalReason(); agentruntime.IsDeliveryRecoveryRefusalReason(reason) {
			return reason
		}
	}
	var required *sessionRecoveryRequiredError
	if errors.As(err, &required) && agentruntime.IsDeliveryRecoveryRefusalReason(required.DeliveryRefusalReason) {
		return required.DeliveryRefusalReason
	}
	switch {
	case errors.Is(err, agentruntime.ErrDeliveryOwnerMismatch), errors.Is(err, agentruntime.ErrDurableAdoptionIdentityMismatch):
		return agentruntime.DeliveryRecoveryRefusalNativeIdentityMismatch
	case errors.Is(err, agentruntime.ErrDeliveryRecoveryBlocked):
		return agentruntime.DeliveryRecoveryRefusalJournalUnresolved
	default:
		return agentruntime.DeliveryRecoveryRefusalUnclassified
	}
}
