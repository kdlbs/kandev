// Package websocket provides WebSocket handlers for the gateway.
package websocket

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

// TerminalHandler handles dedicated binary WebSocket connections for terminal I/O.
// This bypasses JSON encoding for raw PTY communication via xterm.js AttachAddon.
type TerminalHandler struct {
	lifecycleMgr *lifecycle.Manager
	logger       *logger.Logger
}

// NewTerminalHandler creates a new TerminalHandler instance.
func NewTerminalHandler(lifecycleMgr *lifecycle.Manager, log *logger.Logger) *TerminalHandler {
	return &TerminalHandler{
		lifecycleMgr: lifecycleMgr,
		logger:       log.WithFields(zap.String("component", "terminal_handler")),
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

// HandleTerminalWS handles WebSocket connections at /xterm.js/:sessionId
// This creates a binary WebSocket bridge between xterm.js and the PTY.
func (h *TerminalHandler) HandleTerminalWS(c *gin.Context) {
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sessionId is required"})
		return
	}

	h.logger.Info("terminal WebSocket connection request",
		zap.String("session_id", sessionID),
		zap.String("remote_addr", c.Request.RemoteAddr))

	// Get the interactive runner first
	interactiveRunner := h.lifecycleMgr.GetInteractiveRunner()
	if interactiveRunner == nil {
		h.logger.Error("interactive runner not available")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "interactive runner not available"})
		return
	}

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
	var lastCols, lastRows uint16

	// Channel to signal cleanup to any spawned goroutines
	done := make(chan struct{})

	defer func() {
		// Signal any background goroutines to stop
		close(done)

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
			cols, rows := h.handleResize(data[1:], sessionID, interactiveRunner)
			if cols > 0 && rows > 0 {
				lastCols, lastRows = cols, rows
			}

			// After resize, try to set up PTY access if not already done
			if !directOutputSet {
				if h.setupPtyAccess(sessionID, processID, interactiveRunner, wsw, &ptyWriter) {
					directOutputSet = true

					// Trigger a resize to force TUI redraw after reconnect
					// This ensures the TUI app redraws with full colors/styles
					// We resize to different dimensions then back to force a real redraw
					// (some TUI apps ignore same-size resizes)
					if lastCols > 2 && lastRows > 2 {
						// Delay to ensure direct output is fully set up
						time.Sleep(50 * time.Millisecond)

						// Resize to smaller dimensions to trigger a real change
						// Use a more significant change (2 cols/rows) for apps that might ignore small changes
						if err := interactiveRunner.ResizeBySession(sessionID, lastCols-2, lastRows-2); err != nil {
							h.logger.Debug("failed to trigger intermediate resize",
								zap.String("session_id", sessionID),
								zap.Error(err))
						}

						// Longer delay then resize back to actual dimensions
						time.Sleep(100 * time.Millisecond)
						if err := interactiveRunner.ResizeBySession(sessionID, lastCols, lastRows); err != nil {
							h.logger.Debug("failed to trigger final resize",
								zap.String("session_id", sessionID),
								zap.Error(err))
						} else {
							h.logger.Info("triggered TUI redraw via resize",
								zap.String("session_id", sessionID),
								zap.Uint16("cols", lastCols),
								zap.Uint16("rows", lastRows))
						}

						// Some TUI apps (like opencode) need a second redraw trigger
						// after a longer delay to fully render
						go func(sid string, cols, rows uint16, doneCh <-chan struct{}) {
							select {
							case <-doneCh:
								return
							case <-time.After(300 * time.Millisecond):
							}
							if err := interactiveRunner.ResizeBySession(sid, cols-1, rows); err == nil {
								select {
								case <-doneCh:
									return
								case <-time.After(50 * time.Millisecond):
								}
								_ = interactiveRunner.ResizeBySession(sid, cols, rows)
							}
						}(sessionID, lastCols, lastRows, done)
					}
				}
			}
			continue
		}

		// Regular input - write to PTY if available
		if ptyWriter != nil {
			if _, err := ptyWriter.Write(data); err != nil {
				h.logger.Debug("PTY write error, attempting reconnect",
					zap.String("session_id", sessionID),
					zap.Error(err))

				// Try to reconnect to a new process (may have been restarted)
				newWriter, newProcID := h.attemptReconnect(sessionID, interactiveRunner, wsw, done)
				if newWriter != nil {
					ptyWriter = newWriter
					processID = newProcID
					directOutputSet = true

					// Retry write with new writer
					if _, err := ptyWriter.Write(data); err != nil {
						h.logger.Debug("PTY write error after reconnect",
							zap.String("session_id", sessionID),
							zap.Error(err))
						continue
					}
					h.detectInputSubmission(sessionID, data)
				} else {
					h.logger.Debug("reconnect failed, PTY unavailable",
						zap.String("session_id", sessionID))
					// Don't return - keep trying on subsequent inputs
					continue
				}
			} else {
				// Detect Enter key (user submitted input) - notify lifecycle manager
				h.detectInputSubmission(sessionID, data)
			}
		} else {
			// PTY not ready yet - try to set it up
			if h.setupPtyAccess(sessionID, processID, interactiveRunner, wsw, &ptyWriter) {
				directOutputSet = true
				// Double-check ptyWriter is valid after setup
				if ptyWriter == nil {
					h.logger.Error("PTY writer is nil after successful setup",
						zap.String("session_id", sessionID))
					continue
				}
				// Retry writing after setup
				if _, err := ptyWriter.Write(data); err != nil {
					h.logger.Debug("PTY write error after setup",
						zap.String("session_id", sessionID),
						zap.Error(err))
					continue
				}
				// Detect Enter key (user submitted input) - notify lifecycle manager
				h.detectInputSubmission(sessionID, data)
			} else {
				h.logger.Debug("PTY not ready, dropping input",
					zap.String("session_id", sessionID),
					zap.Int("bytes", len(data)))
			}
		}
	}
}

// attemptReconnect tries to reconnect to a new process after the old one exited.
// Returns the new PTY writer and process ID if successful, or nil if not.
func (h *TerminalHandler) attemptReconnect(
	sessionID string,
	interactiveRunner *process.InteractiveRunner,
	wsw *wsWriter,
	done <-chan struct{},
) (io.Writer, string) {
	const maxRetries = 20         // 20 retries
	const retryInterval = 100 * time.Millisecond // 100ms between retries = 2s total

	for i := 0; i < maxRetries; i++ {
		select {
		case <-done:
			// Bridge is shutting down
			return nil, ""
		default:
		}

		// Try to get the PTY writer for the session (may be a new process)
		writer, newProcID, err := interactiveRunner.GetPtyWriterBySession(sessionID)
		if err == nil && writer != nil {
			// Found a new process - set up direct output
			if err := interactiveRunner.SetDirectOutput(newProcID, wsw); err != nil {
				h.logger.Debug("failed to set direct output during reconnect",
					zap.String("session_id", sessionID),
					zap.String("process_id", newProcID),
					zap.Error(err))
			} else {
				h.logger.Info("reconnected to new process after restart",
					zap.String("session_id", sessionID),
					zap.String("process_id", newProcID))
				return writer, newProcID
			}
		}

		// Wait before retrying
		select {
		case <-done:
			return nil, ""
		case <-time.After(retryInterval):
		}
	}

	h.logger.Debug("reconnect attempts exhausted",
		zap.String("session_id", sessionID),
		zap.Int("attempts", maxRetries))
	return nil, ""
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
	// Try to get PTY writer - this will fail if process hasn't started yet
	writer, err := interactiveRunner.GetPtyWriter(processID)
	if err != nil {
		h.logger.Debug("PTY writer not ready yet",
			zap.String("session_id", sessionID),
			zap.String("process_id", processID))
		return false
	}

	// Send buffered output to the terminal before switching to direct mode.
	// This provides scrollback history when reconnecting to an existing session.
	// We batch all chunks into a single write for efficiency.
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

// handleResize processes a resize command from the WebSocket.
// Returns the parsed dimensions (0, 0 if parsing failed).
func (h *TerminalHandler) handleResize(
	data []byte,
	sessionID string,
	interactiveRunner *process.InteractiveRunner,
) (cols, rows uint16) {
	var resize ResizePayload
	if err := json.Unmarshal(data, &resize); err != nil {
		h.logger.Warn("failed to parse resize command",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return 0, 0
	}

	if resize.Cols == 0 || resize.Rows == 0 {
		h.logger.Warn("invalid resize dimensions",
			zap.String("session_id", sessionID),
			zap.Uint16("cols", resize.Cols),
			zap.Uint16("rows", resize.Rows))
		return 0, 0
	}

	// Resize triggers lazy process start if not already started
	if err := interactiveRunner.ResizeBySession(sessionID, resize.Cols, resize.Rows); err != nil {
		h.logger.Warn("failed to resize PTY",
			zap.String("session_id", sessionID),
			zap.Uint16("cols", resize.Cols),
			zap.Uint16("rows", resize.Rows),
			zap.Error(err))
	} else {
		h.logger.Debug("PTY resized",
			zap.String("session_id", sessionID),
			zap.Uint16("cols", resize.Cols),
			zap.Uint16("rows", resize.Rows))
	}

	// Give the process a moment to start if this was the first resize
	time.Sleep(50 * time.Millisecond)

	return resize.Cols, resize.Rows
}
