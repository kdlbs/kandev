package scheduler

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// DefaultIdleTimeout is the default duration after which a terminal-state
// task's agentctl execution is cleaned up if no viewer is connected.
const DefaultIdleTimeout = 5 * time.Minute

// terminalTaskStates lists task states that are considered terminal.
var terminalTaskStates = map[string]bool{
	"COMPLETED": true,
	"CANCELLED": true,
	"FAILED":    true,
}

// IdleTimeoutManager manages idle timers for sessions whose tasks have
// reached a terminal state.
type IdleTimeoutManager struct {
	ss      *SchedulerService
	timeout time.Duration
	timers  map[string]*time.Timer // sessionID -> timer
	mu      sync.Mutex
	logger  *logger.Logger
}

// NewIdleTimeoutManager creates a new IdleTimeoutManager.
func NewIdleTimeoutManager(ss *SchedulerService, timeout time.Duration) *IdleTimeoutManager {
	if timeout <= 0 {
		timeout = DefaultIdleTimeout
	}
	return &IdleTimeoutManager{
		ss:      ss,
		timeout: timeout,
		timers:  make(map[string]*time.Timer),
		logger:  ss.logger.WithFields(zap.String("component", "idle-timeout")),
	}
}

// OnRunFinished is called after a run is marked as finished.
// It checks if the task is in a terminal state and starts an idle timer.
func (m *IdleTimeoutManager) OnRunFinished(sessionID, taskID string) {
	if sessionID == "" || taskID == "" {
		return
	}
	if !m.isTaskTerminal(taskID) {
		return
	}
	m.startTimer(sessionID)
}

// OnViewerConnected cancels any pending idle timer for the session.
func (m *IdleTimeoutManager) OnViewerConnected(sessionID string) {
	m.cancelTimer(sessionID)
}

// OnViewerDisconnected starts an idle timer if the task is terminal.
func (m *IdleTimeoutManager) OnViewerDisconnected(sessionID string, taskTerminal bool) {
	if !taskTerminal {
		return
	}
	m.startTimer(sessionID)
}

// isTaskTerminal checks if the task's current state is terminal.
func (m *IdleTimeoutManager) isTaskTerminal(taskID string) bool {
	fields, err := m.ss.repo.GetTaskExecutionFields(context.Background(), taskID)
	if err != nil {
		m.logger.Debug("failed to check task state for idle timeout",
			zap.String("task_id", taskID), zap.Error(err))
		return false
	}
	return terminalTaskStates[fields.State]
}

// startTimer starts (or restarts) the idle timer for a session.
func (m *IdleTimeoutManager) startTimer(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Cancel existing timer if present.
	if existing, ok := m.timers[sessionID]; ok {
		existing.Stop()
	}

	m.logger.Info("idle timeout started for terminal session",
		zap.String("session_id", sessionID),
		zap.Duration("timeout", m.timeout))

	m.timers[sessionID] = time.AfterFunc(m.timeout, func() {
		m.cleanup(sessionID)
	})
}

// cancelTimer cancels a pending idle timer for a session.
func (m *IdleTimeoutManager) cancelTimer(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if timer, ok := m.timers[sessionID]; ok {
		timer.Stop()
		delete(m.timers, sessionID)
		m.logger.Info("idle timeout cancelled (viewer connected)",
			zap.String("session_id", sessionID))
	}
}

// cleanup performs the actual cleanup when the idle timer fires.
func (m *IdleTimeoutManager) cleanup(sessionID string) {
	m.mu.Lock()
	delete(m.timers, sessionID)
	m.mu.Unlock()

	m.logger.Info("idle timeout expired, cleaning up execution",
		zap.String("session_id", sessionID))

	// Log activity. We don't have a workspace ID here, so use empty string.
	m.ss.svc.LogActivityWithRun(context.Background(), "",
		"system", "idle-timeout",
		"execution_idle_cleanup", "session", sessionID,
		mustJSON(map[string]string{
			"reason": "terminal task idle timeout",
		}), "", sessionID)
}

// PendingCount returns the number of active idle timers (for testing).
func (m *IdleTimeoutManager) PendingCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.timers)
}
