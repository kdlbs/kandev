package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
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
	return m.bindResumeAttemptToExecution(execution, attemptID)
}

func (m *Manager) bindResumeAttemptToExecution(execution *AgentExecution, attemptID string) error {
	// Startup callbacks acquire startupCallbackMu before they take the
	// execution-store lock. Acquire the same locks in that order here so an
	// adopted execution cannot deadlock a callback while its identity is bound.
	execution.startupCallbackMu.Lock()
	defer execution.startupCallbackMu.Unlock()
	bound := false
	err := m.executionStore.WithLock(execution.ID, func(current *AgentExecution) {
		if current == execution {
			bound = current.bindStartupAttemptIDWithLease(attemptID)
		}
	})
	if errors.Is(err, ErrExecutionNotFound) || (err == nil && !bound) {
		return models.ErrExecutionRotated
	}
	return err
}
