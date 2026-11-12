package config

import (
	"encoding/base64"
	"testing"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestNewBuiltinMCPCredentialIsRandom256Bits(t *testing.T) {
	first, err := NewBuiltinMCPCredential()
	require.NoError(t, err)
	second, err := NewBuiltinMCPCredential()
	require.NoError(t, err)
	raw, err := base64.RawURLEncoding.DecodeString(first)
	require.NoError(t, err)
	require.Len(t, raw, 32)
	require.NotEqual(t, first, second)
}

func TestBuiltinMCPServers_CarryCredentialHeader(t *testing.T) {
	want := map[string]string{"Authorization": "Bearer cred"}
	legacy, err := BuiltinMCPServers(nil, "localhost", 41234, "cred")
	require.NoError(t, err)
	require.Len(t, legacy, 2)
	manifest := &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: p.SurfaceKanbanTask,
		Packages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageConfiguration: "task"}}
	packaged, err := BuiltinMCPServers(manifest, "localhost", 41234, "cred")
	require.NoError(t, err)
	require.Len(t, packaged, 4)
	for _, server := range append(legacy, packaged...) {
		require.Equal(t, want, server.Headers, server.Name+" "+server.Type)
	}
	headerless, err := BuiltinMCPServers(nil, "localhost", 41234, "")
	require.NoError(t, err)
	for _, server := range headerless {
		require.Nil(t, server.Headers)
	}
}

func TestBuiltinMCPHeadersMatch(t *testing.T) {
	require.True(t, BuiltinMCPHeadersMatch(map[string]string{"Authorization": "Bearer cred"}, "cred"))
	require.True(t, BuiltinMCPHeadersMatch(nil, ""))
	require.False(t, BuiltinMCPHeadersMatch(nil, "cred"))
	require.False(t, BuiltinMCPHeadersMatch(map[string]string{"Authorization": "Bearer other"}, "cred"))
	require.False(t, BuiltinMCPHeadersMatch(map[string]string{"Authorization": "Bearer cred", "X": "1"}, "cred"))
	require.False(t, BuiltinMCPHeadersMatch(map[string]string{"authorization": "Bearer cred"}, "cred"))
	require.False(t, BuiltinMCPHeadersMatch(map[string]string{"Authorization": "Bearer cred"}, ""))
}

func TestNewInstanceConfigEnforcementRequiresCredential(t *testing.T) {
	cfg, err := (&Config{}).NewInstanceConfig(41234, &InstanceOverrides{BuiltinMCPAuthEnforced: true})
	require.NoError(t, err)
	require.False(t, cfg.BuiltinMCPAuthEnforced)
	cfg, err = (&Config{}).NewInstanceConfig(41234, &InstanceOverrides{BuiltinMCPAuthEnforced: true, BuiltinMCPCredential: "cred"})
	require.NoError(t, err)
	require.True(t, cfg.BuiltinMCPAuthEnforced)
	for _, server := range cfg.McpServers {
		if server.Name == kandevMcpServerName {
			require.Equal(t, BuiltinMCPHeaders("cred"), server.Headers)
		}
	}
}
