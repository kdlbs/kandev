// Package websocket provides WebSocket handlers for the gateway.
package websocket

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/scripts"
)

// UserService interface for getting user preferences.
type UserService interface {
	PreferredShell(ctx context.Context) (string, error)
}

// TerminalHandler handles dedicated binary WebSocket connections for terminal I/O.
// This bypasses JSON encoding for raw PTY communication via xterm.js AttachAddon.
type TerminalHandler struct {
	lifecycleMgr  *lifecycle.Manager
	userService   UserService
	scriptService scripts.ScriptService
	logger        *logger.Logger
}

// NewTerminalHandler creates a new TerminalHandler instance.
func NewTerminalHandler(lifecycleMgr *lifecycle.Manager, userService UserService, scriptService scripts.ScriptService, log *logger.Logger) *TerminalHandler {
	return &TerminalHandler{
		lifecycleMgr:  lifecycleMgr,
		userService:   userService,
		scriptService: scriptService,
		logger:        log.WithFields(zap.String("component", "terminal_handler")),
	}
}

// terminalUpgrader is the WebSocket upgrader for terminal connections.
// Uses larger buffers for better TUI performance.
var terminalUpgrader = gorillaws.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     checkWebSocketOrigin,
}

// checkWebSocketOrigin validates the Origin header for WebSocket connections.
// This prevents cross-site WebSocket hijacking attacks.
func checkWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// No origin header - allow (could be a non-browser client)
		return true
	}

	// Allow localhost origins for development
	if strings.HasPrefix(origin, "http://localhost") ||
		strings.HasPrefix(origin, "http://127.0.0.1") ||
		strings.HasPrefix(origin, "https://localhost") ||
		strings.HasPrefix(origin, "https://127.0.0.1") {
		return true
	}

	// Check same-origin: Origin should match the Host header
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}

	// Parse the origin URL to get its host
	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}

	// Compare hosts (ignoring port for flexibility)
	originHost := originURL.Hostname()
	requestHost := host
	if colonIdx := strings.LastIndex(requestHost, ":"); colonIdx != -1 {
		// Strip port from host if present (but be careful with IPv6)
		if !strings.Contains(requestHost, "]") || colonIdx > strings.Index(requestHost, "]") {
			requestHost = requestHost[:colonIdx]
		}
	}

	return originHost == requestHost
}

// resizeCommandByte is the binary protocol marker for resize messages.
// First byte 0x01 indicates resize, followed by JSON {cols, rows}.
const resizeCommandByte = 0x01

// ResizePayload is the JSON payload for resize commands.
type ResizePayload struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

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

// HandleTerminalWS handles WebSocket connections at /terminal/:sessionId
// This creates a binary WebSocket bridge between xterm.js and the PTY.
// Query parameter "mode" determines the type of terminal:
// - "agent": Agent passthrough terminal (CLI passthrough mode)
// - "shell": Independent user shell terminal (requires terminalId query param)
func (h *TerminalHandler) HandleTerminalWS(c *gin.Context) {
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sessionId is required"})
		return
	}

	// Get the interactive runner first
	interactiveRunner := h.lifecycleMgr.GetInteractiveRunner()
	if interactiveRunner == nil {
		h.logger.Error("interactive runner not available")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "interactive runner not available"})
		return
	}

	// Route based on explicit mode parameter
	mode := c.Query("mode")
	switch mode {
	case "agent":
		h.logger.Info("terminal WebSocket connection request",
			zap.String("session_id", sessionID),
			zap.String("mode", "agent"),
			zap.String("remote_addr", c.Request.RemoteAddr))
		h.handleAgentPassthroughWS(c, sessionID, interactiveRunner)

	case "shell":
		terminalID := c.Query("terminalId")
		if terminalID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "terminalId required for shell mode"})
			return
		}
		h.logger.Info("terminal WebSocket connection request",
			zap.String("session_id", sessionID),
			zap.String("mode", "shell"),
			zap.String("terminal_id", terminalID),
			zap.String("remote_addr", c.Request.RemoteAddr))
		h.handleUserShellWS(c, sessionID, terminalID, interactiveRunner)

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode query param required: agent or shell"})
	}
}

// handleAgentPassthroughWS handles WebSocket connections for agent passthrough terminals.
// It ensures a passthrough execution exists, upgrades to WebSocket, and bridges I/O.
func (h *TerminalHandler) handleAgentPassthroughWS(
	c *gin.Context,
	sessionID string,
	interactiveRunner *process.InteractiveRunner,
) {
	// Ensure passthrough execution exists and is running.
	// This handles:
	// 1. Normal case: execution exists with running process
	// 2. Backend restart: no execution, need to create and start
	// 3. Process died: execution exists but process not running, need to restart
	execution, err := h.lifecycleMgr.EnsurePassthroughExecution(c.Request.Context(), sessionID)
	if err != nil {
		h.logger.Warn("failed to ensure passthrough execution",
			zap.String("session_id", sessionID),
			zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	processID := execution.PassthroughProcessID
	if processID == "" {
		h.logger.Error("passthrough process ID is empty after ensure",
			zap.String("session_id", sessionID))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "passthrough process not started"})
		return
	}

	// Verify the process exists (either running or pending start for deferred-start processes)
	if !interactiveRunner.IsProcessReadyOrPending(processID) {
		h.logger.Error("passthrough process not found or exited",
			zap.String("session_id", sessionID),
			zap.String("process_id", processID))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "passthrough process failed to start"})
		return
	}

	// Upgrade to WebSocket - we'll get PTY access after the first resize
	// triggers the lazy process start
	conn, err := terminalUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("failed to upgrade to WebSocket",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	h.logger.Info("terminal WebSocket connected",
		zap.String("session_id", sessionID),
		zap.String("process_id", processID))

	// Create WebSocket writer for output
	wsw := newWsWriter(conn)

	// Run the terminal bridge
	h.runTerminalBridge(conn, sessionID, processID, interactiveRunner, wsw)
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

		// Check for resize command (first byte 0x01)
		if data[0] == resizeCommandByte {
			// Set up PTY access BEFORE handling resize if not already done.
			// This ensures buffered output is sent to the terminal before the resize
			// triggers a TUI redraw (which would overwrite the scrollback).
			if !directOutputSet {
				if h.setupPtyAccess(sessionID, processID, interactiveRunner, wsw, &ptyWriter) {
					directOutputSet = true
				}
			}

			// Handle resize — may return updated processID if process was restarted.
			// Falls back to session-level resize when the old process is gone.
			newProcessID := h.handleResize(data[1:], sessionID, processID, interactiveRunner)
			if newProcessID != processID {
				processID = newProcessID
				directOutputSet = false
				ptyWriter = nil
			}

			// Retry setupPtyAccess after resize — process may have been lazily started
			// by the resize (deferred-start), so the PTY is now available.
			if !directOutputSet {
				if h.setupPtyAccess(sessionID, processID, interactiveRunner, wsw, &ptyWriter) {
					directOutputSet = true
				}
			}
			continue
		}

		// Regular input - write to PTY if available
		if ptyWriter != nil {
			if _, err := ptyWriter.Write(data); err != nil {
				h.logger.Debug("PTY write error",
					zap.String("session_id", sessionID),
					zap.String("process_id", processID),
					zap.Error(err))

				// Reset state and discover the new process immediately.
				// Without this, the bridge would be stuck with the old processID
				// until a resize message arrives (which may never come after restart).
				ptyWriter = nil
				directOutputSet = false
				processID = h.discoverProcessBySession(sessionID, processID, interactiveRunner)
				continue
			}
			// Detect Enter key (user submitted input) - notify lifecycle manager
			h.detectInputSubmission(sessionID, data)
		} else {
			// PTY not ready — try setup. If it fails, processID may be stale
			// (process was restarted with a new ID). Discover the current process
			// by session before giving up.
			if !h.setupPtyAccess(sessionID, processID, interactiveRunner, wsw, &ptyWriter) {
				newID := h.discoverProcessBySession(sessionID, processID, interactiveRunner)
				if newID != processID {
					processID = newID
					h.setupPtyAccess(sessionID, processID, interactiveRunner, wsw, &ptyWriter)
				}
			}
			if ptyWriter == nil {
				h.logger.Debug("PTY not ready, dropping input",
					zap.String("session_id", sessionID),
					zap.Int("bytes", len(data)))
				continue
			}
			directOutputSet = true
			if _, err := ptyWriter.Write(data); err != nil {
				h.logger.Debug("PTY write error after setup",
					zap.String("session_id", sessionID),
					zap.Error(err))
				continue
			}
			h.detectInputSubmission(sessionID, data)
		}
	}
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

// handleUserShellWS handles WebSocket connections for user shell terminals.
// Each terminal tab gets its own independent shell process.
func (h *TerminalHandler) handleUserShellWS(
	c *gin.Context,
	sessionID string,
	terminalID string,
	interactiveRunner *process.InteractiveRunner,
) {
	// Get optional parameters from query
	scriptID := c.Query("scriptId") // Repository script ID to run
	labelParam := c.Query("label")  // Label for plain shell terminals

	var label string
	var initialCommand string

	// If scriptId is provided, look up the script from the database
	if scriptID != "" && h.scriptService != nil {
		script, err := h.scriptService.GetRepositoryScript(c.Request.Context(), scriptID)
		if err != nil {
			h.logger.Error("failed to get repository script",
				zap.String("script_id", scriptID),
				zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid script ID"})
			return
		}
		label = script.Name
		initialCommand = script.Command
		h.logger.Info("handleUserShellWS: resolved script",
			zap.String("script_id", scriptID),
			zap.String("script_name", script.Name),
			zap.String("script_command", script.Command))
	} else if labelParam != "" {
		// Use label from query param for plain shell terminals
		label = labelParam
	}

	h.logger.Info("handleUserShellWS: starting user shell handling",
		zap.String("session_id", sessionID),
		zap.String("terminal_id", terminalID),
		zap.String("script_id", scriptID),
		zap.String("label", label),
		zap.String("initial_command", initialCommand))

	// Get preferred shell from user settings
	var preferredShell string
	if h.userService != nil {
		shell, err := h.userService.PreferredShell(c.Request.Context())
		if err != nil {
			h.logger.Debug("failed to get preferred shell, using default",
				zap.Error(err))
		} else {
			preferredShell = shell
		}
	}

	// Get working directory from execution (workspace path)
	workingDir := ""
	if execution, exists := h.lifecycleMgr.GetExecutionBySessionID(sessionID); exists {
		workingDir = execution.WorkspacePath
		h.logger.Info("handleUserShellWS: got working directory from execution",
			zap.String("working_dir", workingDir))
	} else {
		h.logger.Info("handleUserShellWS: no execution found, using empty working directory")
	}

	// Build shell options
	opts := &process.UserShellOptions{
		Label:          label,
		InitialCommand: initialCommand,
	}

	// Start or get existing user shell for this terminal
	h.logger.Info("handleUserShellWS: calling StartUserShell",
		zap.String("session_id", sessionID),
		zap.String("terminal_id", terminalID),
		zap.String("working_dir", workingDir),
		zap.String("preferred_shell", preferredShell),
		zap.String("label", opts.Label),
		zap.String("initial_command", opts.InitialCommand))

	info, err := interactiveRunner.StartUserShell(
		c.Request.Context(),
		sessionID,
		terminalID,
		workingDir,
		preferredShell,
		opts,
	)
	if err != nil {
		h.logger.Error("failed to start user shell",
			zap.String("session_id", sessionID),
			zap.String("terminal_id", terminalID),
			zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	processID := info.ID
	h.logger.Info("handleUserShellWS: user shell started successfully",
		zap.String("session_id", sessionID),
		zap.String("terminal_id", terminalID),
		zap.String("process_id", processID))

	// Upgrade to WebSocket
	conn, err := terminalUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("failed to upgrade to WebSocket",
			zap.String("session_id", sessionID),
			zap.String("terminal_id", terminalID),
			zap.Error(err))
		return
	}

	h.logger.Info("user shell WebSocket connected",
		zap.String("session_id", sessionID),
		zap.String("terminal_id", terminalID),
		zap.String("process_id", processID))

	// Create WebSocket writer for output
	wsw := newWsWriter(conn)

	// Run the user shell terminal bridge
	h.runUserShellBridge(conn, sessionID, terminalID, processID, interactiveRunner, wsw)
}

// runUserShellBridge handles WebSocket I/O for user shell terminals.
func (h *TerminalHandler) runUserShellBridge(
	conn *gorillaws.Conn,
	sessionID string,
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
		interactiveRunner.ClearUserShellDirectOutput(sessionID, terminalID)

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

		// Check for resize command (first byte 0x01)
		if data[0] == resizeCommandByte {
			// Set up PTY access BEFORE handling resize if not already done
			if !directOutputSet {
				if h.setupUserShellPtyAccess(sessionID, terminalID, processID, interactiveRunner, wsw, &ptyWriter) {
					directOutputSet = true
				}
			}

			// Handle resize for user shell
			h.handleUserShellResize(data[1:], sessionID, terminalID, interactiveRunner)
			continue
		}

		// Regular input - write to PTY if available
		if ptyWriter != nil {
			if _, err := ptyWriter.Write(data); err != nil {
				h.logger.Debug("PTY write error",
					zap.String("session_id", sessionID),
					zap.String("terminal_id", terminalID),
					zap.Error(err))
				// Try to re-establish PTY access
				if h.setupUserShellPtyAccess(sessionID, terminalID, processID, interactiveRunner, wsw, &ptyWriter) {
					directOutputSet = true
					// Retry write
					if _, err := ptyWriter.Write(data); err != nil {
						h.logger.Debug("PTY write error after reconnect",
							zap.String("session_id", sessionID),
							zap.String("terminal_id", terminalID),
							zap.Error(err))
					}
				}
			}
		} else {
			// PTY not ready yet - try to set it up
			if h.setupUserShellPtyAccess(sessionID, terminalID, processID, interactiveRunner, wsw, &ptyWriter) {
				directOutputSet = true
				// Retry writing after setup
				if _, err := ptyWriter.Write(data); err != nil {
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
		}
	}
}

// setupUserShellPtyAccess sets up PTY access for a user shell terminal.
func (h *TerminalHandler) setupUserShellPtyAccess(
	sessionID string,
	terminalID string,
	processID string,
	interactiveRunner *process.InteractiveRunner,
	wsw *wsWriter,
	ptyWriter *io.Writer,
) bool {
	getWriter := func() (io.Writer, error) {
		writer, _, err := interactiveRunner.GetUserShellPtyWriter(sessionID, terminalID)
		return writer, err
	}
	return h.setupPtyAccessCommon(sessionID, processID, interactiveRunner, wsw, ptyWriter, getWriter)
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
	if chunks, ok := interactiveRunner.GetBuffer(processID); ok && len(chunks) > 0 {
		var combined bytes.Buffer
		for _, chunk := range chunks {
			combined.WriteString(chunk.Data)
		}
		if combined.Len() > 0 {
			h.logger.Debug("sending buffered output to terminal",
				zap.String("session_id", sessionID),
				zap.Int("chunks", len(chunks)),
				zap.Int("total_bytes", combined.Len()))
			if _, writeErr := wsw.Write(combined.Bytes()); writeErr != nil {
				h.logger.Warn("failed to send buffered output",
					zap.String("session_id", sessionID),
					zap.Error(writeErr))
			}
		}
	}

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

	// Resize the user shell PTY
	if err := interactiveRunner.ResizeUserShell(sessionID, terminalID, resize.Cols, resize.Rows); err != nil {
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
