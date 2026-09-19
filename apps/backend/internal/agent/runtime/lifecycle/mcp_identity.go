package lifecycle

import (
	"context"
	"errors"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

var errMCPHandlerUnavailable = errors.New("MCP handler is not configured")

// MCPIdentityScoper attaches the identity of the user who owns taskID to ctx,
// so the in-session MCP tools an agent gets automatically are authorized as
// that user instead of running unscoped. Returning an error denies the
// dispatch — see internal/mcp/scope for the production implementation and why
// it fails closed rather than falling back to full access.
type MCPIdentityScoper func(ctx context.Context, taskID string) (context.Context, error)

// MCPPrincipalScoper attaches the server-derived task/session MCP principal.
// It runs after the owner identity scoper and is independent of auth mode:
// automation self/workspace boundaries are required even on single-user
// installations.
type MCPPrincipalScoper func(ctx context.Context, taskID, sessionID string) (context.Context, error)

// taskScopedMCPHandler scopes every MCP request on one agent stream to the
// owner of that stream's task.
//
// The task ID comes from the AgentExecution this stream belongs to, never from
// the request payload: an agent controls its own payloads, so honoring a
// payload session_id would let it name another user's session and inherit
// their identity — turning the scoping fix into a privilege escalation.
type taskScopedMCPHandler struct {
	inner       agentctl.MCPHandler
	scope       MCPIdentityScoper
	principal   MCPPrincipalScoper
	executionID string
	taskID      string
	sessionID   string
	logger      *logger.Logger
}

type currentMCPHandler struct {
	streamManager *StreamManager
	executionID   string
	taskID        string
	sessionID     string
}

func (h *currentMCPHandler) Dispatch(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	inner, scope, principal := h.streamManager.mcpDispatchState()
	if inner == nil {
		return nil, errMCPHandlerUnavailable
	}
	if h.taskID == "" {
		return inner.Dispatch(ctx, msg)
	}
	return (&taskScopedMCPHandler{
		inner:       inner,
		scope:       scope,
		principal:   principal,
		executionID: h.executionID,
		taskID:      h.taskID,
		sessionID:   h.sessionID,
		logger:      h.streamManager.logger,
	}).Dispatch(ctx, msg)
}

func (h *taskScopedMCPHandler) Dispatch(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	ctx = streams.WithMCPExecutionContext(ctx, streams.MCPExecutionContext{
		ExecutionID: h.executionID,
		TaskID:      h.taskID,
		SessionID:   h.sessionID,
	})
	scoped := ctx
	if h.scope != nil {
		var err error
		scoped, err = h.scope(ctx, h.taskID)
		if err != nil {
			h.logger.Error("denying in-session MCP request: cannot resolve the task owner",
				zap.String("task_id", h.taskID),
				zap.String("action", msg.Action),
				zap.Error(err))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
				"failed to resolve the session owner", nil)
		}
	}
	if h.principal == nil {
		return h.inner.Dispatch(scoped, msg)
	}
	principalScoped, err := h.principal(scoped, h.taskID, h.sessionID)
	if err != nil {
		h.logger.Error("denying in-session MCP request: cannot resolve the caller principal",
			zap.String("task_id", h.taskID),
			zap.String("session_id", h.sessionID),
			zap.String("action", msg.Action),
			zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"failed to resolve the session principal", nil)
	}
	return h.inner.Dispatch(principalScoped, msg)
}

// mcpHandlerFor returns the MCP handler for one execution's stream. The
// backend-owned execution identity is always attached; user identity is also
// attached when per-user scoping has been wired.
func (sm *StreamManager) mcpHandlerFor(execution *AgentExecution) agentctl.MCPHandler {
	return &currentMCPHandler{
		streamManager: sm,
		executionID:   execution.ID,
		taskID:        execution.TaskID,
		sessionID:     execution.SessionID,
	}
}

func (sm *StreamManager) mcpDispatchState() (agentctl.MCPHandler, MCPIdentityScoper, MCPPrincipalScoper) {
	sm.mcpMu.RLock()
	defer sm.mcpMu.RUnlock()
	return sm.mcpHandler, sm.mcpIdentityScoper, sm.mcpPrincipalScoper
}

func (sm *StreamManager) setMCPHandler(handler agentctl.MCPHandler) {
	sm.mcpMu.Lock()
	defer sm.mcpMu.Unlock()
	sm.mcpHandler = handler
}

func (sm *StreamManager) setMCPIdentityScoper(scoper MCPIdentityScoper) {
	sm.mcpMu.Lock()
	defer sm.mcpMu.Unlock()
	sm.mcpIdentityScoper = scoper
}

func (sm *StreamManager) setMCPPrincipalScoper(scoper MCPPrincipalScoper) {
	sm.mcpMu.Lock()
	defer sm.mcpMu.Unlock()
	sm.mcpPrincipalScoper = scoper
}
