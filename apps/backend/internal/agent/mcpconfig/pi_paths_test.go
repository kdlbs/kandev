package mcpconfig

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/stretchr/testify/require"
)

func TestPiStrategy_IsolatedConfiguration(t *testing.T) {
	home := t.TempDir()
	for _, tc := range []struct {
		name, override, relative string
		outside, missing, sse    bool
	}{
		{name: "default", relative: ".pi/agent"},
		{name: "tilde", override: "~", relative: "."},
		{name: "tilde subdirectory", override: "~/custom", relative: "custom"},
		{name: "absolute private", override: filepath.Join(home, "private"), relative: "private"},
		{name: "relative private", override: "config", relative: "work/config"},
		{name: "escape", override: "../../shared", outside: true},
		{name: "shared", override: t.TempDir(), outside: true},
		{name: "missing state", missing: true},
		{name: "SSE unsupported", sse: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paths := PassthroughPaths{Isolated: true, IsolatedHomeDir: home, WorkspaceDir: filepath.Join(home, "work"), Environment: map[string]string{"PI_CODING_AGENT_DIR": tc.override}}
			if tc.missing {
				paths.IsolatedHomeDir = ""
			}
			servers := sampleServers
			if tc.sse {
				servers = []types.McpServer{{Name: "unsupported", Type: "sse", URL: "https://example.test/sse"}}
			}
			artifacts, err := (PiStrategy{}).BuildPassthroughMCP(servers, paths)
			if tc.outside || tc.missing || tc.sse {
				require.Error(t, err)
				require.Empty(t, artifacts.Files)
				return
			}
			require.NoError(t, err)
			require.Len(t, artifacts.Files, 1)
			require.Equal(t, filepath.Join(home, tc.relative, "mcp.json"), artifacts.Files[0].Path)
			var payload map[string]map[string]map[string]any
			require.NoError(t, json.Unmarshal(artifacts.Files[0].Content, &payload))
			require.Equal(t, "streamable-http", payload["mcpServers"]["kandev"]["type"])
			require.Equal(t, "stdio", payload["mcpServers"]["github"]["type"])
			require.Empty(t, artifacts.Args)
			require.Empty(t, artifacts.Env)
		})
	}
}
