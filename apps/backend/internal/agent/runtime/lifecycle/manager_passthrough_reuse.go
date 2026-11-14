package lifecycle

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// hasLiveLocalPassthroughLocked requires both process lifecycle locks.
func (m *Manager) hasLiveLocalPassthroughLocked(execution *AgentExecution) bool {
	if isSSHPassthrough(execution) || execution.PassthroughProcessID == "" {
		return false
	}
	runner := m.passthroughRunnerForExecution(execution)
	return runner != nil && runner.IsProcessReadyOrPending(execution.PassthroughProcessID)
}

func (m *Manager) hasLiveLocalPassthrough(execution *AgentExecution) (bool, error) {
	execution.remoteInstanceLifecycleMu.Lock()
	defer execution.remoteInstanceLifecycleMu.Unlock()
	execution.passthroughLifecycleMu.Lock()
	defer execution.passthroughLifecycleMu.Unlock()
	current, exists := m.executionStore.Get(execution.ID)
	if !exists || current != execution {
		return false, models.ErrExecutionRotated
	}
	return m.hasLiveLocalPassthroughLocked(execution), nil
}

func (m *Manager) bindPassthroughStartupOrBegin(ctx context.Context, execution *AgentExecution, passthrough bool) error {
	attemptID := ResumeAttemptIDFromContext(ctx)
	if passthrough {
		live, err := m.hasLiveLocalPassthrough(execution)
		if err != nil {
			return err
		}
		if live {
			if attemptID == "" {
				return nil
			}
			// The process already owns its startup generation and accepted MCP setup.
			// Release process locks before acquiring the startup callback writer.
			return m.bindResumeAttemptToExecution(execution, attemptID)
		}
	}
	execution.beginStartupAttemptWithID(attemptID)
	return nil
}

// IsLocalPassthroughReadyForResume distinguishes current READY from process liveness.
// Callers must still validate SQL state and prompt ownership at their admission boundary.
func (m *Manager) IsLocalPassthroughReadyForResume(sessionID, executionID, attemptID string) bool {
	if attemptID == "" {
		return false
	}
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists || execution.ID != executionID || isSSHPassthrough(execution) {
		return false
	}
	// Resume can publish MCP evidence while holding these locks. Its subscriber
	// needs the caller's cancellation guard, so contention must defer this check.
	if !execution.remoteInstanceLifecycleMu.TryLock() {
		return false
	}
	defer execution.remoteInstanceLifecycleMu.Unlock()
	if !execution.passthroughLifecycleMu.TryLock() {
		return false
	}
	defer execution.passthroughLifecycleMu.Unlock()
	current, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists || current != execution || !m.hasLiveLocalPassthroughLocked(execution) {
		return false
	}
	runner := m.GetInteractiveRunner()
	if runner == nil || !runner.HasObservedFirstIdle(execution.PassthroughProcessID) {
		return false
	}
	ready := false
	err := m.executionStore.WithRLock(execution.ID, func(current *AgentExecution) {
		ready = current.Status == v1.AgentStatusReady && current.currentStartupAttemptID() == attemptID
	})
	return err == nil && ready
}
