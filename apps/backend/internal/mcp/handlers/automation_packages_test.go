package handlers

import (
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
	denied, _, err := h.authorizeAutomationRequest(ctx, makeWSMessage(t, ws.ActionMCPListSharedPrompts, nil))
	require.NoError(t, err)
	require.Nil(t, denied)
	denied, _, err = h.authorizeAutomationRequest(ctx, makeWSMessage(t, ws.ActionMCPGetDiagnosticBundle, nil))
	require.NoError(t, err)
	require.NotNil(t, denied)
	denied, _, err = h.authorizeAutomationRequest(ctx, makeWSMessage(t, ws.ActionMCPListSharedPrompts, map[string]any{"workspace_id": "foreign"}))
	require.NoError(t, err)
	require.NotNil(t, denied)
}
