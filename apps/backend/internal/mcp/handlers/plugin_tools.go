package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type pluginToolsListRequest struct {
	Surface string `json:"surface"`
}

type pluginToolInvocationRequest struct {
	PluginID     string         `json:"plugin_id"`
	LocalName    string         `json:"local_name"`
	InvocationID string         `json:"invocation_id"`
	TaskID       string         `json:"task_id"`
	SessionID    string         `json:"session_id"`
	WorkspaceID  string         `json:"workspace_id"`
	Surface      string         `json:"surface"`
	Arguments    map[string]any `json:"arguments"`
}

func (h *Handlers) handleListPluginTools(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req pluginToolsListRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
	}
	policy, managed, response, err := h.managedToolPolicyForRequest(ctx, msg, req.Surface)
	if err != nil || response != nil {
		return response, err
	}
	snapshot, err := h.pluginSvc.AgentToolCatalog()
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	if managed {
		filtered := snapshot
		filtered.Tools = make([]plugintools.Definition, 0, len(snapshot.Tools))
		for _, tool := range snapshot.Tools {
			if tool.PluginID == policy.PluginID && policy.Allows(tool.PluginID, tool.LocalName) && slices.Contains(tool.Surfaces, string(mcpprofile.SurfaceManagedConversation)) {
				filtered.Tools = append(filtered.Tools, tool)
			}
		}
		snapshot = filtered
	} else if req.Surface != "" {
		filtered := snapshot
		filtered.Tools = make([]plugintools.Definition, 0, len(snapshot.Tools))
		for _, tool := range snapshot.Tools {
			if slices.Contains(tool.Surfaces, req.Surface) {
				filtered.Tools = append(filtered.Tools, tool)
			}
		}
		snapshot = filtered
	}
	return ws.NewResponse(msg.ID, msg.Action, snapshot)
}

func (h *Handlers) handleInvokePluginTool(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req pluginToolInvocationRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
	}
	if req.PluginID == "" || req.LocalName == "" || req.Surface == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "plugin_id, local_name, and surface are required", nil)
	}
	policy, managed, policyErrorResponse, policyErr := h.managedToolPolicyForRequest(ctx, msg, req.Surface)
	if policyErr != nil || policyErrorResponse != nil {
		return policyErrorResponse, policyErr
	}
	if managed {
		if !policy.Allows(req.PluginID, req.LocalName) {
			h.logManagedToolPolicyDenial(ctx, msg.Action, "tool_not_selected")
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "tool is not allowed by the managed tool policy", nil)
		}
		req.Surface = string(mcpprofile.SurfaceManagedConversation)
	}
	invocation, contextErrorResponse, err := h.resolvePluginToolInvocationContext(ctx, msg, req)
	if err != nil || contextErrorResponse != nil {
		if managed {
			h.logManagedToolPolicyDenial(ctx, msg.Action, "request_context_mismatch")
		}
		return contextErrorResponse, err
	}
	if managed {
		invocation.ManagedToolPolicy = policy
	}
	result, err := h.pluginSvc.InvokeAgentTool(ctx, req.PluginID, req.LocalName, req.Arguments, invocation)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, fmt.Sprintf("plugin tool invocation failed: %v", err), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{
		"text": result.Text, "structured_content": result.StructuredContent, "is_error": result.IsError,
	})
}

func (h *Handlers) managedToolPolicyForRequest(ctx context.Context, msg *ws.Message, requestedSurface string) (*mcpprofile.ManagedToolPolicy, bool, *ws.Message, error) {
	execution, ok := streams.MCPExecutionContextFromContext(ctx)
	if !ok {
		return nil, false, nil, nil
	}
	if !execution.ManagedToolPolicyRequired && execution.ManagedToolPolicy == nil {
		return nil, false, nil, nil
	}
	policy := execution.ManagedToolPolicy
	if execution.ManagedToolPolicyRequired && policy == nil {
		h.logManagedToolPolicyDenial(ctx, msg.Action, "policy_metadata_invalid")
		response, err := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "managed tool policy metadata is invalid", nil)
		return nil, true, response, err
	}
	if policy == nil || policy.Validate() != nil || requestedSurface != string(mcpprofile.SurfaceManagedConversation) {
		h.logManagedToolPolicyDenial(ctx, msg.Action, "policy_or_surface_invalid")
		response, err := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "request is not authorized by the managed tool policy", nil)
		return nil, true, response, err
	}
	if err := h.validateManagedToolExecution(ctx, execution, *policy); err != nil {
		h.logManagedToolPolicyDenial(ctx, msg.Action, "execution_provenance_denied")
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "managed tool policy denied this execution: "+err.Error(), nil)
		return nil, true, response, responseErr
	}
	return policy, true, nil, nil
}

func (h *Handlers) logManagedToolPolicyDenial(ctx context.Context, action, reason string) {
	if h == nil || h.logger == nil {
		return
	}
	execution, _ := streams.MCPExecutionContextFromContext(ctx)
	h.logger.Warn("managed agent tool policy denied an MCP request",
		zap.String("action", action),
		zap.String("reason", reason),
		zap.String("task_id", execution.TaskID),
		zap.String("session_id", execution.SessionID))
}

type managedToolExecutionReader interface {
	GetExecutorRunningBySessionID(context.Context, string) (*models.ExecutorRunning, error)
}

//nolint:cyclop // Fail closed across caller, profile, broker, and tool identity before dispatch.
func (h *Handlers) validateManagedToolExecution(ctx context.Context, execution streams.MCPExecutionContext, policy mcpprofile.ManagedToolPolicy) error {
	if execution.ExecutionID == "" || policy.WorkspaceID == "" || h.sessionRepo == nil || h.taskSvc == nil {
		return errors.New("trusted execution metadata is unavailable")
	}
	reader, ok := h.sessionRepo.(managedToolExecutionReader)
	if !ok {
		return errors.New("current execution generation is unavailable")
	}
	running, err := reader.GetExecutorRunningBySessionID(ctx, execution.SessionID)
	if err != nil || running == nil || running.AgentExecutionID != execution.ExecutionID || running.Status != models.ExecutorRunningStatusRunning {
		return errors.New("execution generation is stale")
	}
	session, err := h.sessionRepo.GetTaskSession(ctx, execution.SessionID)
	if err != nil || session == nil || session.TaskID != execution.TaskID || session.State != models.TaskSessionStateRunning {
		return errors.New("managed tool session is not bound to task")
	}
	task, err := h.taskSvc.GetTask(ctx, execution.TaskID)
	if err != nil || task == nil || task.WorkspaceID != policy.WorkspaceID {
		return errors.New("managed tool workspace provenance is unavailable")
	}
	metadata := task.Metadata
	if metadata == nil || !metadataBoolIs(metadata, "kandev.managed_retained", true) ||
		models.StringFromAny(metadata["kandev.managed_by_plugin"]) != policy.PluginID ||
		models.StringFromAny(metadata["kandev.installation_id"]) != policy.InstallationID ||
		models.StringFromAny(metadata["kandev.workspace_id"]) != policy.WorkspaceID ||
		models.StringFromAny(metadata["kandev.instance_key"]) != policy.InstanceKey ||
		models.StringFromAny(metadata["kandev.manifest_digest"]) != policy.ManifestDigest ||
		managedToolRevision(metadata["kandev.conversation_revision"]) != policy.ConversationRevision ||
		managedToolRevision(metadata["kandev.approval_revision"]) != policy.ApprovalRevision {
		return errors.New("managed conversation identity or revision is stale")
	}
	if !metadataBoolIs(metadata, "kandev.desired_paused", false) || !metadataBoolIs(metadata, "kandev.detached", false) {
		return errors.New("managed conversation is paused or detached")
	}
	allowed, err := managedToolNames(metadata["kandev.agent_tool_names"])
	if err != nil || !slices.Equal(allowed, policy.AgentToolNames) {
		return errors.New("managed tool selection is stale")
	}
	return nil
}

func metadataBoolIs(metadata map[string]interface{}, key string, expected bool) bool {
	value, ok := metadata[key].(bool)
	return ok && value == expected
}

func managedToolRevision(value any) uint64 {
	revision, _ := strconv.ParseUint(strings.TrimSpace(models.StringFromAny(value)), 10, 64)
	return revision
}

func managedToolNames(value any) ([]string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var names []string
	if err := json.Unmarshal(encoded, &names); err != nil {
		return nil, err
	}
	slices.Sort(names)
	return names, nil
}

func (h *Handlers) resolvePluginToolInvocationContext(ctx context.Context, msg *ws.Message, req pluginToolInvocationRequest) (plugins.AgentToolInvocationContext, *ws.Message, error) {
	execution, ok := streams.MCPExecutionContextFromContext(ctx)
	if !ok || h.sessionRepo == nil || h.taskSvc == nil {
		return pluginToolContextError(msg, ws.ErrorCodeInternalError, "plugin tool execution context is unavailable")
	}
	taskID, sessionID := execution.TaskID, execution.SessionID
	if pluginToolRequestContextMismatch(req, execution) {
		return pluginToolContextError(msg, ws.ErrorCodeBadRequest, "request context does not match the running execution")
	}
	session, err := h.sessionRepo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID {
		return pluginToolContextError(msg, ws.ErrorCodeBadRequest, "running session is not bound to task")
	}
	task, err := h.taskSvc.GetTask(ctx, taskID)
	if err != nil || task == nil || task.WorkspaceID == "" {
		return pluginToolContextError(msg, ws.ErrorCodeInternalError, "failed to resolve the execution workspace")
	}
	if req.WorkspaceID != "" && req.WorkspaceID != task.WorkspaceID {
		return pluginToolContextError(msg, ws.ErrorCodeBadRequest, "request context does not match the running execution")
	}
	return plugins.AgentToolInvocationContext{
		InvocationID: req.InvocationID, TaskID: taskID, SessionID: sessionID,
		WorkspaceID: task.WorkspaceID, Surface: req.Surface,
		ExecutionID: execution.ExecutionID,
	}, nil, nil
}

func pluginToolRequestContextMismatch(req pluginToolInvocationRequest, execution streams.MCPExecutionContext) bool {
	return (req.TaskID != "" && req.TaskID != execution.TaskID) ||
		(req.SessionID != "" && req.SessionID != execution.SessionID)
}

func pluginToolContextError(msg *ws.Message, code, message string) (plugins.AgentToolInvocationContext, *ws.Message, error) {
	response, err := ws.NewError(msg.ID, msg.Action, code, message, nil)
	return plugins.AgentToolInvocationContext{}, response, err
}
