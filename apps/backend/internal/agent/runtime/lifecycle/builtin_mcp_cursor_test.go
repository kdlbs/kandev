package lifecycle

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestBuiltinCursorRefreshPreservesUserChanges(t *testing.T) {
	mgr := newTestManager(t)
	workspace := t.TempDir()
	execution := &AgentExecution{TaskID: "task-a", WorkspaceID: "workspace", WorkspacePath: workspace, standalonePort: 41234}
	bind := func(ids ...p.PackageID) {
		encoded, err := p.MarshalBuiltinBinding(builtinDispatchProfile(ids...))
		require.NoError(t, err)
		execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
	}
	key := cursorProjectMCPWorkspaceKey(workspace)
	generation := beginCursorProjectMCPPreparation(key)
	defer finishCursorProjectMCPPreparation(key, generation)
	refresh := func() error {
		_, err := mgr.reconcileCursorProjectMCPFiles(context.Background(), workspace, key, generation, execution, nil, nil)
		return err
	}
	file := filepath.Join(workspace, ".cursor", "mcp.json")
	bind(p.PackageCore, p.PackageConfiguration)
	require.NoError(t, refresh())
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	var config struct {
		Servers map[string]map[string]any `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(data, &config))
	require.Contains(t, config.Servers, "kandev-configuration")
	require.Equal(t, "http://localhost:41234/mcp/packages/configuration", config.Servers["kandev-configuration"]["url"])
	require.NoError(t, refresh(), "generated entries are not user collisions")
	bind(p.PackageCore)
	require.NoError(t, refresh())
	data, err = os.ReadFile(file)
	require.NoError(t, err)
	require.NotContains(t, string(data), "kandev-configuration")
	config.Servers["kandev-configuration"] = map[string]any{"url": "https://user.example/mcp"}
	data, err = json.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, data, 0600))
	bind(p.PackageCore, p.PackageConfiguration)
	require.Error(t, refresh(), "never overwrite an existing reserved collision")
	preserved, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, data, preserved)
}
