package process

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func TestScanAntigravityNativeMCPNamesUsesConfiguredHomeAndWorkspace(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	writeNativeMCPConfig(t, filepath.Join(home, ".gemini", "config", "mcp_config.json"), "global-competitor")
	writeNativeMCPConfig(t, filepath.Join(workDir, ".agents", "mcp_config.json"), "workspace-competitor")
	pluginRoot := filepath.Join(home, ".gemini", "config", "plugins", "plugin-id")
	if err := os.MkdirAll(pluginRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(map[string]string{"name": "plugin-id"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginRoot, "plugin.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	writeNativeMCPConfig(t, filepath.Join(pluginRoot, "mcp_config.json"), "plugin-server")

	names, complete := scanAntigravityNativeMCPNames(&config.InstanceConfig{
		WorkDir:  workDir,
		AgentEnv: []string{"HOME=" + home},
	})
	if !complete {
		t.Fatal("native MCP inventory was not complete")
	}
	for _, name := range []string{"global-competitor", "workspace-competitor", "plugin-id_plugin-server"} {
		if !slices.Contains(names, name) {
			t.Fatalf("native MCP names = %q, missing %q", names, name)
		}
	}
}

func TestNewManagerLoadsAntigravityNativeMCPInventory(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	writeNativeMCPConfig(t, filepath.Join(home, ".gemini", "config", "mcp_config.json"), "global-competitor")
	cfg := &config.InstanceConfig{
		AgentType: acpcompat.AntigravityAgentID,
		AuthPolicy: &acpcompat.AuthPolicy{
			RuntimeVariant: acpcompat.AntigravityRuntimeCLI,
		},
		AgentEnv: []string{"HOME=" + home},
		WorkDir:  workDir,
	}
	manager := NewManager(cfg, newTestLogger(t))
	t.Cleanup(manager.lifetimeCancel)

	if !manager.antigravityMCPNamesKnown || !slices.Contains(manager.antigravityMCPNames, "global-competitor") {
		t.Fatalf("manager native inventory = (%q, %t), want configured global server", manager.antigravityMCPNames, manager.antigravityMCPNamesKnown)
	}
}

func writeNativeMCPConfig(t *testing.T, path, serverName string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"mcpServers": map[string]any{
		serverName: map[string]any{"command": "native-server"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
