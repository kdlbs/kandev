package mcp

import (
	"context"
	"encoding/json"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompactSettingsToolsExposeOnlyStableEnvelopes(t *testing.T) {
	server := newTestServer(t, &testBackend{})
	for _, name := range []string{
		"search_settings_kandev", "describe_setting_kandev", "get_settings_kandev",
		"update_settings_kandev", "list_settings_resources_kandev",
	} {
		tool, ok := server.mcpServer.ListTools()[name]
		require.True(t, ok, "missing compact settings tool %q", name)
		data := tool.Tool.RawInputSchema
		if len(data) == 0 {
			var err error
			data, err = json.Marshal(tool.Tool.InputSchema)
			require.NoError(t, err)
		}
		var schema map[string]any
		require.NoError(t, json.Unmarshal(data, &schema))
		assert.Equal(t, "object", schema["type"])
		assert.Equal(t, false, schema["additionalProperties"], "tool %q must keep its outer envelope closed", name)
	}
	var updateSchema map[string]any
	require.NoError(t, json.Unmarshal(server.mcpServer.ListTools()["update_settings_kandev"].Tool.RawInputSchema, &updateSchema))
	properties, ok := updateSchema["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, properties, "target")
	assert.Contains(t, properties, "changes")
	assert.Contains(t, properties, "options")
	assert.NotContains(t, properties, "agent_profile")
}

func TestCompactSettingsToolForwardsStableAction(t *testing.T) {
	backend := &testBackend{}
	server := newTestServer(t, backend)
	tool := server.mcpServer.ListTools()["update_settings_kandev"]
	req := mcplib.CallToolRequest{}
	req.Params.Name = "update_settings_kandev"
	req.Params.Arguments = map[string]any{
		"target":  map[string]any{"resource_type": "agent_profile", "resource_id": "profile-1"},
		"changes": map[string]any{"auto_approve": false},
	}
	_, err := tool.Handler(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, ws.ActionMCPUpdateSettings, backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, payload["changes"].(map[string]any)["auto_approve"])
}
