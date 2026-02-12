package api

import (
	"context"
	"encoding/json"
	"fmt"
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

// PermissionRespondRequest is a request to respond to a permission request
type PermissionRespondRequest struct {
	PendingID string `json:"pending_id"`
	OptionID  string `json:"option_id,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`
}

// PermissionRespondResponse is the response to a permission respond call
type PermissionRespondResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// AgentStderrResponse contains recent stderr lines from the agent process.
type AgentStderrResponse struct {
	Lines []string `json:"lines"`
}

// CancelResponse is the response from a cancel request.
type CancelResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// handleAgentStreamWS streams agent session notifications via WebSocket.
// This is a bidirectional stream:
// - agentctl -> backend: agent events, MCP requests
// - backend -> agentctl: MCP responses, agent operation requests
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

	// Read goroutine: reads MCP responses and agent operation requests from the backend
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

			// Parse as WebSocket message
			var msg ws.Message
			if err := json.Unmarshal(message, &msg); err != nil {
				s.logger.Warn("failed to parse message", zap.Error(err))
				continue
			}

			// Handle agent operation requests (type=request)
			if msg.Type == ws.MessageTypeRequest {
				go func(reqMsg ws.Message) {
					resp := s.handleAgentStreamRequest(ctx, &reqMsg)
					if resp != nil {
						data, err := json.Marshal(resp)
						if err != nil {
							s.logger.Error("failed to marshal WS response", zap.Error(err))
							return
						}
						if err := writeMessage(data); err != nil {
							s.logger.Debug("failed to write WS response", zap.Error(err))
						}
					}
				}(msg)
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

// handleAgentStreamRequest dispatches agent operation requests received on the WebSocket stream.
func (s *Server) handleAgentStreamRequest(ctx context.Context, msg *ws.Message) *ws.Message {
	switch msg.Action {
	case "agent.initialize":
		return s.handleWSInitialize(ctx, msg)
	case "agent.session.new":
		return s.handleWSNewSession(ctx, msg)
	case "agent.session.load":
		return s.handleWSLoadSession(ctx, msg)
	case "agent.prompt":
		return s.handleWSPrompt(ctx, msg)
	case "agent.cancel":
		return s.handleWSCancel(ctx, msg)
	case "agent.permissions.respond":
		return s.handleWSPermissionRespond(ctx, msg)
	case "agent.stderr":
		return s.handleWSStderr(ctx, msg)
	default:
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnknownAction, fmt.Sprintf("unknown action: %s", msg.Action), nil)
		return resp
	}
}

func (s *Server) handleWSInitialize(ctx context.Context, msg *ws.Message) *ws.Message {
	var req InitializeRequest
	if err := msg.ParsePayload(&req); err != nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
		return resp
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	adapter := s.procMgr.GetAdapter()
	if adapter == nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "agent not running", nil)
		return resp
	}

	if err := adapter.Initialize(ctx); err != nil {
		s.logger.Error("initialize failed", zap.Error(err))
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		return resp
	}

	// Get agent info after successful initialization
	var agentInfoResp *AgentInfoResponse
	if info := adapter.GetAgentInfo(); info != nil {
		agentInfoResp = &AgentInfoResponse{
			Name:    info.Name,
			Version: info.Version,
		}
	}

	resp, _ := ws.NewResponse(msg.ID, msg.Action, InitializeResponse{
		Success:   true,
		AgentInfo: agentInfoResp,
	})
	return resp
}

func (s *Server) handleWSNewSession(ctx context.Context, msg *ws.Message) *ws.Message {
	var req NewSessionRequest
	if err := msg.ParsePayload(&req); err != nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
		return resp
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	adapter := s.procMgr.GetAdapter()
	if adapter == nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "agent not running", nil)
		return resp
	}

	// If MCP server is enabled, prepend the local kandev MCP server to the list.
	mcpServers := req.McpServers
	if s.mcpServer != nil {
		localKandevMcp := types.McpServer{
			Name: "kandev",
			Type: "sse",
			URL:  fmt.Sprintf("http://localhost:%d/sse", s.cfg.Port),
		}
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
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		return resp
	}

	resp, _ := ws.NewResponse(msg.ID, msg.Action, NewSessionResponse{
		Success:   true,
		SessionID: sessionID,
	})
	return resp
}

func (s *Server) handleWSLoadSession(ctx context.Context, msg *ws.Message) *ws.Message {
	var req LoadSessionRequest
	if err := msg.ParsePayload(&req); err != nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
		return resp
	}
	if req.SessionID == "" {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "session_id is required", nil)
		return resp
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	adapter := s.procMgr.GetAdapter()
	if adapter == nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "agent not running", nil)
		return resp
	}

	if err := adapter.LoadSession(ctx, req.SessionID); err != nil {
		s.logger.Error("load session failed", zap.Error(err))
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		return resp
	}

	resp, _ := ws.NewResponse(msg.ID, msg.Action, LoadSessionResponse{
		Success:   true,
		SessionID: req.SessionID,
	})
	return resp
}

func (s *Server) handleWSPrompt(ctx context.Context, msg *ws.Message) *ws.Message {
	var req PromptRequest
	if err := msg.ParsePayload(&req); err != nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
		return resp
	}

	adapter := s.procMgr.GetAdapter()
	if adapter == nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "agent not running", nil)
		return resp
	}

	sessionID := s.procMgr.GetSessionID()
	if sessionID == "" {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "no active session - call new_session first", nil)
		return resp
	}

	// Start prompt processing asynchronously.
	// Completion is signaled via the WebSocket complete event, not this response.
	go func() {
		promptCtx, cancel := context.WithTimeout(context.Background(), constants.PromptTimeout)
		defer cancel()
		if err := adapter.Prompt(promptCtx, req.Text, req.Attachments); err != nil {
			s.logger.Error("async prompt failed", zap.Error(err))
		}
	}()

	s.logger.Info("prompt accepted (async)", zap.Int("attachments", len(req.Attachments)))

	// Return immediately — completion comes via WebSocket complete event
	resp, _ := ws.NewResponse(msg.ID, msg.Action, PromptResponse{Success: true})
	return resp
}

func (s *Server) handleWSCancel(ctx context.Context, msg *ws.Message) *ws.Message {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	adapter := s.procMgr.GetAdapter()
	if adapter == nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "agent not running", nil)
		return resp
	}

	if err := adapter.Cancel(ctx); err != nil {
		s.logger.Error("cancel failed", zap.Error(err))
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		return resp
	}

	s.logger.Info("cancel completed")
	resp, _ := ws.NewResponse(msg.ID, msg.Action, CancelResponse{Success: true})
	return resp
}

func (s *Server) handleWSPermissionRespond(_ context.Context, msg *ws.Message) *ws.Message {
	var req PermissionRespondRequest
	if err := msg.ParsePayload(&req); err != nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
		return resp
	}

	s.logger.Info("received permission response",
		zap.String("pending_id", req.PendingID),
		zap.String("option_id", req.OptionID),
		zap.Bool("cancelled", req.Cancelled))

	if err := s.procMgr.RespondToPermission(req.PendingID, req.OptionID, req.Cancelled); err != nil {
		s.logger.Error("failed to respond to permission", zap.Error(err))
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, err.Error(), nil)
		return resp
	}

	resp, _ := ws.NewResponse(msg.ID, msg.Action, PermissionRespondResponse{Success: true})
	return resp
}

func (s *Server) handleWSStderr(_ context.Context, msg *ws.Message) *ws.Message {
	lines := s.procMgr.GetRecentStderr()
	resp, _ := ws.NewResponse(msg.ID, msg.Action, AgentStderrResponse{Lines: lines})
	return resp
}
