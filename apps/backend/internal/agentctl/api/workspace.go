package api

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/process"
	"go.uber.org/zap"
)

// GitStatusUpdate represents a git status update
type GitStatusUpdate struct {
	Timestamp   string              `json:"timestamp"`
	Modified    []string            `json:"modified"`
	Added       []string            `json:"added"`
	Deleted     []string            `json:"deleted"`
	Untracked   []string            `json:"untracked"`
	Renamed     []string            `json:"renamed"`
	Ahead       int                 `json:"ahead"`
	Behind      int                 `json:"behind"`
	Branch      string              `json:"branch"`
	RemoteBranch string             `json:"remote_branch,omitempty"`
	Files       map[string]FileInfo `json:"files,omitempty"`
}

// FileInfo represents information about a file
type FileInfo struct {
	Path         string `json:"path"`
	Status       string `json:"status"` // modified, added, deleted, untracked, renamed
	Additions    int    `json:"additions,omitempty"`
	Deletions    int    `json:"deletions,omitempty"`
	OldPath      string `json:"old_path,omitempty"` // For renamed files
	Diff         string `json:"diff,omitempty"`
}

// handleGitStatusStreamWS streams git status updates via WebSocket
func (s *Server) handleGitStatusStreamWS(c *gin.Context) {
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

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
	defer conn.Close()

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
	defer conn.Close()

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

// handleFileTreeWS handles file tree requests via WebSocket (request/response pattern)
func (s *Server) handleFileTreeWS(c *gin.Context) {
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	s.logger.Info("File tree WebSocket connected")

	// Read and handle requests
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				s.logger.Info("file tree WebSocket closed normally")
			} else {
				s.logger.Debug("file tree WebSocket error", zap.Error(err))
			}
			return
		}

		var req process.FileTreeRequest
		if err := json.Unmarshal(message, &req); err != nil {
			s.logger.Warn("failed to parse file tree request", zap.Error(err))
			continue
		}

		// Get file tree from workspace tracker
		tree, err := s.procMgr.GetWorkspaceTracker().GetFileTree(req.Path, req.Depth)

		var response process.FileTreeResponse
		if err != nil {
			response = process.FileTreeResponse{
				Error: err.Error(),
			}
		} else {
			response = process.FileTreeResponse{
				Root: tree,
			}
		}

		// Send response
		data, err := json.Marshal(response)
		if err != nil {
			s.logger.Error("failed to marshal file tree response", zap.Error(err))
			continue
		}

		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			s.logger.Debug("failed to send file tree response", zap.Error(err))
			return
		}
	}
}

// handleFileContentWS handles file content requests via WebSocket (request/response pattern)
func (s *Server) handleFileContentWS(c *gin.Context) {
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	s.logger.Info("File content WebSocket connected")

	// Read and handle requests
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				s.logger.Info("file content WebSocket closed normally")
			} else {
				s.logger.Debug("file content WebSocket error", zap.Error(err))
			}
			return
		}

		var req process.FileContentRequest
		if err := json.Unmarshal(message, &req); err != nil {
			s.logger.Warn("failed to parse file content request", zap.Error(err))
			continue
		}

		// Get file content from workspace tracker
		content, size, err := s.procMgr.GetWorkspaceTracker().GetFileContent(req.Path)

		var response process.FileContentResponse
		response.Path = req.Path
		if err != nil {
			response.Error = err.Error()
			response.Size = size
		} else {
			response.Content = content
			response.Size = size
		}

		// Send response
		data, err := json.Marshal(response)
		if err != nil {
			s.logger.Error("failed to marshal file content response", zap.Error(err))
			continue
		}

		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			s.logger.Debug("failed to send file content response", zap.Error(err))
			return
		}
	}
}
