package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// InitializeRequest is a request to initialize the ACP session
type InitializeRequest struct {
	ClientName    string `json:"client_name"`
	ClientVersion string `json:"client_version"`
}

// AgentInfoResponse contains information about the connected agent.
type AgentInfoResponse struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResponse is the response to an initialize call
type InitializeResponse struct {
	Success   bool               `json:"success"`
	AgentInfo *AgentInfoResponse `json:"agent_info,omitempty"`
	Error     string             `json:"error,omitempty"`
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

	adapter := s.procMgr.GetAdapter()
	if adapter == nil {
		c.JSON(http.StatusServiceUnavailable, InitializeResponse{
			Success: false,
			Error:   "agent not running",
		})
		return
	}

	err := adapter.Initialize(ctx)
	if err != nil {
		s.logger.Error("initialize failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, InitializeResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Get agent info after successful initialization
	var agentInfoResp *AgentInfoResponse
	if info := adapter.GetAgentInfo(); info != nil {
		agentInfoResp = &AgentInfoResponse{
			Name:    info.Name,
			Version: info.Version,
		}
	}

	c.JSON(http.StatusOK, InitializeResponse{
		Success:   true,
		AgentInfo: agentInfoResp,
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

	adapter := s.procMgr.GetAdapter()
	if adapter == nil {
		c.JSON(http.StatusServiceUnavailable, NewSessionResponse{
			Success: false,
			Error:   "agent not running",
		})
		return
	}

	sessionID, err := adapter.NewSession(ctx)
	if err != nil {
		s.logger.Error("new session failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, NewSessionResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Session ID is stored in the adapter - no need to duplicate in Manager
	c.JSON(http.StatusOK, NewSessionResponse{
		Success:   true,
		SessionID: sessionID,
	})
}

// LoadSessionRequest is a request to load an existing ACP session
type LoadSessionRequest struct {
	SessionID string `json:"session_id"`
}

// LoadSessionResponse is the response to a load session call
type LoadSessionResponse struct {
	Success   bool   `json:"success"`
	SessionID string `json:"session_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (s *Server) handleACPLoadSession(c *gin.Context) {
	var req LoadSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, LoadSessionResponse{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}
	if req.SessionID == "" {
		c.JSON(http.StatusBadRequest, LoadSessionResponse{
			Success: false,
			Error:   "session_id is required",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	adapter := s.procMgr.GetAdapter()
	if adapter == nil {
		c.JSON(http.StatusServiceUnavailable, LoadSessionResponse{
			Success: false,
			Error:   "agent not running",
		})
		return
	}

	if err := adapter.LoadSession(ctx, req.SessionID); err != nil {
		s.logger.Error("load session failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, LoadSessionResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, LoadSessionResponse{
		Success:   true,
		SessionID: req.SessionID,
	})
}

// PromptRequest is a request to send a prompt to the agent
type PromptRequest struct {
	Text string `json:"text"` // Simple text prompt
}

// PromptResponse is the response to a prompt call
type PromptResponse struct {
	Success    bool   `json:"success"`
	StopReason string `json:"stop_reason,omitempty"`
	Error      string `json:"error,omitempty"`
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

	adapter := s.procMgr.GetAdapter()
	if adapter == nil {
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

	err := adapter.Prompt(ctx, req.Text)
	if err != nil {
		s.logger.Error("prompt failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, PromptResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	s.logger.Info("prompt completed")

	c.JSON(http.StatusOK, PromptResponse{
		Success: true,
	})
}

// handleACPStreamWS streams ACP session notifications via WebSocket
func (s *Server) handleACPStreamWS(c *gin.Context) {
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			s.logger.Debug("failed to close ACP stream websocket", zap.Error(err))
		}
	}()

	s.logger.Info("ACP stream WebSocket connected")

	// Get the session updates channel
	updatesCh := s.procMgr.GetUpdates()

	// Stream session notifications to WebSocket
	for notification := range updatesCh {
		data, err := json.Marshal(notification)
		if err != nil {
			s.logger.Error("failed to marshal notification", zap.Error(err))
			continue
		}

		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			return
		}
	}
}

// PermissionRespondRequest is a request to respond to a permission request
type PermissionRespondRequest struct {
	PendingID string `json:"pending_id" binding:"required"`
	OptionID  string `json:"option_id,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`
}

// PermissionRespondResponse is the response to a permission respond call
type PermissionRespondResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) handlePermissionRespond(c *gin.Context) {
	var req PermissionRespondRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, PermissionRespondResponse{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	s.logger.Info("received permission response",
		zap.String("pending_id", req.PendingID),
		zap.String("option_id", req.OptionID),
		zap.Bool("cancelled", req.Cancelled))

	if err := s.procMgr.RespondToPermission(req.PendingID, req.OptionID, req.Cancelled); err != nil {
		s.logger.Error("failed to respond to permission", zap.Error(err))
		c.JSON(http.StatusNotFound, PermissionRespondResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, PermissionRespondResponse{
		Success: true,
	})
}

// PendingPermissionResponse represents a pending permission for the API
type PendingPermissionResponse struct {
	PendingID  string                 `json:"pending_id"`
	SessionID  string                 `json:"session_id"`
	ToolCallID string                 `json:"tool_call_id"`
	Title      string                 `json:"title"`
	Options    []PermissionOptionJSON `json:"options"`
	CreatedAt  string                 `json:"created_at"`
}

// PermissionOptionJSON is the JSON representation of a permission option
type PermissionOptionJSON struct {
	OptionID string `json:"option_id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

func (s *Server) handleGetPendingPermissions(c *gin.Context) {
	pending := s.procMgr.GetPendingPermissions()

	result := make([]PendingPermissionResponse, 0, len(pending))
	for _, p := range pending {
		options := make([]PermissionOptionJSON, len(p.Request.Options))
		for i, opt := range p.Request.Options {
			options[i] = PermissionOptionJSON{
				OptionID: opt.OptionID,
				Name:     opt.Name,
				Kind:     opt.Kind,
			}
		}
		result = append(result, PendingPermissionResponse{
			PendingID:  p.ID,
			SessionID:  p.Request.SessionID,
			ToolCallID: p.Request.ToolCallID,
			Title:      p.Request.Title,
			Options:    options,
			CreatedAt:  p.CreatedAt.Format(time.RFC3339),
		})
	}

	c.JSON(http.StatusOK, result)
}

// handlePermissionStreamWS streams permission request notifications via WebSocket
func (s *Server) handlePermissionStreamWS(c *gin.Context) {
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			s.logger.Debug("failed to close permission websocket", zap.Error(err))
		}
	}()

	s.logger.Info("Permission stream WebSocket connected")

	// Get the permission requests channel
	permissionCh := s.procMgr.GetPermissionRequests()

	// Stream permission notifications to WebSocket
	for notification := range permissionCh {
		data, err := json.Marshal(notification)
		if err != nil {
			s.logger.Error("failed to marshal permission notification", zap.Error(err))
			continue
		}

		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			s.logger.Debug("WebSocket write error", zap.Error(err))
			return
		}
	}
}
