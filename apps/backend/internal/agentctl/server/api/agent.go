package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/constants"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// InitializeRequest is a request to initialize the agent session.
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

func (s *Server) handleAgentInitialize(c *gin.Context) {
	var req InitializeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, InitializeResponse{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
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
	Cwd        string            `json:"cwd"` // Working directory for the session
	McpServers []types.McpServer `json:"mcp_servers,omitempty"`
}

// NewSessionResponse is the response to a new session call
type NewSessionResponse struct {
	Success   bool   `json:"success"`
	SessionID string `json:"session_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (s *Server) handleAgentNewSession(c *gin.Context) {
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

	// If MCP server is enabled, prepend the local kandev MCP server to the list.
	// This replaces the old external MCP server URL (http://localhost:9090/sse) with
	// the local agentctl MCP server (http://localhost:{port}/sse).
	mcpServers := req.McpServers
	if s.mcpServer != nil {
		localKandevMcp := types.McpServer{
			Name: "kandev",
			Type: "sse",
			URL:  fmt.Sprintf("http://localhost:%d/sse", s.cfg.Port),
		}
		// Filter out any existing kandev server from the list (to avoid duplicates)
		filtered := make([]types.McpServer, 0, len(mcpServers)+1)
		filtered = append(filtered, localKandevMcp)
		for _, srv := range mcpServers {
			if srv.Name != "kandev" {
				filtered = append(filtered, srv)
			}
		}
		mcpServers = filtered
		s.logger.Debug("injected local kandev MCP server",
			zap.String("url", localKandevMcp.URL),
			zap.Int("total_servers", len(mcpServers)))
	}

	sessionID, err := adapter.NewSession(ctx, mcpServers)
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

func (s *Server) handleAgentLoadSession(c *gin.Context) {
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
	Text        string                 `json:"text"`                  // Simple text prompt
	Attachments []v1.MessageAttachment `json:"attachments,omitempty"` // Optional image attachments
}

// PromptResponse is the response to a prompt call
type PromptResponse struct {
	Success    bool   `json:"success"`
	StopReason string `json:"stop_reason,omitempty"`
	Error      string `json:"error,omitempty"`
}

func (s *Server) handleAgentPrompt(c *gin.Context) {
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
	ctx, cancel := context.WithTimeout(c.Request.Context(), constants.PromptTimeout)
	defer cancel()

	err := adapter.Prompt(ctx, req.Text, req.Attachments)
	if err != nil {
		s.logger.Error("prompt failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, PromptResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	s.logger.Info("prompt completed", zap.Int("attachments", len(req.Attachments)))

	c.JSON(http.StatusOK, PromptResponse{
		Success: true,
	})
}

// handleAgentStreamWS streams agent session notifications via WebSocket.
// This is a bidirectional stream:
// - agentctl -> backend: agent events, MCP requests
// - backend -> agentctl: MCP responses
func (s *Server) handleAgentStreamWS(c *gin.Context) {
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}

	s.logger.Info("agent stream WebSocket connected")

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	// Use a mutex for writing to the WebSocket
	var writeMu sync.Mutex
	writeMessage := func(data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteMessage(websocket.TextMessage, data)
	}

	// Get the session updates channel
	updatesCh := s.procMgr.GetUpdates()

	// Get MCP request channel (if MCP is enabled)
	var mcpRequestCh <-chan *ws.Message
	if s.mcpBackendClient != nil {
		mcpRequestCh = s.mcpBackendClient.GetRequestChannel()
	}

	// WaitGroup for cleanup
	var wg sync.WaitGroup

	// Read goroutine: reads MCP responses from the backend
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					s.logger.Info("agent stream closed normally")
				} else {
					s.logger.Debug("agent stream read error", zap.Error(err))
				}
				return
			}

			// Parse as WebSocket message (MCP response)
			var msg ws.Message
			if err := json.Unmarshal(message, &msg); err != nil {
				s.logger.Warn("failed to parse message", zap.Error(err))
				continue
			}

			// Forward MCP response to the backend client
			if s.mcpBackendClient != nil && (msg.Type == ws.MessageTypeResponse || msg.Type == ws.MessageTypeError) {
				s.mcpBackendClient.HandleResponse(&msg)
			}
		}
	}()

	// Write goroutine: sends agent events and MCP requests to the backend
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if err := conn.Close(); err != nil {
				s.logger.Debug("failed to close agent stream websocket", zap.Error(err))
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case notification, ok := <-updatesCh:
				if !ok {
					return
				}
				data, err := json.Marshal(notification)
				if err != nil {
					s.logger.Error("failed to marshal notification", zap.Error(err))
					continue
				}
				if err := writeMessage(data); err != nil {
					s.logger.Debug("failed to write notification", zap.Error(err))
					return
				}
			case mcpReq, ok := <-mcpRequestCh:
				if !ok {
					// MCP channel closed, continue without MCP
					mcpRequestCh = nil
					continue
				}
				data, err := json.Marshal(mcpReq)
				if err != nil {
					s.logger.Error("failed to marshal MCP request", zap.Error(err))
					continue
				}
				if err := writeMessage(data); err != nil {
					s.logger.Debug("failed to write MCP request", zap.Error(err))
					return
				}
			}
		}
	}()

	wg.Wait()
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
// CancelResponse is the response from a cancel request.
type CancelResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// handleAgentCancel interrupts the current agent turn.
func (s *Server) handleAgentCancel(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	adapter := s.procMgr.GetAdapter()
	if adapter == nil {
		c.JSON(http.StatusServiceUnavailable, CancelResponse{
			Success: false,
			Error:   "agent not running",
		})
		return
	}

	if err := adapter.Cancel(ctx); err != nil {
		s.logger.Error("cancel failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, CancelResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	s.logger.Info("cancel completed")
	c.JSON(http.StatusOK, CancelResponse{Success: true})
}
