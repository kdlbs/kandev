package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// InitializeRequest is a request to initialize the ACP session
type InitializeRequest struct {
	ClientName    string `json:"client_name"`
	ClientVersion string `json:"client_version"`
}

// InitializeResponse is the response to an initialize call
type InitializeResponse struct {
	Success  bool                    `json:"success"`
	Response *acp.InitializeResponse `json:"response,omitempty"`
	Error    string                  `json:"error,omitempty"`
}

func (s *Server) handleACPInitialize(c *gin.Context) {
	var req InitializeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, InitializeResponse{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	conn := s.procMgr.GetConnection()
	if conn == nil {
		c.JSON(http.StatusServiceUnavailable, InitializeResponse{
			Success: false,
			Error:   "agent not running",
		})
		return
	}

	resp, err := conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientInfo: &acp.Implementation{
			Name:    req.ClientName,
			Version: req.ClientVersion,
		},
	})
	if err != nil {
		s.logger.Error("initialize failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, InitializeResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, InitializeResponse{
		Success:  true,
		Response: &resp,
	})
}

// NewSessionRequest is a request to create a new ACP session
type NewSessionRequest struct {
	Cwd string `json:"cwd"` // Working directory for the session
}

// NewSessionResponse is the response to a new session call
type NewSessionResponse struct {
	Success   bool   `json:"success"`
	SessionID string `json:"session_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (s *Server) handleACPNewSession(c *gin.Context) {
	var req NewSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, NewSessionResponse{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	conn := s.procMgr.GetConnection()
	if conn == nil {
		c.JSON(http.StatusServiceUnavailable, NewSessionResponse{
			Success: false,
			Error:   "agent not running",
		})
		return
	}

	cwd := req.Cwd
	if cwd == "" {
		cwd = "/workspace"
	}

	resp, err := conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        cwd,
		McpServers: []acp.McpServer{}, // Required: must be an empty array, not nil
	})
	if err != nil {
		s.logger.Error("new session failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, NewSessionResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Store session ID
	s.procMgr.SetSessionID(resp.SessionId)

	c.JSON(http.StatusOK, NewSessionResponse{
		Success:   true,
		SessionID: string(resp.SessionId),
	})
}

// PromptRequest is a request to send a prompt to the agent
type PromptRequest struct {
	Text string `json:"text"` // Simple text prompt
}

// PromptResponse is the response to a prompt call
type PromptResponse struct {
	Success    bool           `json:"success"`
	StopReason acp.StopReason `json:"stop_reason,omitempty"`
	Error      string         `json:"error,omitempty"`
}

func (s *Server) handleACPPrompt(c *gin.Context) {
	var req PromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, PromptResponse{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	conn := s.procMgr.GetConnection()
	if conn == nil {
		c.JSON(http.StatusServiceUnavailable, PromptResponse{
			Success: false,
			Error:   "agent not running",
		})
		return
	}

	sessionID := s.procMgr.GetSessionID()
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, PromptResponse{
			Success: false,
			Error:   "no active session - call new_session first",
		})
		return
	}

	// Use a long timeout for prompt - agent may take time to complete
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()

	// Build prompt content - simple text block
	resp, err := conn.Prompt(ctx, acp.PromptRequest{
		SessionId: sessionID,
		Prompt:    []acp.ContentBlock{acp.TextBlock(req.Text)},
	})
	if err != nil {
		s.logger.Error("prompt failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, PromptResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	s.logger.Info("prompt completed",
		zap.String("stop_reason", string(resp.StopReason)))

	c.JSON(http.StatusOK, PromptResponse{
		Success:    true,
		StopReason: resp.StopReason,
	})
}

// handleACPStreamWS streams ACP session notifications via WebSocket
func (s *Server) handleACPStreamWS(c *gin.Context) {
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	s.logger.Info("ACP stream WebSocket connected")

	// Get the session updates channel
	updatesCh := s.procMgr.GetUpdates()

	// Stream session notifications to WebSocket
	for {
		select {
		case notification, ok := <-updatesCh:
			if !ok {
				return
			}

			data, err := json.Marshal(notification)
			if err != nil {
				s.logger.Error("failed to marshal notification", zap.Error(err))
				continue
			}

			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				s.logger.Debug("WebSocket write error", zap.Error(err))
				return
			}
		}
	}
}

