package handlers

import (
	"context"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
	scope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
	"testing"
)

// @covers AC-OFFICE-AUTOMATION-MCP-002.2, AC-OFFICE-AUTOMATION-MCP-002.5
func TestAutomationMCPRawDispatch(t *testing.T) {
	svc, _ := newTestTaskService(t)
	workspaces, err := svc.ListWorkspaces(t.Context())
	require.NoError(t, err)
	workspace := workspaces[0].ID
	manifest := &p.BuiltinManifest{Version: 2, CatalogVersion: 1, WorkspaceID: workspace, TaskID: "task", Surface: p.SurfaceAutomation, AutomationID: "automation", AutomationRevision: 1, Packages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageConfiguration: "automation"}}
	require.NoError(t, manifest.Validate())
	ctx := scope.WithPrincipal(t.Context(), scope.Principal{AutomationID: "automation", WorkspaceID: workspace, CallerTaskID: "task", CallerSessionID: "session", Surface: p.SurfaceAutomation})
	ctx = streams.WithMCPExecutionContext(ctx, streams.MCPExecutionContext{TaskID: "task", SessionID: "session", BuiltinAuthority: streams.BuiltinMCPAuthority{Required: true, Manifest: manifest, Actions: []string{ws.ActionMCPListSharedPrompts}}})
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	d := &guardedMCPDispatcher{Dispatcher: ws.NewDispatcher(), handlers: h}
	for _, action := range []string{ws.ActionMCPListSharedPrompts, ws.ActionMCPGetDiagnosticBundle} {
		d.RegisterFunc(action, func(_ context.Context, msg *ws.Message) (*ws.Message, error) {
			return ws.NewResponse(msg.ID, msg.Action, nil)
		})
	}
	denied, err := d.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPListSharedPrompts, nil))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, denied.Type)
	denied, err = d.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPGetDiagnosticBundle, nil))
	require.NoError(t, err)
	assertWSError(t, denied, ws.ErrorCodeUnknownAction)
	denied, err = d.Dispatch(ctx, makeWSMessage(t, ws.ActionMCPListSharedPrompts, map[string]any{"workspace_id": "foreign"}))
	require.NoError(t, err)
	assertWSError(t, denied, ws.ErrorCodeNotFound)
}

func TestAutomationMCPRejectsNoncanonicalResourceKeys(t *testing.T) {
	svc, _ := newTestTaskService(t)
	workspaces, err := svc.ListWorkspaces(t.Context())
	require.NoError(t, err)
	workspace := workspaces[0].ID
	manifest := &p.BuiltinManifest{Version: 2, CatalogVersion: 1, WorkspaceID: workspace, TaskID: "task", Surface: p.SurfaceAutomation, AutomationID: "automation", AutomationRevision: 1, Packages: []p.PackageID{p.PackageCore, p.PackageDiagnostics}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageDiagnostics: "automation"}}
	ctx := scope.WithPrincipal(t.Context(), scope.Principal{AutomationID: "automation", WorkspaceID: workspace, CallerTaskID: "task", CallerSessionID: "session", Surface: p.SurfaceAutomation})
	ctx = streams.WithMCPExecutionContext(ctx, streams.MCPExecutionContext{TaskID: "task", SessionID: "session", BuiltinAuthority: streams.BuiltinMCPAuthority{Required: true, Manifest: manifest, Actions: []string{ws.ActionMCPGetDiagnosticBundle}}})
	h := &Handlers{taskSvc: svc, logger: testLogger(t)}
	for _, fields := range []map[string]any{{}, {"TASK_ID": "foreign"}, {"SESSION_ID": "foreign"}, {"task_id": "task", "TASK_ID": "foreign"}, {"WORKSPACE_ID": "foreign"}} {
		denied, _, err := h.authorizeAutomationRequest(ctx, makeWSMessage(t, ws.ActionMCPGetDiagnosticBundle, fields))
		require.NoError(t, err)
		if len(fields) == 0 {
			require.Nil(t, denied)
		} else {
			require.NotNil(t, denied, "%v", fields)
		}
	}
}

func TestAutomationMCPConfigurationProfileOwnership(t *testing.T) {
	h, settings, _ := newAgentSettingsHandlers(t)
	svc, _ := newTestTaskService(t)
	h.taskSvc = svc
	workspaces, err := svc.ListWorkspaces(t.Context())
	require.NoError(t, err)
	workspace := workspaces[0].ID
	agent := &settingsmodels.Agent{Name: "codex-acp"}
	require.NoError(t, settings.CreateAgent(t.Context(), agent))
	for _, owner := range []string{workspace, "foreign", ""} {
		profile := &settingsmodels.AgentProfile{AgentID: agent.ID, Name: "Profile", WorkspaceID: owner}
		require.NoError(t, settings.CreateAgentProfile(t.Context(), profile))
		manifest := &p.BuiltinManifest{Version: 2, CatalogVersion: 1, WorkspaceID: workspace, TaskID: "task", Surface: p.SurfaceAutomation, AutomationID: "automation", AutomationRevision: 1, Packages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageConfiguration: "automation"}}
		ctx := scope.WithPrincipal(t.Context(), scope.Principal{AutomationID: "automation", WorkspaceID: workspace, CallerTaskID: "task", CallerSessionID: "session", Surface: p.SurfaceAutomation})
		ctx = streams.WithMCPExecutionContext(ctx, streams.MCPExecutionContext{TaskID: "task", SessionID: "session", BuiltinAuthority: streams.BuiltinMCPAuthority{Required: true, Manifest: manifest, Actions: []string{ws.ActionMCPGetMcpConfig, ws.ActionMCPUpdateMcpConfig}}})
		for _, action := range []string{ws.ActionMCPGetMcpConfig, ws.ActionMCPUpdateMcpConfig} {
			denied, _, err := h.authorizeAutomationRequest(ctx, makeWSMessage(t, action, map[string]any{"profile_id": profile.ID}))
			require.NoError(t, err)
			if owner == "foreign" {
				require.NotNil(t, denied)
			} else {
				require.Nil(t, denied)
			}
		}
	}
}
