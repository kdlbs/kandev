package websocket

import (
	"bytes"
	"encoding/json"
	"io"
	"sync"

	gorillaws "github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/server/process"
)

// wsWriter wraps a gorilla WebSocket to implement process.DirectOutputWriter.
// It writes binary frames to the WebSocket.
type wsWriter struct {
	conn   *gorillaws.Conn
	mu     sync.Mutex
	closed bool
}

func newWsWriter(conn *gorillaws.Conn) *wsWriter {
	return &wsWriter{conn: conn}
}

func (w *wsWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return 0, io.ErrClosedPipe
	}

	if err := w.conn.WriteMessage(gorillaws.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *wsWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return nil
}

// bridgeBinaryWebSockets forwards binary WebSocket frames bidirectionally between
// a client connection and an agentctl connection until either side disconnects.
func (h *TerminalHandler) bridgeBinaryWebSockets(clientConn, agentctlConn *gorillaws.Conn, terminalID string) {
	done := make(chan struct{})

	// agentctl → client (shell output)
	go func() {
		defer close(done)
		for {
			msgType, data, err := agentctlConn.ReadMessage()
			if err != nil {
				if !gorillaws.IsCloseError(err, gorillaws.CloseNormalClosure, gorillaws.CloseGoingAway) {
					h.logger.Debug("remote terminal shell upstream read error",
						zap.String("terminal_id", terminalID),
						zap.Error(err))
				}
				return
			}
			if err := clientConn.WriteMessage(msgType, data); err != nil {
				h.logger.Debug("remote terminal shell client write error",
					zap.String("terminal_id", terminalID),
					zap.Error(err))
				return
			}
		}
	}()

	// client → agentctl (input + resize)
	go func() {
		for {
			msgType, data, err := clientConn.ReadMessage()
			if err != nil {
				if !gorillaws.IsCloseError(err, gorillaws.CloseNormalClosure, gorillaws.CloseGoingAway) {
					h.logger.Debug("remote terminal shell client read error",
						zap.String("terminal_id", terminalID),
						zap.Error(err))
				}
				_ = agentctlConn.Close()
				return
			}
			if err := agentctlConn.WriteMessage(msgType, data); err != nil {
				h.logger.Debug("remote terminal shell upstream write error",
					zap.String("terminal_id", terminalID),
					zap.Error(err))
				return
			}
		}
	}()

	<-done
}

// runTerminalBridge handles WebSocket I/O and manages the PTY connection.
// It waits for the first resize to trigger process start, then sets up bidirectional I/O.
func (h *TerminalHandler) runTerminalBridge(
	conn *gorillaws.Conn,
	sessionID string,
	processID string,
	interactiveRunner *process.InteractiveRunner,
	wsw *wsWriter,
) {
	var ptyWriter io.Writer
	var directOutputSet bool

	defer func() {
		// Clean up: clear direct output at session level (process may have been restarted with new ID)
		interactiveRunner.ClearDirectOutputBySession(sessionID)
		_ = wsw.Close()
		_ = conn.Close()

		h.logger.Info("terminal WebSocket disconnected",
			zap.String("session_id", sessionID))
	}()

	// Read from WebSocket and handle input/resize
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if !gorillaws.IsCloseError(err, gorillaws.CloseNormalClosure, gorillaws.CloseGoingAway) {
				h.logger.Debug("WebSocket read error",
					zap.String("session_id", sessionID),
					zap.Error(err))
			}
			return
		}

		if messageType != gorillaws.BinaryMessage && messageType != gorillaws.TextMessage {
			continue
		}

		if len(data) == 0 {
			continue
		}

		if isResizeCommand(data) {
			newID, newDirect := h.handleResizeCommand(data[1:], sessionID, processID, interactiveRunner, wsw, directOutputSet, &ptyWriter)
			processID = newID
			directOutputSet = newDirect
			continue
		}

		newID, newDirect := h.handleTerminalInput(data, sessionID, processID, interactiveRunner, wsw, &ptyWriter, directOutputSet)
		processID = newID
		directOutputSet = newDirect
	}
}

// handleResizeCommand processes a resize message in the terminal bridge loop.
// Returns the (possibly updated) processID and directOutputSet flag.
func (h *TerminalHandler) handleResizeCommand(
	payload []byte,
	sessionID string,
	processID string,
	interactiveRunner *process.InteractiveRunner,
	wsw *wsWriter,
	directOutputSet bool,
	ptyWriter *io.Writer,
) (string, bool) {
	// Set up PTY access BEFORE handling resize if not already done.
	// This ensures buffered output is sent to the terminal before the resize
	// triggers a TUI redraw (which would overwrite the scrollback).
	if !directOutputSet {
		if h.setupPtyAccess(sessionID, processID, interactiveRunner, wsw, ptyWriter) {
			directOutputSet = true
		}
	}

	// Handle resize — may return updated processID if process was restarted.
	// Falls back to session-level resize when the old process is gone.
	newProcessID := h.handleResize(payload, sessionID, processID, interactiveRunner)
	if newProcessID != processID {
		processID = newProcessID
		directOutputSet = false
		*ptyWriter = nil
	}

	// Retry setupPtyAccess after resize — process may have been lazily started
	// by the resize (deferred-start), so the PTY is now available.
	if !directOutputSet {
		if h.setupPtyAccess(sessionID, processID, interactiveRunner, wsw, ptyWriter) {
			directOutputSet = true
		}
	}

	return processID, directOutputSet
}

// handleTerminalInput processes regular (non-resize) input in the terminal bridge loop.
// Returns the (possibly updated) processID and directOutputSet flag.
func (h *TerminalHandler) handleTerminalInput(
	data []byte,
	sessionID string,
	processID string,
	interactiveRunner *process.InteractiveRunner,
	wsw *wsWriter,
	ptyWriter *io.Writer,
	directOutputSet bool,
) (string, bool) {
	if *ptyWriter != nil {
		return h.writeToReadyPty(data, sessionID, processID, interactiveRunner, wsw, ptyWriter, directOutputSet)
	}
	return h.writeToUnreadyPty(data, sessionID, processID, interactiveRunner, wsw, ptyWriter, directOutputSet)
}

// writeToReadyPty writes input to an already-established PTY writer.
// If writing fails it resets state and discovers the new process.
func (h *TerminalHandler) writeToReadyPty(
	data []byte,
	sessionID string,
	processID string,
	interactiveRunner *process.InteractiveRunner,
	wsw *wsWriter,
	ptyWriter *io.Writer,
	directOutputSet bool,
) (string, bool) {
	if _, err := (*ptyWriter).Write(data); err != nil {
		h.logger.Debug("PTY write error",
			zap.String("session_id", sessionID),
			zap.String("process_id", processID),
			zap.Error(err))

		// Reset state and discover the new process immediately.
		// Without this, the bridge would be stuck with the old processID
		// until a resize message arrives (which may never come after restart).
		*ptyWriter = nil
		directOutputSet = false
		processID = h.discoverProcessBySession(sessionID, processID, interactiveRunner)
		return processID, directOutputSet
	}
	// Detect Enter key (user submitted input) - notify lifecycle manager
	h.detectInputSubmission(sessionID, data)
	return processID, directOutputSet
}

// writeToUnreadyPty attempts to set up PTY access and then write input.
// If setup fails it tries to discover the current process by session.
func (h *TerminalHandler) writeToUnreadyPty(
	data []byte,
	sessionID string,
	processID string,
	interactiveRunner *process.InteractiveRunner,
	wsw *wsWriter,
	ptyWriter *io.Writer,
	directOutputSet bool,
) (string, bool) {
	// PTY not ready — try setup. If it fails, processID may be stale
	// (process was restarted with a new ID). Discover the current process
	// by session before giving up.
	if !h.setupPtyAccess(sessionID, processID, interactiveRunner, wsw, ptyWriter) {
		newID := h.discoverProcessBySession(sessionID, processID, interactiveRunner)
		if newID != processID {
			processID = newID
			h.setupPtyAccess(sessionID, processID, interactiveRunner, wsw, ptyWriter)
		}
	}
	if *ptyWriter == nil {
		h.logger.Debug("PTY not ready, dropping input",
			zap.String("session_id", sessionID),
			zap.Int("bytes", len(data)))
		return processID, directOutputSet
	}
	directOutputSet = true
	if _, err := (*ptyWriter).Write(data); err != nil {
		h.logger.Debug("PTY write error after setup",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return processID, directOutputSet
	}
	h.detectInputSubmission(sessionID, data)
	return processID, directOutputSet
}

// detectInputSubmission checks if the input contains Enter key and notifies
// the lifecycle manager to update the execution state to Running.
func (h *TerminalHandler) detectInputSubmission(sessionID string, data []byte) {
	// Check for Enter key (CR or LF)
	hasEnter := false
	for _, b := range data {
		if b == '\n' || b == '\r' {
			hasEnter = true
			break
		}
	}

	if hasEnter {
		// Notify lifecycle manager that user submitted input
		if err := h.lifecycleMgr.MarkPassthroughRunning(sessionID); err != nil {
			// Log at debug level - may fail if session ended or not in passthrough mode
			h.logger.Debug("failed to mark passthrough as running",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}
}

// setupPtyAccess attempts to get PTY writer and set up direct output.
// Returns true if successful.
func (h *TerminalHandler) setupPtyAccess(
	sessionID string,
	processID string,
	interactiveRunner *process.InteractiveRunner,
	wsw *wsWriter,
	ptyWriter *io.Writer,
) bool {
	getWriter := func() (io.Writer, error) {
		return interactiveRunner.GetPtyWriter(processID)
	}
	return h.setupPtyAccessCommon(sessionID, processID, interactiveRunner, wsw, ptyWriter, getWriter)
}

// handleResize processes a resize command from the WebSocket.
// Uses process-specific resize to avoid targeting the wrong process when multiple
// processes exist for the same session (e.g., passthrough + user shell).
// If the process was restarted (old process gone), falls back to session-level resize
// and returns the new process ID.
func (h *TerminalHandler) handleResize(
	data []byte,
	sessionID string,
	processID string,
	interactiveRunner *process.InteractiveRunner,
) (effectiveProcessID string) {
	effectiveProcessID = processID

	var resize ResizePayload
	if err := json.Unmarshal(data, &resize); err != nil {
		h.logger.Warn("failed to parse resize command",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return effectiveProcessID
	}

	if resize.Cols == 0 || resize.Rows == 0 {
		h.logger.Warn("invalid resize dimensions",
			zap.String("session_id", sessionID),
			zap.Uint16("cols", resize.Cols),
			zap.Uint16("rows", resize.Rows))
		return effectiveProcessID
	}

	// Resize the specific process by ID to avoid targeting the wrong process.
	// If the process was restarted (old ID gone), fall back to session-level resize.
	resizeErr := interactiveRunner.ResizeByProcessID(processID, resize.Cols, resize.Rows)
	if resizeErr == nil {
		h.logger.Debug("PTY resized",
			zap.String("session_id", sessionID),
			zap.String("process_id", processID),
			zap.Uint16("cols", resize.Cols),
			zap.Uint16("rows", resize.Rows))
	}

	if resizeErr != nil {
		h.logger.Debug("process-specific resize failed, trying session-level fallback",
			zap.String("session_id", sessionID),
			zap.String("process_id", processID),
			zap.Error(resizeErr))
		effectiveProcessID = h.resizeBySessionFallback(sessionID, processID, resize.Cols, resize.Rows, interactiveRunner)
	}

	return effectiveProcessID
}

// discoverProcessBySession looks up the current passthrough process for a session.
// Returns the new processID if found and different from oldProcessID.
// This is used when the bridge has a stale processID after a process restart.
func (h *TerminalHandler) discoverProcessBySession(
	sessionID string,
	oldProcessID string,
	interactiveRunner *process.InteractiveRunner,
) string {
	_, newProcID, _ := interactiveRunner.GetPtyWriterBySession(sessionID)
	if newProcID != "" && newProcID != oldProcessID {
		h.logger.Info("discovered new process by session lookup",
			zap.String("session_id", sessionID),
			zap.String("old_process_id", oldProcessID),
			zap.String("new_process_id", newProcID))
		return newProcID
	}
	return oldProcessID
}

// resizeBySessionFallback attempts a session-level resize when the process-specific
// resize failed (e.g., process was restarted). Returns the new processID if found.
func (h *TerminalHandler) resizeBySessionFallback(
	sessionID string,
	oldProcessID string,
	cols, rows uint16,
	interactiveRunner *process.InteractiveRunner,
) string {
	sessionErr := interactiveRunner.ResizeBySession(sessionID, cols, rows)
	if sessionErr != nil {
		h.logger.Warn("failed to resize PTY (both process and session)",
			zap.String("session_id", sessionID),
			zap.String("process_id", oldProcessID),
			zap.Uint16("cols", cols),
			zap.Uint16("rows", rows),
			zap.Error(sessionErr))
		return oldProcessID
	}

	// Session-level resize succeeded — look up the new process ID
	_, newProcID, _ := interactiveRunner.GetPtyWriterBySession(sessionID)
	if newProcID != "" && newProcID != oldProcessID {
		h.logger.Info("resize fallback succeeded, process ID updated",
			zap.String("session_id", sessionID),
			zap.String("old_process_id", oldProcessID),
			zap.String("new_process_id", newProcID))
		return newProcID
	}

	return oldProcessID
}

// runUserShellBridge handles WebSocket I/O for user shell terminals.
func (h *TerminalHandler) runUserShellBridge(
	conn *gorillaws.Conn,
	sessionID string,
	scopeID string,
	terminalID string,
	processID string,
	interactiveRunner *process.InteractiveRunner,
	wsw *wsWriter,
) {
	var ptyWriter io.Writer
	var directOutputSet bool

	defer func() {
		// Clean up WebSocket resources but DON'T stop the process
		// The process should only be stopped via explicit user_shell.stop message from frontend
		// This allows reconnection after React remounts or temporary disconnects
		interactiveRunner.ClearUserShellDirectOutput(scopeID, terminalID)

		_ = wsw.Close()
		_ = conn.Close()

		h.logger.Info("user shell WebSocket disconnected (process still running for potential reconnect)",
			zap.String("session_id", sessionID),
			zap.String("terminal_id", terminalID))
	}()

	// Read from WebSocket and handle input/resize
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if !gorillaws.IsCloseError(err, gorillaws.CloseNormalClosure, gorillaws.CloseGoingAway) {
				h.logger.Debug("WebSocket read error",
					zap.String("session_id", sessionID),
					zap.String("terminal_id", terminalID),
					zap.Error(err))
			}
			return
		}

		if messageType != gorillaws.BinaryMessage && messageType != gorillaws.TextMessage {
			continue
		}

		if len(data) == 0 {
			continue
		}

		if isResizeCommand(data) {
			directOutputSet = h.handleUserShellResizeCommand(
				data[1:], sessionID, scopeID, terminalID, processID,
				interactiveRunner, wsw, directOutputSet, &ptyWriter,
			)
			continue
		}

		directOutputSet = h.handleUserShellInput(
			data, sessionID, scopeID, terminalID, processID,
			interactiveRunner, wsw, &ptyWriter, directOutputSet,
		)
	}
}

// handleUserShellResizeCommand processes a resize message in the user shell bridge loop.
// Returns the updated directOutputSet flag.
func (h *TerminalHandler) handleUserShellResizeCommand(
	payload []byte,
	sessionID string,
	scopeID string,
	terminalID string,
	processID string,
	interactiveRunner *process.InteractiveRunner,
	wsw *wsWriter,
	directOutputSet bool,
	ptyWriter *io.Writer,
) bool {
	// Set up PTY access BEFORE handling resize if not already done
	if !directOutputSet {
		if h.setupUserShellPtyAccess(sessionID, scopeID, terminalID, processID, interactiveRunner, wsw, ptyWriter) {
			directOutputSet = true
		}
	}
	h.handleUserShellResize(payload, sessionID, scopeID, terminalID, interactiveRunner)
	return directOutputSet
}

// handleUserShellInput processes regular (non-resize) input in the user shell bridge loop.
// Returns the updated directOutputSet flag.
func (h *TerminalHandler) handleUserShellInput(
	data []byte,
	sessionID string,
	scopeID string,
	terminalID string,
	processID string,
	interactiveRunner *process.InteractiveRunner,
	wsw *wsWriter,
	ptyWriter *io.Writer,
	directOutputSet bool,
) bool {
	if *ptyWriter != nil {
		return h.writeToReadyUserShellPty(data, sessionID, scopeID, terminalID, processID, interactiveRunner, wsw, ptyWriter, directOutputSet)
	}
	return h.writeToUnreadyUserShellPty(data, sessionID, scopeID, terminalID, processID, interactiveRunner, wsw, ptyWriter, directOutputSet)
}

// writeToReadyUserShellPty writes input to an established user shell PTY.
// On failure it attempts to re-establish PTY access and retries the write.
func (h *TerminalHandler) writeToReadyUserShellPty(
	data []byte,
	sessionID string,
	scopeID string,
	terminalID string,
	processID string,
	interactiveRunner *process.InteractiveRunner,
	wsw *wsWriter,
	ptyWriter *io.Writer,
	directOutputSet bool,
) bool {
	if _, err := (*ptyWriter).Write(data); err != nil {
		h.logger.Debug("PTY write error",
			zap.String("session_id", sessionID),
			zap.String("terminal_id", terminalID),
			zap.Error(err))
		// Try to re-establish PTY access
		if h.setupUserShellPtyAccess(sessionID, scopeID, terminalID, processID, interactiveRunner, wsw, ptyWriter) {
			directOutputSet = true
			if _, err := (*ptyWriter).Write(data); err != nil {
				h.logger.Debug("PTY write error after reconnect",
					zap.String("session_id", sessionID),
					zap.String("terminal_id", terminalID),
					zap.Error(err))
			}
		}
	}
	return directOutputSet
}

// writeToUnreadyUserShellPty sets up PTY access and writes input for a user shell.
// Drops the input if setup fails.
func (h *TerminalHandler) writeToUnreadyUserShellPty(
	data []byte,
	sessionID string,
	scopeID string,
	terminalID string,
	processID string,
	interactiveRunner *process.InteractiveRunner,
	wsw *wsWriter,
	ptyWriter *io.Writer,
	directOutputSet bool,
) bool {
	if h.setupUserShellPtyAccess(sessionID, scopeID, terminalID, processID, interactiveRunner, wsw, ptyWriter) {
		directOutputSet = true
		if _, err := (*ptyWriter).Write(data); err != nil {
			h.logger.Debug("PTY write error after setup",
				zap.String("session_id", sessionID),
				zap.String("terminal_id", terminalID),
				zap.Error(err))
		}
	} else {
		h.logger.Debug("PTY not ready, dropping input",
			zap.String("session_id", sessionID),
			zap.String("terminal_id", terminalID),
			zap.Int("bytes", len(data)))
	}
	return directOutputSet
}

// setupUserShellPtyAccess sets up PTY access for a user shell terminal.
func (h *TerminalHandler) setupUserShellPtyAccess(
	sessionID string,
	scopeID string,
	terminalID string,
	processID string,
	interactiveRunner *process.InteractiveRunner,
	wsw *wsWriter,
	ptyWriter *io.Writer,
) bool {
	getWriter := func() (io.Writer, error) {
		writer, _, err := interactiveRunner.GetUserShellPtyWriter(scopeID, terminalID)
		return writer, err
	}
	return h.setupPtyAccessCommon(sessionID, processID, interactiveRunner, wsw, ptyWriter, getWriter)
}

// replayBufferedOutput flushes any buffered PTY output for a process to the WebSocket
// before direct output is wired up. This is what gives a reconnecting terminal its
// scrollback history. Terminal query response sequences are stripped first so they
// don't render as visible garbage on replay.
func (h *TerminalHandler) replayBufferedOutput(
	sessionID string,
	processID string,
	interactiveRunner *process.InteractiveRunner,
	wsw *wsWriter,
) {
	chunks, ok := interactiveRunner.GetBuffer(processID)
	if !ok || len(chunks) == 0 {
		return
	}
	var combined bytes.Buffer
	for _, chunk := range chunks {
		combined.WriteString(chunk.Data)
	}
	if combined.Len() == 0 {
		return
	}
	cleaned := stripTerminalResponses(combined.Bytes())
	h.logger.Debug("sending buffered output to terminal",
		zap.String("session_id", sessionID),
		zap.Int("chunks", len(chunks)),
		zap.Int("total_bytes", combined.Len()),
		zap.Int("cleaned_bytes", len(cleaned)))
	if len(cleaned) == 0 {
		return
	}
	if _, writeErr := wsw.Write(cleaned); writeErr != nil {
		h.logger.Warn("failed to send buffered output",
			zap.String("session_id", sessionID),
			zap.Error(writeErr))
	}
}

// setupPtyAccessCommon is the shared implementation for setupPtyAccess and setupUserShellPtyAccess.
// It gets a PTY writer via getWriter, sends buffered output, and sets up direct output.
func (h *TerminalHandler) setupPtyAccessCommon(
	sessionID string,
	processID string,
	interactiveRunner *process.InteractiveRunner,
	wsw *wsWriter,
	ptyWriter *io.Writer,
	getWriter func() (io.Writer, error),
) bool {
	writer, err := getWriter()
	if err != nil {
		h.logger.Debug("PTY writer not ready yet",
			zap.String("session_id", sessionID),
			zap.String("process_id", processID))
		return false
	}

	// Send buffered output to the terminal before switching to direct mode.
	// This provides scrollback history when reconnecting to an existing session.
	h.replayBufferedOutput(sessionID, processID, interactiveRunner, wsw)

	// Set up direct output
	if err := interactiveRunner.SetDirectOutput(processID, wsw); err != nil {
		h.logger.Error("failed to set direct output",
			zap.String("session_id", sessionID),
			zap.String("process_id", processID),
			zap.Error(err))
		return false
	}

	*ptyWriter = writer

	h.logger.Info("PTY access established",
		zap.String("session_id", sessionID),
		zap.String("process_id", processID))

	return true
}

// handleUserShellResize processes resize commands for user shell terminals.
func (h *TerminalHandler) handleUserShellResize(
	data []byte,
	sessionID string,
	scopeID string,
	terminalID string,
	interactiveRunner *process.InteractiveRunner,
) {
	var resize ResizePayload
	if err := json.Unmarshal(data, &resize); err != nil {
		h.logger.Warn("failed to parse resize command",
			zap.String("session_id", sessionID),
			zap.String("terminal_id", terminalID),
			zap.Error(err))
		return
	}

	if resize.Cols == 0 || resize.Rows == 0 {
		h.logger.Warn("invalid resize dimensions",
			zap.String("session_id", sessionID),
			zap.String("terminal_id", terminalID),
			zap.Uint16("cols", resize.Cols),
			zap.Uint16("rows", resize.Rows))
		return
	}

	if err := interactiveRunner.ResizeUserShell(scopeID, terminalID, resize.Cols, resize.Rows); err != nil {
		h.logger.Warn("failed to resize user shell PTY",
			zap.String("session_id", sessionID),
			zap.String("terminal_id", terminalID),
			zap.Uint16("cols", resize.Cols),
			zap.Uint16("rows", resize.Rows),
			zap.Error(err))
	} else {
		h.logger.Debug("user shell PTY resized",
			zap.String("session_id", sessionID),
			zap.String("terminal_id", terminalID),
			zap.Uint16("cols", resize.Cols),
			zap.Uint16("rows", resize.Rows))
	}
}
