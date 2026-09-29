package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func (m *Manager) handleRuntimeAvailabilityChanged(_ context.Context, event *bus.Event) error {
	if event == nil || event.Type != events.AgentRuntimeAvailabilityChanged || m.runtimeOwner == nil {
		return nil
	}

	snapshot, ok := runtimeAvailabilitySnapshot(event.Data)
	if !ok || snapshot.RuntimeEpoch == 0 {
		return nil
	}
	ownerSnapshot, ownerPublished := m.runtimeOwner.Snapshot()
	if !ownerPublished || snapshot.BootID == "" || snapshot.BootID != ownerSnapshot.BootID {
		return nil
	}
	if snapshot.Status != agentctl.AvailabilityStatusUnavailable && snapshot.Status != agentctl.AvailabilityStatusRecovering {
		return nil
	}
	m.reconcileRuntimeLoss(snapshot.RuntimeEpoch)
	return nil
}

func runtimeAvailabilitySnapshot(data any) (agentctl.AvailabilitySnapshot, bool) {
	switch typed := data.(type) {
	case agentctl.AvailabilitySnapshot:
		return typed, true
	case *agentctl.AvailabilitySnapshot:
		if typed != nil {
			return *typed, true
		}
		return agentctl.AvailabilitySnapshot{}, false
	default:
		payload, err := json.Marshal(data)
		if err != nil {
			return agentctl.AvailabilitySnapshot{}, false
		}
		var decoded agentctl.AvailabilitySnapshot
		if err := json.Unmarshal(payload, &decoded); err != nil {
			return agentctl.AvailabilitySnapshot{}, false
		}
		return decoded, true
	}
}

type runtimeLossExecutionState struct {
	promptGeneration uint64
	submissionID     string
	reconcile        bool
	uncertain        bool
}

func (m *Manager) reconcileRuntimeLoss(runtimeEpoch uint64) {
	if runtimeEpoch == 0 || m.executionStore == nil {
		return
	}
	select {
	case <-m.stopCh:
		return
	default:
	}

	for _, execution := range m.ListExecutions() {
		state := m.runtimeLossState(execution, runtimeEpoch)
		if !state.reconcile {
			continue
		}

		disconnectErr := agentctl.ErrRuntimeLeaseRetired
		if state.uncertain {
			disconnectErr = errors.Join(disconnectErr, ErrUncertainPromptDelivery)
			if execution.DeliveryMode == DurableDeliveryV1 {
				m.logger.Debug("persisting uncertain delivery after confirmed local runtime loss",
					zap.String("execution_id", execution.ID),
					zap.String("session_id", execution.SessionID),
					zap.String("submission_id", state.submissionID),
					zap.Uint64("runtime_epoch", runtimeEpoch))
				m.recordRetiredRuntimeDeliveryRecovery(execution)
			}
		}
		m.handleStreamDisconnectWithStartupGeneration(
			execution,
			disconnectErr,
			state.promptGeneration,
			execution.startupAttemptSnapshot(),
		)
	}
}

func (m *Manager) recordRetiredRuntimeDeliveryRecovery(execution *AgentExecution) {
	if !m.isRetiredLocalExecution(execution) {
		return
	}
	identity, err := captureDeliveryReconciliationIdentity(execution)
	if err != nil {
		m.logger.Warn("cannot persist local runtime delivery loss without complete submission identity",
			zap.String("execution_id", execution.ID), zap.Error(err))
		return
	}
	m.recordRetiredRuntimeDeliveryRecoveryForIdentity(execution, identity)
}

func (m *Manager) recordRetiredRuntimeDeliveryRecoveryForIdentity(
	execution *AgentExecution,
	identity DeliveryReconciliationIdentity,
) {
	if !m.isRetiredLocalExecution(execution) {
		return
	}
	m.recordDeliveryReconciliationPhaseWithRetiredRuntime(
		execution, identity, DeliveryReconciliationPhaseUncertain,
	)
}

func (m *Manager) runtimeLossState(execution *AgentExecution, runtimeEpoch uint64) runtimeLossExecutionState {
	var state runtimeLossExecutionState
	if err := m.executionStore.WithRLock(execution.ID, func(current *AgentExecution) {
		if !matchesRuntimeLossExecution(current, execution, runtimeEpoch) {
			return
		}
		state.promptGeneration = current.promptGeneration
		state.submissionID = current.deliverySubmissionIDSnapshot()
		state.reconcile, state.uncertain = classifyRuntimeLossStatus(current.Status, state.submissionID)
	}); err != nil {
		return runtimeLossExecutionState{}
	}
	return state
}

func matchesRuntimeLossExecution(current, expected *AgentExecution, runtimeEpoch uint64) bool {
	return current == expected && current.RuntimeName == executor.NameStandalone && !current.IsPassthrough &&
		current.runtimeEpoch == runtimeEpoch
}

func classifyRuntimeLossStatus(status v1.AgentStatus, submissionID string) (bool, bool) {
	switch status {
	case v1.AgentStatusStarting:
		return true, submissionID != ""
	case v1.AgentStatusRunning:
		return true, true
	case v1.AgentStatusReady:
		return submissionID != "", submissionID != ""
	default:
		return false, false
	}
}

func (m *Manager) isRetiredLocalExecution(execution *AgentExecution) bool {
	return execution != nil && !execution.IsPassthrough && execution.RuntimeName == executor.NameStandalone &&
		execution.runtimeEpoch != 0 && !m.runtimeExecutionCurrent(execution)
}

func (m *Manager) isIdleSettledRetiredLocalExecution(execution *AgentExecution) bool {
	return m.isRetiredLocalExecution(execution) && execution.Status == v1.AgentStatusReady &&
		execution.deliverySubmissionIDSnapshot() == ""
}

func (m *Manager) runtimeReplacementRecoveryError(execution *AgentExecution) error {
	if !m.isRetiredLocalExecution(execution) {
		return nil
	}
	return &RestoreRequiredError{
		Decision: RestoreDecision{
			Outcome:                RestoreOutcomeBlocked,
			Reason:                 RestoreReasonUnknown,
			PreserveNativeIdentity: true,
		},
		Cause: fmt.Errorf("local runtime generation changed before prompt admission: %w", agentctl.ErrRuntimeLeaseRetired),
	}
}

func (m *Manager) retireStaleLocalExecution(execution *AgentExecution) {
	if execution == nil || !m.isRetiredLocalExecution(execution) {
		return
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	releaseClient()
	execution.agentctlLifecycleMu.Lock()
	execution.detachAgentctlClient()
	execution.agentctlLifecycleMu.Unlock()
	if client != nil {
		client.Close()
	}
	execution.EndSessionSpan()
	m.RemoveExecution(execution.ID)
}
