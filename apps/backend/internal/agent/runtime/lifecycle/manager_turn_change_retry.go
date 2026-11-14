package lifecycle

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"go.uber.org/zap"
)

const turnChangeMaximumAttempts = 3

// Retries retain the captured terminal identity, including across the inline
// completion attempt and its background continuation.
func (m *Manager) retryTurnChangeOperation(
	execution *AgentExecution, terminal TurnChangeTerminal, completed int, timeout time.Duration,
	operation func(context.Context, TurnChangeTerminal, TurnChangeCheckpointClient) error,
) error {
	var err error
	for attempt := completed; attempt < turnChangeMaximumAttempts; attempt++ {
		if !m.waitTurnChangeRetry(attempt) {
			return context.Canceled
		}
		client, release := execution.AcquireAgentCtlClient()
		ctx, cancel := m.turnChangeAttemptContext(timeout)
		err = operation(ctx, terminal, client)
		cancel()
		release()
		if err == nil || permanentTurnChangeError(err) {
			return err
		}
	}
	return err
}

func (m *Manager) waitTurnChangeRetry(attempt int) bool {
	delay := time.Duration(0)
	if attempt > 0 {
		delay = min(turnChangeRetryInitialDelay<<uint(attempt-1), turnChangeRetryMaximumDelay)
	}
	return m.waitTurnChangeDelay(delay)
}

func (m *Manager) waitTurnChangeDelay(delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-m.stopCh:
		return false
	case <-timer.C:
		return !m.shuttingDown.Load()
	}
}

// Every leaf of a joined error must be permanent. An unrelated storage failure
// must retain its retry policy even when it accompanies a missing summary.
func permanentTurnChangeError(err error) bool {
	if err == repoerrors.ErrTurnChangeSetNotFound || err == repoerrors.ErrTurnChangeRelationship {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !permanentTurnChangeError(child) {
				return false
			}
		}
		return true
	}
	if wrapped := errors.Unwrap(err); wrapped != nil {
		return permanentTurnChangeError(wrapped)
	}
	return false
}

func (m *Manager) abandonTurnChange(execution *AgentExecution, terminal TurnChangeTerminal, cause error) bool {
	select {
	case <-m.stopCh:
		return false
	default:
	}
	handler, ok := m.turnChangeCaptureHandler.(interface {
		AbandonTurnChanges(context.Context, TurnChangeTerminal, TurnChangeCheckpointClient) error
	})
	if !ok {
		m.logger.Error("turn-change terminal failure has no settlement handler",
			zap.String("execution_id", terminal.ExecutionID), zap.Error(cause))
		return false
	}
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	ctx, cancel := m.turnChangeAttemptContext(turnChangeTerminalCaptureTimeout)
	defer cancel()
	err := handler.AbandonTurnChanges(ctx, terminal, client)
	m.logger.Warn("turn-change processing stopped after terminal failure",
		zap.String("execution_id", terminal.ExecutionID), zap.String("turn_id", terminal.TurnID),
		zap.Uint64("prompt_generation", terminal.PromptGeneration), zap.Error(cause), zap.NamedError("settlement_error", err))
	return err == nil
}

// settleTurnChange continues only the durable terminal write after processing
// has stopped. Every attempt keeps the captured terminal identity, and
// ownership is re-checked before each write; shutdown cancels the wait.
func (m *Manager) settleTurnChange(
	execution *AgentExecution, terminal TurnChangeTerminal, cause error, stillOwned func() bool,
) bool {
	if _, ok := m.turnChangeCaptureHandler.(interface {
		AbandonTurnChanges(context.Context, TurnChangeTerminal, TurnChangeCheckpointClient) error
	}); !ok {
		return m.abandonTurnChange(execution, terminal, cause)
	}
	delay := time.Duration(0)
	for {
		if !m.waitTurnChangeDelay(delay) || !stillOwned() {
			return false
		}
		if m.abandonTurnChange(execution, terminal, cause) {
			return true
		}
		if delay == 0 {
			delay = turnChangeRetryInitialDelay
		} else {
			delay = min(delay*2, turnChangeRetryMaximumDelay)
		}
	}
}

// settleTurnChangeCapture continues only the durable terminal write after
// capture has stopped. Storage uncertainty keeps the original fence held;
// neither backoff nor an attempt deadline authorizes a successor prompt.
func (m *Manager) settleTurnChangeCapture(execution *AgentExecution, terminal TurnChangeTerminal, cause error) bool {
	return m.settleTurnChange(execution, terminal, cause, func() bool {
		return turnChangeCaptureStillOwned(execution, terminal)
	})
}

// settleTurnChangeSummary continues only the durable settlement after the
// summary budget is spent. Capture already released its fence, so the terminal
// identity is the ownership boundary: the repository settles only that exact
// interval and treats a missing or superseded one as a durable no-op.
func (m *Manager) settleTurnChangeSummary(execution *AgentExecution, terminal TurnChangeTerminal, cause error) bool {
	return m.settleTurnChange(execution, terminal, cause, func() bool {
		return execution.ID == terminal.ExecutionID && execution.currentStartupAttemptID() == terminal.StartupAttemptID
	})
}

func turnChangeCaptureStillOwned(execution *AgentExecution, terminal TurnChangeTerminal) bool {
	execution.startupCallbackMu.RLock()
	defer execution.startupCallbackMu.RUnlock()
	if execution.ID != terminal.ExecutionID || execution.currentStartupAttemptID() != terminal.StartupAttemptID {
		return false
	}
	execution.promptLifecycleMu.Lock()
	defer execution.promptLifecycleMu.Unlock()
	return execution.turnChangeCaptureGeneration == terminal.PromptGeneration
}

func finishRetriedTurnChangeCapture(execution *AgentExecution, terminal TurnChangeTerminal) {
	execution.startupCallbackMu.RLock()
	defer execution.startupCallbackMu.RUnlock()
	if execution.ID != terminal.ExecutionID || execution.currentStartupAttemptID() != terminal.StartupAttemptID {
		return
	}
	execution.promptLifecycleMu.Lock()
	defer execution.promptLifecycleMu.Unlock()
	clearTurnChangeCaptureAnchor(execution, terminal.PromptGeneration)
	finishTurnChangeCaptureLocked(execution, terminal.PromptGeneration)
}

func (m *Manager) startTurnChangeRetry(work func()) {
	if m == nil || m.stopCh == nil || m.shuttingDown.Load() {
		return
	}
	m.turnChangeRetryMu.Lock()
	defer m.turnChangeRetryMu.Unlock()
	select {
	case <-m.stopCh:
		return
	default:
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		work()
	}()
}
