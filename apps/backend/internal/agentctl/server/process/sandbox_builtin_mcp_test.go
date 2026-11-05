package process

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/sandbox"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestSandboxBuiltinConfigurationIsAtomicAndScoped(t *testing.T) {
	content := []byte("fixture runtime")
	digest := sha256.Sum256(content)
	files, err := sandbox.MaterializeRuntime(t.Context(), t.TempDir(), content, hex.EncodeToString(digest[:]), nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, files.Close()) })
	profile := p.New(p.SurfaceKanbanTask, nil, nil)
	profile.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: profile.Surface,
		Packages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageConfiguration: "task"}}
	cfg, err := (&config.Config{}).NewInstanceConfig(43210, &config.InstanceOverrides{TaskID: "task", McpProfile: &profile, McpServers: []config.McpServerConfig{{Name: "external", Type: "http", URL: "https://example.test/mcp"}}})
	require.NoError(t, err)
	manager := NewManager(cfg, newTestLogger(t))
	manager.SetBuiltinToolCatalog(func() map[p.PackageID][]string {
		return map[p.PackageID][]string{p.PackageConfiguration: {"create_workflow_kandev"}}
	})
	launch := &sandbox.Launch{Version: 1, AttemptID: "attempt", Generation: 1, PolicyDigest: strings.Repeat("a", 64), PackageDigest: strings.Repeat("b", 64), Root: files.Root, Prefix: []string{files.Executable}, Digests: files.Digests, MCPURL: "http://127.0.0.1:45679/" + strings.Repeat("c", 64) + "/mcp"}
	before := append([]config.McpServerConfig(nil), cfg.McpServers...)
	request := ExecutionConfiguration{Command: "agent", ArgsPresent: true, Args: []string{"agent"}, Sandbox: launch}
	require.Error(t, manager.ConfigureExecution(request), "the selected package has no captured route")
	require.Equal(t, before, cfg.McpServers)
	require.Empty(t, cfg.AgentCommand)
	require.Nil(t, cfg.Sandbox)
	launch.MCPPaths = []string{"/mcp", "/mcp/packages/configuration"}
	require.NoError(t, manager.ConfigureExecution(request))
	require.Equal(t, []config.McpServerConfig{{Name: "kandev", Type: "http", URL: launch.MCPURL}, {Name: "kandev-configuration", Type: "http", URL: launch.MCPURL + "/packages/configuration"}, {Name: "external", Type: "http", URL: "https://example.test/mcp"}}, cfg.McpServers)
	require.True(t, manager.builtinPermissionAllowed("kandev-configuration", "create_workflow_kandev"))
	cfg.McpServers[1].URL = "http://localhost:43210/mcp/packages/configuration"
	require.False(t, manager.builtinPermissionAllowed("kandev-configuration", "create_workflow_kandev"))
	launch.MCPURL = ""
	launch.MCPPaths = nil
	require.NoError(t, manager.ConfigureExecution(request))
	require.Equal(t, []config.McpServerConfig{{Name: "external", Type: "http", URL: "https://example.test/mcp"}}, cfg.McpServers)
	require.False(t, cfg.InjectedKandevMCP)
	require.False(t, manager.builtinPermissionAllowed("kandev-configuration", "create_workflow_kandev"))
}
