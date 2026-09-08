package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMCPToolNamePresentation_AllProfiles covers
// AC-TASKS-MCP-TOOL-NAMES-001.1 and .3. It models Auggie's client behavior by
// appending the injected server name after the per-instance transport returns
// its tool list.
func TestMCPToolNamePresentation_AllProfiles(t *testing.T) {
	profiles := []struct {
		name    string
		profile mcpprofile.Context
	}{
		{name: ModeTask, profile: mcpprofile.Legacy(ModeTask, false, nil)},
		{name: ModeConfig, profile: mcpprofile.Legacy(ModeConfig, false, nil)},
		{name: ModeExternal, profile: mcpprofile.Legacy(ModeExternal, true, nil)},
		{name: ModeOffice, profile: mcpprofile.Legacy(ModeOffice, false, nil)},
		{name: ModeAutomation, profile: mcpprofile.NewAutomation()},
	}

	for _, tc := range profiles {
		t.Run(tc.name, func(t *testing.T) {
			s := namespacedTestServer(t, &testBackend{}, tc.profile)
			canonical := s.mcpServer.ListTools()
			toolNames := transportToolNames(t, s)

			for canonicalName := range canonical {
				if !strings.HasSuffix(canonicalName, "_kandev") {
					continue
				}
				transportName := strings.TrimSuffix(canonicalName, "_kandev")
				assert.Contains(t, toolNames, transportName, "missing transport alias for %q", canonicalName)
				assert.NotContains(t, toolNames, canonicalName+"_kandev")
				assert.Equal(t, canonicalName, transportName+"_kandev",
					"Auggie model name for %q must be canonical", canonicalName)
			}

			if tc.name == ModeTask {
				assert.Contains(t, toolNames, "ask_user_question")
				assert.Contains(t, toolNames, "get_task_plan")
			}
		})
	}
}

// TestMCPToolNamePresentation_DefaultFalsePreservesCanonicalNames covers
// AC-TASKS-MCP-TOOL-NAMES-001.2 for a representative non-namespacing client.
func TestMCPToolNamePresentation_DefaultFalsePreservesCanonicalNames(t *testing.T) {
	s := NewWithProfile(
		&testBackend{}, "test-session", "test-task", 10005, newTestLogger(t), "", false,
		mcpprofile.Legacy(ModeTask, false, nil),
	)
	names := transportToolNames(t, s)

	assert.Contains(t, names, "ask_user_question_kandev")
	assert.Contains(t, names, "get_task_plan_kandev")
	assert.NotContains(t, names, "ask_user_question")
	assert.NotContains(t, names, "get_task_plan")
}

// TestMCPToolNamePresentation_RestoresCanonicalCallName covers
// AC-TASKS-MCP-TOOL-NAMES-001.4. The call is deliberately sent with the bare
// transport name that Auggie sends after removing its server namespace.
func TestMCPToolNamePresentation_RestoresCanonicalCallName(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"workspaces": []interface{}{}}}
	s := namespacedTestServer(t, backend, mcpprofile.Legacy(ModeTask, false, nil))

	response := s.mcpServer.HandleMessage(context.Background(), []byte(`{
		"jsonrpc":"2.0","id":1,"method":"tools/call",
		"params":{"name":"list_workspaces","arguments":{}}
	}`))
	raw, err := json.Marshal(response)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "tool 'list_workspaces' not found")
	assert.Equal(t, ws.ActionMCPListWorkspaces, backend.lastAction)
}

// TestMCPToolNamePresentation_UsesLiveRegistry covers the dynamic profile
// replacement and plugin-name boundaries described by the system design.
func TestMCPToolNamePresentation_UsesLiveRegistry(t *testing.T) {
	s := namespacedTestServer(t, &testBackend{}, mcpprofile.Legacy(ModeTask, false, nil))
	s.SetMode(ModeConfig)
	s.mcpServer.AddTool(mcplib.NewTool("kandev_plugin_echo"), func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return mcplib.NewToolResultText("ok"), nil
	})

	names := transportToolNames(t, s)
	assert.Contains(t, names, "kandev_plugin_echo")
	assert.NotContains(t, names, "kandev_plugin_echo_kandev")
	assert.Contains(t, names, "list_workflows")
	assert.NotContains(t, names, "list_workflows_kandev_kandev")
}

func TestMCPToolNamePresentation_PreservesPluginAliasCollision(t *testing.T) {
	s := namespacedTestServer(t, &testBackend{}, mcpprofile.Legacy(ModeTask, false, nil))
	for _, name := range []string{"plugin_echo", "plugin_echo_kandev"} {
		s.mcpServer.AddTool(mcplib.NewTool(name), func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			return mcplib.NewToolResultText("ok"), nil
		})
	}

	names := transportToolNames(t, s)
	counts := make(map[string]int)
	for _, name := range names {
		counts[name]++
	}

	assert.Equal(t, 1, counts["plugin_echo"])
	assert.Equal(t, 1, counts["plugin_echo_kandev"])
}

func namespacedTestServer(t *testing.T, backend BackendClient, profile mcpprofile.Context) *Server {
	t.Helper()
	return NewWithProfile(
		backend, "test-session", "test-task", 10005, newTestLogger(t), "", false, profile,
		WithMCPToolNamespacingByServer(true),
	)
}

func transportToolNames(t *testing.T, s *Server) []string {
	t.Helper()
	response := s.mcpServer.HandleMessage(context.Background(), []byte(`{
		"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}
	}`))
	raw, err := json.Marshal(response)
	require.NoError(t, err)
	var decoded struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))

	names := make([]string, 0, len(decoded.Result.Tools))
	for _, tool := range decoded.Result.Tools {
		names = append(names, tool.Name)
	}
	return names
}
