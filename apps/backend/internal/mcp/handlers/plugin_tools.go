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
	"github.com/kandev/kandev/internal/auth/authn"
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/kdlbs/kandev/pluginsdk"
	"go.uber.org/zap"
)

const (
	externalWorkspaceArgumentDescription = "Workspace ID to read. Use list_workspaces_kandev to discover workspace IDs."
	externalWorkspaceUnavailableMessage  = "workspace is not available"
)

type pluginToolsListRequest struct {
	Surface string `json:"surface"`
}

// AutomationPluginTools is the Office-owned authority the plugin bridge uses
// for filtered discovery and per-call approval checks. MCP payload fields are
// never used to construct these identities.
type AutomationPluginTools interface {
	PluginToolOptions(ctx context.Context, workspaceID string) ([]plugintools.OptionGroup, error)
	ApprovedPluginTools(ctx context.Context, automationID, workspaceID string) ([]plugintools.Definition, error)
	AutomationToolAuthorization(ctx context.Context, automationID, workspaceID, pluginID string) (*plugintools.AutomationAuthorization, error)
}

// SetAutomationPluginTools wires the automation selection authority used by
// the only plugin surface that supports saved approvals.
func (h *Handlers) SetAutomationPluginTools(provider AutomationPluginTools) {
	h.automationPluginTools = provider
}

// SetPluginToolAuthEnabled reports whether endpoint authentication is
// enforced. The external workspace scope fails closed when enforcement is on
// but the request context carries no real identity.
func (h *Handlers) SetPluginToolAuthEnabled(authEnabled func() bool) {
	h.pluginToolAuthEnabled = authEnabled
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
	if principal, ok := mcpscope.PrincipalFromContext(ctx); ok && principal.IsAutomation() {
		return h.handleListAutomationPluginTools(ctx, msg, req, principal)
	}
	if principal, ok := mcpscope.PrincipalFromContext(ctx); ok && principal.IsCoordinator() {
		if req.Surface != string(mcpprofile.SurfaceCoordinator) {
			h.logPluginToolDenial(ctx, msg.Action, "coordinator_surface_forged")
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "coordinator plugin tools require the coordinator surface", nil)
		}
		return h.handleListCoordinatorPluginTools(ctx, msg, req)
	}
	switch req.Surface {
	case string(mcpprofile.SurfaceAutomation):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "automation plugin tools require an automation execution", nil)
	case string(mcpprofile.SurfaceExternal):
		return h.handleListExternalPluginTools(ctx, msg, req)
	case string(mcpprofile.SurfaceCoordinator):
		return h.handleListCoordinatorPluginTools(ctx, msg, req)
	}
	// Ordinary pulled catalogs must stay compatible with older retained
	// agentctl validators that do not know the instance-only surfaces.
	policy, managed, response, err := h.managedToolPolicyForRequest(ctx, msg, req.Surface)
	if err != nil || response != nil {
		return response, err
	}
	if principal, ok := mcpscope.PrincipalFromContext(ctx); ok && req.Surface != "" &&
		string(principal.Surface) != req.Surface {
		h.logPluginToolDenial(ctx, msg.Action, "principal_surface_mismatch")
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "plugin tool surface does not match the caller principal", nil)
	}
	snapshot, err := h.pluginSvc.AgentToolCatalog()
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	snapshot = plugintools.ProjectLegacySafe(snapshot)
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

func (h *Handlers) handleListAutomationPluginTools(ctx context.Context, msg *ws.Message, req pluginToolsListRequest, principal mcpscope.Principal) (*ws.Message, error) {
	if req.Surface != string(mcpprofile.SurfaceAutomation) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "automation execution may only request the automation surface", nil)
	}
	if h.automationPluginTools == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "automation plugin tools are unavailable", nil)
	}
	definitions, err := h.automationPluginTools.ApprovedPluginTools(ctx, principal.AutomationID, principal.WorkspaceID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to load approved plugin tools", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, plugintools.Snapshot{
		Generation: "automation-" + principal.AutomationID,
		Revision:   1,
		Tools:      definitions,
	})
}

// handleListExternalPluginTools serves the trusted external MCP transport.
// Caller payloads cannot upgrade a task execution to this context.
func (h *Handlers) handleListExternalPluginTools(ctx context.Context, msg *ws.Message, req pluginToolsListRequest) (*ws.Message, error) {
	if err := h.authorizeExternalPluginToolRequest(ctx); err != nil {
		h.logPluginToolDenial(ctx, msg.Action, "untrusted_or_forged_transport")
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, err.Error(), nil)
	}
	if h.pluginSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "plugin tools are unavailable", nil)
	}
	snapshot, err := h.filteredPluginToolSnapshot(func(definition plugintools.Definition) bool {
		return slices.Contains(definition.Surfaces, string(mcpprofile.SurfaceExternal)) &&
			(definition.IsInstanceRead() || definition.IsWorkspaceRead())
	})
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("external plugin tool catalog failed", zap.Error(err))
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to load plugin tools", nil)
	}
	for i := range snapshot.Tools {
		definition, injectErr := withExternalWorkspaceArgument(snapshot.Tools[i])
		if injectErr != nil {
			if h.logger != nil {
				h.logger.Warn("external plugin tool catalog failed", zap.Error(injectErr))
			}
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to load plugin tools", nil)
		}
		snapshot.Tools[i] = definition
	}
	return ws.NewResponse(msg.ID, msg.Action, snapshot)
}

// handleListCoordinatorPluginTools serves a coordinator session's exact
// version-2 plugin binding. A missing, invalid, or version-1 binding grants
// no plugin reads.
func (h *Handlers) handleListCoordinatorPluginTools(ctx context.Context, msg *ws.Message, req pluginToolsListRequest) (*ws.Message, error) {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || !principal.IsCoordinator() {
		h.logPluginToolDenial(ctx, msg.Action, "coordinator_principal_required")
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "coordinator plugin tools require a coordinator session", nil)
	}
	if h.pluginSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "plugin tools are unavailable", nil)
	}
	if !h.coordinatorPhase2Enabled() {
		h.logPluginToolDenial(ctx, msg.Action, "phase2_disabled")
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "coordinator plugin tools require phase 2", nil)
	}
	binding, ok := h.coordinatorPluginBinding(ctx, principal)
	if !ok {
		return ws.NewResponse(msg.ID, msg.Action, emptyCoordinatorPluginToolSnapshot(principal.CallerTaskID))
	}
	snapshot, err := h.filteredPluginToolSnapshot(func(definition plugintools.Definition) bool {
		return slices.Contains(definition.Surfaces, string(mcpprofile.SurfaceCoordinator)) &&
			definition.IsInstanceRead() &&
			binding.AllowsPlugin(definition.PluginID, definition.LocalName, definition.InstallationID, definition.PolicyVersion)
	})
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("coordinator plugin tool catalog failed", zap.Error(err))
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to load plugin tools", nil)
	}
	snapshot.Generation = "coordinator-" + principal.CallerTaskID
	return ws.NewResponse(msg.ID, msg.Action, snapshot)
}

func (h *Handlers) filteredPluginToolSnapshot(eligible func(plugintools.Definition) bool) (plugintools.Snapshot, error) {
	snapshot, err := h.pluginSvc.AgentToolCatalog()
	if err != nil {
		return plugintools.Snapshot{}, err
	}
	filtered := snapshot
	filtered.Tools = make([]plugintools.Definition, 0, len(snapshot.Tools))
	for _, definition := range snapshot.Tools {
		if !eligible(definition) {
			continue
		}
		filtered.Tools = append(filtered.Tools, definition)
	}
	return filtered, nil
}

// withExternalWorkspaceArgument copies one external catalog entry and adds the
// host-owned workspace argument to workspace reads. The plugin's own catalog
// keeps the unmodified schema.
func withExternalWorkspaceArgument(definition plugintools.Definition) (plugintools.Definition, error) {
	if !definition.IsWorkspaceRead() {
		return definition, nil
	}
	schema, err := addExternalWorkspaceArgument(definition.InputSchema)
	if err != nil {
		return definition, fmt.Errorf("plugin tool %q input schema: %w", definition.ExposedName, err)
	}
	definition.InputSchema = schema
	return definition, nil
}

func addExternalWorkspaceArgument(raw json.RawMessage) (json.RawMessage, error) {
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	if document == nil {
		document = map[string]any{}
	}
	properties, _ := document["properties"].(map[string]any)
	if properties == nil {
		properties = map[string]any{}
		document["properties"] = properties
	}
	properties[manifest.AgentToolWorkspaceArgument] = map[string]any{
		"type": "string", "minLength": 1, "description": externalWorkspaceArgumentDescription,
	}
	document["required"] = appendRequiredArgument(document["required"], manifest.AgentToolWorkspaceArgument)
	adjustRootPropertyCount(document, "maxProperties", 1)
	adjustRootPropertyCount(document, "minProperties", 1)
	return json.Marshal(document)
}

// adjustRootPropertyCount widens an object-wide property bound by delta so the
// injected host argument is accepted alongside the plugin-declared inputs.
func adjustRootPropertyCount(document map[string]any, field string, delta float64) {
	value, ok := document[field]
	if !ok {
		return
	}
	count, ok := value.(float64)
	if !ok || count < 0 {
		return
	}
	document[field] = count + delta
}

func appendRequiredArgument(value any, argument string) []string {
	required := make([]string, 0, 8)
	switch list := value.(type) {
	case []any:
		for _, item := range list {
			if name, ok := item.(string); ok {
				required = append(required, name)
			}
		}
	case []string:
		required = append(required, list...)
	}
	if slices.Contains(required, argument) {
		return required
	}
	return append(required, argument)
}

func emptyCoordinatorPluginToolSnapshot(conversationTaskID string) plugintools.Snapshot {
	return plugintools.Snapshot{Generation: "coordinator-" + conversationTaskID, Revision: 1, Tools: []plugintools.Definition{}}
}

func (h *Handlers) coordinatorPluginBinding(ctx context.Context, principal mcpscope.Principal) (*mcpprofile.CoordinatorToolPolicy, bool) {
	execution, ok := streams.MCPExecutionContextFromContext(ctx)
	if !ok || execution.CoordinatorToolPolicy == nil {
		return nil, false
	}
	binding := execution.CoordinatorToolPolicy
	if binding.Version != mcpprofile.CoordinatorToolPolicyVersionV2 || !coordinatorBindingMatches(binding, principal) {
		return nil, false
	}
	if !h.coordinatorConversationCurrent(ctx, principal) {
		return nil, false
	}
	return binding, true
}

func (h *Handlers) coordinatorConversationCurrent(ctx context.Context, principal mcpscope.Principal) bool {
	if h.coordinatorConversationForTest != nil {
		coordinatorID, ok, err := h.coordinatorConversationForTest(ctx, principal.CallerTaskID)
		return err == nil && ok && coordinatorID == principal.CoordinatorID
	}
	if h.coordinatorSvc == nil {
		return false
	}
	coordinatorID, ok, err := h.coordinatorSvc.CoordinatorForConversationTask(ctx, principal.CallerTaskID)
	return err == nil && ok && coordinatorID == principal.CoordinatorID
}

func (h *Handlers) authorizeExternalPluginToolRequest(ctx context.Context) error {
	if !mcporigin.IsTrustedExternalTransport(ctx) {
		return errors.New("external plugin tools require the trusted external transport")
	}
	if _, ok := streams.MCPExecutionContextFromContext(ctx); ok {
		return errors.New("an execution context cannot claim the external surface")
	}
	if _, ok := mcpscope.PrincipalFromContext(ctx); ok {
		return errors.New("a task principal cannot claim the external surface")
	}
	return nil
}

func (h *Handlers) logPluginToolDenial(ctx context.Context, action, reason string) {
	if h == nil || h.logger == nil {
		return
	}
	execution, _ := streams.MCPExecutionContextFromContext(ctx)
	h.logger.Warn("plugin tool request denied",
		zap.String("action", action),
		zap.String("reason", reason),
		zap.String("task_id", execution.TaskID),
		zap.String("session_id", execution.SessionID))
}

func (h *Handlers) handleInvokePluginTool(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req pluginToolInvocationRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil)
	}
	if req.PluginID == "" || req.LocalName == "" || req.Surface == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "plugin_id, local_name, and surface are required", nil)
	}
	if principal, ok := mcpscope.PrincipalFromContext(ctx); ok && principal.IsAutomation() {
		return h.handleInvokeAutomationPluginTool(ctx, msg, req, principal)
	}
	if principal, ok := mcpscope.PrincipalFromContext(ctx); ok && principal.IsCoordinator() {
		if req.Surface != string(mcpprofile.SurfaceCoordinator) {
			h.logPluginToolDenial(ctx, msg.Action, "coordinator_surface_forged")
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "coordinator plugin tools require the coordinator surface", nil)
		}
		return h.handleInvokeCoordinatorPluginTool(ctx, msg, req)
	}
	switch req.Surface {
	case string(mcpprofile.SurfaceAutomation):
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "automation plugin tools require an automation execution", nil)
	case string(mcpprofile.SurfaceExternal):
		return h.handleInvokeExternalPluginTool(ctx, msg, req)
	case string(mcpprofile.SurfaceCoordinator):
		return h.handleInvokeCoordinatorPluginTool(ctx, msg, req)
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
	if principal, ok := mcpscope.PrincipalFromContext(ctx); ok && string(principal.Surface) != req.Surface {
		h.logPluginToolDenial(ctx, msg.Action, "principal_surface_mismatch")
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "plugin tool surface does not match the caller principal", nil)
	}
	result, err := h.pluginSvc.InvokeAgentTool(ctx, req.PluginID, req.LocalName, req.Arguments, invocation)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, fmt.Sprintf("plugin tool invocation failed: %v", err), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{
		"text": result.Text, "structured_content": result.StructuredContent, "is_error": result.IsError,
	})
}

func (h *Handlers) handleInvokeAutomationPluginTool(ctx context.Context, msg *ws.Message, req pluginToolInvocationRequest, principal mcpscope.Principal) (*ws.Message, error) {
	if req.Surface != string(mcpprofile.SurfaceAutomation) {
		h.logAutomationPluginToolDenial(ctx, msg.Action, "surface_forged", principal)
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "automation execution may only invoke the automation surface", nil)
	}
	if h.automationPluginTools == nil || h.pluginSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "automation plugin tools are unavailable", nil)
	}
	invocation, contextErrorResponse, err := h.resolvePluginToolInvocationContext(ctx, msg, req)
	if err != nil || contextErrorResponse != nil {
		h.logAutomationPluginToolDenial(ctx, msg.Action, "request_context_mismatch", principal)
		return contextErrorResponse, err
	}
	authorization, err := h.automationPluginTools.AutomationToolAuthorization(ctx, principal.AutomationID, principal.WorkspaceID, req.PluginID)
	if err != nil {
		h.logAutomationPluginToolDenial(ctx, msg.Action, "approval_read_failed", principal)
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to load automation tool approval", nil)
	}
	if authorization == nil || !authorization.Allows(req.PluginID, req.LocalName) {
		h.logAutomationPluginToolDenial(ctx, msg.Action, "tool_not_approved", principal)
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "tool is not approved for this automation", nil)
	}
	invocation.Surface = string(mcpprofile.SurfaceAutomation)
	invocation.AutomationToolAuthorization = authorization
	result, err := h.pluginSvc.InvokeAgentTool(ctx, req.PluginID, req.LocalName, req.Arguments, invocation)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("automation plugin tool invocation failed",
				zap.String("plugin_id", req.PluginID),
				zap.String("local_name", req.LocalName),
				zap.String("automation_id", principal.AutomationID),
				zap.Error(err))
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "plugin tool invocation failed", nil)
	}
	return pluginToolSuccessResponse(msg, result)
}

// handleInvokeExternalPluginTool dispatches an external call that entered
// through the trusted external MCP endpoint. Instance reads carry no task,
// session, or workspace identity. Workspace reads carry only the workspace
// that this handler verified for the authenticated caller.
func (h *Handlers) handleInvokeExternalPluginTool(ctx context.Context, msg *ws.Message, req pluginToolInvocationRequest) (*ws.Message, error) {
	if err := h.authorizeExternalPluginToolRequest(ctx); err != nil {
		h.logPluginToolDenial(ctx, msg.Action, "untrusted_or_forged_transport")
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, err.Error(), nil)
	}
	if req.TaskID != "" || req.SessionID != "" || req.WorkspaceID != "" {
		h.logPluginToolDenial(ctx, msg.Action, "forged_identity")
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "external plugin tool calls carry no task identity", nil)
	}
	if h.pluginSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "plugin tools are unavailable", nil)
	}
	definition, found, err := h.lookupPluginToolDefinition(req.PluginID, req.LocalName)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("external plugin tool catalog failed", zap.Error(err))
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "plugin tools are unavailable", nil)
	}
	if !found {
		h.logPluginToolDenial(ctx, msg.Action, "tool_not_declared")
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "plugin tool is not available", nil)
	}
	arguments := req.Arguments
	invocation := plugins.AgentToolInvocationContext{
		InvocationID: req.InvocationID,
		Surface:      string(mcpprofile.SurfaceExternal),
	}
	if definition.InvocationScope == plugintools.InvocationScopeWorkspace {
		workspaceID, stripped, extractErr := extractExternalWorkspaceArgument(arguments)
		if extractErr != nil {
			h.logExternalPluginToolDenial(ctx, msg.Action, "workspace_argument_invalid", req)
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest,
				manifest.AgentToolWorkspaceArgument+" must be a non-empty string", nil)
		}
		if authErr := h.authorizeExternalWorkspaceCall(ctx, workspaceID); authErr != nil {
			h.logExternalPluginToolDenial(ctx, msg.Action, "workspace_unavailable", req)
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, externalWorkspaceUnavailableMessage, nil)
		}
		arguments = stripped
		invocation.WorkspaceID = workspaceID
	}
	result, err := h.pluginSvc.InvokeAgentTool(ctx, req.PluginID, req.LocalName, arguments, invocation)
	if err != nil {
		h.logPluginToolDenial(ctx, msg.Action, "invocation_failed")
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "plugin tool invocation failed", nil)
	}
	return pluginToolSuccessResponse(msg, result)
}

// lookupPluginToolDefinition reads one live descriptor so dispatch follows the
// catalog's current scope. A missing descriptor fails closed before the RPC.
func (h *Handlers) lookupPluginToolDefinition(pluginID, localName string) (plugintools.Definition, bool, error) {
	snapshot, err := h.pluginSvc.AgentToolCatalog()
	if err != nil {
		return plugintools.Definition{}, false, err
	}
	for _, definition := range snapshot.Tools {
		if definition.PluginID == pluginID && definition.LocalName == localName {
			return definition, true, nil
		}
	}
	return plugintools.Definition{}, false, nil
}

// extractExternalWorkspaceArgument removes the host-owned argument from a copy
// of the plugin arguments and returns the raw workspace ID.
func extractExternalWorkspaceArgument(arguments map[string]any) (string, map[string]any, error) {
	value, ok := arguments[manifest.AgentToolWorkspaceArgument]
	if !ok {
		return "", nil, errors.New("workspace argument is absent")
	}
	workspaceID, ok := value.(string)
	if !ok || strings.TrimSpace(workspaceID) == "" {
		return "", nil, errors.New("workspace argument is not a non-empty string")
	}
	stripped := make(map[string]any, len(arguments))
	for key, item := range arguments {
		if key == manifest.AgentToolWorkspaceArgument {
			continue
		}
		stripped[key] = item
	}
	return workspaceID, stripped, nil
}

// authorizeExternalWorkspaceCall resolves the workspace and checks the
// caller's current read permission on every call. With authentication
// disabled the task service accepts any existing workspace.
func (h *Handlers) authorizeExternalWorkspaceCall(ctx context.Context, workspaceID string) error {
	if h.pluginToolAuthEnabled != nil && h.pluginToolAuthEnabled() {
		identity, ok := authn.IdentityFromContext(ctx)
		if !ok || identity.Synthetic {
			return errors.New(externalWorkspaceUnavailableMessage)
		}
	}
	if h.taskSvc == nil {
		return errors.New(externalWorkspaceUnavailableMessage)
	}
	if _, err := h.taskSvc.GetWorkspace(ctx, workspaceID); err != nil {
		return errors.New(externalWorkspaceUnavailableMessage)
	}
	if err := h.taskSvc.AuthorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return errors.New(externalWorkspaceUnavailableMessage)
	}
	return nil
}

func (h *Handlers) logExternalPluginToolDenial(ctx context.Context, action, reason string, req pluginToolInvocationRequest) {
	if h == nil || h.logger == nil {
		return
	}
	h.logger.Warn("plugin tool request denied",
		zap.String("action", action),
		zap.String("reason", reason),
		zap.String("plugin_id", req.PluginID),
		zap.String("local_name", req.LocalName))
}

// handleInvokeCoordinatorPluginTool authorizes one raw coordinator plugin call
// against the exact version-2 binding and the live installation/policy
// version before it reaches the plugin.
func (h *Handlers) handleInvokeCoordinatorPluginTool(ctx context.Context, msg *ws.Message, req pluginToolInvocationRequest) (*ws.Message, error) {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || !principal.IsCoordinator() {
		h.logPluginToolDenial(ctx, msg.Action, "coordinator_principal_required")
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "coordinator plugin tools require a coordinator session", nil)
	}
	if h.pluginSvc == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "plugin tools are unavailable", nil)
	}
	if !h.coordinatorPhase2Enabled() {
		h.logPluginToolDenial(ctx, msg.Action, "phase2_disabled")
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "coordinator plugin tools require phase 2", nil)
	}
	binding, ok := h.coordinatorPluginBinding(ctx, principal)
	if !ok {
		h.logPluginToolDenial(ctx, msg.Action, "binding_missing_or_invalid")
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "coordinator plugin tool is not bound", nil)
	}
	reference, ok := bindingPluginTool(binding, req.PluginID, req.LocalName)
	if !ok {
		h.logPluginToolDenial(ctx, msg.Action, "tool_not_bound")
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "tool is not bound to this coordinator conversation", nil)
	}
	// Fast fail before context reads; the plugin service re-checks the exact
	// installation and policy against the record inside the dispatch lease.
	if err := h.pluginSvc.ValidateCoordinatorPluginTool(req.PluginID, req.LocalName, reference.InstallationID, reference.PolicyVersion); err != nil {
		h.logPluginToolDenial(ctx, msg.Action, "installation_or_policy_stale")
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "coordinator plugin tool is no longer eligible", nil)
	}
	invocation, contextErrorResponse, err := h.resolvePluginToolInvocationContext(ctx, msg, req)
	if err != nil || contextErrorResponse != nil {
		h.logPluginToolDenial(ctx, msg.Action, "request_context_mismatch")
		return contextErrorResponse, err
	}
	invocation.Surface = string(mcpprofile.SurfaceCoordinator)
	invocation.CoordinatorToolAuthorization = &plugintools.CoordinatorToolAuthorization{
		CoordinatorID:      principal.CoordinatorID,
		WorkspaceID:        principal.WorkspaceID,
		ConversationTaskID: principal.CallerTaskID,
		PluginID:           req.PluginID,
		LocalName:          req.LocalName,
		InstallationID:     reference.InstallationID,
		PolicyVersion:      reference.PolicyVersion,
	}
	result, err := h.pluginSvc.InvokeAgentTool(ctx, req.PluginID, req.LocalName, req.Arguments, invocation)
	if err != nil {
		h.logPluginToolDenial(ctx, msg.Action, "invocation_failed")
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "plugin tool invocation failed", nil)
	}
	return pluginToolSuccessResponse(msg, result)
}

func bindingPluginTool(binding *mcpprofile.CoordinatorToolPolicy, pluginID, localName string) (mcpprofile.CoordinatorPluginTool, bool) {
	for _, tool := range binding.PluginTools {
		if tool.PluginID == pluginID && tool.LocalName == localName {
			return tool, true
		}
	}
	return mcpprofile.CoordinatorPluginTool{}, false
}

func pluginToolSuccessResponse(msg *ws.Message, result *pluginsdk.AgentToolResult) (*ws.Message, error) {
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{
		"text": result.Text, "structured_content": result.StructuredContent, "is_error": result.IsError,
	})
}

func (h *Handlers) logAutomationPluginToolDenial(ctx context.Context, action, reason string, principal mcpscope.Principal) {
	if h == nil || h.logger == nil {
		return
	}
	execution, _ := streams.MCPExecutionContextFromContext(ctx)
	h.logger.Warn("automation plugin tool request denied",
		zap.String("action", action),
		zap.String("reason", reason),
		zap.String("automation_id", principal.AutomationID),
		zap.String("workspace_id", principal.WorkspaceID),
		zap.String("task_id", execution.TaskID),
		zap.String("session_id", execution.SessionID))
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
