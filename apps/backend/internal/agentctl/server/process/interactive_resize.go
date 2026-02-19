package process

import (
	"fmt"

	"go.uber.org/zap"
)

// lazyStartAndResize handles the common pattern of lazy-starting a process on the first
// resize and then resizing its PTY and status tracker. All three public Resize* methods
// delegate here to avoid duplicating the start-once + resize logic.
func (r *InteractiveRunner) lazyStartAndResize(proc *interactiveProcess, cols, rows uint16, logFields ...zap.Field) error {
	// Lazy start: spawn process on first resize when we have exact dimensions
	var startErr error
	proc.startOnce.Do(func() {
		fields := append([]zap.Field{zap.Uint16("cols", cols), zap.Uint16("rows", rows)}, logFields...)
		r.logger.Info("first resize received - starting process", fields...)
		startErr = r.startProcess(proc, int(cols), int(rows))
	})
	if startErr != nil {
		return fmt.Errorf("failed to start process on first resize: %w", startErr)
	}

	proc.mu.Lock()
	ptyInstance := proc.ptmx
	statusTracker := proc.statusTracker
	proc.mu.Unlock()

	// Resize the PTY (Unix: SIGWINCH, Windows: ConPTY dimensions)
	if ptyInstance != nil {
		if err := ptyInstance.Resize(cols, rows); err != nil {
			return fmt.Errorf("failed to resize PTY: %w", err)
		}
	}

	// Also resize the status tracker's virtual terminal (if present)
	if statusTracker != nil {
		statusTracker.Resize(int(cols), int(rows))
	}

	// Store dimensions at session level so restarted processes use the correct size
	if !proc.isUserShell {
		r.sessionWsMu.RLock()
		sessWs, exists := r.sessionWs[proc.info.SessionID]
		r.sessionWsMu.RUnlock()
		if exists && sessWs != nil {
			sessWs.mu.Lock()
			sessWs.lastCols = cols
			sessWs.lastRows = rows
			sessWs.mu.Unlock()
		}
	}

	return nil
}

// ResizeByProcessID resizes the PTY for a specific process by its ID.
// On first resize, this triggers lazy process start at the exact frontend dimensions.
// This is preferred over ResizeBySession when the process ID is known, as it avoids
// ambiguity when multiple processes exist for the same session.
func (r *InteractiveRunner) ResizeByProcessID(processID string, cols, rows uint16) error {
	proc, ok := r.get(processID)
	if !ok {
		return fmt.Errorf("process not found: %s", processID)
	}

	if err := r.lazyStartAndResize(proc, cols, rows,
		zap.String("process_id", processID),
		zap.String("session_id", proc.info.SessionID),
	); err != nil {
		return err
	}

	r.logger.Debug("resized PTY",
		zap.String("process_id", processID),
		zap.String("session_id", proc.info.SessionID),
		zap.Uint16("cols", cols),
		zap.Uint16("rows", rows))

	return nil
}

// ResizeBySession resizes the PTY for a process by session ID.
// On first resize, this triggers lazy process start at the exact frontend dimensions.
// Skips user shell processes to avoid conflicts with passthrough processes.
func (r *InteractiveRunner) ResizeBySession(sessionID string, cols, rows uint16) error {
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
		return fmt.Errorf("no process found for session %s", sessionID)
	}

	if err := r.lazyStartAndResize(proc, cols, rows,
		zap.String("session_id", sessionID),
	); err != nil {
		return err
	}

	r.logger.Debug("resized PTY",
		zap.String("session_id", sessionID),
		zap.Uint16("cols", cols),
		zap.Uint16("rows", rows))

	return nil
}
