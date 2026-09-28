package lifecycle

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

type mockMcpConfigProvider struct {
	configs map[string]*mcpconfig.ProfileConfig
}

func (p *mockMcpConfigProvider) GetConfigByProfileID(ctx context.Context, profileID string) (*mcpconfig.ProfileConfig, error) {
	if cfg, ok := p.configs[profileID]; ok {
		return cfg, nil
	}
	return nil, nil
}

func TestCursorPluginMCPPolicy_DeniesAndProtectsReservedNames(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// Create a plugin defining "blocked-srv" and "allowed-srv"
	pluginDir := filepath.Join(tempHome, ".cursor", "plugins", "local", "policy-plugin")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	pluginJSON := `{
		"mcpServers": {
			"blocked-srv": {
				"type": "http",
				"url": "https://blocked.example.com"
			},
			"denied-by-profile": {
				"type": "http",
				"url": "https://plugin-resurrect.example.com"
			},
			"allowed-srv": {
				"type": "http",
				"url": "https://allowed.example.com"
			}
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "mcp.json"), []byte(pluginJSON), 0o644))

	workspaceDir := t.TempDir()
	mgr := newTestManager(t)

	// Configure profile with a server that policy will deny ("denied-by-profile")
	profileID := "test-profile-1"
	mgr.mcpProvider = &mockMcpConfigProvider{
		configs: map[string]*mcpconfig.ProfileConfig{
			profileID: {
				Enabled: true,
				Servers: map[string]mcpconfig.ServerDef{
					"denied-by-profile": {
						Type: mcpconfig.ServerTypeHTTP,
						URL:  "https://profile-denied.example.com",
					},
				},
			},
		},
	}

	exec := &AgentExecution{
		ID:             "exec-1",
		SessionID:      "sess-1",
		AgentID:        "cursor-acp",
		WorkspacePath:  workspaceDir,
		standalonePort: 4567,
	}
	exec.setMetadataValue("executor_mcp_policy", mcpconfig.Policy{
		AllowHTTP:        true,
		DenylistServers:  []string{"blocked-srv", "denied-by-profile"},
		AllowlistServers: nil,
	})

	agent := agents.NewCursorACP()
	profileInfo := &AgentProfileInfo{ProfileID: profileID, CursorPluginsMCPEnabled: true}

	err := mgr.reconcileAndMaterializeCursorProjectMCP(
		context.Background(),
		exec,
		agent,
		profileInfo,
		string(executor.NameLocal),
		agent.Runtime().ProjectMCPStrategy,
	)
	require.NoError(t, err)

	projectFile := filepath.Join(workspaceDir, ".cursor", "mcp.json")
	require.FileExists(t, projectFile)

	data, err := os.ReadFile(projectFile)
	require.NoError(t, err)

	var parsed struct {
		MCPServers map[string]any `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(data, &parsed))

	require.Contains(t, parsed.MCPServers, "kandev")
	require.Contains(t, parsed.MCPServers, "allowed-srv")
	require.NotContains(t, parsed.MCPServers, "blocked-srv", "blocked-srv should be filtered out by executor policy")
	require.NotContains(t, parsed.MCPServers, "denied-by-profile", "plugin should not resurrect denied profile server")
}

func TestCursorPluginMCPPrecedence_Matrix(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// 1. Global config defines "overlap-global" and "overlap-project"
	require.NoError(t, os.MkdirAll(filepath.Join(tempHome, ".cursor"), 0o755))
	userJSON := `{
		"mcpServers": {
			"overlap-project": {"url": "https://global.example.com/project"},
			"overlap-plugin": {"url": "https://global.example.com/plugin"}
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(tempHome, ".cursor", "mcp.json"), []byte(userJSON), 0o644))

	// 2. Plugin defines "overlap-profile", "overlap-project", "overlap-plugin"
	pluginDir := filepath.Join(tempHome, ".cursor", "plugins", "local", "p1")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	pluginJSON := `{
		"mcpServers": {
			"overlap-profile": {"url": "https://plugin.example.com/profile"},
			"overlap-project": {"url": "https://plugin.example.com/project"},
			"overlap-plugin": {"url": "https://plugin.example.com/plugin"},
			"plugin-only": {"url": "https://plugin.example.com/only"}
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "mcp.json"), []byte(pluginJSON), 0o644))

	workspaceDir := t.TempDir()
	// 3. Project file already exists with a user-owned entry and custom top-level property
	require.NoError(t, os.MkdirAll(filepath.Join(workspaceDir, ".cursor"), 0o755))
	existingProjectJSON := `{
		"customKey": "preserved-value",
		"mcpServers": {
			"overlap-profile": {"url": "https://project.example.com/profile"},
			"overlap-project": {"url": "https://project.example.com/project"}
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(workspaceDir, ".cursor", "mcp.json"), []byte(existingProjectJSON), 0o644))

	// 4. Profile defines "overlap-profile"
	profileID := "p-prec"
	mgr := newTestManager(t)
	mgr.mcpProvider = &mockMcpConfigProvider{
		configs: map[string]*mcpconfig.ProfileConfig{
			profileID: {
				Enabled: true,
				Servers: map[string]mcpconfig.ServerDef{
					"overlap-profile": {
						Type: mcpconfig.ServerTypeHTTP,
						URL:  "https://profile.example.com/winner",
					},
				},
			},
		},
	}

	exec := &AgentExecution{
		ID:             "exec-prec",
		WorkspacePath:  workspaceDir,
		standalonePort: 9999,
	}
	agent := agents.NewCursorACP()
	profileInfo := &AgentProfileInfo{ProfileID: profileID, CursorPluginsMCPEnabled: true}

	err := mgr.reconcileAndMaterializeCursorProjectMCP(
		context.Background(),
		exec,
		agent,
		profileInfo,
		string(executor.NameLocal),
		agent.Runtime().ProjectMCPStrategy,
	)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(workspaceDir, ".cursor", "mcp.json"))
	require.NoError(t, err)

	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &parsed))
	require.Equal(t, `"preserved-value"`, string(parsed["customKey"]))

	var servers map[string]struct {
		URL string `json:"url"`
	}
	require.NoError(t, json.Unmarshal(parsed["mcpServers"], &servers))

	// Assert precedence:
	// A. Profile wins over project
	require.Equal(t, "https://profile.example.com/winner", servers["overlap-profile"].URL)
	// B. User project server wins over global and plugin
	require.Equal(t, "https://project.example.com/project", servers["overlap-project"].URL)
	// C. Global wins over plugin
	require.Equal(t, "https://global.example.com/plugin", servers["overlap-plugin"].URL)
	// D. Plugin only is imported
	require.Equal(t, "https://plugin.example.com/only", servers["plugin-only"].URL)
}

func TestCursorPluginMCPReconciliation_DisabledAndSourceRemoval(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	pluginDir := filepath.Join(tempHome, ".cursor", "plugins", "local", "rec-plugin")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	pluginJSON := `{
		"mcpServers": {
			"imported-srv": {"url": "https://imported.example.com"},
			"user-will-edit": {"url": "https://edit-me.example.com"}
		}
	}`
	pluginManifest := filepath.Join(pluginDir, "mcp.json")
	require.NoError(t, os.WriteFile(pluginManifest, []byte(pluginJSON), 0o644))

	workspaceDir := t.TempDir()
	mgr := newTestManager(t)
	exec := &AgentExecution{
		ID:             "exec-rec",
		WorkspacePath:  workspaceDir,
		standalonePort: 1234,
	}
	agent := agents.NewCursorACP()

	// 1. Initial materialization with enabled preference
	profileInfoEnabled := &AgentProfileInfo{ProfileID: "p1", CursorPluginsMCPEnabled: true}
	err := mgr.reconcileAndMaterializeCursorProjectMCP(
		context.Background(),
		exec,
		agent,
		profileInfoEnabled,
		string(executor.NameLocal),
		agent.Runtime().ProjectMCPStrategy,
	)
	require.NoError(t, err)

	projectFile := filepath.Join(workspaceDir, ".cursor", "mcp.json")
	require.FileExists(t, projectFile)

	// User edits "user-will-edit" server
	data, err := os.ReadFile(projectFile)
	require.NoError(t, err)
	var parsed struct {
		MCPServers map[string]any `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(data, &parsed))
	require.Contains(t, parsed.MCPServers, "imported-srv")
	require.Contains(t, parsed.MCPServers, "user-will-edit")

	parsed.MCPServers["user-will-edit"] = map[string]any{"url": "https://user-edited.example.com"}
	newData, err := json.MarshalIndent(parsed, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(projectFile, newData, 0o644))

	// 2. Second materialization with disabled preference
	profileInfoDisabled := &AgentProfileInfo{ProfileID: "p1", CursorPluginsMCPEnabled: false}
	err = mgr.reconcileAndMaterializeCursorProjectMCP(
		context.Background(),
		exec,
		agent,
		profileInfoDisabled,
		string(executor.NameLocal),
		agent.Runtime().ProjectMCPStrategy,
	)
	require.NoError(t, err)

	dataAfterDisable, err := os.ReadFile(projectFile)
	require.NoError(t, err)
	var parsedAfterDisable struct {
		MCPServers map[string]struct {
			URL string `json:"url"`
		} `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(dataAfterDisable, &parsedAfterDisable))

	require.Contains(t, parsedAfterDisable.MCPServers, "kandev")
	require.NotContains(t, parsedAfterDisable.MCPServers, "imported-srv", "unchanged imported server should be removed when preference disabled")
	require.Contains(t, parsedAfterDisable.MCPServers, "user-will-edit", "user-edited server must survive disablement")
	require.Equal(t, "https://user-edited.example.com", parsedAfterDisable.MCPServers["user-will-edit"].URL)

	// 3. Third materialization: re-enable, but delete plugin manifest from disk
	require.NoError(t, os.Remove(pluginManifest))
	err = mgr.reconcileAndMaterializeCursorProjectMCP(
		context.Background(),
		exec,
		agent,
		profileInfoEnabled,
		string(executor.NameLocal),
		agent.Runtime().ProjectMCPStrategy,
	)
	require.NoError(t, err)

	dataAfterDelete, err := os.ReadFile(projectFile)
	require.NoError(t, err)
	var parsedAfterDelete struct {
		MCPServers map[string]struct {
			URL string `json:"url"`
		} `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(dataAfterDelete, &parsedAfterDelete))
	require.NotContains(t, parsedAfterDelete.MCPServers, "imported-srv", "deleted plugin server should be removed on next run")
	require.Contains(t, parsedAfterDelete.MCPServers, "user-will-edit", "user-edited server must survive source deletion")
}

func TestCursorPluginMCPSafety_SymlinkAndMalformedIgnored(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	workspaceDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workspaceDir, ".cursor"), 0o755))

	// 1. Destination is a symlink
	outsideFile := filepath.Join(t.TempDir(), "target.json")
	require.NoError(t, os.WriteFile(outsideFile, []byte(`{"mcpServers":{}}`), 0o644))
	linkPath := filepath.Join(workspaceDir, ".cursor", "mcp.json")
	require.NoError(t, os.Symlink(outsideFile, linkPath))

	mgr := newTestManager(t)
	exec := &AgentExecution{
		ID:             "exec-symlink",
		WorkspacePath:  workspaceDir,
		standalonePort: 1234,
	}
	agent := agents.NewCursorACP()
	profileInfo := &AgentProfileInfo{ProfileID: "p1", CursorPluginsMCPEnabled: true}

	err := mgr.reconcileAndMaterializeCursorProjectMCP(
		context.Background(),
		exec,
		agent,
		profileInfo,
		string(executor.NameLocal),
		agent.Runtime().ProjectMCPStrategy,
	)
	require.NoError(t, err)

	info, err := os.Lstat(linkPath)
	require.NoError(t, err)
	require.True(t, info.Mode()&os.ModeSymlink != 0, "symlink destination must be left untouched")
}
