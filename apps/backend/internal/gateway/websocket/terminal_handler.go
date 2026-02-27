// Package websocket provides WebSocket handlers for the gateway.
package websocket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
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

	// Route based on explicit mode parameter
	mode := c.Query("mode")
	switch mode {
	case "agent":
		interactiveRunner := h.lifecycleMgr.GetInteractiveRunner()
		if interactiveRunner == nil {
			h.logger.Error("interactive runner not available")
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "interactive runner not available"})
			return
		}
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
		if h.shouldUseWorkspaceShell(sessionID) {
			h.handleRemoteUserShellWS(c, sessionID, terminalID)
			return
		}
		interactiveRunner := h.lifecycleMgr.GetInteractiveRunner()
		if interactiveRunner == nil {
			h.logger.Error("interactive runner not available")
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "interactive runner not available"})
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

func (h *TerminalHandler) shouldUseWorkspaceShell(sessionID string) bool {
	execution, exists := h.lifecycleMgr.GetExecutionBySessionID(sessionID)
	if !exists {
		return false
	}
	if execution.RuntimeName == "sprites" {
		return true
	}
	if execution.Metadata == nil {
		return false
	}
	isRemote, ok := execution.Metadata[lifecycle.MetadataKeyIsRemote].(bool)
	return ok && isRemote
}

func (h *TerminalHandler) handleRemoteUserShellWS(c *gin.Context, sessionID, terminalID string) {
	execution, exists := h.lifecycleMgr.GetExecutionBySessionID(sessionID)
	if !exists || execution.GetAgentCtlClient() == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
		return
	}
	baseClient := execution.GetAgentCtlClient()
	streamClient, err := cloneAgentctlClientForWorkspace(baseClient, h.logger)
	if err != nil {
		h.logger.Error("failed to create remote shell stream client",
			zap.String("session_id", sessionID),
			zap.String("terminal_id", terminalID),
			zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	defer streamClient.Close()

	_, initialCommand, httpErr := h.resolveShellLabel(c)
	if httpErr != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": httpErr})
		return
	}

	if err := baseClient.StartShell(c.Request.Context()); err != nil {
		h.logger.Error("failed to start remote shell",
			zap.String("session_id", sessionID),
			zap.String("terminal_id", terminalID),
			zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	conn, err := terminalUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("failed to upgrade remote shell websocket",
			zap.String("session_id", sessionID),
			zap.String("terminal_id", terminalID),
			zap.Error(err))
		return
	}

	stream, err := streamClient.StreamWorkspace(c.Request.Context(), agentctl.WorkspaceStreamCallbacks{
		OnShellOutput: func(data string) {
			_ = conn.WriteMessage(gorillaws.BinaryMessage, []byte(data))
		},
	})
	if err != nil {
		_ = conn.Close()
		h.logger.Error("failed to open remote workspace stream",
			zap.String("session_id", sessionID),
			zap.String("terminal_id", terminalID),
			zap.Error(err))
		return
	}
	defer func() {
		stream.Close()
		_ = conn.Close()
	}()

	// Replay recent buffered shell output so prompt/current state appears immediately
	// without requiring user input to trigger the first visible update.
	if buffered, err := baseClient.ShellBuffer(c.Request.Context()); err == nil && buffered != "" {
		_ = conn.WriteMessage(gorillaws.BinaryMessage, []byte(buffered))
	}

	if initialCommand != "" {
		_ = stream.WriteShellInput(initialCommand + "\n")
	}

	h.remoteShellReadLoop(conn, stream, sessionID, terminalID)
}

func (h *TerminalHandler) remoteShellReadLoop(conn *gorillaws.Conn, stream *agentctl.WorkspaceStream, sessionID, terminalID string) {
	for {
		messageType, data, readErr := conn.ReadMessage()
		if readErr != nil {
			if !gorillaws.IsCloseError(readErr, gorillaws.CloseNormalClosure, gorillaws.CloseGoingAway) {
				h.logger.Debug("remote shell websocket read error",
					zap.String("session_id", sessionID),
					zap.String("terminal_id", terminalID),
					zap.Error(readErr))
			}
			return
		}
		if messageType != gorillaws.BinaryMessage && messageType != gorillaws.TextMessage {
			continue
		}
		if len(data) > 0 {
			h.processRemoteShellMessage(data, stream, sessionID, terminalID)
		}
	}
}

func (h *TerminalHandler) processRemoteShellMessage(data []byte, stream *agentctl.WorkspaceStream, sessionID, terminalID string) {
	if data[0] == resizeCommandByte {
		var resize ResizePayload
		if err := json.Unmarshal(data[1:], &resize); err != nil {
			h.logger.Debug("failed to decode remote shell resize",
				zap.String("session_id", sessionID),
				zap.String("terminal_id", terminalID),
				zap.Error(err))
			return
		}
		if err := stream.ResizeShell(int(resize.Cols), int(resize.Rows)); err != nil {
			h.logger.Debug("failed to resize remote shell",
				zap.String("session_id", sessionID),
				zap.String("terminal_id", terminalID),
				zap.Uint16("cols", resize.Cols),
				zap.Uint16("rows", resize.Rows),
				zap.Error(err))
		}
		return
	}
	if err := stream.WriteShellInput(string(data)); err != nil {
		h.logger.Debug("failed to write remote shell input",
			zap.String("session_id", sessionID),
			zap.String("terminal_id", terminalID),
			zap.Error(err))
	}
}

func cloneAgentctlClientForWorkspace(base *agentctl.Client, log *logger.Logger) (*agentctl.Client, error) {
	parsed, err := url.Parse(base.BaseURL())
	if err != nil {
		return nil, fmt.Errorf("invalid agentctl base url: %w", err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		return nil, fmt.Errorf("invalid agentctl port: %w", err)
	}
	return agentctl.NewClient(parsed.Hostname(), port, log), nil
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

		if data[0] == resizeCommandByte {
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

// shellScriptInfo holds the label and initial command resolved from a script.
type shellScriptInfo struct {
	label          string
	initialCommand string
}

// resolveShellScript looks up a script by ID and returns its label and command.
// Returns an error if the script cannot be retrieved.
func (h *TerminalHandler) resolveShellScript(ctx context.Context, scriptID string) (*shellScriptInfo, error) {
	script, err := h.scriptService.GetRepositoryScript(ctx, scriptID)
	if err != nil {
		h.logger.Error("failed to get repository script",
			zap.String("script_id", scriptID),
			zap.Error(err))
		return nil, err
	}
	h.logger.Info("handleUserShellWS: resolved script",
		zap.String("script_id", scriptID),
		zap.String("script_name", script.Name),
		zap.String("script_command", script.Command))
	return &shellScriptInfo{label: script.Name, initialCommand: script.Command}, nil
}

// resolvePreferredShell returns the user's preferred shell, or empty string on error.
func (h *TerminalHandler) resolvePreferredShell(ctx context.Context) string {
	if h.userService == nil {
		return ""
	}
	shell, err := h.userService.PreferredShell(ctx)
	if err != nil {
		h.logger.Debug("failed to get preferred shell, using default", zap.Error(err))
		return ""
	}
	return shell
}

// resolveWorkingDir returns the workspace path for the given session, if available.
func (h *TerminalHandler) resolveWorkingDir(sessionID string) string {
	execution, exists := h.lifecycleMgr.GetExecutionBySessionID(sessionID)
	if !exists {
		h.logger.Info("handleUserShellWS: no execution found, using empty working directory")
		return ""
	}
	h.logger.Info("handleUserShellWS: got working directory from execution",
		zap.String("working_dir", execution.WorkspacePath))
	return execution.WorkspacePath
}

// resolveShellLabel returns the label and initial command for a user shell, derived
// from either a script ID lookup or a plain label query parameter.
// Returns an HTTP error string (non-empty) when the script lookup fails.
func (h *TerminalHandler) resolveShellLabel(c *gin.Context) (label, initialCommand, httpErr string) {
	scriptID := c.Query("scriptId")
	labelParam := c.Query("label")

	if scriptID != "" && h.scriptService != nil {
		info, err := h.resolveShellScript(c.Request.Context(), scriptID)
		if err != nil {
			return "", "", "invalid script ID"
		}
		return info.label, info.initialCommand, ""
	}
	if labelParam != "" {
		return labelParam, "", ""
	}
	return "", "", ""
}

// startUserShellProcess resolves shell parameters and starts (or reconnects to) the
// user shell process. Returns the process ID on success, or an HTTP status + message on failure.
func (h *TerminalHandler) startUserShellProcess(
	c *gin.Context,
	sessionID string,
	terminalID string,
	interactiveRunner *process.InteractiveRunner,
) (processID string, httpStatus int, errMsg string) {
	label, initialCommand, httpErr := h.resolveShellLabel(c)
	if httpErr != "" {
		return "", http.StatusBadRequest, httpErr
	}

	h.logger.Info("handleUserShellWS: starting user shell handling",
		zap.String("session_id", sessionID),
		zap.String("terminal_id", terminalID),
		zap.String("label", label),
		zap.String("initial_command", initialCommand))

	preferredShell := h.resolvePreferredShell(c.Request.Context())
	workingDir := h.resolveWorkingDir(sessionID)

	opts := &process.UserShellOptions{Label: label, InitialCommand: initialCommand}

	h.logger.Info("handleUserShellWS: calling StartUserShell",
		zap.String("session_id", sessionID),
		zap.String("terminal_id", terminalID),
		zap.String("working_dir", workingDir),
		zap.String("preferred_shell", preferredShell),
		zap.String("label", opts.Label),
		zap.String("initial_command", opts.InitialCommand))

	info, err := interactiveRunner.StartUserShell(
		c.Request.Context(), sessionID, terminalID, workingDir, preferredShell, opts,
	)
	if err != nil {
		h.logger.Error("failed to start user shell",
			zap.String("session_id", sessionID),
			zap.String("terminal_id", terminalID),
			zap.Error(err))
		return "", http.StatusServiceUnavailable, err.Error()
	}

	h.logger.Info("handleUserShellWS: user shell started successfully",
		zap.String("session_id", sessionID),
		zap.String("terminal_id", terminalID),
		zap.String("process_id", info.ID))

	return info.ID, 0, ""
}

// handleUserShellWS handles WebSocket connections for user shell terminals.
// Each terminal tab gets its own independent shell process.
func (h *TerminalHandler) handleUserShellWS(
	c *gin.Context,
	sessionID string,
	terminalID string,
	interactiveRunner *process.InteractiveRunner,
) {
	processID, httpStatus, errMsg := h.startUserShellProcess(c, sessionID, terminalID, interactiveRunner)
	if errMsg != "" {
		c.JSON(httpStatus, gin.H{"error": errMsg})
		return
	}

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

	wsw := newWsWriter(conn)
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

		if data[0] == resizeCommandByte {
			directOutputSet = h.handleUserShellResizeCommand(
				data[1:], sessionID, terminalID, processID,
				interactiveRunner, wsw, directOutputSet, &ptyWriter,
			)
			continue
		}

		directOutputSet = h.handleUserShellInput(
			data, sessionID, terminalID, processID,
			interactiveRunner, wsw, &ptyWriter, directOutputSet,
		)
	}
}

// handleUserShellResizeCommand processes a resize message in the user shell bridge loop.
// Returns the updated directOutputSet flag.
func (h *TerminalHandler) handleUserShellResizeCommand(
	payload []byte,
	sessionID string,
	terminalID string,
	processID string,
	interactiveRunner *process.InteractiveRunner,
	wsw *wsWriter,
	directOutputSet bool,
	ptyWriter *io.Writer,
) bool {
	// Set up PTY access BEFORE handling resize if not already done
	if !directOutputSet {
		if h.setupUserShellPtyAccess(sessionID, terminalID, processID, interactiveRunner, wsw, ptyWriter) {
			directOutputSet = true
		}
	}
	h.handleUserShellResize(payload, sessionID, terminalID, interactiveRunner)
	return directOutputSet
}

// handleUserShellInput processes regular (non-resize) input in the user shell bridge loop.
// Returns the updated directOutputSet flag.
func (h *TerminalHandler) handleUserShellInput(
	data []byte,
	sessionID string,
	terminalID string,
	processID string,
	interactiveRunner *process.InteractiveRunner,
	wsw *wsWriter,
	ptyWriter *io.Writer,
	directOutputSet bool,
) bool {
	if *ptyWriter != nil {
		return h.writeToReadyUserShellPty(data, sessionID, terminalID, processID, interactiveRunner, wsw, ptyWriter, directOutputSet)
	}
	return h.writeToUnreadyUserShellPty(data, sessionID, terminalID, processID, interactiveRunner, wsw, ptyWriter, directOutputSet)
}

// writeToReadyUserShellPty writes input to an established user shell PTY.
// On failure it attempts to re-establish PTY access and retries the write.
func (h *TerminalHandler) writeToReadyUserShellPty(
	data []byte,
	sessionID string,
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
		if h.setupUserShellPtyAccess(sessionID, terminalID, processID, interactiveRunner, wsw, ptyWriter) {
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
	terminalID string,
	processID string,
	interactiveRunner *process.InteractiveRunner,
	wsw *wsWriter,
	ptyWriter *io.Writer,
	directOutputSet bool,
) bool {
	if h.setupUserShellPtyAccess(sessionID, terminalID, processID, interactiveRunner, wsw, ptyWriter) {
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

// terminalResponsePattern matches terminal query responses that should not be replayed.
// These are responses from the terminal emulator to queries sent by the shell, which
// appear as PTY output and get captured in the ring buffer. On replay they render as
// visible garbage text because they are response sequences, not display sequences.
//
// Matched sequences:
//   - OSC 11 response: \x1b]11;rgb:XXXX/XXXX/XXXX ST (background color, ST = ESC\ or BEL)
//   - DA1 response: \x1b[?<params>c (device attributes)
//   - CPR response: \x1b[<row>;<col>R or \x1b[<row>R (cursor position report)
var terminalResponsePattern = regexp.MustCompile(
	`\x1b]11;rgb:[0-9a-fA-F/]+(?:\x1b\\|\x07)` + // OSC 11 (ESC\ or BEL terminator)
		`|\x1b\[\?[0-9;]*c` + // DA1 response
		`|\x1b\[\d+(?:;\d+)?R`, // CPR response
)

// stripTerminalResponses removes terminal query response sequences from data.
// Used to clean buffered PTY output before replaying it on reconnect.
func stripTerminalResponses(data []byte) []byte {
	return terminalResponsePattern.ReplaceAll(data, nil)
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
			cleaned := stripTerminalResponses(combined.Bytes())
			h.logger.Debug("sending buffered output to terminal",
				zap.String("session_id", sessionID),
				zap.Int("chunks", len(chunks)),
				zap.Int("total_bytes", combined.Len()),
				zap.Int("cleaned_bytes", len(cleaned)))
			if len(cleaned) > 0 {
				if _, writeErr := wsw.Write(cleaned); writeErr != nil {
					h.logger.Warn("failed to send buffered output",
						zap.String("session_id", sessionID),
						zap.Error(writeErr))
				}
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
