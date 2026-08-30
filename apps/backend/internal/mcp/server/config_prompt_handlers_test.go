package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSharedPromptTools_AreOnlyRegisteredForConfigAndExternalModes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mode      string
		available bool
	}{
		{name: "config", mode: ModeConfig, available: true},
		{name: "external", mode: ModeExternal, available: true},
		{name: "task", mode: ModeTask, available: false},
		{name: "office", mode: ModeOffice, available: false},
		{name: "automation", mode: ModeAutomation, available: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(&testBackend{}, "test-session", "test-task", 10005, newTestLogger(t), "", false, tc.mode)
			tools := s.mcpServer.ListTools()
			if tc.available {
				assert.Contains(t, tools, "list_shared_prompts_kandev")
				assert.Contains(t, tools, "get_shared_prompt_kandev")
				return
			}
			assert.NotContains(t, tools, "list_shared_prompts_kandev")
			assert.NotContains(t, tools, "get_shared_prompt_kandev")
		})
	}
}

func TestSharedPromptTools_ExposeReadOnlySchemas(t *testing.T) {
	s := newTestServer(t, &testBackend{})

	listTool, ok := s.mcpServer.ListTools()["list_shared_prompts_kandev"]
	require.True(t, ok)
	getTool, ok := s.mcpServer.ListTools()["get_shared_prompt_kandev"]
	require.True(t, ok)

	assert.Empty(t, rawToolInputProperties(t, s, "list_shared_prompts_kandev"))
	assert.Contains(t, toolInputProperties(t, s, "get_shared_prompt_kandev"), "name")
	assert.Contains(t, listTool.Tool.Description, "saved prompts")
	assert.Contains(t, getTool.Tool.Description, "saved prompt")
	assert.True(t, *listTool.Tool.Annotations.ReadOnlyHint)
	assert.False(t, *listTool.Tool.Annotations.DestructiveHint)
	assert.True(t, *listTool.Tool.Annotations.IdempotentHint)
	assert.False(t, *listTool.Tool.Annotations.OpenWorldHint)
	assert.True(t, *getTool.Tool.Annotations.ReadOnlyHint)
	assert.False(t, *getTool.Tool.Annotations.DestructiveHint)
	assert.True(t, *getTool.Tool.Annotations.IdempotentHint)
	assert.False(t, *getTool.Tool.Annotations.OpenWorldHint)
}

func rawToolInputProperties(t *testing.T, s *Server, toolName string) map[string]interface{} {
	t.Helper()
	toolsMap := s.mcpServer.ListTools()
	st, ok := toolsMap[toolName]
	require.True(t, ok, "tool %q not registered", toolName)

	data, err := json.Marshal(st.Tool)
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &parsed))
	schema, ok := parsed["inputSchema"].(map[string]interface{})
	require.True(t, ok, "tool schema should have inputSchema")
	props, ok := schema["properties"].(map[string]interface{})
	require.True(t, ok, "schema should have properties")
	return props
}

func TestSharedPromptTools_ForwardRequestsAndRejectBlankName(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"shared_prompts": []interface{}{},
		"total":          float64(0),
	}}
	s := newTestServer(t, backend)

	listResult := callTool(t, s, "list_shared_prompts_kandev", map[string]interface{}{})
	require.False(t, listResult.IsError)
	require.Equal(t, "mcp.list_shared_prompts", backend.lastAction)

	getResult := callTool(t, s, "get_shared_prompt_kandev", map[string]interface{}{"name": "  code-review  "})
	require.False(t, getResult.IsError)
	require.Equal(t, "mcp.get_shared_prompt", backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "code-review", payload["name"])

	blankResult := callTool(t, s, "get_shared_prompt_kandev", map[string]interface{}{"name": " \t"})
	assert.True(t, blankResult.IsError)
}
