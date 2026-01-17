package api

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

// handleGitStatusStreamWS streams git status updates via WebSocket
func (s *Server) handleGitStatusStreamWS(c *gin.Context) {
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			s.logger.Debug("failed to close git status websocket", zap.Error(err))
		}
	}()

	// Subscribe to git status updates
	sub := s.procMgr.GetWorkspaceTracker().SubscribeGitStatus()
	defer s.procMgr.GetWorkspaceTracker().UnsubscribeGitStatus(sub)

	// Handle WebSocket close
	closeCh := make(chan struct{})
	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				close(closeCh)
				return
			}
		}
	}()

	// Stream git status updates to client
	for {
		select {
		case update, ok := <-sub:
			if !ok {
				return
			}
			data, err := json.Marshal(update)
			if err != nil {
				s.logger.Error("failed to marshal git status update", zap.Error(err))
				continue
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-closeCh:
			return
		}
	}
}

// handleFilesStreamWS streams file listing updates via WebSocket
func (s *Server) handleFilesStreamWS(c *gin.Context) {
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			s.logger.Debug("failed to close files websocket", zap.Error(err))
		}
	}()

	// Subscribe to file updates
	sub := s.procMgr.GetWorkspaceTracker().SubscribeFiles()
	defer s.procMgr.GetWorkspaceTracker().UnsubscribeFiles(sub)

	// Handle WebSocket close
	closeCh := make(chan struct{})
	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				close(closeCh)
				return
			}
		}
	}()

	// Stream file updates to client
	for {
		select {
		case update, ok := <-sub:
			if !ok {
				return
			}
			data, err := json.Marshal(update)
			if err != nil {
				s.logger.Error("failed to marshal file update", zap.Error(err))
				continue
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-closeCh:
			return
		}
	}
}

// handleFileChangesStreamWS streams filesystem change notifications via WebSocket
func (s *Server) handleFileChangesStreamWS(c *gin.Context) {
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			s.logger.Debug("failed to close file changes websocket", zap.Error(err))
		}
	}()

	// Subscribe to file change notifications
	sub := s.procMgr.GetWorkspaceTracker().SubscribeFileChanges()
	defer s.procMgr.GetWorkspaceTracker().UnsubscribeFileChanges(sub)

	// Handle WebSocket close
	closeCh := make(chan struct{})
	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				close(closeCh)
				return
			}
		}
	}()

	// Stream file change notifications to client
	for {
		select {
		case notification, ok := <-sub:
			if !ok {
				return
			}
			data, err := json.Marshal(notification)
			if err != nil {
				s.logger.Error("failed to marshal file change notification", zap.Error(err))
				continue
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-closeCh:
			return
		}
	}
}

// handleFileTree handles file tree requests via HTTP GET
func (s *Server) handleFileTree(c *gin.Context) {
	path := c.Query("path")
	depth := 1
	if d := c.Query("depth"); d != "" {
		if _, err := json.Number(d).Int64(); err == nil {
			depth = int(mustParseInt(d))
		}
	}

	tree, err := s.procMgr.GetWorkspaceTracker().GetFileTree(path, depth)
	if err != nil {
		c.JSON(400, types.FileTreeResponse{Error: err.Error()})
		return
	}

	c.JSON(200, types.FileTreeResponse{Root: tree})
}

// handleFileContent handles file content requests via HTTP GET
func (s *Server) handleFileContent(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(400, types.FileContentResponse{Error: "path is required"})
		return
	}

	content, size, err := s.procMgr.GetWorkspaceTracker().GetFileContent(path)
	if err != nil {
		c.JSON(400, types.FileContentResponse{Path: path, Error: err.Error(), Size: size})
		return
	}

	c.JSON(200, types.FileContentResponse{Path: path, Content: content, Size: size})
}

// mustParseInt parses a string to int, returns 0 on error
func mustParseInt(s string) int {
	var n int
	if err := json.Unmarshal([]byte(s), &n); err != nil {
		return 0
	}
	return n
}

// handleWorkspaceStreamWS handles the unified workspace stream WebSocket endpoint.
// This consolidates all workspace streams (shell I/O, git status, file changes)
// into a single bidirectional WebSocket connection.
func (s *Server) handleWorkspaceStreamWS(c *gin.Context) {
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			s.logger.Debug("failed to close workspace stream websocket", zap.Error(err))
		}
	}()

	s.logger.Info("Unified workspace stream WebSocket connected")

	// Subscribe to unified workspace updates
	workspaceSub := s.procMgr.GetWorkspaceTracker().SubscribeWorkspaceStream()
	defer s.procMgr.GetWorkspaceTracker().UnsubscribeWorkspaceStream(workspaceSub)

	// Also subscribe to shell output if available
	shell := s.procMgr.Shell()
	var shellOutputCh chan []byte
	if shell != nil {
		shellOutputCh = make(chan []byte, 256)
		shell.Subscribe(shellOutputCh)
		defer shell.Unsubscribe(shellOutputCh)
	}

	// Send connected message
	if err := conn.WriteJSON(types.NewWorkspaceConnected()); err != nil {
		s.logger.Debug("workspace stream write error", zap.Error(err))
		return
	}

	// Done channel to signal goroutine shutdown
	done := make(chan struct{})
	defer close(done)

	// Read input from WebSocket (for shell input and other commands)
	go s.handleWorkspaceStreamInput(conn, done, shell)

	// Write output from all sources to WebSocket
	for {
		select {
		case <-done:
			return

		case msg, ok := <-workspaceSub:
			if !ok {
				return
			}
			if err := conn.WriteJSON(msg); err != nil {
				s.logger.Debug("workspace stream write error", zap.Error(err))
				return
			}

		case data, ok := <-shellOutputCh:
			if !ok {
				// Shell output channel closed, continue without shell
				shellOutputCh = nil
				continue
			}
			msg := types.NewWorkspaceShellOutput(string(data))
			if err := conn.WriteJSON(msg); err != nil {
				s.logger.Debug("workspace stream write error", zap.Error(err))
				return
			}
		}
	}
}

// handleWorkspaceStreamInput handles incoming messages from the workspace stream WebSocket
func (s *Server) handleWorkspaceStreamInput(conn *websocket.Conn, done chan struct{}, shell interface {
	Write([]byte) (int, error)
	Resize(cols, rows int) error
}) {
	for {
		select {
		case <-done:
			return
		default:
			var msg types.WorkspaceStreamMessage
			if err := conn.ReadJSON(&msg); err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					s.logger.Debug("workspace stream read error", zap.Error(err))
				}
				close(done)
				return
			}

			switch msg.Type {
			case types.WorkspaceMessageTypeShellInput:
				if shell != nil && msg.Data != "" {
					if _, err := shell.Write([]byte(msg.Data)); err != nil {
						s.logger.Debug("shell write error", zap.Error(err))
					}
				}

			case types.WorkspaceMessageTypeShellResize:
				if shell != nil && msg.Cols > 0 && msg.Rows > 0 {
					if err := shell.Resize(msg.Cols, msg.Rows); err != nil {
						s.logger.Debug("shell resize error", zap.Error(err))
					}
				}

			case types.WorkspaceMessageTypePing:
				// Respond with pong
				if err := conn.WriteJSON(types.NewWorkspacePong()); err != nil {
					s.logger.Debug("workspace stream pong write error", zap.Error(err))
					return
				}
			}
		}
	}
}
