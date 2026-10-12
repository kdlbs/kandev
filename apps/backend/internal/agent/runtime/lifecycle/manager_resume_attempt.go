package lifecycle

import (
	"context"
	"fmt"
)

// BindResumeAttempt attaches a prompt-owned recovery identity to the startup
// generation of an execution that is already being initialized. The execution
// store guard makes the session-to-execution lookup and identity update one
// mutation, so a replacement execution cannot be rebound accidentally.
func (m *Manager) BindResumeAttempt(_ context.Context, sessionID, attemptID string) error {
	if m == nil || m.executionStore == nil {
		return fmt.Errorf("execution store is unavailable")
	}
	if sessionID == "" || attemptID == "" {
		return fmt.Errorf("session ID and attempt ID are required")
	}

	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists || execution == nil {
		return ErrExecutionNotFound
	}
	// Startup callbacks acquire startupCallbackMu before they take the
	// execution-store lock. Acquire the same locks in that order here so an
	// adopted execution cannot deadlock a callback while its identity is bound.
	execution.startupCallbackMu.Lock()
	defer execution.startupCallbackMu.Unlock()
	bound := false
	err := m.executionStore.WithLock(execution.ID, func(current *AgentExecution) {
		if current == execution && current.SessionID == sessionID {
			bound = current.bindStartupAttemptIDWithLease(attemptID)
		}
	})
	if err != nil {
		return err
	}
	if !bound {
		return fmt.Errorf("execution %q has no startup generation", execution.ID)
	}
	return nil
}
