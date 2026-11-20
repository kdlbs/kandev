package lifecycle

import (
	"context"
	"errors"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
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
	builtinAuthority          streams.BuiltinMCPAuthority
	inner                     agentctl.MCPHandler
	scope                     MCPIdentityScoper
	principal                 MCPPrincipalScoper
	executionID               string
	taskID                    string
	sessionID                 string
	managedToolPolicy         *mcpprofile.ManagedToolPolicy
	managedToolPolicyRequired bool
	coordinatorToolPolicy     *mcpprofile.CoordinatorToolPolicy
	coordinatorPolicyRequired bool
	logger                    *logger.Logger
}

type currentMCPHandler struct {
	execution                 *AgentExecution
	streamManager             *StreamManager
	executionID               string
	taskID                    string
	sessionID                 string
	managedToolPolicy         *mcpprofile.ManagedToolPolicy
	managedToolPolicyRequired bool
	coordinatorToolPolicy     *mcpprofile.CoordinatorToolPolicy
	coordinatorPolicyRequired bool
}

func (h *currentMCPHandler) Dispatch(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.execution != nil {
		if _, bound := h.execution.metadataValue(mcpprofile.BuiltinBindingMetadataKey); bound &&
			(h.taskID == "" || h.sessionID == "" || h.execution.ID != h.executionID || h.execution.TaskID != h.taskID || h.execution.SessionID != h.sessionID ||
				(h.streamManager.isExecutionCurrent != nil && !h.streamManager.isExecutionCurrent(h.execution))) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnknownAction, "tool is not available in this execution", nil)
		}
	}
	inner, scope, principal := h.streamManager.mcpDispatchState()
	if inner == nil {
		return nil, errMCPHandlerUnavailable
	}
	if h.taskID == "" {
		return inner.Dispatch(ctx, msg)
	}
	return (&taskScopedMCPHandler{
		inner:                     inner,
		builtinAuthority:          h.execution.builtinMCPAuthority(h.streamManager.builtinResolver()),
		scope:                     scope,
		principal:                 principal,
		executionID:               h.executionID,
		taskID:                    h.taskID,
		sessionID:                 h.sessionID,
		managedToolPolicy:         h.managedToolPolicy,
		managedToolPolicyRequired: h.managedToolPolicyRequired,
		coordinatorToolPolicy:     h.coordinatorToolPolicy,
		coordinatorPolicyRequired: h.coordinatorPolicyRequired,
		logger:                    h.streamManager.logger,
	}).Dispatch(ctx, msg)
}

func (h *taskScopedMCPHandler) Dispatch(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if !h.builtinAuthority.Allows(msg.Action) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnknownAction, "tool is not available in this execution", nil)
	}
	if mcporigin.MessageCarriesTrustedInternalCall(msg) {
		ctx = mcporigin.WithTrustedInternalCall(ctx)
	}
	ctx = streams.WithMCPExecutionContext(ctx, streams.MCPExecutionContext{
		ExecutionID:               h.executionID,
		BuiltinAuthority:          h.builtinAuthority,
		TaskID:                    h.taskID,
		SessionID:                 h.sessionID,
		ManagedToolPolicy:         h.managedToolPolicy,
		ManagedToolPolicyRequired: h.managedToolPolicyRequired,

		CoordinatorToolPolicy:         h.coordinatorToolPolicy,
		CoordinatorToolPolicyRequired: h.coordinatorPolicyRequired,
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
		return h.dispatchBuiltinScoped(scoped, msg)
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
	return h.dispatchBuiltinScoped(principalScoped, msg)
}

// mcpHandlerFor returns the MCP handler for one execution's stream. The
// backend-owned execution identity is always attached; user identity is also
// attached when per-user scoping has been wired.
func (sm *StreamManager) mcpHandlerFor(execution *AgentExecution) agentctl.MCPHandler {
	var managedToolPolicy *mcpprofile.ManagedToolPolicy
	managedToolPolicyRequired := false
	if value, present := execution.metadataValue(mcpprofile.ManagedToolPolicyMetadataKey); present {
		managedToolPolicyRequired = true
		var err error
		managedToolPolicy, err = mcpprofile.ParseManagedToolPolicyMetadata(value)
		if err != nil {
			sm.logger.Warn("managed agent execution has invalid tool policy metadata",
				zap.String("execution_id", execution.ID), zap.Error(err))
		}
	}
	var coordinatorToolPolicy *mcpprofile.CoordinatorToolPolicy
	coordinatorPolicyRequired := false
	if value, present := execution.metadataValue(mcpprofile.CoordinatorToolPolicyMetadataKey); present {
		coordinatorPolicyRequired = true
		var err error
		coordinatorToolPolicy, err = mcpprofile.ParseCoordinatorToolPolicyMetadata(value)
		if err != nil {
			sm.logger.Warn("coordinator agent execution has invalid tool policy metadata",
				zap.String("execution_id", execution.ID), zap.Error(err))
		}
	}
	return &currentMCPHandler{
		execution:                 execution,
		streamManager:             sm,
		executionID:               execution.ID,
		taskID:                    execution.TaskID,
		sessionID:                 execution.SessionID,
		managedToolPolicy:         managedToolPolicy,
		managedToolPolicyRequired: managedToolPolicyRequired,
		coordinatorToolPolicy:     coordinatorToolPolicy,
		coordinatorPolicyRequired: coordinatorPolicyRequired,
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

func (h *taskScopedMCPHandler) dispatchBuiltinScoped(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.builtinAuthority.Required && !mcpscope.BuiltinPrincipalMatches(ctx, h.builtinAuthority.Manifest) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnknownAction, "built-in MCP binding does not match the session owner", nil)
	}
	return h.inner.Dispatch(ctx, msg)
}
