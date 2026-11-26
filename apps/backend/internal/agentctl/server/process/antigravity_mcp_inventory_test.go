package process

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/pkg/agent"
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

	names, known := manager.antigravityMCPInventorySnapshot()
	if !known || !slices.Contains(names, "global-competitor") {
		t.Fatalf("manager native inventory = (%q, %t), want configured global server", names, known)
	}
}

func TestManagerRefreshesAntigravityNativeMCPInventoryBeforeReplacementStart(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	configPath := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	writeNativeMCPConfig(t, configPath, "initial-server")
	manager := newAntigravityInventoryTestManager(t, workDir, home)
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start initial agent: %v", err)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatalf("stop initial agent: %v", err)
	}

	writeNativeMCPConfig(t, configPath, "replacement-server")
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start replacement agent: %v", err)
	}

	names, known := manager.antigravityMCPInventorySnapshot()
	if !known || !slices.Contains(names, "replacement-server") {
		t.Fatalf("replacement start inventory = (%q, %t), want replacement server", names, known)
	}
	if slices.Contains(names, "initial-server") {
		t.Fatalf("replacement start retained stale inventory: %q", names)
	}
}

func TestManagerDisablesAntigravityImplicitApprovalWhenInventoryRefreshFails(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	configPath := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	writeNativeMCPConfig(t, configPath, "initial-server")
	manager := newAntigravityInventoryTestManager(t, workDir, home)

	if err := os.WriteFile(configPath, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start agent with unavailable native inventory: %v", err)
	}

	_, known := manager.antigravityMCPInventorySnapshot()
	if known {
		t.Fatal("failed native inventory refresh left implicit approval enabled")
	}
}

func newAntigravityInventoryTestManager(t *testing.T, workDir, home string) *Manager {
	t.Helper()
	t.Setenv("HOME", home)
	manager := NewManager(&config.InstanceConfig{
		AgentType: acpcompat.AntigravityAgentID,
		AuthPolicy: &acpcompat.AuthPolicy{
			RuntimeVariant: acpcompat.AntigravityRuntimeCLI,
		},
		AgentArgs: fixtureArgs(),
		AgentEnv:  fixtureEnvSlice("sleep 60"),
		Protocol:  agent.ProtocolACP,
		WorkDir:   workDir,
	}, newTestLogger(t))
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	return manager
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
