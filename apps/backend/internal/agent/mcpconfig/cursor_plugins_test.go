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
	commit := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	pluginDir := filepath.Join(tempHome, "plugins", "cache", "cursor-public", "atlassian", commit)
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

	servers, err := DiscoverCursorPluginMCPServersWithInventory(tempHome, CursorNativeInventory{
		Plugins: []CursorNativePlugin{{Name: "atlassian", Marketplace: "cursor-public", Revision: commit}},
	})
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
	commit := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	pluginDir := filepath.Join(tempHome, "plugins", "cache", "pub", "p1", commit)
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	pJSON := `{"mcpServers": {"server-a": {"url": "https://plugin-url"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "mcp.json"), []byte(pJSON), 0o644))

	// User defines server-a pointing to user-url
	userJSON := `{"mcpServers": {"server-a": {"url": "https://user-url"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(tempHome, "mcp.json"), []byte(userJSON), 0o644))

	servers, err := DiscoverCursorPluginMCPServersWithInventory(tempHome, CursorNativeInventory{
		Plugins: []CursorNativePlugin{{Name: "p1", Marketplace: "pub", Revision: commit}},
	})
	require.NoError(t, err)
	require.Len(t, servers, 1)
	require.Equal(t, "server-a", servers[0].Name)
	require.Equal(t, "https://user-url", servers[0].URL, "user config should outrank plugin config")
}

func TestDiscoverCursorPluginMCPServers_NewerPluginPrecedence(t *testing.T) {
	tempHome := t.TempDir()

	// Older cached commit
	oldCommit := "cccccccccccccccccccccccccccccccccccccccc"
	dir1 := filepath.Join(tempHome, "plugins", "cache", "pub", "p1", oldCommit)
	require.NoError(t, os.MkdirAll(dir1, 0o755))
	p1JSON := `{"mcpServers": {"server-a": {"url": "https://v1-url"}}}`
	f1 := filepath.Join(dir1, "mcp.json")
	require.NoError(t, os.WriteFile(f1, []byte(p1JSON), 0o644))

	// The selected commit has an older cache timestamp than the other commit.
	selectedCommit := "dddddddddddddddddddddddddddddddddddddddd"
	dir2 := filepath.Join(tempHome, "plugins", "cache", "pub", "p1", selectedCommit)
	require.NoError(t, os.MkdirAll(dir2, 0o755))
	p2JSON := `{"mcpServers": {"server-a": {"url": "https://v2-url"}}}`
	f2 := filepath.Join(dir2, "mcp.json")
	require.NoError(t, os.WriteFile(f2, []byte(p2JSON), 0o644))

	// Set mtime: f2 is newer than f1
	oldTime := time.Now().Add(-1 * time.Hour)
	require.NoError(t, os.Chtimes(f1, oldTime, oldTime))

	servers, err := DiscoverCursorPluginMCPServersWithInventory(tempHome, CursorNativeInventory{
		Plugins: []CursorNativePlugin{{Name: "p1", Marketplace: "pub", Revision: selectedCommit}},
	})
	require.NoError(t, err)
	require.Len(t, servers, 1)
	require.Equal(t, "https://v2-url", servers[0].URL, "the selected exact revision wins over cache mtime")
}

func TestCursorInventoryRequiredForCachedMarketplacePlugins(t *testing.T) {
	tempHome := t.TempDir()

	// 1. Uninstalled plugin folder under cache
	uninstalledDir := filepath.Join(tempHome, "plugins", "cache", "store", "uninstalled", "v1")
	require.NoError(t, os.MkdirAll(uninstalledDir, 0o755))
	uninstalledJSON := `{"mcpServers": {"uninstalled-srv": {"command": "untrusted-command"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(uninstalledDir, "mcp.json"), []byte(uninstalledJSON), 0o644))

	// An invented registry file cannot enable cached marketplace content.
	require.NoError(t, os.WriteFile(filepath.Join(tempHome, "plugins", "installed.json"), []byte(`{"enabled-plugin":true}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tempHome, "mcp.json"), []byte(`{"mcpServers":{"global":{"url":"https://global.example/mcp"}}}`), 0o600))

	servers, err := DiscoverCursorPluginMCPServers(tempHome)
	require.NoError(t, err)
	require.Len(t, servers, 1)
	require.Equal(t, "global", servers[0].Name)
}

func TestDiscoverCursorPluginMCPServers_DescriptorFileReference(t *testing.T) {
	tempHome := t.TempDir()

	// 1. Plugin descriptor referencing a relative file
	pluginDir := filepath.Join(tempHome, "plugins", "local", "ref-plugin")
	require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, "config"), 0o755))

	pluginDesc := `{
		"name": "ref-plugin",
		"mcpServers": "config/servers.json"
	}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(pluginDesc), 0o644))

	serversJSON := `{
		"mcpServers": {
			"ref-server": {
				"type": "http",
				"url": "https://ref.example.com/mcp"
			}
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "config", "servers.json"), []byte(serversJSON), 0o644))

	// 2. Plugin descriptor attempting path traversal (should be skipped without erroring out sibling)
	evilPluginDir := filepath.Join(tempHome, "plugins", "local", "evil-plugin")
	require.NoError(t, os.MkdirAll(evilPluginDir, 0o755))
	evilDesc := `{
		"name": "evil-plugin",
		"mcpServers": "../../../etc/passwd"
	}`
	require.NoError(t, os.WriteFile(filepath.Join(evilPluginDir, "plugin.json"), []byte(evilDesc), 0o644))

	servers, err := DiscoverCursorPluginMCPServers(tempHome)
	require.NoError(t, err)
	require.Len(t, servers, 1)
	require.Equal(t, "ref-server", servers[0].Name)
	require.Equal(t, "https://ref.example.com/mcp", servers[0].URL)
}

func TestDiscoverCursorPluginMCPServers_PluginRootExpansion(t *testing.T) {
	tempHome := t.TempDir()

	pluginDir := filepath.Join(tempHome, "plugins", "local", "path with spaces")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))

	manifest := `{
		"mcpServers": {
			"expanded-server": {
				"command": "${CURSOR_PLUGIN_ROOT}/bin/server",
				"args": ["--root", "${PLUGIN_ROOT}", "literal argument"],
				"env": {"DIR": "${CURSOR_PLUGIN_ROOT}/data"}
			},
			"unresolved-server": {
				"command": "${UNRESOLVED_VAR}/bin/server"
			}
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "mcp.json"), []byte(manifest), 0o644))

	servers, err := DiscoverCursorPluginMCPServers(tempHome)
	require.NoError(t, err)
	require.Len(t, servers, 1)
	require.Equal(t, "expanded-server", servers[0].Name)
	require.Equal(t, filepath.Join(pluginDir, "bin", "server"), servers[0].Command)
	require.Equal(t, []string{"--root", pluginDir, "literal argument"}, servers[0].Args)
	require.Equal(t, filepath.Join(pluginDir, "data"), servers[0].Env["DIR"])
}

func TestDiscoverCursorPluginMCPServers_UnsupportedCWD(t *testing.T) {
	tempHome := t.TempDir()

	pluginDir := filepath.Join(tempHome, "plugins", "local", "cwd-plugin")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))

	manifest := `{
		"mcpServers": {
			"cwd-server": {
				"command": "node",
				"args": ["server.js"],
				"cwd": "${CURSOR_PLUGIN_ROOT}"
			},
			"valid-sibling": {
				"command": "node",
				"args": ["valid.js"]
			}
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "mcp.json"), []byte(manifest), 0o644))

	servers, err := DiscoverCursorPluginMCPServers(tempHome)
	require.NoError(t, err)
	require.Len(t, servers, 1)
	require.Equal(t, "valid-sibling", servers[0].Name)
}

func TestDiscoverCursorPluginMCPServers_TransportValidation(t *testing.T) {
	tempHome := t.TempDir()

	pluginDir := filepath.Join(tempHome, "plugins", "local", "transport-plugin")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))

	manifest := `{
		"mcpServers": {
			"sse-server": {
				"type": "sse",
				"url": "https://example.com/sse"
			},
			"streamable-server": {
				"type": "streamable_http",
				"url": "https://example.com/stream"
			},
			"conflicting-shape": {
				"command": "run.sh",
				"url": "https://example.com/mcp"
			},
			"invalid-stdio-url": {
				"type": "stdio",
				"url": "https://example.com/mcp"
			},
			"invalid-network-cmd": {
				"type": "http",
				"command": "run.sh"
			},
			"unknown-transport": {
				"type": "grpc",
				"url": "https://example.com/mcp"
			}
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "mcp.json"), []byte(manifest), 0o644))

	servers, err := DiscoverCursorPluginMCPServers(tempHome)
	require.NoError(t, err)
	require.Len(t, servers, 2)

	serverMap := make(map[string]string)
	for _, s := range servers {
		serverMap[s.Name] = s.Type
	}
	require.Equal(t, string(ServerTypeSSE), serverMap["sse-server"])
	require.Equal(t, string(ServerTypeStreamableHTTP), serverMap["streamable-server"])
}

func TestCursorPluginCacheWithoutEnabledStateIsNotImported(t *testing.T) {
	for _, registry := range []string{"", "{invalid"} {
		t.Run(registry, func(t *testing.T) {
			home := t.TempDir()
			dir := filepath.Join(home, "plugins", "cache", "cursor-public", "harness", "revision")
			require.NoError(t, os.MkdirAll(dir, 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "mcp.json"),
				[]byte(`{"mcpServers":{"figma":{"url":"https://mcp.figma.com/mcp"}}}`), 0o600))
			if registry != "" {
				require.NoError(t, os.WriteFile(filepath.Join(home, "plugins", "installed.json"), []byte(registry), 0o600))
			}
			require.NoError(t, os.WriteFile(filepath.Join(home, "mcp.json"),
				[]byte(`{"mcpServers":{"explicit":{"url":"https://explicit.example/mcp"}}}`), 0o600))
			servers, err := DiscoverCursorPluginCandidates(home)
			require.NoError(t, err)
			require.Len(t, servers, 1, "cached files cannot establish enabled state")
			require.Equal(t, "explicit", servers[0].Name)
		})
	}
}
