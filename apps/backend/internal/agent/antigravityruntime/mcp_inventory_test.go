package antigravityruntime

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestScanNativeMCPServerNamesIncludesNativeAndPluginSources(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	writeMCPConfig(t, filepath.Join(home, ".gemini", "config", "mcp_config.json"), map[string]any{
		"global-native": map[string]any{"command": "native"},
	})
	writeMCPConfig(t, filepath.Join(workDir, ".agents", "mcp_config.json"), map[string]any{
		"workspace-native": map[string]any{"command": "native"},
	})
	globalPluginRoot := filepath.Join(home, ".gemini", "config", "plugins", "plugin-folder")
	writePluginManifest(t, filepath.Join(globalPluginRoot, "plugin.json"), "kandev")
	writeMCPConfig(t, filepath.Join(globalPluginRoot, "mcp_config.json"), map[string]any{
		"set_task_title": map[string]any{"command": "native"},
	})
	workspacePluginRoot := filepath.Join(workDir, ".agents", "plugins", "workspace-plugin")
	writePluginManifest(t, filepath.Join(workspacePluginRoot, ".antigravity-plugin", "plugin.json"), "workspace-plugin")
	writeMCPConfig(t, filepath.Join(workspacePluginRoot, "mcp_config.json"), map[string]any{
		"workspace-server": map[string]any{"command": "native"},
	})

	names, complete := ScanNativeMCPServerNames(workDir, home)
	want := []string{"global-native", "kandev_set_task_title", "workspace-native", "workspace-plugin_workspace-server"}
	if !complete {
		t.Fatal("inventory marked incomplete despite readable native configuration")
	}
	if !slices.Equal(names, want) {
		t.Fatalf("native server names = %q, want %q", names, want)
	}
}

func TestScanNativeMCPServerNamesFailsClosedOnMalformedSource(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	path := filepath.Join(workDir, ".agents", "mcp_config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"mcpServers":`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, complete := ScanNativeMCPServerNames(workDir, home)
	if complete {
		t.Fatal("malformed native registration source was marked complete")
	}
}

func TestScanNativeMCPServerNamesFailsClosedWhenPluginNameIsUnavailable(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	writeMCPConfig(t, filepath.Join(home, ".gemini", "config", "plugins", "plugin-folder", "mcp_config.json"), map[string]any{
		"step_complete": map[string]any{"command": "native"},
	})

	_, complete := ScanNativeMCPServerNames(workDir, home)
	if complete {
		t.Fatal("plugin registration without a readable manifest was marked complete")
	}
}

func TestScanNativeMCPServerNamesFailsClosedOnUnscannedPluginManifests(t *testing.T) {
	for _, test := range []struct {
		name         string
		manifestPath func(workDir, home string) string
	}{
		{
			name: "global manifest",
			manifestPath: func(_, home string) string {
				return filepath.Join(home, ".gemini", "config", "plugins.json")
			},
		},
		{
			name: "workspace manifest",
			manifestPath: func(workDir, _ string) string {
				return filepath.Join(workDir, ".agents", "plugins.json")
			},
		},
		{
			name: "repository root manifest",
			manifestPath: func(workDir, _ string) string {
				return filepath.Join(filepath.Dir(workDir), ".agents", "plugins.json")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			workDir := t.TempDir()
			home := t.TempDir()
			if test.name == "repository root manifest" {
				repoRoot := workDir
				workDir = filepath.Join(repoRoot, "nested")
				if err := os.MkdirAll(workDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
					t.Fatal(err)
				}
			}

			pluginRoot := filepath.Join(t.TempDir(), "external-plugin")
			writePluginManifest(t, filepath.Join(pluginRoot, "plugin.json"), "kandev")
			writeMCPConfig(t, filepath.Join(pluginRoot, "mcp_config.json"), map[string]any{
				"step_complete": map[string]any{"command": "native"},
			})
			manifestPath := test.manifestPath(workDir, home)
			if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
				t.Fatal(err)
			}
			manifest, err := json.Marshal(map[string]any{
				"entries": []map[string]string{{"path": pluginRoot}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(manifestPath, manifest, 0o600); err != nil {
				t.Fatal(err)
			}

			_, complete := ScanNativeMCPServerNames(workDir, home)
			if complete {
				t.Fatal("inventory marked complete despite a manifest that can register an external plugin MCP server")
			}
		})
	}
}

func TestScanNativeMCPServerNamesFailsClosedOnOversizedSources(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, workDir, home string)
	}{
		{
			name: "MCP config",
			setup: func(t *testing.T, workDir, _ string) {
				t.Helper()
				data, err := json.Marshal(map[string]any{"mcpServers": map[string]any{}})
				if err != nil {
					t.Fatal(err)
				}
				writeOversizedJSON(t, filepath.Join(workDir, ".agents", "mcp_config.json"), data)
			},
		},
		{
			name: "plugin manifest",
			setup: func(t *testing.T, workDir, home string) {
				t.Helper()
				pluginRoot := filepath.Join(home, ".gemini", "config", "plugins", "plugin")
				writeMCPConfig(t, filepath.Join(pluginRoot, "mcp_config.json"), map[string]any{
					"native": map[string]any{"command": "native"},
				})
				data, err := json.Marshal(map[string]string{"name": "plugin"})
				if err != nil {
					t.Fatal(err)
				}
				writeOversizedJSON(t, filepath.Join(pluginRoot, "plugin.json"), data)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			workDir := t.TempDir()
			home := t.TempDir()
			test.setup(t, workDir, home)

			_, complete := ScanNativeMCPServerNames(workDir, home)
			if complete {
				t.Fatal("inventory marked an oversized registration source complete")
			}
		})
	}
}

func TestScanNativeMCPServerNamesRejectsSymlinkToSpecialFile(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux /dev/zero reproduces unbounded special-file reads")
	}
	workDir := t.TempDir()
	home := t.TempDir()
	configPath := filepath.Join(home, ".gemini", "config", nativeMCPConfigName)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/dev/zero", configPath); err != nil {
		t.Fatal(err)
	}

	_, complete := ScanNativeMCPServerNames(workDir, home)
	if complete {
		t.Fatal("inventory accepted a symlink to a special file")
	}
}

func TestReadNativeMCPSourceRejectsSpecialFile(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("uses Linux /dev/zero as a character device")
	}
	_, exists, complete := readNativeMCPSource("/dev/zero")
	if !exists || complete {
		t.Fatalf("special file read = (exists=%t, complete=%t), want present and rejected", exists, complete)
	}
}

func writeOversizedJSON(t *testing.T, path string, prefix []byte) {
	t.Helper()
	const fileLimit = 1 << 20
	if len(prefix) > fileLimit {
		t.Fatalf("test JSON prefix is already larger than the file limit: %d bytes", len(prefix))
	}
	data := append(append([]byte(nil), prefix...), bytes.Repeat([]byte(" "), fileLimit+1-len(prefix))...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeMCPConfig(t *testing.T, path string, servers map[string]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"mcpServers": servers})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writePluginManifest(t *testing.T, path, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"name":"`+name+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
}
