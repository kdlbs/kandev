package lifecycle

import (
	"context"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// MCPIdentityScoper attaches the identity of the user who owns taskID to ctx,
// so the in-session MCP tools an agent gets automatically are authorized as
// that user instead of running unscoped. Returning an error denies the
// dispatch — see internal/mcp/scope for the production implementation and why
// it fails closed rather than falling back to full access.
type MCPIdentityScoper func(ctx context.Context, taskID string) (context.Context, error)

// taskScopedMCPHandler scopes every MCP request on one agent stream to the
// owner of that stream's task.
//
// The task ID comes from the AgentExecution this stream belongs to, never from
// the request payload: an agent controls its own payloads, so honoring a
// payload session_id would let it name another user's session and inherit
// their identity — turning the scoping fix into a privilege escalation.
type taskScopedMCPHandler struct {
	inner  agentctl.MCPHandler
	scope  MCPIdentityScoper
	taskID string
	logger *logger.Logger
}

func (h *taskScopedMCPHandler) Dispatch(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	scoped, err := h.scope(ctx, h.taskID)
	if err != nil {
		h.logger.Error("denying in-session MCP request: cannot resolve the task owner",
			zap.String("task_id", h.taskID),
			zap.String("action", msg.Action),
			zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"failed to resolve the session owner", nil)
	}
	return h.inner.Dispatch(scoped, msg)
}

// mcpHandlerFor returns the MCP handler for one execution's stream, scoped to
// the task's owning user when per-user scoping has been wired. Without a
// scoper (tests, or the runtime tier used standalone) the handler is passed
// through unchanged.
func (sm *StreamManager) mcpHandlerFor(execution *AgentExecution) agentctl.MCPHandler {
	if sm.mcpHandler == nil || sm.mcpIdentityScoper == nil || execution.TaskID == "" {
		return sm.mcpHandler
	}
	return &taskScopedMCPHandler{
		inner:  sm.mcpHandler,
		scope:  sm.mcpIdentityScoper,
		taskID: execution.TaskID,
		logger: sm.logger,
	}
}
