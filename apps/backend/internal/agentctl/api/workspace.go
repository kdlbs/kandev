package api

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
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
