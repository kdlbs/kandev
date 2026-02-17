package api

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

// handleWorkspaceStreamWS handles the unified workspace stream WebSocket endpoint.
// It streams git status, file changes, file lists, and shell I/O over a single connection.
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

	s.logger.Info("Workspace stream WebSocket connected")

	// Subscribe to unified workspace updates
	sub := s.procMgr.GetWorkspaceTracker().SubscribeWorkspaceStream()
	defer s.procMgr.GetWorkspaceTracker().UnsubscribeWorkspaceStream(sub)

	// Get shell for input handling (may be nil)
	shell := s.procMgr.Shell()

	// Subscribe to shell output if shell is available
	var shellOutputCh chan []byte
	if shell != nil {
		shellOutputCh = make(chan []byte, 256)
		shell.Subscribe(shellOutputCh)
		defer shell.Unsubscribe(shellOutputCh)
	}

	// Send connected message
	connectedMsg := types.NewWorkspaceConnected()
	if err := conn.WriteJSON(connectedMsg); err != nil {
		s.logger.Debug("failed to send connected message", zap.Error(err))
		return
	}

	// Done channel to signal goroutine shutdown
	done := make(chan struct{})
	defer close(done)

	// Handle incoming messages (shell_input, shell_resize, ping) in a goroutine
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				var msg types.WorkspaceStreamMessage
				if err := conn.ReadJSON(&msg); err != nil {
					s.logger.Debug("workspace stream WebSocket read error", zap.Error(err))
					return
				}

				switch msg.Type {
				case types.WorkspaceMessageTypeShellInput:
					if shell != nil {
						if _, err := shell.Write([]byte(msg.Data)); err != nil {
							s.logger.Debug("shell write error", zap.Error(err))
						}
					}
				case types.WorkspaceMessageTypeShellResize:
					// Resize is currently not implemented in the shell session
					// TODO: Implement shell.Resize(cols, rows) when needed
					s.logger.Debug("shell resize requested", zap.Int("cols", msg.Cols), zap.Int("rows", msg.Rows))
				case types.WorkspaceMessageTypePing:
					// Respond with pong
					pongMsg := types.NewWorkspacePong()
					if err := conn.WriteJSON(pongMsg); err != nil {
						s.logger.Debug("workspace stream pong write error", zap.Error(err))
						return
					}
				}
			}
		}
	}()

	// Forward all workspace events to WebSocket
	for {
		select {
		case <-done:
			return
		case msg, ok := <-sub:
			if !ok {
				return
			}
			if err := conn.WriteJSON(msg); err != nil {
				s.logger.Debug("workspace stream write error", zap.Error(err))
				return
			}
		case data, ok := <-shellOutputCh:
			if !ok {
				// Shell output channel closed
				shellOutputCh = nil
				continue
			}
			// Forward shell output as workspace stream message
			shellMsg := types.NewWorkspaceShellOutput(string(data))
			if err := conn.WriteJSON(shellMsg); err != nil {
				s.logger.Debug("workspace stream shell output write error", zap.Error(err))
				return
			}
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

	content, size, isBinary, err := s.procMgr.GetWorkspaceTracker().GetFileContent(path)
	if err != nil {
		c.JSON(400, types.FileContentResponse{Path: path, Error: err.Error(), Size: size})
		return
	}

	c.JSON(200, types.FileContentResponse{Path: path, Content: content, Size: size, IsBinary: isBinary})
}

// handleFileUpdate handles file update requests via HTTP POST
func (s *Server) handleFileUpdate(c *gin.Context) {
	var req streams.FileUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, streams.FileUpdateResponse{Error: "invalid request: " + err.Error()})
		return
	}

	if req.Path == "" {
		c.JSON(400, streams.FileUpdateResponse{Error: "path is required"})
		return
	}

	if req.Diff == "" {
		c.JSON(400, streams.FileUpdateResponse{Error: "diff is required"})
		return
	}

	// Apply the diff
	newHash, err := s.procMgr.GetWorkspaceTracker().ApplyFileDiff(req.Path, req.Diff, req.OriginalHash)
	if err != nil {
		c.JSON(400, streams.FileUpdateResponse{
			Path:    req.Path,
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(200, streams.FileUpdateResponse{
		Path:    req.Path,
		Success: true,
		NewHash: newHash,
	})
}

// handleFileSearch handles file search requests via HTTP GET
func (s *Server) handleFileSearch(c *gin.Context) {
	query := c.Query("q")
	limit := 20
	if l := c.Query("limit"); l != "" {
		if parsed := mustParseInt(l); parsed > 0 {
			limit = parsed
		}
	}

	files := s.procMgr.GetWorkspaceTracker().SearchFiles(query, limit)

	c.JSON(200, types.FileSearchResponse{Files: files})
}

// handleFileCreate handles file create requests via HTTP POST
func (s *Server) handleFileCreate(c *gin.Context) {
	var req streams.FileCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, streams.FileCreateResponse{Error: "invalid request: " + err.Error()})
		return
	}

	if req.Path == "" {
		c.JSON(400, streams.FileCreateResponse{Error: "path is required"})
		return
	}

	s.logger.Info("handleFileCreate: creating file", zap.String("path", req.Path))

	err := s.procMgr.GetWorkspaceTracker().CreateFile(req.Path)
	if err != nil {
		c.JSON(400, streams.FileCreateResponse{
			Path:    req.Path,
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	s.logger.Info("handleFileCreate: file created successfully", zap.String("path", req.Path))

	c.JSON(200, streams.FileCreateResponse{
		Path:    req.Path,
		Success: true,
	})
}

// handleFileDelete handles file delete requests via HTTP DELETE
func (s *Server) handleFileDelete(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(400, streams.FileDeleteResponse{Error: "path is required"})
		return
	}

	err := s.procMgr.GetWorkspaceTracker().DeleteFile(path)
	if err != nil {
		c.JSON(400, streams.FileDeleteResponse{
			Path:    path,
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(200, streams.FileDeleteResponse{
		Path:    path,
		Success: true,
	})
}

// mustParseInt parses a string to int, returns 0 on error
func mustParseInt(s string) int {
	var n int
	if err := json.Unmarshal([]byte(s), &n); err != nil {
		return 0
	}
	return n
}
