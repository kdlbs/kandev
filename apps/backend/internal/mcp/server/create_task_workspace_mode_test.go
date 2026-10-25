package mcp

import (
	"encoding/json"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-MCP-WORKSPACE-MODE-004.1, AC-TASKS-MCP-WORKSPACE-MODE-004.2
func TestCreateTaskWorkspaceModeSchema(t *testing.T) {
	for _, mode := range []string{ModeTask, ModeExternal} {
		t.Run(mode, func(t *testing.T) {
			s := New(&testBackend{}, "", "", 10005, newTestLogger(t), "", false, mode)
			tool, ok := s.mcpServer.ListTools()["create_task_kandev"]
			require.True(t, ok)
			catalog, err := json.Marshal(tool.Tool.InputSchema)
			require.NoError(t, err)
			for name, raw := range map[string][]byte{
				"catalog": catalog, "attachment": mcpToolInputSchema(tool.Tool),
			} {
				t.Run(name, func(t *testing.T) {
					var schema struct {
						Properties map[string]map[string]any
						Required   []string
					}
					require.NoError(t, json.Unmarshal(raw, &schema))
					property := schema.Properties["workspace_mode"]
					require.NotNil(t, property)
					assert.Equal(t, "string", property["type"])
					assert.Equal(t, []any{"inherit_parent", "new_workspace"}, property["enum"])
					assert.NotContains(t, schema.Required, "workspace_mode")
					assert.NotContains(t, property, "default")
					description, ok := property["description"].(string)
					require.True(t, ok)
					for _, phrase := range []string{
						"Omit for subtasks", "inherit_parent requires parent_id",
						"reuses the parent's materialized workspace/worktree",
						"new_workspace requests a separate workspace/worktree",
					} {
						assert.Contains(t, description, phrase)
					}
				})
			}
		})
	}
}

// @covers AC-TASKS-MCP-WORKSPACE-MODE-004.3, AC-TASKS-MCP-WORKSPACE-MODE-004.4
func TestCreateTaskWorkspaceModeArguments(t *testing.T) {
	for _, mode := range []string{ModeTask, ModeExternal} {
		t.Run(mode, func(t *testing.T) {
			for _, tc := range []struct {
				name  string
				value string
				omit  bool
				valid bool
			}{
				{name: "omitted", omit: true, valid: true},
				{name: "inherit", value: "inherit_parent", valid: true},
				{name: "new", value: "new_workspace", valid: true},
				{name: "empty"},
				{name: "whitespace", value: " \t"},
				{name: "padded", value: " new_workspace "},
				{name: "shared", value: "shared"},
				{name: "shared group", value: "shared_group"},
				{name: "unknown", value: "unknown"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					backend := &testBackend{response: map[string]any{"id": "child"}}
					s := New(backend, "", "", 10005, newTestLogger(t), "", false, mode)
					args := map[string]any{"title": "Child task", "parent_id": "parent", "start_agent": false}
					if !tc.omit {
						args["workspace_mode"] = tc.value
					}
					result := callTool(t, s, "create_task_kandev", args)
					if !tc.valid {
						require.True(t, result.IsError)
						assert.Empty(t, backend.lastAction)
						require.NotEmpty(t, result.Content)
						content, ok := result.Content[0].(mcp.TextContent)
						require.True(t, ok)
						assert.Contains(t, content.Text, "/workspace_mode")
						assert.Contains(t, content.Text, "keyword: enum")
						return
					}
					require.False(t, result.IsError)
					assert.Equal(t, ws.ActionMCPCreateTask, backend.lastAction)
					payload, ok := backend.lastPayload.(map[string]any)
					require.True(t, ok)
					assert.Equal(t, tc.value, payload["workspace_mode"])
				})
			}
		})
	}
}
