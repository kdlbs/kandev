package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuiltinMCPInstanceInjectionUsesEffectiveMode(t *testing.T) {
	legacy, err := (&Config{ListenHostOverride: "localhost"}).NewInstanceConfig(43210, &InstanceOverrides{InstanceID: "exec-1"})
	require.NoError(t, err)
	require.Equal(t, "http://localhost:43210/mcp", legacy.McpServers[0].URL)

	scoped, err := (&Config{ListenHostOverride: "localhost"}).NewInstanceConfig(43210, &InstanceOverrides{InstanceID: "exec-1", ScopedMCPRoutes: true})
	require.NoError(t, err)
	require.Equal(t, "/i/exec-1", scoped.NativeMCPRoutePrefix)
	require.Equal(t, "http://localhost:43210/i/exec-1/mcp", scoped.McpServers[0].URL)
	require.Equal(t, "http://localhost:43210/i/exec-1/sse", scoped.McpServers[1].URL)
}

func TestBuiltinMCPInstanceConfigRejectsMissingOrInvalidScopedIdentity(t *testing.T) {
	for _, id := range []string{"", "exec/2", ".", "exec%2f2"} {
		instance, err := (&Config{}).NewInstanceConfig(43210, &InstanceOverrides{InstanceID: id, ScopedMCPRoutes: true})
		require.Error(t, err, "ID %q", id)
		require.Nil(t, instance)
	}
	legacy, err := (&Config{}).NewInstanceConfig(43210, nil)
	require.NoError(t, err, "legacy direct construction still permits an empty ID")
	require.False(t, legacy.ScopedMCPRoutes)
}

func TestBuiltinMCPInstancePackageInjectionUsesScopedPath(t *testing.T) {
	profile := builtinInstanceTestProfile()
	servers, err := BuiltinMCPServersForInstance(profile.BuiltinPackages, "127.0.0.1", 43210, "exec-1", true)
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:43210/i/exec-1/mcp", servers[0].URL)
	require.Equal(t, "http://127.0.0.1:43210/i/exec-1/mcp/packages/configuration", servers[2].URL)
	require.Equal(t, "http://127.0.0.1:43210/i/exec-1/sse/packages/rich-output", servers[5].URL)
}
