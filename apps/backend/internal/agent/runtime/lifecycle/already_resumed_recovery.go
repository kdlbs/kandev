package lifecycle

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/common/processidentity"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ReconcileAlreadyResumedDelivery keeps a loaded native successor while retiring
// only its proven-dead predecessor's matching unknown delivery history.
func (m *Manager) ReconcileAlreadyResumedDelivery(ctx context.Context, historical AgentDeliveryRecoveryIdentity, nativeID string) (bool, error) {
	execution, exists := m.GetExecutionBySessionID(historical.SessionID)
	if !exists || execution.ID == historical.ExecutionID {
		return false, nil
	}
	if !execution.promptMu.TryLock() {
		return false, ErrDeliveryRecoveryBlocked
	}
	defer execution.promptMu.Unlock()
	if !loadedRecoverySuccessorMatches(execution, historical) || nativeID == "" || execution.ACPSessionID != nativeID {
		return false, ErrDeliveryOwnerMismatch
	}
	if err := proveHistoricalDeliveryOwnerDead(historical); err != nil {
		return false, err
	}
	if err := m.AcknowledgeNativeResumeDelivery(ctx, historical.SessionID); err != nil {
		return false, err
	}
	return true, nil
}

func loadedRecoverySuccessorMatches(execution *AgentExecution, historical AgentDeliveryRecoveryIdentity) bool {
	return completeAgentDeliveryRecoveryIdentity(historical) && execution != nil &&
		execution.TaskID == historical.TaskID && execution.SessionID == historical.SessionID &&
		execution.DeliveryMode == DurableDeliveryV1 && execution.DeliveryIncarnationID == historical.IncarnationID &&
		execution.DeliveryHarnessGeneration == historical.HarnessGeneration &&
		execution.Status == v1.AgentStatusReady && !execution.dispatchedPromptPending.Load() &&
		execution.isSessionInitialized() && execution.ACPSessionID != ""
}

func proveHistoricalDeliveryOwnerDead(historical AgentDeliveryRecoveryIdentity) error {
	if historical.OriginalRuntime.Validate() != nil {
		return ErrDeliveryRecoveryBlocked
	}
	dead, err := processidentity.OwnedSessionTerminated(historical.OriginalRuntime)
	if err != nil || !dead {
		return fmt.Errorf("%w: historical delivery owner is not proven terminated", ErrDeliveryRecoveryBlocked)
	}
	return nil
}

func (m *Manager) recoverLoadedSuccessorDelivery(ctx context.Context, execution *AgentExecution, historical AgentDeliveryRecoveryIdentity) DeliveryReconciliationResult {
	identity := deliveryReconciliationIdentity(historical)
	if !execution.promptMu.TryLock() {
		return deliveryRecoveryResult(identity, DeliveryReconciliationBlocked, "successor_prompt_active", ErrDeliveryRecoveryBlocked)
	}
	defer execution.promptMu.Unlock()
	if !loadedRecoverySuccessorMatches(execution, historical) || proveHistoricalDeliveryOwnerDead(historical) != nil {
		return deliveryRecoveryResult(identity, DeliveryReconciliationOwnerMismatch, "execution_identity_changed", ErrDeliveryOwnerMismatch)
	}
	if m.runtimeOwner == nil {
		return deliveryRecoveryResult(identity, DeliveryReconciliationTransportUnavailable, "runtime_unavailable", ErrDeliveryTransportUnavailable)
	}
	lease, err := m.runtimeOwner.Acquire(ctx)
	if err != nil {
		return deliveryRecoveryResult(identity, DeliveryReconciliationTransportUnavailable, "runtime_unavailable", err)
	}
	defer lease.Close()
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil || client.RuntimeEpoch() != lease.Epoch() {
		return deliveryRecoveryResult(identity, DeliveryReconciliationOwnerMismatch, "runtime_owner_changed", ErrDeliveryOwnerMismatch)
	}
	return m.recoverRetainedInstance(ctx, lease, client, historical)
}
