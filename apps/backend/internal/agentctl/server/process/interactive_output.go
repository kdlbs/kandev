package process

import (
	"fmt"
	"io"

	"go.uber.org/zap"
)

// SetDirectOutput sets a direct output writer for a process.
// When set, PTY output bypasses the event bus and goes directly to this writer.
// This is used for the dedicated binary WebSocket in passthrough mode.
// For non-user-shell processes, also tracks the WebSocket at the session level
// to survive process restarts. User shells are excluded from session-level tracking
// to prevent overwriting the agent terminal's WebSocket.
// Returns error if process not found.
func (r *InteractiveRunner) SetDirectOutput(processID string, writer DirectOutputWriter) error {
	proc, ok := r.get(processID)
	if !ok {
		return fmt.Errorf("process not found: %s", processID)
	}

	sessionID := proc.info.SessionID

	proc.directOutputMu.Lock()
	proc.directOutput = writer
	proc.hasActiveWebSocket = true
	proc.directOutputMu.Unlock()

	// Track at session level for restart support (agent passthrough only).
	// User shells have their own tracking via userShells map and must not
	// overwrite the agent terminal's session-level WebSocket.
	if !proc.isUserShell {
		r.sessionWsMu.Lock()
		r.sessionWs[sessionID] = &sessionWebSocket{writer: writer}
		r.sessionWsMu.Unlock()
	}

	r.logger.Info("direct output set for process",
		zap.String("process_id", processID),
		zap.String("session_id", sessionID),
		zap.Bool("is_user_shell", proc.isUserShell))

	return nil
}

// ClearDirectOutput clears the direct output writer for a process.
// Output will return to the normal event bus path.
// For non-user-shell processes, also clears the session-level WebSocket tracking.
func (r *InteractiveRunner) ClearDirectOutput(processID string) error {
	proc, ok := r.get(processID)
	if !ok {
		// Process may have been deleted - still try to clear session tracking
		r.logger.Debug("process not found during ClearDirectOutput, trying to clear by session",
			zap.String("process_id", processID))
		return nil
	}

	sessionID := proc.info.SessionID

	proc.directOutputMu.Lock()
	proc.directOutput = nil
	proc.hasActiveWebSocket = false
	proc.directOutputMu.Unlock()

	// Clear session-level tracking (agent passthrough only)
	if !proc.isUserShell {
		r.sessionWsMu.Lock()
		delete(r.sessionWs, sessionID)
		r.sessionWsMu.Unlock()
	}

	r.logger.Info("direct output cleared for process",
		zap.String("process_id", processID),
		zap.String("session_id", sessionID))

	return nil
}

// ClearDirectOutputBySession clears the direct output for a session.
// This is used when the agent terminal WebSocket disconnects.
// Only clears non-user-shell processes to avoid disrupting user shell terminals.
func (r *InteractiveRunner) ClearDirectOutputBySession(sessionID string) {
	// Clear session-level tracking
	r.sessionWsMu.Lock()
	delete(r.sessionWs, sessionID)
	r.sessionWsMu.Unlock()

	// Clear from non-user-shell processes with this session
	r.mu.RLock()
	for _, proc := range r.processes {
		if proc.info.SessionID == sessionID && !proc.isUserShell {
			proc.directOutputMu.Lock()
			proc.directOutput = nil
			proc.hasActiveWebSocket = false
			proc.directOutputMu.Unlock()
		}
	}
	r.mu.RUnlock()

	r.logger.Info("direct output cleared for session",
		zap.String("session_id", sessionID))
}

// ConnectSessionWebSocket connects an existing session WebSocket to a process.
// This is called when a new process starts for a session that already has an active WebSocket.
// Returns true if a WebSocket was connected.
func (r *InteractiveRunner) ConnectSessionWebSocket(processID string) bool {
	proc, ok := r.get(processID)
	if !ok {
		return false
	}

	sessionID := proc.info.SessionID

	r.sessionWsMu.RLock()
	sessWs, exists := r.sessionWs[sessionID]
	r.sessionWsMu.RUnlock()

	if !exists || sessWs == nil {
		return false
	}

	sessWs.mu.RLock()
	writer := sessWs.writer
	sessWs.mu.RUnlock()

	if writer == nil {
		return false
	}

	proc.directOutputMu.Lock()
	proc.directOutput = writer
	proc.hasActiveWebSocket = true
	proc.directOutputMu.Unlock()

	r.logger.Info("connected existing session WebSocket to new process",
		zap.String("process_id", processID),
		zap.String("session_id", sessionID))

	return true
}

// GetPtyWriter returns a writer for sending input to the PTY.
// This is used for the dedicated binary WebSocket in passthrough mode.
func (r *InteractiveRunner) GetPtyWriter(processID string) (io.Writer, error) {
	proc, ok := r.get(processID)
	if !ok {
		return nil, fmt.Errorf("process not found: %s", processID)
	}

	proc.mu.Lock()
	defer proc.mu.Unlock()

	if !proc.started {
		return nil, fmt.Errorf("process not started yet - waiting for terminal dimensions")
	}

	if proc.ptmx == nil {
		return nil, fmt.Errorf("PTY not available")
	}

	return proc.ptmx, nil
}

// GetPtyWriterBySession returns a writer for sending input to the PTY for a session.
// This is used to reconnect after a process restart.
// Skips user shell processes to avoid conflicts with passthrough processes.
func (r *InteractiveRunner) GetPtyWriterBySession(sessionID string) (io.Writer, string, error) {
	r.mu.RLock()
	var proc *interactiveProcess
	for _, p := range r.processes {
		if p.info.SessionID == sessionID && !p.isUserShell {
			proc = p
			break
		}
	}
	r.mu.RUnlock()

	if proc == nil {
		return nil, "", fmt.Errorf("no process found for session: %s", sessionID)
	}

	proc.mu.Lock()
	defer proc.mu.Unlock()

	if !proc.started {
		return nil, proc.info.ID, fmt.Errorf("process not started yet - waiting for terminal dimensions")
	}

	if proc.ptmx == nil {
		return nil, proc.info.ID, fmt.Errorf("PTY not available")
	}

	return proc.ptmx, proc.info.ID, nil
}

// HasActiveWebSocket checks if a process has an active WebSocket connection.
// This is used to determine if auto-restart should be attempted on process exit.
func (r *InteractiveRunner) HasActiveWebSocket(processID string) bool {
	proc, ok := r.get(processID)
	if !ok {
		return false
	}

	proc.directOutputMu.RLock()
	defer proc.directOutputMu.RUnlock()
	return proc.hasActiveWebSocket
}

// HasActiveWebSocketBySession checks if a session has an active WebSocket connection.
// Uses session-level tracking which survives process restarts.
func (r *InteractiveRunner) HasActiveWebSocketBySession(sessionID string) bool {
	r.sessionWsMu.RLock()
	sessWs, exists := r.sessionWs[sessionID]
	r.sessionWsMu.RUnlock()

	if !exists || sessWs == nil {
		return false
	}

	sessWs.mu.RLock()
	hasWriter := sessWs.writer != nil
	sessWs.mu.RUnlock()

	return hasWriter
}

// WriteToDirectOutput writes data directly to the WebSocket output for a process.
// This is used to send messages like restart notifications to the terminal.
// Returns error if process not found or no direct output is set.
func (r *InteractiveRunner) WriteToDirectOutput(processID string, data []byte) error {
	proc, ok := r.get(processID)
	if !ok {
		return fmt.Errorf("process not found: %s", processID)
	}

	proc.directOutputMu.RLock()
	directWriter := proc.directOutput
	proc.directOutputMu.RUnlock()

	if directWriter == nil {
		return fmt.Errorf("no direct output writer set for process: %s", processID)
	}

	_, err := directWriter.Write(data)
	return err
}

// WriteToDirectOutputBySession writes data directly to the WebSocket output for a session.
// This is used to send messages like restart notifications to the terminal.
// Uses session-level tracking which survives process restarts.
func (r *InteractiveRunner) WriteToDirectOutputBySession(sessionID string, data []byte) error {
	r.sessionWsMu.RLock()
	sessWs, exists := r.sessionWs[sessionID]
	r.sessionWsMu.RUnlock()

	if !exists || sessWs == nil {
		return fmt.Errorf("no WebSocket found for session: %s", sessionID)
	}

	sessWs.mu.RLock()
	writer := sessWs.writer
	sessWs.mu.RUnlock()

	if writer == nil {
		return fmt.Errorf("no direct output writer set for session: %s", sessionID)
	}

	_, err := writer.Write(data)
	return err
}
