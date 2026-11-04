package automation

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type fakePluginToolOptions struct {
	groups []plugintools.OptionGroup
	err    error
}

func (f *fakePluginToolOptions) PluginToolOptions(context.Context, string) ([]plugintools.OptionGroup, error) {
	return f.groups, f.err
}

func pluginToolGroups(policyVersion string) []plugintools.OptionGroup {
	return []plugintools.OptionGroup{{
		PluginID:       "kandev-security",
		DisplayName:    "Kandev Security",
		Version:        "1.0.0",
		InstallationID: "installation-1",
		PolicyVersion:  policyVersion,
		Tools: []plugintools.OptionTool{
			{LocalName: "list_findings", Description: "List findings", ReadOnlyHint: true},
			{LocalName: "complete_report", Description: "Complete a report"},
		},
	}}
}

func pluginToolSelection(policyVersion string, names ...string) []plugintools.Selection {
	return []plugintools.Selection{{
		PluginID:       "kandev-security",
		InstallationID: "installation-1",
		PolicyVersion:  policyVersion,
		ToolNames:      names,
	}}
}

func TestAutomationPluginToolsPersistence(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	provider := &fakePluginToolOptions{groups: pluginToolGroups("policy-1")}
	svc.SetPluginToolOptionsProvider(provider)

	created, err := svc.CreateAutomation(WithNativePluginToolIntent(ctx), &CreateAutomationRequest{
		WorkspaceID: "ws-1",
		Name:        "Security digest",
		PluginTools: pluginToolSelection("policy-1", "list_findings"),
	})
	if err != nil {
		t.Fatalf("CreateAutomation: %v", err)
	}
	if len(created.PluginTools) != 1 || created.PluginTools[0].ToolNames[0] != "list_findings" {
		t.Fatalf("stored plugin tools = %#v", created.PluginTools)
	}
	listed, err := svc.ListAutomations(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListAutomations: %v", err)
	}
	if len(listed) != 1 || len(listed[0].PluginTools) != 1 {
		t.Fatalf("listed plugin tools = %#v", listed)
	}

	// An unrelated update omits the field and preserves the stored selection.
	if _, err := svc.UpdateAutomation(ctx, created.ID, &UpdateAutomationRequest{}); err != nil {
		t.Fatalf("unrelated update: %v", err)
	}
	stored, err := svc.GetAutomation(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetAutomation: %v", err)
	}
	if len(stored.PluginTools) != 1 {
		t.Fatalf("unrelated update changed plugin tools: %#v", stored.PluginTools)
	}

	// An explicit empty selection clears access.
	empty := []plugintools.Selection{}
	if _, err := svc.UpdateAutomation(WithNativePluginToolIntent(ctx), created.ID, &UpdateAutomationRequest{
		PluginTools: &empty,
	}); err != nil {
		t.Fatalf("clear plugin tools: %v", err)
	}
	stored, err = svc.GetAutomation(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetAutomation after clear: %v", err)
	}
	if len(stored.PluginTools) != 0 {
		t.Fatalf("cleared plugin tools = %#v", stored.PluginTools)
	}
}

func TestAutomationPluginToolsNativeSaveOnly(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	svc.SetPluginToolOptionsProvider(&fakePluginToolOptions{groups: pluginToolGroups("policy-1")})

	if _, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID: "ws-1",
		Name:        "MCP approved",
		PluginTools: pluginToolSelection("policy-1", "list_findings"),
	}); err == nil {
		t.Fatal("non-native create with plugin tools must fail")
	}

	created, err := svc.CreateAutomation(WithNativePluginToolIntent(ctx), &CreateAutomationRequest{
		WorkspaceID: "ws-1",
		Name:        "Native approved",
		PluginTools: pluginToolSelection("policy-1", "list_findings"),
	})
	if err != nil {
		t.Fatalf("CreateAutomation: %v", err)
	}
	if _, err := svc.UpdateAutomation(ctx, created.ID, &UpdateAutomationRequest{
		PluginTools: &[]plugintools.Selection{},
	}); err == nil {
		t.Fatal("non-native update with plugin tools must fail")
	}
}

func TestAutomationPluginToolsRejectStaleOptions(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	provider := &fakePluginToolOptions{groups: pluginToolGroups("policy-1")}
	svc.SetPluginToolOptionsProvider(provider)

	created, err := svc.CreateAutomation(WithNativePluginToolIntent(ctx), &CreateAutomationRequest{
		WorkspaceID: "ws-1",
		Name:        "Stale",
		PluginTools: pluginToolSelection("policy-1", "list_findings"),
	})
	if err != nil {
		t.Fatalf("CreateAutomation: %v", err)
	}
	provider.groups = pluginToolGroups("policy-2")

	selection := pluginToolSelection("policy-1", "complete_report")
	if _, err := svc.UpdateAutomation(WithNativePluginToolIntent(ctx), created.ID, &UpdateAutomationRequest{
		PluginTools: &selection,
	}); err == nil {
		t.Fatal("stale policy version must be rejected")
	}
	authorization, err := svc.AutomationToolAuthorization(ctx, created.ID, "ws-1", "kandev-security")
	if err != nil {
		t.Fatalf("AutomationToolAuthorization: %v", err)
	}
	if authorization != nil {
		t.Fatalf("stale selection granted authorization: %#v", authorization)
	}
	definitions, err := svc.ApprovedPluginTools(ctx, created.ID, "ws-1")
	if err != nil {
		t.Fatalf("ApprovedPluginTools: %v", err)
	}
	if len(definitions) != 0 {
		t.Fatalf("stale selection approved tools: %#v", definitions)
	}
}

func TestAutomationPluginToolsModeChangeClearsSelection(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	svc.SetPluginToolOptionsProvider(&fakePluginToolOptions{groups: pluginToolGroups("policy-1")})

	created, err := svc.CreateAutomation(WithNativePluginToolIntent(ctx), &CreateAutomationRequest{
		WorkspaceID: "ws-1",
		Name:        "Mode change",
		WorkflowID:  "wf-1",
		PluginTools: pluginToolSelection("policy-1", "list_findings"),
	})
	if err != nil {
		t.Fatalf("CreateAutomation: %v", err)
	}
	mode := TaskModeNormalTask
	if _, err := svc.UpdateAutomation(WithNativePluginToolIntent(ctx), created.ID, &UpdateAutomationRequest{TaskMode: &mode}); err != nil {
		t.Fatalf("mode change: %v", err)
	}
	stored, err := svc.GetAutomation(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetAutomation: %v", err)
	}
	if len(stored.PluginTools) != 0 {
		t.Fatalf("mode change retained plugin tools: %#v", stored.PluginTools)
	}
}

func TestAutomationPluginToolsRejectSharedDispatcherMCPMutation(t *testing.T) {
	svc := newTestService(t)
	dispatcher := ws.NewDispatcher()
	registerWSHandlers(dispatcher, svc, svc.logger)

	ctx := streams.WithMCPExecutionContext(context.Background(), streams.MCPExecutionContext{
		ExecutionID: "execution-1", TaskID: "task-1", SessionID: "session-1",
	})
	ctx = mcpscope.WithPrincipal(ctx, mcpscope.Principal{
		AutomationID: "automation-1", WorkspaceID: "ws-1",
		CallerTaskID: "task-1", CallerSessionID: "session-1", Surface: mcpprofile.SurfaceAutomation,
	})
	selection := pluginToolSelection("policy-1", "list_findings")
	request, err := ws.NewRequest("request-1", ws.ActionAutomationUpdate, map[string]any{
		"id": "automation-1", "plugin_tools": selection,
	})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	response, err := dispatcher.Dispatch(ctx, request)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if response.Type != ws.MessageTypeError {
		t.Fatalf("response = %#v, want an MCP mutation denial", response)
	}
	if !containsMessage(response.Payload, "not available from an MCP session") {
		t.Fatalf("response payload = %s", response.Payload)
	}

	// Service-level backstop: an MCP context can never gain the native marker,
	// even if a future handler marks it unconditionally.
	marked := WithNativePluginToolIntent(ctx)
	if hasNativePluginToolIntent(marked) {
		t.Fatal("MCP context retained native plugin-tool intent")
	}
	if _, err := svc.CreateAutomation(marked, &CreateAutomationRequest{
		WorkspaceID: "ws-1", Name: "MCP approval",
		PluginTools: selection,
	}); err == nil {
		t.Fatal("MCP context approved plugin tools")
	}
}

func TestDecodePluginToolsRejectsMalformedStoredAuthority(t *testing.T) {
	raw := `[{"plugin_id":"p","installation_id":"i","policy_version":"v","tool_names":["a"]},` +
		`{"plugin_id":"p","installation_id":"i","policy_version":"v","tool_names":["b"]}]`
	if got := decodePluginTools(raw); len(got) != 0 {
		t.Fatalf("malformed stored selection = %#v", got)
	}
}

func TestAutomationPluginToolsRejectNullUpdatePayload(t *testing.T) {
	svc := newTestService(t)
	logger := svc.logger
	msg, err := ws.NewRequest("request-null", ws.ActionAutomationUpdate, map[string]any{
		"id": "automation-1", "plugin_tools": nil,
	})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	response, err := wsUpdate(svc, logger)(context.Background(), msg)
	if err != nil {
		t.Fatalf("wsUpdate: %v", err)
	}
	if response.Type != ws.MessageTypeError || !strings.Contains(string(response.Payload), "cannot be null") {
		t.Fatalf("response = %#v", response)
	}
}

func containsMessage(payload []byte, want string) bool {
	return strings.Contains(string(payload), want)
}
