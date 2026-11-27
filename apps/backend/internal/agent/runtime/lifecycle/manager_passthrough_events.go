package lifecycle

import (
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

func (m *Manager) passthroughTurnCompleteExecution(sessionID, processID string, sequence uint64) *AgentExecution {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		m.logger.Debug("turn complete for unknown session (may have ended)",
			zap.String("session_id", sessionID))
		return nil
	}
	execution.passthroughLifecycleMu.Lock()
	activeProcessID := execution.PassthroughProcessID
	initialPromptPending := execution.passthroughInitialPromptProcessID == processID
	staleInitialIdle := sequence != 0 && sequence <= remotePassthroughRecordFor(execution).InitialSubmissionSequence
	execution.passthroughLifecycleMu.Unlock()
	if staleInitialIdle {
		return nil
	}
	if activeProcessID != processID {
		m.logger.Debug("ignoring stale passthrough turn complete",
			zap.String("session_id", sessionID),
			zap.String("process_id", processID),
			zap.String("active_process_id", activeProcessID))
		return nil
	}
	if initialPromptPending {
		m.logger.Debug("ignoring passthrough turn complete while initial prompt is pending",
			zap.String("session_id", sessionID),
			zap.String("process_id", processID))
		return nil
	}

	m.logger.Info("passthrough turn complete",
		zap.String("session_id", sessionID),
		zap.String("execution_id", execution.ID))

	return execution
}

func (m *Manager) handlePassthroughStatusForExecution(execution *AgentExecution, status *agentctltypes.ProcessStatusUpdate) {
	current, exists := m.executionStore.GetBySessionID(status.SessionID)
	if !exists || current != execution {
		return
	}
	// Convert to agentctl client type for event publisher
	clientStatus := &agentctl.ProcessStatusUpdate{
		SessionID:  status.SessionID,
		ProcessID:  status.ProcessID,
		Kind:       status.Kind,
		Command:    status.Command,
		ScriptName: status.ScriptName,
		WorkingDir: status.WorkingDir,
		Status:     status.Status,
		ExitCode:   status.ExitCode,
		Timestamp:  status.Timestamp,
	}

	m.eventPublisher.PublishProcessStatus(execution, clientStatus)

	// Check if process exited and should be auto-restarted
	// Only restart if this is the ACTUAL passthrough process, not user shell terminals
	// Run asynchronously to allow the old process to be cleaned up first
	if status.Status == agentctltypes.ProcessStatusExited || status.Status == agentctltypes.ProcessStatusFailed {
		// Only trigger auto-restart for the passthrough process, not for user shell terminals
		if execution.PassthroughProcessID != "" && status.ProcessID == execution.PassthroughProcessID {
			// Snapshot the start time and resume-launch flag synchronously so the
			// goroutine doesn't race the next launch's writes to those fields.
			startedAt := execution.PassthroughStartedAt
			usedResume := execution.passthroughLaunchUsedResume
			// A failed resume forces the next launch to start fresh. Record the
			// fallback before a terminal reconnect can reuse the failed intent.
			// Successful exits retain the provider conversation's resume intent.
			if usedResume && passthroughExitCode(status) != 0 {
				execution.passthroughResumeFailed = true
				execution.isResumedSession = false
				execution.passthroughLaunchUsedResume = false
			}
			go m.handlePassthroughExit(execution, status, startedAt, usedResume)
		} else {
			m.logger.Debug("process exited but not the passthrough process, skipping auto-restart",
				zap.String("session_id", status.SessionID),
				zap.String("exited_process_id", status.ProcessID),
				zap.String("passthrough_process_id", execution.PassthroughProcessID))
		}
	}
}
