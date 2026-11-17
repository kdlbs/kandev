package config

import (
	"strings"
	"testing"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestProviderReplacementBuiltinEndpointsAreScopedAndCredentialed(t *testing.T) {
	profile := p.New(p.SurfaceKanbanTask, nil, nil)
	profile.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: profile.Surface, Packages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageConfiguration: "task"}}
	digest, err := p.BuiltinBindingDigest(profile)
	require.NoError(t, err)
	cfg := &InstanceConfig{TaskID: "task", McpProfile: &profile, MCPHost: "127.0.0.1", Port: 12345, ProviderMCP: &ProviderMCPConfig{Scope: p.ProviderMCPScope{BindingID: "new-binding", ExecutionID: "execution", HelperID: "helper", ProcessGeneration: 2, HarnessGeneration: 2, ProfileDigest: digest}, Credential: strings.Repeat("x", 64)}}
	servers, err := injectInstanceBuiltinMCP(cfg)
	require.NoError(t, err)
	require.Len(t, servers, 2)
	require.Equal(t, "http://127.0.0.1:12345/mcp/generations/new-binding", servers[0].URL)
	require.Equal(t, "http://127.0.0.1:12345/mcp/generations/new-binding/packages/configuration", servers[1].URL)
	require.Equal(t, map[string]string{"Authorization": "Bearer " + strings.Repeat("x", 64)}, servers[0].Headers)
	require.Equal(t, "http", servers[0].Type)
	cfg.InstanceID = "execution"
	cfg.ScopedMCPRoutes = true
	servers, err = injectInstanceBuiltinMCP(cfg)
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:12345/i/execution/mcp/generations/new-binding", servers[0].URL)
	require.Equal(t, "http://127.0.0.1:12345/i/execution/mcp/generations/new-binding/packages/configuration", servers[1].URL)
	cfg.InstanceID = "another-execution"
	_, err = injectInstanceBuiltinMCP(cfg)
	require.Error(t, err)
}
