package antigravityruntime

import (
	"encoding/json"
	"os"
	"path/filepath"
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
