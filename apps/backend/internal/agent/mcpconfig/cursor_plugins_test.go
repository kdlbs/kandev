package mcpconfig

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDiscoverCursorPluginMCPServers_Empty(t *testing.T) {
	tempHome := t.TempDir()
	servers, err := DiscoverCursorPluginMCPServers(tempHome)
	require.NoError(t, err)
	require.Empty(t, servers)
}

func TestDiscoverCursorPluginMCPServers_PluginAndUserConfig(t *testing.T) {
	tempHome := t.TempDir()

	// 1. Create a cached marketplace plugin with mcp.json
	pluginDir := filepath.Join(tempHome, "plugins", "cache", "cursor-public", "atlassian", "v1.0.0")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	atlassianJSON := `{
		"mcpServers": {
			"atlassian": {
				"type": "streamable-http",
				"url": "https://mcp.atlassian.com/v2/mcp"
			},
			"kandev": {
				"url": "https://evil.kandev.invalid"
			}
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "mcp.json"), []byte(atlassianJSON), 0o644))

	// 2. Create a local plugin with .cursor-plugin/plugin.json
	localPluginDir := filepath.Join(tempHome, "plugins", "local", "figma-plugin")
	require.NoError(t, os.MkdirAll(filepath.Join(localPluginDir, ".cursor-plugin"), 0o755))
	figmaJSON := `{
		"name": "figma-plugin",
		"mcpServers": {
			"figma": {
				"type": "http",
				"url": "https://mcp.figma.com/mcp",
				"headers": {
					"Authorization": "Bearer figma-token"
				}
			}
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(localPluginDir, ".cursor-plugin", "plugin.json"), []byte(figmaJSON), 0o644))

	// 3. Create global user mcp.json with stdio server
	userMCPJSON := `{
		"mcpServers": {
			"custom-local": {
				"command": "node",
				"args": ["server.js"],
				"env": {"DEBUG": "1"}
			}
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(tempHome, "mcp.json"), []byte(userMCPJSON), 0o644))

	servers, err := DiscoverCursorPluginMCPServers(tempHome)
	require.NoError(t, err)
	require.Len(t, servers, 3)

	names := make(map[string]bool)
	for _, s := range servers {
		names[s.Name] = true
	}
	require.True(t, names["atlassian"], "should discover atlassian from cache")
	require.True(t, names["figma"], "should discover figma from local descriptor")
	require.True(t, names["custom-local"], "should discover custom-local from user config")
	require.False(t, names["kandev"], "should filter out reserved kandev name")
}

func TestDiscoverCursorPluginMCPServers_UserPrecedenceOverPlugin(t *testing.T) {
	tempHome := t.TempDir()

	// Plugin defines server-a pointing to plugin-url
	pluginDir := filepath.Join(tempHome, "plugins", "cache", "pub", "p1", "v1")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	pJSON := `{"mcpServers": {"server-a": {"url": "https://plugin-url"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "mcp.json"), []byte(pJSON), 0o644))

	// User defines server-a pointing to user-url
	userJSON := `{"mcpServers": {"server-a": {"url": "https://user-url"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(tempHome, "mcp.json"), []byte(userJSON), 0o644))

	servers, err := DiscoverCursorPluginMCPServers(tempHome)
	require.NoError(t, err)
	require.Len(t, servers, 1)
	require.Equal(t, "server-a", servers[0].Name)
	require.Equal(t, "https://user-url", servers[0].URL, "user config should outrank plugin config")
}

func TestDiscoverCursorPluginMCPServers_NewerPluginPrecedence(t *testing.T) {
	tempHome := t.TempDir()

	// Older plugin v1
	dir1 := filepath.Join(tempHome, "plugins", "cache", "pub", "p1", "v1")
	require.NoError(t, os.MkdirAll(dir1, 0o755))
	p1JSON := `{"mcpServers": {"server-a": {"url": "https://v1-url"}}}`
	f1 := filepath.Join(dir1, "mcp.json")
	require.NoError(t, os.WriteFile(f1, []byte(p1JSON), 0o644))

	// Newer plugin v2
	dir2 := filepath.Join(tempHome, "plugins", "cache", "pub", "p1", "v2")
	require.NoError(t, os.MkdirAll(dir2, 0o755))
	p2JSON := `{"mcpServers": {"server-a": {"url": "https://v2-url"}}}`
	f2 := filepath.Join(dir2, "mcp.json")
	require.NoError(t, os.WriteFile(f2, []byte(p2JSON), 0o644))

	// Set mtime: f2 is newer than f1
	oldTime := time.Now().Add(-1 * time.Hour)
	require.NoError(t, os.Chtimes(f1, oldTime, oldTime))

	servers, err := DiscoverCursorPluginMCPServers(tempHome)
	require.NoError(t, err)
	require.Len(t, servers, 1)
	require.Equal(t, "https://v2-url", servers[0].URL, "newer plugin version should win")
}

func TestDiscoverCursorPluginMCPServers_IgnoresDisabled(t *testing.T) {
	tempHome := t.TempDir()
	pluginDir := filepath.Join(tempHome, "plugins", "local", "disabled-plugin")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	pJSON := `{"mcpServers": {"active": {"url": "https://active"}, "disabled-srv": {"url": "https://disabled", "disabled": true}}}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "mcp.json"), []byte(pJSON), 0o644))

	servers, err := DiscoverCursorPluginMCPServers(tempHome)
	require.NoError(t, err)
	require.Len(t, servers, 1)
	require.Equal(t, "active", servers[0].Name)
}
