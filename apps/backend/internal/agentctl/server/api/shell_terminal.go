package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/server/shell"
	"go.uber.org/zap"
)

// shellTerminalStartRequest is the request body for POST /shell/terminal/start.
type shellTerminalStartRequest struct {
	TerminalID string `json:"terminal_id"`
	Cols       int    `json:"cols"`
	Rows       int    `json:"rows"`
}

// handleShellTerminalStart creates a new per-terminal shell session.
func (s *Server) handleShellTerminalStart(c *gin.Context) {
	var req shellTerminalStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}
	if req.TerminalID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "terminal_id is required"})
		return
	}

	cfg := shell.DefaultConfig(s.cfg.WorkDir)
	if req.Cols > 0 {
		cfg.Cols = req.Cols
	}
	if req.Rows > 0 {
		cfg.Rows = req.Rows
	}

	mgr := s.procMgr.ShellManager()
	if _, err := mgr.Start(req.TerminalID, cfg); err != nil {
		s.logger.Error("failed to start terminal shell",
			zap.String("terminal_id", req.TerminalID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "started", "terminal_id": req.TerminalID})
}

// handleShellTerminalStreamWS is a binary WebSocket endpoint for per-terminal shell I/O.
// Protocol: byte 0x01 prefix = resize command (followed by JSON {cols, rows}),
// all other bytes = raw PTY input. Output is sent as binary frames.
func (s *Server) handleShellTerminalStreamWS(c *gin.Context) {
	terminalID := c.Param("id")
	if terminalID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "terminal id is required"})
		return
	}

	mgr := s.procMgr.ShellManager()
	session, ok := mgr.Get(terminalID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "terminal not found"})
		return
	}

	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("failed to upgrade terminal shell websocket",
			zap.String("terminal_id", terminalID),
			zap.Error(err))
		return
	}
	defer func() { _ = conn.Close() }()

	s.logger.Info("terminal shell WebSocket connected", zap.String("terminal_id", terminalID))

	// Subscribe to shell output
	outputCh := make(chan []byte, 256)
	session.Subscribe(outputCh)
	defer session.Unsubscribe(outputCh)

	// Replay buffered output for immediate state
	if buf := session.GetBufferedOutput(); len(buf) > 0 {
		if err := conn.WriteMessage(websocket.BinaryMessage, buf); err != nil {
			s.logger.Debug("failed to replay shell buffer", zap.Error(err))
			return
		}
	}

	done := make(chan struct{})
	defer close(done)

	// Forward shell output to WebSocket
	go func() {
		for {
			select {
			case <-done:
				return
			case data, ok := <-outputCh:
				if !ok {
					return
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
					s.logger.Debug("terminal shell output write error",
						zap.String("terminal_id", terminalID),
						zap.Error(err))
					return
				}
			}
		}
	}()

	// Read input from WebSocket
	s.shellTerminalReadLoop(conn, session, terminalID)
}

const shellResizeByte = 0x01

func (s *Server) shellTerminalReadLoop(conn *websocket.Conn, session *shell.Session, terminalID string) {
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				s.logger.Debug("terminal shell read error",
					zap.String("terminal_id", terminalID),
					zap.Error(err))
			}
			return
		}
		if messageType != websocket.BinaryMessage && messageType != websocket.TextMessage {
			continue
		}
		if len(data) == 0 {
			continue
		}

		if data[0] == shellResizeByte {
			var resize struct {
				Cols uint16 `json:"cols"`
				Rows uint16 `json:"rows"`
			}
			if err := json.Unmarshal(data[1:], &resize); err != nil {
				s.logger.Debug("failed to decode terminal shell resize",
					zap.String("terminal_id", terminalID),
					zap.Error(err))
				continue
			}
			if err := session.Resize(resize.Cols, resize.Rows); err != nil {
				s.logger.Debug("failed to resize terminal shell",
					zap.String("terminal_id", terminalID),
					zap.Error(err))
			}
			continue
		}

		if _, err := io.Writer(session).Write(data); err != nil {
			s.logger.Debug("failed to write terminal shell input",
				zap.String("terminal_id", terminalID),
				zap.Error(err))
		}
	}
}

// handleShellTerminalBuffer returns the buffered output for a terminal.
func (s *Server) handleShellTerminalBuffer(c *gin.Context) {
	terminalID := c.Param("id")
	mgr := s.procMgr.ShellManager()
	buf, err := mgr.Buffer(terminalID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ShellBufferResponse{Data: string(buf)})
}

// handleShellTerminalStop stops a per-terminal shell session.
func (s *Server) handleShellTerminalStop(c *gin.Context) {
	terminalID := c.Param("id")
	mgr := s.procMgr.ShellManager()
	if err := mgr.Stop(terminalID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "stopped", "terminal_id": terminalID})
}
