package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	acptransport "github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/constants"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.uber.org/zap"
)

// MCP server constants for kandev injection.
const (
	kandevMcpServerName = "kandev"
	mcpTransportSSE     = "sse"
	mcpTransportHTTP    = "http"
	mcpPathSSE          = "/sse"
	mcpPathHTTP         = "/mcp"
)

// InitializeRequest is a request to initialize the agent session.
type InitializeRequest struct {
	ClientName    string `json:"client_name"`
	ClientVersion string `json:"client_version"`
}

// AgentInfoResponse contains information about the connected agent.
type AgentInfoResponse struct {
	Name                  string `json:"name"`
	Version               string `json:"version"`
	SupportsSessionResume bool   `json:"supports_session_resume,omitempty"`
	SupportsSessionLoad   bool   `json:"supports_session_load,omitempty"`
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
	Success             bool                       `json:"success"`
	SessionID           string                     `json:"session_id,omitempty"`
	ModelState          *streams.SessionModelState `json:"model_state,omitempty"`
	AttachmentAttemptID string                     `json:"attachment_attempt_id,omitempty"`
	Error               string                     `json:"error,omitempty"`
}

// LoadSessionRequest is a request to load an existing ACP session
type LoadSessionRequest struct {
	SessionID  string            `json:"session_id"`
	McpServers []types.McpServer `json:"mcp_servers,omitempty"`
}

// LoadSessionResponse is the response to a load session call
type LoadSessionResponse struct {
	Success             bool                       `json:"success"`
	SessionID           string                     `json:"session_id,omitempty"`
	ModelState          *streams.SessionModelState `json:"model_state,omitempty"`
	AttachmentAttemptID string                     `json:"attachment_attempt_id,omitempty"`
	Error               string                     `json:"error,omitempty"`
}

// ResumeSessionRequest is a request to reconnect an existing ACP session
// without replaying its transcript.
type ResumeSessionRequest struct {
	SessionID  string            `json:"session_id"`
	Cwd        string            `json:"cwd,omitempty"`
	McpServers []types.McpServer `json:"mcp_servers,omitempty"`
}

// ResumeSessionResponse is the response to a session resume call.
type ResumeSessionResponse struct {
	Success             bool                       `json:"success"`
	SessionID           string                     `json:"session_id,omitempty"`
	ModelState          *streams.SessionModelState `json:"model_state,omitempty"`
	AttachmentAttemptID string                     `json:"attachment_attempt_id,omitempty"`
	Error               string                     `json:"error,omitempty"`
}

// PromptRequest is a request to send a prompt to the agent
type PromptRequest struct {
	Text             string                 `json:"text"`                  // Simple text prompt
	Attachments      []v1.MessageAttachment `json:"attachments,omitempty"` // Optional image attachments
	PromptGeneration uint64                 `json:"prompt_generation,omitempty"`
	// Steer asks for delivery into a turn that is still generating rather than
	// waiting for it to end. Honored only when the adapter implements
	// SteerablePrompter and the connected agent advertised the capability;
	// otherwise it degrades silently to an ordinary prompt, which is the
	// specified behavior for an agent that cannot steer.
	Steer bool `json:"steer,omitempty"`
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
	Success         bool   `json:"success"`
	Error           string `json:"error,omitempty"`
	NotAcknowledged bool   `json:"not_acknowledged,omitempty"`
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

	var wg sync.WaitGroup
	wg.Add(1)
	go s.runAgentStreamReader(ctx, conn, writeMessage, cancel, &wg)
	wg.Add(1)
	go s.runAgentStreamWriter(ctx, conn, updatesCh, mcpRequestCh, writeMessage, &wg)
	wg.Wait()
}

// runAgentStreamReader reads MCP responses and agent operation requests from the backend connection.
func (s *Server) runAgentStreamReader(ctx context.Context, conn *websocket.Conn, writeMessage func([]byte) error, cancel context.CancelFunc, wg *sync.WaitGroup) {
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
		var msg ws.Message
		if err := json.Unmarshal(message, &msg); err != nil {
			s.logger.Warn("failed to parse message", zap.Error(err))
			continue
		}
		if msg.Type == ws.MessageTypeRequest {
			go func(reqMsg ws.Message) {
				resp := s.handleAgentStreamRequest(ctx, &reqMsg)
				if resp == nil {
					return
				}
				data, err := json.Marshal(resp)
				if err != nil {
					s.logger.Error("failed to marshal WS response", zap.Error(err))
					return
				}
				if err := writeMessage(data); err != nil {
					s.logger.Debug("failed to write WS response", zap.Error(err))
				}
			}(msg)
			continue
		}
		if s.mcpBackendClient != nil && (msg.Type == ws.MessageTypeResponse || msg.Type == ws.MessageTypeError) {
			s.mcpBackendClient.HandleResponse(&msg)
		}
	}
}

// runAgentStreamWriter sends agent events and MCP requests to the backend connection.
func (s *Server) runAgentStreamWriter(ctx context.Context, conn *websocket.Conn, updatesCh <-chan adapter.AgentEvent, mcpRequestCh <-chan *ws.Message, writeMessage func([]byte) error, wg *sync.WaitGroup) {
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
}

// handleAgentStreamRequest dispatches agent operation requests received on the WebSocket stream.
func (s *Server) handleAgentStreamRequest(ctx context.Context, msg *ws.Message) *ws.Message {
	// Extract remote trace context from WS message metadata for cross-process span linking
	if len(msg.Metadata) > 0 {
		ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(msg.Metadata))
	}

	switch msg.Action {
	case "agent.initialize":
		return s.handleWSInitialize(ctx, msg)
	case "agent.session.new":
		return s.handleWSNewSession(ctx, msg)
	case "agent.session.load":
		return s.handleWSLoadSession(ctx, msg)
	case "agent.session.resume":
		return s.handleWSResumeSession(ctx, msg)
	case "agent.prompt":
		return s.handleWSPrompt(ctx, msg)
	case "agent.cancel":
		return s.handleWSCancel(ctx, msg)
	case "agent.permissions.respond":
		return s.handleWSPermissionRespond(ctx, msg)
	case "agent.permissions.list":
		return s.handleWSPermissionList(msg)
	case "agent.permissions.resolve":
		return s.handleWSPermissionResolve(msg)
	case "agent.permissions.cancel":
		return s.handleWSPermissionCancel(msg)
	case "agent.stderr":
		return s.handleWSStderr(ctx, msg)
	case "agent.session.set_mode":
		return s.handleWSSetMode(ctx, msg)
	case "agent.session.set_model":
		return s.handleWSSetModel(ctx, msg)
	case "agent.session.set_config_option":
		return s.handleWSSetConfigOption(ctx, msg)
	case "agent.session.authenticate":
		return s.handleWSAuthenticate(ctx, msg)
	case "agent.session.reset":
		return s.handleWSResetSession(ctx, msg)
	default:
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnknownAction, fmt.Sprintf("unknown action: %s", msg.Action), nil)
		return resp
	}
}

func (s *Server) handleWSPermissionCancel(msg *ws.Message) *ws.Message {
	var req streams.PermissionCancelRequest
	if err := msg.ParsePayload(&req); err != nil || req.RequestID == "" || req.PendingID == "" {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "request_id and pending_id are required", nil)
		return resp
	}
	result, err := s.procMgr.CancelPermission(req.RequestID, req.PendingID)
	if err == nil {
		resp, _ := ws.NewResponse(msg.ID, msg.Action, result)
		return resp
	}
	var operationErr *process.PermissionOperationError
	if !errors.As(err, &operationErr) {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "permission cancellation failed", nil)
		return resp
	}
	code := ws.ErrorCodeConflict
	switch operationErr.Code {
	case streams.PermissionErrorNotFound:
		code = ws.ErrorCodeNotFound
	case streams.PermissionErrorDeliveryFailed:
		code = ws.ErrorCodeInternalError
	}
	resp, _ := ws.NewError(msg.ID, msg.Action, code, operationErr.Code, map[string]any{"permission_code": operationErr.Code})
	return resp
}

func (s *Server) handleWSPermissionList(msg *ws.Message) *ws.Message {
	permissions := s.procMgr.ListPendingPermissions()
	resp, _ := ws.NewResponse(msg.ID, msg.Action, streams.PermissionListResponse{
		Permissions: permissions,
		Total:       len(permissions),
	})
	return resp
}

func (s *Server) handleWSPermissionResolve(msg *ws.Message) *ws.Message {
	var req streams.PermissionResolveRequest
	if err := msg.ParsePayload(&req); err != nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request", nil)
		return resp
	}
	if req.RequestID == "" || req.PendingID == "" || req.OptionID == "" {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "request_id, pending_id, and option_id are required", nil)
		return resp
	}

	result, err := s.procMgr.ResolvePermission(req.RequestID, req.PendingID, req.OptionID)
	if err == nil {
		resp, _ := ws.NewResponse(msg.ID, msg.Action, result)
		return resp
	}

	var operationErr *process.PermissionOperationError
	if !errors.As(err, &operationErr) {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "permission resolution failed", nil)
		return resp
	}
	code := ws.ErrorCodeConflict
	switch operationErr.Code {
	case streams.PermissionErrorNotFound:
		code = ws.ErrorCodeNotFound
	case streams.PermissionErrorOptionNotOffered:
		code = ws.ErrorCodeValidation
	case streams.PermissionErrorDeliveryFailed:
		code = ws.ErrorCodeInternalError
	}
	resp, _ := ws.NewError(msg.ID, msg.Action, code, operationErr.Code, map[string]any{
		"permission_code": operationErr.Code,
	})
	return resp
}

func (s *Server) handleWSInitialize(ctx context.Context, msg *ws.Message) *ws.Message {
	var req InitializeRequest
	if err := msg.ParsePayload(&req); err != nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
		return resp
	}

	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
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
	if capabilities, ok := adapter.(interface {
		SupportsSessionResume() bool
		SupportsSessionLoad() bool
	}); ok {
		if agentInfoResp == nil {
			agentInfoResp = &AgentInfoResponse{}
		}
		agentInfoResp.SupportsSessionResume = capabilities.SupportsSessionResume()
		agentInfoResp.SupportsSessionLoad = capabilities.SupportsSessionLoad()
	}

	resp, _ := ws.NewResponse(msg.ID, msg.Action, InitializeResponse{
		Success:   true,
		AgentInfo: agentInfoResp,
	})
	return resp
}

// injectKandevMcpServers prepends the local kandev MCP server to the list of MCP servers.
// Both HTTP and SSE variants are injected - the agent's capability filtering will select
// the appropriate one based on what the agent supports. HTTP is listed first so that
// when an agent advertises both transports the "first surviving entry wins" dedup keeps
// the HTTP entry (modern streamable MCP); SSE remains as a fallback for SSE-only agents.
// Any existing kandev server in the list is filtered out to avoid duplicates.
func (s *Server) injectKandevMcpServers(mcpServers []types.McpServer) []types.McpServer {
	kandevMcpSse := types.McpServer{
		Name: kandevMcpServerName,
		Type: mcpTransportSSE,
		URL:  fmt.Sprintf("http://localhost:%d%s", s.cfg.Port, mcpPathSSE),
	}
	kandevMcpHttp := types.McpServer{
		Name: kandevMcpServerName,
		Type: mcpTransportHTTP,
		URL:  fmt.Sprintf("http://localhost:%d%s", s.cfg.Port, mcpPathHTTP),
	}
	filtered := make([]types.McpServer, 0, len(mcpServers)+2)
	filtered = append(filtered, kandevMcpHttp, kandevMcpSse)
	for _, srv := range mcpServers {
		if srv.Name != kandevMcpServerName {
			filtered = append(filtered, srv)
		}
	}
	s.logger.Debug("injected local kandev MCP servers (http+sse)",
		zap.String("http_url", kandevMcpHttp.URL),
		zap.String("sse_url", kandevMcpSse.URL),
		zap.Int("total_servers", len(filtered)))
	return filtered
}

// startMCPAttachmentAttempt records backend-owned identity before an adapter
// receives MCP configuration. The context is passed only to the adapter call;
// agents cannot choose the task, session, execution, or attempt identities.
func (s *Server) startMCPAttachmentAttempt(ctx context.Context, servers []types.McpServer) context.Context {
	attempt := streams.MCPAttachmentAttempt{
		AttemptID:   uuid.NewString(),
		TaskID:      s.cfg.TaskID,
		SessionID:   s.cfg.SessionID,
		ExecutionID: s.cfg.InstanceID,
		AgentID:     s.cfg.AgentType,
		StartedAt:   time.Now().UTC(),
	}
	s.procMgr.PublishMCPAttachmentAttempt(attempt)
	if s.mcpServer != nil {
		s.mcpServer.SetAttachmentAttempt(attempt)
	}
	for _, mcpServer := range servers {
		s.procMgr.PublishMCPAttachment(mcpAttachmentEvidence(attempt.AttemptID, mcpServer, streams.MCPAttachmentEvidenceConfigured, "", ""))
	}
	return streams.WithMCPAttachmentContext(ctx, attempt)
}

func (s *Server) publishMCPAttachmentResult(attemptID string, servers []types.McpServer, err error) {
	if adapter, ok := s.procMgr.GetAdapter().(mcpAttachmentResultPublisher); ok && adapter.PublishesMCPAttachmentResults() {
		return
	}
	kind := streams.MCPAttachmentEvidenceSessionAccepted
	reasonCode := ""
	summary := ""
	if err != nil {
		kind = streams.MCPAttachmentEvidenceExplicitError
		reasonCode = "session_request_failed"
		summary = err.Error()
	}
	for _, mcpServer := range servers {
		s.procMgr.PublishMCPAttachment(mcpAttachmentEvidence(attemptID, mcpServer, kind, reasonCode, summary))
	}
}

// mcpAttachmentResultPublisher identifies adapters that emit per-server
// delivery and session-result evidence after their own MCP capability filter.
// Generic adapters still receive result evidence from this API boundary.
type mcpAttachmentResultPublisher interface {
	PublishesMCPAttachmentResults() bool
}

func mcpAttachmentEvidence(
	attemptID string,
	mcpServer types.McpServer,
	kind streams.MCPAttachmentEvidenceKind,
	reasonCode string,
	summary string,
) streams.MCPAttachmentEvidence {
	target := streams.SanitizeMCPStdioTarget(mcpServer.Command)
	if mcpServer.Type == mcpTransportHTTP || mcpServer.Type == mcpTransportSSE || mcpServer.Type == "streamable_http" {
		target = streams.SanitizeMCPNetworkTarget(mcpServer.URL)
	}
	source := streams.MCPServerSourceProfile
	if mcpServer.Name == kandevMcpServerName {
		source = streams.MCPServerSourceKandev
	}
	return streams.MCPAttachmentEvidence{
		AttemptID:  attemptID,
		ServerName: mcpServer.Name,
		Kind:       kind,
		OccurredAt: time.Now().UTC(),
		Source:     source,
		Transport:  mcpServer.Type,
		Target:     target,
		ReasonCode: reasonCode,
		Summary:    streams.SanitizeMCPErrorSummary(summary),
	}
}

func (s *Server) handleWSNewSession(ctx context.Context, msg *ws.Message) *ws.Message {
	var req NewSessionRequest
	if err := msg.ParsePayload(&req); err != nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
		return resp
	}

	ctx, cancel := context.WithTimeout(ctx, constants.SessionNewTimeout)
	defer cancel()

	adapter := s.procMgr.GetAdapter()
	if adapter == nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "agent not running", nil)
		return resp
	}

	// Reset MCP backend client state to clear any pending requests from previous session.
	// This prevents stale MCP requests from interfering with the new session.
	if s.mcpBackendClient != nil {
		s.mcpBackendClient.Reset()
		s.logger.Debug("reset MCP backend client for new session")
	}

	// If MCP server is enabled, prepend the local kandev MCP server to the list.
	mcpServers := req.McpServers
	if s.mcpServer != nil {
		mcpServers = s.injectKandevMcpServers(mcpServers)
	}

	ctx = s.startMCPAttachmentAttempt(ctx, mcpServers)
	attachmentContext, _ := streams.MCPAttachmentContextFromContext(ctx)
	sessionID, err := adapter.NewSession(ctx, mcpServers)
	s.publishMCPAttachmentResult(attachmentContext.Attempt.AttemptID, mcpServers, err)
	if err != nil {
		s.logger.Error("new session failed", zap.Error(err))
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		return resp
	}

	resp, _ := ws.NewResponse(msg.ID, msg.Action, NewSessionResponse{
		Success:             true,
		SessionID:           sessionID,
		ModelState:          sessionModelState(adapter),
		AttachmentAttemptID: attachmentContext.Attempt.AttemptID,
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

	ctx, cancel := context.WithTimeout(ctx, constants.SessionLoadTimeout)
	defer cancel()

	adapter := s.procMgr.GetAdapter()
	if adapter == nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "agent not running", nil)
		return resp
	}

	// Reset MCP backend client state to clear any pending requests from previous session.
	// This prevents stale MCP requests from interfering with the loaded session.
	if s.mcpBackendClient != nil {
		s.mcpBackendClient.Reset()
		s.logger.Debug("reset MCP backend client for loaded session")
	}

	// If MCP server is enabled, prepend the local kandev MCP server to the list.
	mcpServers := req.McpServers
	if s.mcpServer != nil {
		mcpServers = s.injectKandevMcpServers(mcpServers)
	}

	ctx = s.startMCPAttachmentAttempt(ctx, mcpServers)
	attachmentContext, _ := streams.MCPAttachmentContextFromContext(ctx)
	if err := adapter.LoadSession(ctx, req.SessionID, mcpServers); err != nil {
		s.publishMCPAttachmentResult(attachmentContext.Attempt.AttemptID, mcpServers, err)
		s.logger.Error("load session failed", zap.Error(err))
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		return resp
	}
	s.publishMCPAttachmentResult(attachmentContext.Attempt.AttemptID, mcpServers, nil)

	resp, _ := ws.NewResponse(msg.ID, msg.Action, LoadSessionResponse{
		Success:             true,
		SessionID:           req.SessionID,
		ModelState:          sessionModelState(adapter),
		AttachmentAttemptID: attachmentContext.Attempt.AttemptID,
	})
	return resp
}

func (s *Server) handleWSResumeSession(ctx context.Context, msg *ws.Message) *ws.Message {
	var req ResumeSessionRequest
	if err := msg.ParsePayload(&req); err != nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
		return resp
	}
	if req.SessionID == "" {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "session_id is required", nil)
		return resp
	}

	ctx, cancel := context.WithTimeout(ctx, constants.SessionLoadTimeout)
	defer cancel()
	adapter := s.procMgr.GetAdapter()
	if adapter == nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "agent not running", nil)
		return resp
	}
	capabilities, ok := adapter.(interface {
		SupportsSessionResume() bool
		SupportsSessionLoad() bool
	})
	if !ok || !capabilities.SupportsSessionResume() {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "agent does not support session resume", nil)
		return resp
	}
	if s.mcpBackendClient != nil {
		s.mcpBackendClient.Reset()
	}
	mcpServers := req.McpServers
	if s.mcpServer != nil {
		mcpServers = s.injectKandevMcpServers(mcpServers)
	}
	ctx = s.startMCPAttachmentAttempt(ctx, mcpServers)
	attachmentContext, _ := streams.MCPAttachmentContextFromContext(ctx)
	resumer, ok := adapter.(interface {
		ResumeSession(context.Context, string, []types.McpServer) error
	})
	if !ok {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "agent adapter does not support session resume", nil)
		return resp
	}
	if err := resumer.ResumeSession(ctx, req.SessionID, mcpServers); err != nil {
		s.publishMCPAttachmentResult(attachmentContext.Attempt.AttemptID, mcpServers, err)
		s.logger.Error("resume session failed", zap.Error(err))
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "session resume failed", nil)
		return resp
	}
	s.publishMCPAttachmentResult(attachmentContext.Attempt.AttemptID, mcpServers, nil)
	resp, _ := ws.NewResponse(msg.ID, msg.Action, ResumeSessionResponse{
		Success: true, SessionID: req.SessionID, ModelState: sessionModelState(adapter),
		AttachmentAttemptID: attachmentContext.Attempt.AttemptID,
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

	// Cancel any pending permissions so the agent isn't blocked waiting for
	// the user to approve a previous tool call while processing the new prompt.
	s.procMgr.CancelPendingPermissions()

	// Start prompt processing asynchronously.
	// Completion is signaled via the WebSocket complete event, not this response.
	// Use context.Background() so the prompt is NOT tied to the WebSocket connection
	// lifetime. For remote executors, a temporary network glitch would otherwise kill
	// the in-flight prompt even though the agent subprocess keeps running fine.
	// The prompt completes naturally when the agent process exits (stdin/stdout close),
	// the user cancels, or agentctl shuts down.
	go func() {
		if err := promptOrSteer(context.Background(), adapter, req); err != nil {
			if acptransport.IsPromptAbandonedAfterCancel(err) {
				s.logger.Info("async prompt abandoned after cancel; suppressing stale error event",
					zap.Error(err))
				return
			}
			providerError := acptransport.ProviderErrorFromError(err)
			// The raw error string is only safe for the correlated stderr
			// diagnostic (its Error() is the sanitized message). A structured
			// ACP RequestError serializes its Data — including action_url and
			// any other raw provider fields — into Error(), so when a safe
			// provider diagnostic exists it must win for both the event and
			// the log; never let the raw error cross the boundary.
			message := err.Error()
			if providerError != nil {
				message = providerError.Message
			}
			s.logger.Error("async prompt failed", zap.String("error_message", message))
			s.procMgr.SendErrorEventWithProviderError(
				message,
				req.PromptGeneration,
				providerError,
			)
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
		notAck := errors.Is(err, acptransport.ErrTurnCancelNotAcknowledged)
		if notAck {
			s.logger.Warn("cancel not acknowledged by in-flight prompt", zap.Error(err))
		} else {
			s.logger.Error("cancel failed", zap.Error(err))
		}
		resp, _ := ws.NewResponse(msg.ID, msg.Action, CancelResponse{
			Success:         false,
			Error:           err.Error(),
			NotAcknowledged: notAck,
		})
		return resp
	}

	s.logger.Info("cancel acknowledged")
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

func (s *Server) handleWSSetMode(ctx context.Context, msg *ws.Message) *ws.Message {
	var req struct {
		SessionID string `json:"session_id"`
		ModeID    string `json:"mode_id"`
	}
	return s.adapterAction(ctx, msg, &req, func(a adapter.AgentAdapter) error {
		ms, ok := a.(adapter.ModeSettableAdapter)
		if !ok {
			return fmt.Errorf("agent does not support mode switching")
		}
		return ms.SetMode(ctx, req.ModeID)
	})
}

// adapterAction extracts common boilerplate for WS handlers that operate on the agent adapter:
// parse payload, get adapter, assert interface, call action.
func (s *Server) adapterAction(
	ctx context.Context,
	msg *ws.Message,
	payload any,
	action func(agentAdapter adapter.AgentAdapter) error,
) *ws.Message {
	if err := msg.ParsePayload(payload); err != nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
		return resp
	}

	agentAdapter := s.procMgr.GetAdapter()
	if agentAdapter == nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "agent not running", nil)
		return resp
	}

	if err := action(agentAdapter); err != nil {
		s.logger.Error(msg.Action+" failed", zap.Error(err))
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		return resp
	}

	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{"success": true})
	return resp
}

func (s *Server) handleWSSetModel(ctx context.Context, msg *ws.Message) *ws.Message {
	var req struct {
		ModelID string `json:"model_id"`
	}
	return s.adapterAction(ctx, msg, &req, func(a adapter.AgentAdapter) error {
		ms, ok := a.(adapter.ModelSettableAdapter)
		if !ok {
			return fmt.Errorf("agent does not support model switching")
		}
		return ms.SetModel(ctx, req.ModelID)
	})
}

func (s *Server) handleWSSetConfigOption(ctx context.Context, msg *ws.Message) *ws.Message {
	var req struct {
		ConfigID string `json:"config_id"`
		Value    string `json:"value"`
	}
	return s.adapterAction(ctx, msg, &req, func(a adapter.AgentAdapter) error {
		cs, ok := a.(adapter.ConfigOptionSettableAdapter)
		if !ok {
			return fmt.Errorf("agent does not support set_config_option")
		}
		return cs.SetConfigOption(ctx, req.ConfigID, req.Value)
	})
}

func (s *Server) handleWSAuthenticate(ctx context.Context, msg *ws.Message) *ws.Message {
	var req struct {
		MethodID string `json:"method_id"`
	}
	return s.adapterAction(ctx, msg, &req, func(a adapter.AgentAdapter) error {
		aa, ok := a.(adapter.AuthenticatableAdapter)
		if !ok {
			return fmt.Errorf("agent does not support authenticate")
		}
		return aa.Authenticate(ctx, req.MethodID)
	})
}

func (s *Server) handleWSResetSession(ctx context.Context, msg *ws.Message) *ws.Message {
	var req NewSessionRequest
	if err := msg.ParsePayload(&req); err != nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
		return resp
	}

	agentAdapter := s.procMgr.GetAdapter()
	if agentAdapter == nil {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "agent not running", nil)
		return resp
	}

	sr, ok := agentAdapter.(adapter.SessionResettableAdapter)
	if !ok {
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "agent does not support session reset", nil)
		return resp
	}

	// If MCP server is enabled, prepend the local kandev MCP server to the list.
	mcpServers := req.McpServers
	if s.mcpServer != nil {
		mcpServers = s.injectKandevMcpServers(mcpServers)
	}

	ctx = s.startMCPAttachmentAttempt(ctx, mcpServers)
	attachmentContext, _ := streams.MCPAttachmentContextFromContext(ctx)
	sessionID, err := sr.ResetSession(ctx, mcpServers)
	s.publishMCPAttachmentResult(attachmentContext.Attempt.AttemptID, mcpServers, err)
	if err != nil {
		s.logger.Error("session reset failed", zap.Error(err))
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		return resp
	}

	resp, _ := ws.NewResponse(msg.ID, msg.Action, NewSessionResponse{
		Success:             true,
		SessionID:           sessionID,
		ModelState:          sessionModelState(agentAdapter),
		AttachmentAttemptID: attachmentContext.Attempt.AttemptID,
	})
	return resp
}

func sessionModelState(agentAdapter adapter.AgentAdapter) *streams.SessionModelState {
	provider, ok := agentAdapter.(adapter.SessionModelStateProvider)
	if !ok {
		return nil
	}
	return provider.GetSessionModelState()
}

// promptOrSteer routes a prompt to the steering path when the caller asked for it
// and the adapter can actually do it. An adapter that does not implement
// SteerablePrompter, or a connected agent that never advertised the capability,
// falls back to the ordinary prompt — the message still reaches the agent, just
// at the next turn boundary, which is exactly today's behavior.
func promptOrSteer(
	ctx context.Context,
	adpt adapter.AgentAdapter,
	req PromptRequest,
) error {
	if req.Steer {
		if steerable, ok := adpt.(adapter.SteerablePrompter); ok && steerable.SupportsSteering() {
			return steerable.PromptSteer(ctx, req.Text, req.Attachments, req.PromptGeneration)
		}
	}
	return adpt.Prompt(ctx, req.Text, req.Attachments, req.PromptGeneration)
}
