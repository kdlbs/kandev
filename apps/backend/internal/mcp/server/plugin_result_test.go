package mcp

import (
	"testing"

	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestPluginToolPreservesStructuredResultAndErrorStatus(t *testing.T) {
	for _, isError := range []bool{false, true} {
		name := "success"
		if isError {
			name = "error"
		}
		t.Run(name, func(t *testing.T) {
			structured := map[string]any{"status": "uncertain", "recipient": "U01234567", "duplicate": true}
			backend := &testBackend{response: map[string]any{
				"text": "Check delivery before retrying.", "structured_content": structured, "is_error": isError,
			}}
			server := New(backend, "session-1", "task-1", 10005, newTestLogger(t), "", false, ModeTask)
			definition := plugintools.Definition{
				PluginID: "notifications", LocalName: "notify", ExposedName: plugintools.ExposedName("notifications", "notify"),
				Description: "Notify a user", InputSchema: []byte(`{"type":"object"}`),
				Surfaces: []string{plugintools.SurfaceKanban},
			}
			require.NoError(t, server.SetPluginTools(plugintools.Snapshot{
				Generation: "g", Revision: 1, Tools: []plugintools.Definition{definition},
			}))

			result := callTool(t, server, definition.ExposedName, map[string]any{})
			require.Equal(t, isError, result.IsError)
			require.Equal(t, structured, result.StructuredContent)
			require.Len(t, result.Content, 1)
			require.Equal(t, "Check delivery before retrying.", result.Content[0].(mcplib.TextContent).Text)
		})
	}
}
