package process

import (
	"time"

	"go.uber.org/zap"
)

func (r *InteractiveRunner) readOutput(proc *interactiveProcess) {
	buf := make([]byte, 32768) // 32KB buffer for better throughput
	recentOutput := ""         // Keep recent output for prompt pattern matching
	firstRead := true

	for {
		select {
		case <-proc.stopSignal:
			r.logger.Debug("readOutput: stop signal received",
				zap.String("process_id", proc.info.ID))
			return
		default:
		}

		proc.mu.Lock()
		ptyInstance := proc.ptmx
		proc.mu.Unlock()

		if ptyInstance == nil {
			r.logger.Debug("readOutput: pty is nil, exiting",
				zap.String("process_id", proc.info.ID))
			return
		}

		n, err := ptyInstance.Read(buf)
		if firstRead {
			r.logger.Info("readOutput: first read attempt",
				zap.String("process_id", proc.info.ID),
				zap.Int("bytes", n),
				zap.Error(err))
			firstRead = false
		}
		if n > 0 {
			recentOutput = r.processOutputData(proc, ptyInstance, buf[:n], recentOutput)
		}
		if err != nil {
			r.logger.Debug("interactive process output read ended",
				zap.String("process_id", proc.info.ID),
				zap.Error(err))
			return
		}
	}
}

// respondToTerminalQueries sends synthetic terminal responses (DSR/DA1) to the PTY
// when no direct output writer (real terminal) is connected yet.
func (r *InteractiveRunner) respondToTerminalQueries(proc *interactiveProcess, ptyInstance PtyHandle, data []byte) {
	if containsDSRQuery(data) {
		response := "\x1b[1;1R"
		if _, err := ptyInstance.Write([]byte(response)); err != nil {
			r.logger.Debug("failed to respond to cursor position query",
				zap.String("process_id", proc.info.ID),
				zap.Error(err))
		} else {
			r.logger.Debug("responded to cursor position query",
				zap.String("process_id", proc.info.ID))
		}
	}
	if containsDA1Query(data) {
		response := "\x1b[?1;2c"
		if _, err := ptyInstance.Write([]byte(response)); err != nil {
			r.logger.Debug("failed to respond to device attributes query",
				zap.String("process_id", proc.info.ID),
				zap.Error(err))
		}
	}
}

// processOutputData handles a chunk of output data read from the PTY.
// It responds to terminal queries, feeds the status tracker, buffers the output,
// routes it to the direct writer or event bus, and manages the prompt-pattern window.
// Returns the updated recentOutput string.
func (r *InteractiveRunner) processOutputData(proc *interactiveProcess, ptyInstance PtyHandle, data []byte, recentOutput string) string {
	dataStr := string(data)

	// Respond to cursor position queries (DSR) if no terminal is connected yet.
	// Some CLI tools (like Codex) query cursor position on startup with \e[6n
	// and expect a response \e[row;colR. Without this, they timeout and exit.
	// Only respond if no direct output writer is connected (no real terminal yet).
	proc.directOutputMu.RLock()
	hasDirectWriter := proc.directOutput != nil
	proc.directOutputMu.RUnlock()

	if !hasDirectWriter {
		r.respondToTerminalQueries(proc, ptyInstance, data)
	}

	// Feed to status tracker for TUI state detection
	if proc.statusTracker != nil {
		proc.statusTracker.Write(data)

		// Periodically check state (debounced by ShouldCheck)
		if proc.statusTracker.ShouldCheck() {
			newState := proc.statusTracker.CheckAndUpdate()
			if newState != proc.lastState {
				proc.lastState = newState
				r.handleStateChange(proc, newState)
			}
		}
	}

	// Always buffer output for scrollback restoration on reconnect
	chunk := ProcessOutputChunk{
		Stream:    "stdout",
		Data:      dataStr,
		Timestamp: time.Now().UTC(),
	}
	proc.buffer.append(chunk)

	// Check if we have a direct output writer (binary WebSocket mode)
	proc.directOutputMu.RLock()
	directWriter := proc.directOutput
	proc.directOutputMu.RUnlock()

	if directWriter != nil {
		// Direct mode: write raw bytes to the WebSocket
		if _, writeErr := directWriter.Write(data); writeErr != nil {
			r.logger.Debug("direct output write error",
				zap.String("process_id", proc.info.ID),
				zap.Error(writeErr))
			// Don't return - the writer might have closed but process continues
		}
	} else {
		// No WebSocket connected: also publish via event bus
		r.publishOutput(proc, chunk)
	}

	recentOutput = r.updateRecentOutput(recentOutput, dataStr)

	// Check prompt pattern for turn completion
	if proc.promptPattern != nil && proc.promptPattern.MatchString(recentOutput) {
		r.emitTurnComplete(proc)
		recentOutput = "" // Reset after match
	}

	// Reset idle timer on any output
	r.resetIdleTimer(proc)

	return recentOutput
}

// updateRecentOutput appends new output to the rolling 1KB window used for prompt
// pattern matching, trimming the oldest data to stay within the size limit.
func (r *InteractiveRunner) updateRecentOutput(recentOutput, dataStr string) string {
	// Update recent output for prompt pattern matching (keep last 1KB)
	// Trim before appending to prevent temporary memory spikes with large outputs
	maxSize := 1024
	if len(recentOutput)+len(dataStr) > maxSize {
		// Calculate how much to keep from existing buffer
		keepFromExisting := maxSize - len(dataStr)
		if keepFromExisting < 0 {
			keepFromExisting = 0
		}
		if keepFromExisting > 0 && len(recentOutput) > keepFromExisting {
			recentOutput = recentOutput[len(recentOutput)-keepFromExisting:]
		} else if keepFromExisting == 0 {
			recentOutput = ""
		}
	}
	recentOutput += dataStr
	// Final trim in case dataStr itself was larger than maxSize
	if len(recentOutput) > maxSize {
		recentOutput = recentOutput[len(recentOutput)-maxSize:]
	}
	return recentOutput
}

func (r *InteractiveRunner) resetIdleTimer(proc *interactiveProcess) {
	proc.idleTimerMu.Lock()
	defer proc.idleTimerMu.Unlock()

	if proc.idleTimer != nil {
		proc.idleTimer.Stop()
	}

	if proc.idleTimeout > 0 {
		proc.idleTimer = time.AfterFunc(proc.idleTimeout, func() {
			r.emitTurnComplete(proc)
		})
	}
}

func (r *InteractiveRunner) emitTurnComplete(proc *interactiveProcess) {
	if r.turnCompleteCallback != nil {
		r.turnCompleteCallback(proc.info.SessionID)
	}
	r.logger.Debug("turn complete detected",
		zap.String("process_id", proc.info.ID),
		zap.String("session_id", proc.info.SessionID))
}

// handleStateChange processes agent state changes detected by the status tracker.
func (r *InteractiveRunner) handleStateChange(proc *interactiveProcess, state AgentState) {
	r.logger.Debug("agent state changed",
		zap.String("process_id", proc.info.ID),
		zap.String("session_id", proc.info.SessionID),
		zap.String("state", string(state)))

	// WaitingInput state triggers turn complete
	if state == StateWaitingInput {
		r.emitTurnComplete(proc)
	}

	// Invoke the state callback for external handling
	if r.stateCallback != nil {
		r.stateCallback(proc.info.SessionID, state)
	}
}
