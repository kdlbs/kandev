package mcp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.1
func TestSharedPromptWritesCatalog(t *testing.T) {
	for _, mode := range []string{ModeConfig, ModeExternal, ModeTask, ModeOffice, ModeAutomation} {
		t.Run(mode, func(t *testing.T) {
			s := New(&testBackend{}, "session", "task", 10005, newTestLogger(t), "", false, mode)
			for _, name := range []string{"create_shared_prompt_kandev", "update_shared_prompt_kandev"} {
				tool, ok := s.mcpServer.ListTools()[name]
				require.Equal(t, mode == ModeConfig || mode == ModeExternal, ok, name)
				if !ok {
					continue
				}
				require.ElementsMatch(t, []string{"name", "content"}, tool.Tool.InputSchema.Required)
				require.False(t, *tool.Tool.Annotations.ReadOnlyHint)
				require.False(t, *tool.Tool.Annotations.OpenWorldHint)
				require.Equal(t, name == "update_shared_prompt_kandev", *tool.Tool.Annotations.DestructiveHint)
				require.NotContains(t, toolInputProperties(t, s, name), "allow_agent_edits")
			}
		})
	}
}

func TestSharedPromptWritesForwardAndValidate(t *testing.T) {
	for _, verb := range []string{"create", "update"} {
		t.Run(verb, func(t *testing.T) {
			backend := &testBackend{response: map[string]interface{}{"name": "review", "content": "Review."}}
			s := newTestServer(t, backend)
			name := verb + "_shared_prompt_kandev"
			require.Contains(t, s.mcpServer.ListTools(), name)
			result := callTool(t, s, name, map[string]interface{}{"name": " review ", "content": " Review. "})
			require.False(t, result.IsError)
			require.Equal(t, "mcp."+verb+"_shared_prompt", backend.lastAction)
			require.Equal(t, map[string]interface{}{"name": "review", "content": " Review. "}, backend.lastPayload)
			for _, input := range []map[string]interface{}{
				{"name": " ", "content": "content"}, {"name": "review", "content": " "},
				{"name": "review"}, {"name": "review", "content": 7},
			} {
				backend.lastAction = ""
				require.True(t, callTool(t, s, name, input).IsError)
				require.Empty(t, backend.lastAction)
			}
		})
	}
}
