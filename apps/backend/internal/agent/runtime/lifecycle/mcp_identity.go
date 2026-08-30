package lifecycle

import (
	"context"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

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
	if sm.mcpHandler == nil || execution.TaskID == "" {
		return sm.mcpHandler
	}
	return &taskScopedMCPHandler{
		inner:       sm.mcpHandler,
		scope:       sm.mcpIdentityScoper,
		principal:   sm.mcpPrincipalScoper,
		executionID: execution.ID,
		taskID:      execution.TaskID,
		sessionID:   execution.SessionID,
		logger:      sm.logger,
	}
}
