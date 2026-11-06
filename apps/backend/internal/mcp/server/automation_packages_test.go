package mcp

import (
	"context"
	"encoding/json"
	p "github.com/kandev/kandev/internal/mcp/profile"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
	"testing"
)

// @covers AC-OFFICE-AUTOMATION-MCP-002.1, AC-OFFICE-AUTOMATION-MCP-002.5
func TestAutomationMCPPackageInvocation(t *testing.T) {
	profile := p.NewAutomation()
	// Unmarshal the wire contract so the regression fails on runtime behavior before model changes.
	profileJSON := `{"version":2,"catalog_version":1,"workspace_id":"ws","task_id":"task","surface":"automation","automation_id":"automation","automation_revision":1,"workspace_revision":0,"task_revision":0,"packages":["core","configuration","rich-output","walkthroughs","diagnostics"],"origins":{"core":"required","configuration":"automation","rich-output":"automation","walkthroughs":"automation","diagnostics":"automation"}}`
	profile.BuiltinPackages = &p.BuiltinManifest{}
	require.NoError(t, json.Unmarshal([]byte(profileJSON), profile.BuiltinPackages))
	backend := &testBackend{}
	s := NewWithProfile(backend, "session", "task", 10005, newTestLogger(t), "", false, profile)
	t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
	catalog := s.BuiltinCatalog()
	require.Contains(t, catalog[p.PackageConfiguration], "create_automation_kandev")
	require.NotContains(t, catalog[p.PackageConfiguration], "update_mcp_config_kandev")
	require.Contains(t, catalog[p.PackageRichOutput], "show_rich_output_kandev")
	require.Contains(t, catalog[p.PackageWalkthroughs], "show_walkthrough_kandev")
	require.Contains(t, catalog[p.PackageDiagnostics], "get_diagnostic_bundle_kandev")
	require.NotContains(t, catalog[p.PackageCore], "ask_user_question_kandev")
	for _, name := range []string{"answer_question_kandev", "list_pending_questions_kandev", "list_pending_agent_permissions_kandev", "resolve_agent_permission_kandev"} {
		require.Contains(t, catalog[p.PackageCore], name)
	}
	actions, err := ResolveBuiltinActions(profile, newTestLogger(t))
	require.NoError(t, err)
	require.Contains(t, actions, "mcp.create_automation")
	assertAutomationPackageCalls(t, s, backend)
}

func assertAutomationPackageCalls(t *testing.T, s *Server, backend *testBackend) {
	t.Helper()
	cases := []struct {
		id           p.PackageID
		name, action string
		args         map[string]any
	}{
		{p.PackageConfiguration, "list_shared_prompts_kandev", "mcp.list_shared_prompts", map[string]any{}},
		{p.PackageRichOutput, "show_rich_output_kandev", "", map[string]any{"version": 2, "catalog_version": 1, "title": "Report", "spec": map[string]any{"root": "note", "elements": map[string]any{"note": map[string]any{"type": "Callout", "props": map[string]any{"kind": "info", "text": "Report"}, "children": []any{}}}}}},
		{p.PackageWalkthroughs, "show_walkthrough_kandev", "mcp.show_walkthrough", map[string]any{"title": "Report", "steps": []any{map[string]any{"file": "notes.md", "line": 1, "text": "Report"}}}},
		{p.PackageDiagnostics, "get_diagnostic_bundle_kandev", "mcp.get_diagnostic_bundle", map[string]any{"source": "backend"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			backend.lastAction = ""
			tool := s.builtinPackages[tc.id].registry.ListTools()[tc.name]
			require.NotNil(t, tool)
			result, err := tool.Handler(t.Context(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Name: tc.name, Arguments: tc.args}})
			require.NoError(t, err)
			require.False(t, result.IsError, "%+v", result.Content)
			require.Equal(t, tc.action, backend.lastAction)
		})
	}
}

func TestAutomationMCPBindingRejectsTaskSurface(t *testing.T) {
	s := NewWithProfile(&testBackend{}, "session", "task", 10005, newTestLogger(t), "", false, p.NewAutomation())
	t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
	profile := p.New(p.SurfaceKanbanTask, nil, nil)
	profile.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "ws", TaskID: "task", Surface: p.SurfaceKanbanTask, Packages: []p.PackageID{p.PackageCore}, Origins: map[p.PackageID]string{p.PackageCore: "required"}}
	require.EqualError(t, s.BindInitialProfile(profile, 10005), "built-in binding surface mismatch")
}
